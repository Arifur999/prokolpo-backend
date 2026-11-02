package sales

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
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

type saleBody struct {
	UnitID         string        `json:"unitId"`
	CustomerID     string        `json:"customerId"`
	Customer       *customerBody `json:"customer"`
	Date           string        `json:"date"`
	Price          int64         `json:"price"`
	Down           int64         `json:"down"`
	Installments   int           `json:"installments"`
	IntervalMonths int           `json:"intervalMonths"`
	FirstDue       string        `json:"firstDue"`
	AccountID      string        `json:"accountId"`
}

// SaleInput is a validated sale. Installments 0 means the full price on the day.
type SaleInput struct {
	UnitID     string
	CustomerID string
	Customer   *CustomerInput
	Date       time.Time
	Price      int64
	Down       int64
	Terms      Terms
	AccountID  string
}

func (b saleBody) validate(lang string, today time.Time) (SaleInput, error) {
	f := &fields{lang: lang}
	in := SaleInput{UnitID: b.UnitID, CustomerID: b.CustomerID, Price: b.Price, Down: b.Down, AccountID: b.AccountID}
	if b.UnitID == "" {
		f.add("unitId", "unit")
	}
	switch {
	case b.Customer != nil:
		c := b.Customer.validate(f, "customer.")
		in.Customer = &c
	case b.CustomerID == "":
		f.add("customerId", "customer")
	}
	date, err := time.Parse(time.DateOnly, b.Date)
	if err != nil || date.After(today) {
		f.add("date", "date")
	}
	in.Date = date
	if b.Price <= 0 || b.Price > maxAmount {
		f.add("price", "price")
	}
	if b.AccountID == "" && b.Down > 0 {
		f.add("accountId", "account")
	}
	if b.Installments == 0 {
		in.Down = b.Price
	} else {
		in.validateTerms(f, b)
	}
	in.Terms = Terms{Price: in.Price, Down: in.Down, Installments: b.Installments, IntervalMonths: max(b.IntervalMonths, 1), FirstDue: in.Terms.FirstDue}
	return in, f.err()
}

func (in *SaleInput) validateTerms(f *fields, b saleBody) {
	switch {
	case b.Installments < 1 || b.Installments > 240:
		f.add("installments", "count")
	case b.Down < 0 || b.Down >= b.Price:
		f.add("down", "down")
	case installmentFor(b.Price, b.Down, b.Installments) <= 0:
		f.add("down", "tooSmall")
	}
	if b.IntervalMonths != 1 && b.IntervalMonths != 3 && b.IntervalMonths != 6 {
		f.add("intervalMonths", "interval")
	}
	first, err := time.Parse(time.DateOnly, b.FirstDue)
	if err != nil || !first.After(in.Date) {
		f.add("firstDue", "firstDue")
	}
	in.Terms.FirstDue = first
}

// Sold is what the page needs after a sale: the sale and its down payment's receipt.
type Sold struct {
	SaleID    string  `json:"saleId"`
	PaymentID *string `json:"paymentId"`
	ReceiptNo *string `json:"receiptNo"`
}

// CreateSale sells a unit: the customer is added if new, the down payment (or the full
// price) is taken into an account with a receipt, and the installments start.
func (s *Service) CreateSale(ctx context.Context, userID, projectID, lang string, in SaleInput) (Sold, error) {
	if !isID(in.UnitID) {
		return Sold{}, invalid(lang, "unitId", "unit")
	}
	var out Sold
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var unitName string
		err := tx.QueryRow(ctx, `SELECT name FROM units WHERE id = $1 AND project_id = $2 FOR UPDATE`, in.UnitID, projectID).Scan(&unitName)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid(lang, "unitId", "unit")
		}
		if err != nil {
			return fmt.Errorf("load unit: %w", err)
		}
		customerID, err := saleParties(ctx, tx, userID, projectID, lang, in)
		if err != nil {
			return err
		}
		var firstDue *time.Time
		if in.Terms.Installments > 0 {
			firstDue = &in.Terms.FirstDue
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO sales (project_id, unit_id, customer_id, date, price, down, installments, interval_months, first_due, installment, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			projectID, in.UnitID, customerID, in.Date, in.Price, in.Down, in.Terms.Installments, in.Terms.IntervalMonths, firstDue,
			installmentFor(in.Price, in.Down, in.Terms.Installments), userID).Scan(&out.SaleID)
		if isUniqueViolation(err) {
			return messages.Fail(apperr.KindConflict, codeUnitSold)
		}
		if err != nil {
			return fmt.Errorf("insert sale: %w", err)
		}
		if in.Down > 0 {
			id, no, payErr := takePayment(ctx, tx, userID, projectID, out.SaleID, "DOWN", in.AccountID, in.Date, in.Down, nil)
			if payErr != nil {
				return payErr
			}
			out.PaymentID, out.ReceiptNo = &id, &no
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "sale", EntityID: out.SaleID, Summary: unitName,
			Details: map[string]string{"price": strconv.FormatInt(in.Price, 10)},
		})
	})
	return out, err
}

