package shop

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Line is one name and amount in a day's credit or payback list.
type Line struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

// Day is one day's entry. Credit is what was sold on credit that day and Collect what
// came back from the credit book on that date, in the day's entry or on its own.
type Day struct {
	ID              string  `json:"id"`
	Date            string  `json:"date"`
	Cash            int64   `json:"cash"`
	Mobile          int64   `json:"mobile"`
	Credit          int64   `json:"credit"`
	Collect         int64   `json:"collect"`
	Other           int64   `json:"other"`
	Note            *string `json:"note"`
	Counted         *int64  `json:"counted"`
	By              string  `json:"by"`
	CashAccountID   string  `json:"cashAccountId"`
	MobileAccountID string  `json:"mobileAccountId"`
	Credits         []Line  `json:"credits"`
	Collects        []Line  `json:"collects"`
}

// Person is someone in the credit book: what they owe, since their oldest unpaid
// credit, and when they last took goods.
type Person struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Due   int64   `json:"due"`
	Since *string `json:"since"`
	Last  *string `json:"last"`
}

// MonthSpend is what the books show spent in a month (YYYY-MM).
type MonthSpend struct {
	Month  string `json:"month"`
	Amount int64  `json:"amount"`
}

// Book is the shop's sales page: the days of the last year, newest first, everyone in
// the credit book, and the monthly spending to set the sales against.
type Book struct {
	Days     []Day        `json:"days"`
	Khata    []Person     `json:"khata"`
	Expenses []MonthSpend `json:"expenses"`
}

// Book returns the shop's sales page.
func (s *Service) Book(ctx context.Context, userID, projectID string) (Book, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Book{}, err
	}
	today := s.today()
	since := time.Date(today.Year()-1, today.Month(), 1, 0, 0, 0, 0, time.UTC)
	var b Book
	var err error
	if b.Days, err = daysOf(ctx, s.pool, projectID, since); err != nil {
		return Book{}, err
	}
	if b.Khata, err = khataOf(ctx, s.pool, projectID); err != nil {
		return Book{}, err
	}
	if b.Expenses, err = spendOf(ctx, s.pool, projectID, since); err != nil {
		return Book{}, err
	}
	return b, nil
}

func daysOf(ctx context.Context, q db, projectID string, since time.Time) ([]Day, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id, to_char(d.date, 'YYYY-MM-DD'), d.cash, d.mobile, d.other, d.note, d.counted,
		       u.first_name || ' ' || u.last_name, d.cash_account_id, d.mobile_account_id,
		       coalesce((SELECT sum(k.amount) FROM khata_entries k WHERE k.day_id = d.id AND k.kind = 'CREDIT'), 0),
		       coalesce((SELECT sum(k.amount) FROM khata_entries k
		                 WHERE k.project_id = d.project_id AND k.date = d.date AND k.kind = 'PAYMENT'), 0)
		FROM shop_days d JOIN users u ON u.id = d.created_by
		WHERE d.project_id = $1 AND d.date >= $2 ORDER BY d.date DESC`, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("list days: %w", err)
	}
	days, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Day, error) {
		d := Day{Credits: []Line{}, Collects: []Line{}}
		scanErr := row.Scan(&d.ID, &d.Date, &d.Cash, &d.Mobile, &d.Other, &d.Note, &d.Counted, &d.By,
			&d.CashAccountID, &d.MobileAccountID, &d.Credit, &d.Collect)
		return d, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("list days: %w", err)
	}
	return days, linesOf(ctx, q, projectID, since, days)
}

// linesOf fills each day's own credit and payback lines, for editing the day.
func linesOf(ctx context.Context, q db, projectID string, since time.Time, days []Day) error {
	at := make(map[string]int, len(days))
	for i := range days {
		at[days[i].ID] = i
	}
	rows, err := q.Query(ctx, `
		SELECT k.day_id, k.kind, p.name, k.amount
		FROM khata_entries k JOIN khata_people p ON p.id = k.person_id
		WHERE k.project_id = $1 AND k.day_id IS NOT NULL AND k.date >= $2
		ORDER BY k.created_at, k.id`, projectID, since)
	if err != nil {
		return fmt.Errorf("list day lines: %w", err)
	}
	var dayID, kind string
	var l Line
	_, err = pgx.ForEachRow(rows, []any{&dayID, &kind, &l.Name, &l.Amount}, func() error {
		d := &days[at[dayID]]
		if kind == "CREDIT" {
			d.Credits = append(d.Credits, l)
		} else {
			d.Collects = append(d.Collects, l)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("list day lines: %w", err)
	}
	return nil
}

// entry is one line of the credit book.
type entry struct {
	personID, name, kind, date string
	amount                     int64
}

func khataOf(ctx context.Context, q db, projectID string) ([]Person, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.name, k.kind, to_char(k.date, 'YYYY-MM-DD'), k.amount
		FROM khata_entries k JOIN khata_people p ON p.id = k.person_id
		WHERE k.project_id = $1 ORDER BY k.date, k.created_at, k.id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list credit book: %w", err)
	}
	var e entry
	var list []entry
	_, err = pgx.ForEachRow(rows, []any{&e.personID, &e.name, &e.kind, &e.date, &e.amount}, func() error {
		list = append(list, e)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list credit book: %w", err)
	}
	return owing(list), nil
}

// owing works out who owes what from the credit book (oldest line first): paybacks clear
// a person's oldest credit first, so Since is the date of the oldest credit not yet paid.
// It returns those who still owe, the biggest first.
func owing(list []entry) []Person {
	type account struct {
		person  Person
		credits []entry
		paid    int64
	}
	byID := map[string]*account{}
	var order []string
	for _, e := range list {
		a, ok := byID[e.personID]
		if !ok {
			a = &account{person: Person{ID: e.personID, Name: e.name}}
			byID[e.personID] = a
			order = append(order, e.personID)
		}
		if e.kind == "CREDIT" {
			a.credits = append(a.credits, e)
			a.person.Due += e.amount
			a.person.Last = &e.date
		} else {
			a.paid += e.amount
			a.person.Due -= e.amount
		}
	}
	people := []Person{}
	for _, id := range order {
		a := byID[id]
		if a.person.Due <= 0 {
			continue
		}
		left := a.paid
		for _, c := range a.credits {
			if left < c.amount {
				a.person.Since = &c.date
				break
			}
			left -= c.amount
		}
		people = append(people, a.person)
	}
	slices.SortStableFunc(people, func(x, y Person) int { return cmp.Compare(y.Due, x.Due) })
	return people
}

func spendOf(ctx context.Context, q db, projectID string, since time.Time) ([]MonthSpend, error) {
	rows, err := q.Query(ctx, `
		SELECT to_char(date, 'YYYY-MM'), sum(amount) FROM expenses
		WHERE project_id = $1 AND deleted_at IS NULL AND date >= $2
		GROUP BY 1 ORDER BY 1`, projectID, since)
	if err != nil {
		return nil, fmt.Errorf("sum spending: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MonthSpend])
}
