package shop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

const maxLines = 50

type lineBody struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type dayBody struct {
	Date            string     `json:"date"`
	Cash            int64      `json:"cash"`
	Mobile          int64      `json:"mobile"`
	Other           int64      `json:"other"`
	Note            string     `json:"note"`
	Counted         *int64     `json:"counted"`
	CashAccountID   string     `json:"cashAccountId"`
	MobileAccountID string     `json:"mobileAccountId"`
	Credits         []lineBody `json:"credits"`
	Collects        []lineBody `json:"collects"`
}

// DayInput is a validated day. Paybacks go into the cash account.
type DayInput struct {
	Date            time.Time
	Cash            int64
	Mobile          int64
	Other           int64
	Note            *string
	Counted         *int64
	CashAccountID   string
	MobileAccountID string
	Credits         []Line
	Collects        []Line
}

func (b dayBody) validate(lang string, today time.Time) (DayInput, error) {
	f := &fields{lang: lang}
	in := DayInput{Cash: b.Cash, Mobile: b.Mobile, Other: b.Other, Counted: b.Counted, CashAccountID: b.CashAccountID, MobileAccountID: b.MobileAccountID}
	date, err := time.Parse(time.DateOnly, b.Date)
	if err != nil || date.After(today) {
		f.add("date", "date")
	}
	in.Date = date
	for path, v := range map[string]int64{"cash": b.Cash, "mobile": b.Mobile, "other": b.Other} {
		if v < 0 || v > maxAmount {
			f.add(path, "amount")
		}
	}
	if b.Counted != nil && (*b.Counted < 0 || *b.Counted > maxAmount) {
		f.add("counted", "amount")
	}
	if b.CashAccountID == "" || b.MobileAccountID == "" {
		f.add("cashAccountId", "account")
	}
	in.Note = optional(f, "note", b.Note, 200)
	in.Credits = lines(f, "credits", b.Credits)
	in.Collects = lines(f, "collects", b.Collects)
	if err := f.err(); err != nil {
		return in, err
	}
	if in.Cash+in.Mobile+in.Other+sum(in.Credits)+sum(in.Collects) == 0 {
		return in, messages.Fail(apperr.KindInvalid, codeEmptyDay)
	}
	return in, nil
}

// lines drops blank rows and checks the rest have a name and an amount.
func lines(f *fields, path string, rows []lineBody) []Line {
	if len(rows) > maxLines {
		f.add(path, "tooMany")
		return nil
	}
	out := []Line{}
	for i, r := range rows {
		name := strings.TrimSpace(r.Name)
		at := path + "." + strconv.Itoa(i)
		switch {
		case name == "" && r.Amount == 0:
			continue
		case name == "" || len([]rune(name)) > 80:
			f.add(at+".name", "name")
		case r.Amount <= 0 || r.Amount > maxAmount:
			f.add(at+".amount", "amount")
		}
		out = append(out, Line{Name: name, Amount: r.Amount})
	}
	return out
}

func sum(list []Line) int64 {
	var t int64
	for _, l := range list {
		t += l.Amount
	}
	return t
}

// SaveDay writes a day's entry, or rewrites it with its own credit lines when the day was
// already written.
func (s *Service) SaveDay(ctx context.Context, userID, projectID, lang string, in DayInput) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		for _, id := range []string{in.CashAccountID, in.MobileAccountID} {
			if ok, err := inProject(ctx, tx, "money_accounts", projectID, id); err != nil || !ok {
				if err != nil {
					return err
				}
				return invalid(lang, "cashAccountId", "account")
			}
		}
		dayID, action, err := upsertDay(ctx, tx, userID, projectID, in)
		if err != nil {
			return err
		}
		for _, l := range in.Credits {
			personID, err := upsertPerson(ctx, tx, projectID, l.Name)
			if err != nil {
				return err
			}
			if err := addEntry(ctx, tx, userID, projectID, personID, &dayID, "CREDIT", nil, in.Date, l.Amount); err != nil {
				return err
			}
		}
		if err := payBack(ctx, tx, userID, projectID, lang, dayID, in); err != nil {
			return err
		}
		total := in.Cash + in.Mobile + sum(in.Credits)
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: action, Entity: "shop_day", EntityID: dayID, Summary: in.Date.Format(time.DateOnly),
			Details: map[string]string{"amount": strconv.FormatInt(total, 10)},
		})
	})
}