// saleParties finds or adds the buyer and checks the account the down payment goes into.
func saleParties(ctx context.Context, tx pgx.Tx, userID, projectID, lang string, in SaleInput) (string, error) {
	customerID := in.CustomerID
	if in.Customer != nil {
		id, err := upsertCustomer(ctx, tx, userID, projectID, *in.Customer)
		if err != nil {
			return "", err
		}
		customerID = id
	} else if ok, err := inProject(ctx, tx, "customers", projectID, customerID); err != nil || !ok {
		return "", orInvalid(err, invalid(lang, "customerId", "customer"))
	}
	if in.Down > 0 {
		if ok, err := inProject(ctx, tx, "money_accounts", projectID, in.AccountID); err != nil || !ok {
			return "", orInvalid(err, invalid(lang, "accountId", "account"))
		}
	}
	return customerID, nil
}

// takePayment records money a customer paid, with the project's next receipt number.
func takePayment(ctx context.Context, tx pgx.Tx, userID, projectID, saleID, kind, accountID string, date time.Time, amount int64, note *string) (id, receiptNo string, err error) {
	n, err := project.NextReceipt(ctx, tx, projectID)
	if err != nil {
		return "", "", err
	}
	var prefix string
	err = tx.QueryRow(ctx, `
		INSERT INTO sale_payments (project_id, sale_id, account_id, kind, date, amount, note, receipt_no, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, (SELECT receipt_prefix FROM projects WHERE id = $1)`,
		projectID, saleID, accountID, kind, date, amount, note, n, userID).Scan(&id, &prefix)
	if err != nil {
		return "", "", fmt.Errorf("insert sale payment: %w", err)
	}
	return id, project.ReceiptNo(prefix, n), nil
}

type collectBody struct {
	Amount    int64  `json:"amount"`
	AccountID string `json:"accountId"`
	Date      string `json:"date"`
	Note      string `json:"note"`
}

// CollectInput is a validated installment payment.
type CollectInput struct {
	Amount    int64
	AccountID string
	Date      time.Time
	Note      *string
}

func (b collectBody) validate(lang string, today time.Time) (CollectInput, error) {
	f := &fields{lang: lang}
	in := CollectInput{Amount: b.Amount, AccountID: b.AccountID, Date: today}
	if b.Amount <= 0 || b.Amount > maxAmount {
		f.add("amount", "amount")
	}
	if b.AccountID == "" {
		f.add("accountId", "account")
	}
	if b.Date != "" {
		d, err := time.Parse(time.DateOnly, b.Date)
		if err != nil || d.After(today) {
			f.add("date", "date")
		}
		in.Date = d
	}
	in.Note = optional(f, "note", strings.TrimSpace(b.Note), 200)
	return in, f.err()
}

// Collect takes installment money toward a sale; it fills the oldest installments first
// and may not go past what is owed.
func (s *Service) Collect(ctx context.Context, userID, projectID, saleID, lang string, in CollectInput) (Sold, error) {
	if !isID(saleID) {
		return Sold{}, notFound()
	}
	out := Sold{SaleID: saleID}
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM sales WHERE id = $1 FOR UPDATE`, saleID); err != nil {
			return fmt.Errorf("lock sale: %w", err)
		}
		list, err := loadSales(ctx, tx, projectID, saleFilter{saleID: saleID}, s.today())
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return notFound()
		}
		if in.Amount > list[0].Standing.Due {
			return messages.Fail(apperr.KindInvalid, codeOverpaid)
		}
		if ok, checkErr := inProject(ctx, tx, "money_accounts", projectID, in.AccountID); checkErr != nil || !ok {
			return orInvalid(checkErr, invalid(lang, "accountId", "account"))
		}
		id, no, err := takePayment(ctx, tx, userID, projectID, saleID, "INSTALLMENT", in.AccountID, in.Date, in.Amount, in.Note)
		if err != nil {
			return err
		}
		out.PaymentID, out.ReceiptNo = &id, &no
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "installment", EntityID: id,
			Summary: list[0].Customer, Details: map[string]string{"receipt": no, "unit": list[0].UnitName},
		})
	})
	return out, err
}

// orInvalid returns a database error when there is one, else the refusal.
func orInvalid(dbErr, refusal error) error {
	if dbErr != nil {
		return dbErr
	}
	return refusal
}
