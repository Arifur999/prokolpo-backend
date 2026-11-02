package shop

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

type collectBody struct {
	Amount    int64  `json:"amount"`
	AccountID string `json:"accountId"`
}

// CollectInput is credit paid back outside a day's entry.
type CollectInput struct {
	Amount    int64
	AccountID string
}

func (b collectBody) validate(lang string) (CollectInput, error) {
	f := &fields{lang: lang}
	if b.Amount <= 0 || b.Amount > maxAmount {
		f.add("amount", "amount")
	}
	if b.AccountID == "" {
		f.add("accountId", "account")
	}
	return CollectInput(b), f.err()
}

// Collect takes credit paid back today; it counts in today's paybacks.
func (s *Service) Collect(ctx context.Context, userID, projectID, personID, lang string, in CollectInput) error {
	if !isID(personID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT name FROM khata_people WHERE id = $1 AND project_id = $2 FOR UPDATE`, personID, projectID).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("lock credit person: %w", err)
		}
		_, due, err := personDue(ctx, tx, projectID, name)
		if err != nil {
			return err
		}
		if in.Amount > due {
			return messages.Fail(apperr.KindInvalid, codeOverpaid)
		}
		if ok, err := inProject(ctx, tx, "money_accounts", projectID, in.AccountID); err != nil || !ok {
			if err != nil {
				return err
			}
			return invalid(lang, "accountId", "account")
		}
		if err := addEntry(ctx, tx, userID, projectID, personID, nil, "PAYMENT", &in.AccountID, s.today(), in.Amount); err != nil {
			return err
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "khata_payment", EntityID: personID, Summary: name,
			Details: map[string]string{"amount": strconv.FormatInt(in.Amount, 10)},
		})
	})
}