// upsertDay writes the day's figures; an edit clears the lines the day wrote before.
func upsertDay(ctx context.Context, tx pgx.Tx, userID, projectID string, in DayInput) (id, action string, err error) {
	var inserted bool
	err = tx.QueryRow(ctx, `
		INSERT INTO shop_days (project_id, date, cash, mobile, other, note, counted, cash_account_id, mobile_account_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, date) DO UPDATE SET cash = EXCLUDED.cash, mobile = EXCLUDED.mobile, other = EXCLUDED.other,
			note = EXCLUDED.note, counted = EXCLUDED.counted, cash_account_id = EXCLUDED.cash_account_id,
			mobile_account_id = EXCLUDED.mobile_account_id
		RETURNING id, xmax = 0`,
		projectID, in.Date, in.Cash, in.Mobile, in.Other, in.Note, in.Counted, in.CashAccountID, in.MobileAccountID, userID).Scan(&id, &inserted)
	if err != nil {
		return "", "", fmt.Errorf("save day: %w", err)
	}
	if inserted {
		return id, "CREATE", nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM khata_entries WHERE day_id = $1`, id); err != nil {
		return "", "", fmt.Errorf("clear day lines: %w", err)
	}
	return id, "UPDATE", nil
}

// payBack records the day's paybacks; each must be someone in the book who owes at least that.
func payBack(ctx context.Context, tx pgx.Tx, userID, projectID, lang, dayID string, in DayInput) error {
	f := &fields{lang: lang}
	owed := map[string]int64{}
	for i, l := range in.Collects {
		at := "collects." + strconv.Itoa(i)
		personID, due, err := personDue(ctx, tx, projectID, l.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			f.add(at+".name", "unknown")
			continue
		}
		if err != nil {
			return err
		}
		if _, seen := owed[personID]; !seen {
			owed[personID] = due
		}
		if l.Amount > owed[personID] {
			f.add(at+".amount", "overpaid")
			continue
		}
		owed[personID] -= l.Amount
		if err := addEntry(ctx, tx, userID, projectID, personID, &dayID, "PAYMENT", &in.CashAccountID, in.Date, l.Amount); err != nil {
			return err
		}
	}
	return f.err()
}

// personDue finds someone in the credit book by name and what they owe now.
func personDue(ctx context.Context, q db, projectID, name string) (id string, due int64, err error) {
	err = q.QueryRow(ctx, `
		SELECT p.id, coalesce(sum(CASE k.kind WHEN 'CREDIT' THEN k.amount ELSE -k.amount END), 0)
		FROM khata_people p LEFT JOIN khata_entries k ON k.person_id = p.id
		WHERE p.project_id = $1 AND lower(p.name) = lower($2)
		GROUP BY p.id`, projectID, name).Scan(&id, &due)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		err = fmt.Errorf("load credit: %w", err)
	}
	return id, due, err
}

func upsertPerson(ctx context.Context, tx pgx.Tx, projectID, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO khata_people (project_id, name) VALUES ($1, $2)
		ON CONFLICT (project_id, (lower(name))) DO UPDATE SET name = khata_people.name
		RETURNING id`, projectID, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert credit person: %w", err)
	}
	return id, nil
}

func addEntry(ctx context.Context, tx pgx.Tx, userID, projectID, personID string, dayID *string, kind string, accountID *string, date time.Time, amount int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO khata_entries (project_id, person_id, day_id, kind, date, amount, account_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, projectID, personID, dayID, kind, date, amount, accountID, userID)
	if err != nil {
		return fmt.Errorf("insert credit line: %w", err)
	}
	return nil
}
