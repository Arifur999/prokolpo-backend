package books

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

var purposes = []string{"WAGE", "ADVANCE", "INSTALL", "EXTRA", "FINAL", "OTHER"}

// NewPayment is money paid to a supplier or contractor from one account. Paying more than
// is due is allowed: the extra stays with them as an advance.
type NewPayment struct {
	AccountID string
	Amount    int64
	Date      time.Time
	// A supplier payment may name the bill it is for; it does not change the due.
	ExpenseID *string
	Purpose   *string
	Note      *string
}

type paymentBody struct {
	AccountID string `json:"accountId"`
	Amount    int64  `json:"amount"`
	Date      string `json:"date"`
	ExpenseID string `json:"expenseId"`
	Purpose   string `json:"purpose"`
	Note      string `json:"note"`
}

func (b paymentBody) validate(lang string, today time.Time) (NewPayment, error) {
	f := &fields{lang: lang}
	p := NewPayment{
		AccountID: b.AccountID,
		Amount:    f.amount("amount", b.Amount, "amount"),
		Date:      f.date("date", b.Date, today),
		ExpenseID: f.ref("expenseId", b.ExpenseID, "party"),
		Note:      f.text("note", b.Note, maxNoteLen),
	}
	if !isID(b.AccountID) {
		f.add("accountId", "account")
	}
	if b.Purpose != "" {
		if !slices.Contains(purposes, b.Purpose) {
			f.add("purpose", "purpose")
		}
		p.Purpose = &b.Purpose
	}
	return p, f.err()
}

// Party names who is paid: one of "suppliers" or "contractors" and the row id.
type Party struct {
	Table string
	ID    string
}

// Pay records a payment to a supplier or contractor.
func (s *Service) Pay(ctx context.Context, userID, projectID string, to Party, p NewPayment) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		for _, ref := range []struct{ table, id string }{{to.Table, to.ID}, {"money_accounts", p.AccountID}} {
			ok, err := exists(ctx, tx, ref.table, projectID, ref.id)
			if err != nil {
				return fmt.Errorf("check %s: %w", ref.table, err)
			}
			if !ok {
				return notFound()
			}
		}
		var supplierID, contractorID *string
		if to.Table == "suppliers" {
			supplierID = &to.ID
		} else {
			contractorID = &to.ID
			p.ExpenseID = nil
		}
		if p.ExpenseID != nil {
			var ok bool
			err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM expenses WHERE id = $1 AND supplier_id = $2 AND deleted_at IS NULL)`, *p.ExpenseID, to.ID).Scan(&ok)
			if err != nil {
				return fmt.Errorf("check bill: %w", err)
			}
			if !ok {
				return notFound()
			}
		}
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO payments (project_id, account_id, supplier_id, contractor_id, expense_id, date, amount, purpose, note, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
			projectID, p.AccountID, supplierID, contractorID, p.ExpenseID, p.Date, p.Amount, p.Purpose, p.Note, userID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert payment: %w", err)
		}
		if contractorID != nil {
			if markErr := markWorking(ctx, tx, *contractorID); markErr != nil {
				return markErr
			}
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "payment", EntityID: id, Summary: fmt.Sprintf("৳ %d", p.Amount)})
	})
}

// DeletePayment takes a payment out of the books. A payment made with a bill goes only
// with that bill.
func (s *Service) DeletePayment(ctx context.Context, userID, projectID, paymentID string, reason *string) error {
	if !isID(paymentID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		var amount int64
		err := tx.QueryRow(ctx, `
			UPDATE payments SET deleted_at = $3, deleted_by = $4
			WHERE id = $1 AND project_id = $2 AND deleted_at IS NULL AND NOT paid_with_expense RETURNING amount`,
			paymentID, projectID, s.now(), userID).Scan(&amount)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return notFound()
			}
			return fmt.Errorf("delete payment: %w", err)
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "DELETE", Entity: "payment", EntityID: paymentID, Summary: fmt.Sprintf("৳ %d", amount), Reason: reason})
	})
}
