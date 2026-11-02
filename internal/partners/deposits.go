package partners

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

type depositBody struct {
	Amount    int64  `json:"amount"`
	CallID    string `json:"callId"`
	AccountID string `json:"accountId"`
	Date      string `json:"date"`
	Note      string `json:"note"`
}

// NewDeposit is validated money a partner put in. CallID empty means their commitment.
type NewDeposit struct {
	Amount    int64
	CallID    *string
	AccountID string
	Date      time.Time
	Note      *string
}

func (b depositBody) validate(lang string, today time.Time) (NewDeposit, error) {
	var fields []string
	if b.Amount <= 0 || b.Amount > maxAmount {
		fields = append(fields, "amount", "amount")
	}
	if b.AccountID == "" {
		fields = append(fields, "accountId", "account")
	}
	date := today
	if b.Date != "" {
		d, err := time.Parse(time.DateOnly, b.Date)
		if err != nil || d.After(today) {
			fields = append(fields, "date", "date")
		}
		date = d
	}
	note := strings.TrimSpace(b.Note)
	if len([]rune(note)) > 200 {
		fields = append(fields, "note", "note")
	}
	if len(fields) > 0 {
		return NewDeposit{}, invalid(lang, fields...)
	}
	d := NewDeposit{Amount: b.Amount, AccountID: b.AccountID, Date: date}
	if b.CallID != "" {
		d.CallID = &b.CallID
	}
	if note != "" {
		d.Note = &note
	}
	return d, nil
}

// Deposited is what the page needs after a deposit: where its receipt is.
type Deposited struct {
	ID        string `json:"id"`
	ReceiptNo string `json:"receiptNo"`
}

// Deposit records money a partner put in, gives it the next receipt number, and adds it
// to the account it went into. Owners and managers take deposits.
func (s *Service) Deposit(ctx context.Context, userID, projectID, partnerID, lang string, in NewDeposit) (Deposited, error) {
	if !isID(partnerID) {
		return Deposited{}, notFound()
	}
	var out Deposited
	err := s.joint(ctx, func() error { _, err := s.access.Writer(ctx, userID, projectID); return err }, projectID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT name FROM partners WHERE id = $1 AND project_id = $2`, partnerID, projectID).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("load partner: %w", err)
		}
		if ok, checkErr := inProject(ctx, tx, "money_accounts", projectID, in.AccountID); checkErr != nil || !ok {
			return orInvalid(checkErr, invalid(lang, "accountId", "account"))
		}
		if in.CallID != nil {
			var ok bool
			checkErr := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM capital_call_parts WHERE call_id::text = $1 AND partner_id = $2)`, *in.CallID, partnerID).Scan(&ok)
			if checkErr != nil || !ok {
				return orInvalid(checkErr, invalid(lang, "callId", "call"))
			}
		}
		n, err := project.NextReceipt(ctx, tx, projectID)
		if err != nil {
			return err
		}
		var prefix string
		err = tx.QueryRow(ctx, `
			INSERT INTO partner_deposits (project_id, partner_id, call_id, account_id, date, amount, note, receipt_no, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, (SELECT receipt_prefix FROM projects WHERE id = $1)`,
			projectID, partnerID, in.CallID, in.AccountID, in.Date, in.Amount, in.Note, n, userID).Scan(&out.ID, &prefix)
		if err != nil {
			return fmt.Errorf("insert deposit: %w", err)
		}
		out.ReceiptNo = project.ReceiptNo(prefix, n)
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "deposit", EntityID: out.ID, Summary: name,
			Details: map[string]string{"receipt": out.ReceiptNo},
		})
	})
	return out, err
}

// orInvalid returns a database error when there is one, else the refusal.
func orInvalid(dbErr, refusal error) error {
	if dbErr != nil {
		return fmt.Errorf("check reference: %w", dbErr)
	}
	return refusal
}
