package books

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// NewTransfer moves money between two of a project's accounts. It is not spending.
type NewTransfer struct {
	From, To string
	Amount   int64
	Date     time.Time
	Note     *string
}

type transferBody struct {
	From   string `json:"fromAccountId"`
	To     string `json:"toAccountId"`
	Amount int64  `json:"amount"`
	Date   string `json:"date"`
	Note   string `json:"note"`
}

func (b transferBody) validate(lang string, today time.Time) (NewTransfer, error) {
	f := &fields{lang: lang}
	t := NewTransfer{From: b.From, To: b.To, Amount: f.amount("amount", b.Amount, "amount"), Date: f.date("date", b.Date, today)}
	if !isID(b.From) {
		f.add("fromAccountId", "account")
	}
	if !isID(b.To) {
		f.add("toAccountId", "account")
	} else if b.To == b.From {
		f.add("toAccountId", "toAccount")
	}
	t.Note = f.text("note", b.Note, maxNoteLen)
	return t, f.err()
}

// Transfer records the move. Balances may go below zero; the screens only warn.
func (s *Service) Transfer(ctx context.Context, userID, projectID string, t NewTransfer) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		for _, id := range []string{t.From, t.To} {
			ok, err := exists(ctx, tx, "money_accounts", projectID, id)
			if err != nil {
				return fmt.Errorf("check account: %w", err)
			}
			if !ok {
				return notFound()
			}
		}
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO transfers (project_id, from_account_id, to_account_id, date, amount, note, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			projectID, t.From, t.To, t.Date, t.Amount, t.Note, userID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert transfer: %w", err)
		}
		return logActivity(ctx, tx, activity{projectID: projectID, actorID: userID, action: "CREATE", entity: "transfer", entityID: id, summary: strconv.FormatInt(t.Amount, 10)})
	})
}
