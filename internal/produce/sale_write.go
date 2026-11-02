package produce

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

type saleBody struct {
	Product   string  `json:"product"`
	PhaseID   string  `json:"phaseId"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	Rate      int64   `json:"rate"`
	Buyer     string  `json:"buyer"`
	Date      string  `json:"date"`
	Received  int64   `json:"received"`
	AccountID string  `json:"accountId"`
}

// SaleInput is a validated sale; the product is checked against the farm on save.
type SaleInput struct {
	Product   string
	PhaseID   *string
	Quantity  float64
	Unit      string
	Rate      int64
	Amount    int64
	Buyer     string
	Date      time.Time
	Received  int64
	AccountID string
}

func (b saleBody) validate(lang string, today time.Time) (SaleInput, error) {
	f := &fields{lang: lang}
	in := SaleInput{Product: b.Product, Unit: b.Unit, Rate: b.Rate, Received: b.Received, AccountID: b.AccountID}
	if b.PhaseID != "" {
		in.PhaseID = &b.PhaseID
	}
	// Three decimals, as the column keeps them.
	in.Quantity = math.Round(b.Quantity*1000) / 1000
	if in.Quantity <= 0 || in.Quantity > 1e9 {
		f.add("quantity", "quantity")
	}
	if b.Rate <= 0 || b.Rate > maxAmount {
		f.add("rate", "rate")
	}
	// Checked as a float first: a huge product would not fit in int64. A sale also has to come
	// to at least one taka (the column refuses 0).
	if amount := math.Round(in.Quantity * float64(b.Rate)); amount > maxAmount {
		f.add("quantity", "quantity")
	} else {
		in.Amount = int64(amount)
		if in.Amount < 1 && in.Quantity > 0 && b.Rate > 0 {
			f.add("rate", "rate")
		}
	}
	in.Buyer = strings.TrimSpace(b.Buyer)
	if in.Buyer == "" || len([]rune(in.Buyer)) > 80 {
		f.add("buyer", "buyer")
	}
	date, err := time.Parse(time.DateOnly, b.Date)
	if err != nil || date.After(today) {
		f.add("date", "date")
	}
	in.Date = date
	in.validateReceived(f)
	return in, f.err()
}

// validateReceived checks the money taken on the spot against the sale.
func (in SaleInput) validateReceived(f *fields) {
	if in.Received < 0 || (in.Amount > 0 && in.Received > in.Amount) {
		f.add("received", "received")
	}
	if in.Received > 0 && in.AccountID == "" {
		f.add("accountId", "account")
	}
}

// Saved is the sale just written.
type Saved struct {
	ID     string `json:"id"`
	Amount int64  `json:"amount"`
	Left   int64  `json:"left"`
}

// CreateSale records a sale: the buyer is added if new, and money taken on the spot goes
// into an account.
func (s *Service) CreateSale(ctx context.Context, userID, projectID, lang string, in SaleInput) (Saved, error) {
	out := Saved{Amount: in.Amount, Left: in.Amount - in.Received}
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		if err := checkSale(ctx, tx, projectID, lang, in); err != nil {
			return err
		}
		buyerID, err := upsertBuyer(ctx, tx, projectID, in.Buyer)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO produce_sales (project_id, phase_id, buyer_id, date, product, quantity, unit, rate, amount, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
			projectID, in.PhaseID, buyerID, in.Date, in.Product, in.Quantity, in.Unit, in.Rate, in.Amount, userID).Scan(&out.ID)
		if err != nil {
			return fmt.Errorf("insert produce sale: %w", err)
		}
		if in.Received > 0 {
			saleID := out.ID
			if err := receive(ctx, tx, userID, projectID, buyerID, &saleID, in.AccountID, in.Date, in.Received); err != nil {
				return err
			}
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "produce", EntityID: out.ID, Summary: in.Buyer,
			Details: map[string]string{
				"product": in.Product, "unit": in.Unit, "quantity": strconv.FormatFloat(in.Quantity, 'f', -1, 64),
				"amount": strconv.FormatInt(in.Amount, 10), "left": strconv.FormatInt(out.Left, 10),
			},
		})
	})
	return out, err
}

// checkSale checks the sale against the farm: what it sells, its batches, its accounts.
func checkSale(ctx context.Context, tx pgx.Tx, projectID, lang string, in SaleInput) error {
	var farmType *string
	if err := tx.QueryRow(ctx, `SELECT farm_type FROM projects WHERE id = $1`, projectID).Scan(&farmType); err != nil {
		return fmt.Errorf("load farm type: %w", err)
	}
	if farmType == nil {
		return messages.Fail(apperr.KindInvalid, codeNotFarm)
	}
	f := &fields{lang: lang}
	if !sellable(*farmType, in.Product, in.Unit) {
		f.add("product", "product")
	}
	if in.PhaseID != nil {
		ok, err := isBatch(ctx, tx, projectID, *in.PhaseID)
		if err != nil {
			return err
		}
		if !ok {
			f.add("phaseId", "batch")
		}
	}
	if in.Received > 0 {
		if ok, err := inProject(ctx, tx, "money_accounts", projectID, in.AccountID); err != nil || !ok {
			if err != nil {
				return err
			}
			f.add("accountId", "account")
		}
	}
	return f.err()
}

// isBatch reports whether a phase is one of the farm's batches (not its setup).
func isBatch(ctx context.Context, q db, projectID, phaseID string) (bool, error) {
	if !isID(phaseID) {
		return false, nil
	}
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_phases WHERE id = $1 AND project_id = $2 AND NOT setup)`, phaseID, projectID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check batch: %w", err)
	}
	return ok, nil
}

// upsertBuyer finds a buyer by name, ignoring case, or adds them.
func upsertBuyer(ctx context.Context, tx pgx.Tx, projectID, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO produce_buyers (project_id, name) VALUES ($1, $2)
		ON CONFLICT (project_id, (lower(name))) DO UPDATE SET name = produce_buyers.name
		RETURNING id`, projectID, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert buyer: %w", err)
	}
	return id, nil
}

func receive(ctx context.Context, tx pgx.Tx, userID, projectID, buyerID string, saleID *string, accountID string, date time.Time, amount int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO produce_receipts (project_id, buyer_id, sale_id, account_id, date, amount, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, projectID, buyerID, saleID, accountID, date, amount, userID)
	if err != nil {
		return fmt.Errorf("insert produce receipt: %w", err)
	}
	return nil
}

type collectBody struct {
	Amount    int64  `json:"amount"`
	AccountID string `json:"accountId"`
}

// CollectInput is money a buyer paid toward what they owe.
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

// Collect takes money a buyer owed; it may not go past their due.
func (s *Service) Collect(ctx context.Context, userID, projectID, buyerID, lang string, in CollectInput) error {
	if !isID(buyerID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT name FROM produce_buyers WHERE id = $1 AND project_id = $2 FOR UPDATE`, buyerID, projectID).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("lock buyer: %w", err)
		}
		due, err := buyerDue(ctx, tx, projectID, buyerID)
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
		if err := receive(ctx, tx, userID, projectID, buyerID, nil, in.AccountID, s.today(), in.Amount); err != nil {
			return err
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "produce_payment", EntityID: buyerID, Summary: name,
			Details: map[string]string{"amount": strconv.FormatInt(in.Amount, 10)},
		})
	})
}

// buyerDue is what a buyer still owes across their sales.
func buyerDue(ctx context.Context, q db, projectID, buyerID string) (int64, error) {
	var due int64
	err := q.QueryRow(ctx, `
		SELECT coalesce((SELECT sum(amount) FROM produce_sales WHERE project_id = $1 AND buyer_id = $2), 0)
		     - coalesce((SELECT sum(amount) FROM produce_receipts WHERE project_id = $1 AND buyer_id = $2), 0)`,
		projectID, buyerID).Scan(&due)
	if err != nil {
		return 0, fmt.Errorf("sum buyer due: %w", err)
	}
	return due, nil
}
