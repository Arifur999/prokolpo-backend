package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Receipt is everything a printed sale receipt shows (board Receipt).
type Receipt struct {
	PaymentID    string    `json:"paymentId"`
	SaleID       string    `json:"saleId"`
	No           string    `json:"no"`
	Kind         string    `json:"kind"`
	Date         time.Time `json:"date"`
	Amount       int64     `json:"amount"`
	Payer        string    `json:"payer"`
	PayerPhone   string    `json:"payerPhone"`
	Unit         string    `json:"unit"`
	UnitType     string    `json:"unitType"`
	Floor        int       `json:"floor"`
	Size         *int      `json:"size"`
	Project      string    `json:"project"`
	Location     *string   `json:"location"`
	Company      string    `json:"company"`
	OwnerPhone   string    `json:"ownerPhone"`
	Account      string    `json:"account"`
	AccountKind  string    `json:"accountKind"`
	Note         *string   `json:"note"`
	TakenBy      string    `json:"takenBy"`
	TakenByRole  *string   `json:"takenByRole"`
	WrittenAt    time.Time `json:"writtenAt"`
	Prints       int       `json:"prints"`
	Price        int64     `json:"price"`
	PaidSoFar    int64     `json:"paidSoFar"`
	Installments int       `json:"installments"`
	// Covered are the installments this payment went toward; Next is what came due after it.
	Covered []Row   `json:"covered"`
	Next    *RowDue `json:"next"`
	Others  []Brief `json:"others"`
}

// Brief is one line of "all this sale's receipts".
type Brief struct {
	PaymentID string    `json:"paymentId"`
	No        string    `json:"no"`
	Kind      string    `json:"kind"`
	Date      time.Time `json:"date"`
	Amount    int64     `json:"amount"`
}

// Receipt returns a sale payment's receipt; the figures are as of that receipt.
func (s *Service) Receipt(ctx context.Context, userID, projectID, paymentID string) (Receipt, error) {
	if !isID(paymentID) {
		return Receipt{}, notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Receipt{}, err
	}
	var r Receipt
	var receiptNo int
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.sale_id, pr.receipt_prefix || '-' || lpad(p.receipt_no::text, 6, '0'), p.receipt_no, p.kind, p.date, p.amount,
		       c.name, c.phone, u.name, u.type, u.floor, u.size, pr.name, pr.location,
		       coalesce(o.company_name, o.first_name || ' ' || o.last_name), o.phone, a.name, a.kind, p.note,
		       t.first_name || ' ' || t.last_name, (SELECT m.role FROM project_members m WHERE m.project_id = pr.id AND m.user_id = t.id),
		       p.created_at, p.prints, sl.price, sl.installments
		FROM sale_payments p
		JOIN sales sl ON sl.id = p.sale_id
		JOIN customers c ON c.id = sl.customer_id
		JOIN units u ON u.id = sl.unit_id
		JOIN projects pr ON pr.id = p.project_id
		JOIN users o ON o.id = pr.owner_id
		JOIN users t ON t.id = p.created_by
		JOIN money_accounts a ON a.id = p.account_id
		WHERE p.id = $1 AND p.project_id = $2`, paymentID, projectID).Scan(
		&r.PaymentID, &r.SaleID, &r.No, &receiptNo, &r.Kind, &r.Date, &r.Amount, &r.Payer, &r.PayerPhone, &r.Unit, &r.UnitType, &r.Floor, &r.Size,
		&r.Project, &r.Location, &r.Company, &r.OwnerPhone, &r.Account, &r.AccountKind, &r.Note, &r.TakenBy, &r.TakenByRole,
		&r.WrittenAt, &r.Prints, &r.Price, &r.Installments)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, notFound()
	}
	if err != nil {
		return Receipt{}, fmt.Errorf("load receipt: %w", err)
	}
	if replayErr := s.replay(ctx, projectID, receiptNo, &r); replayErr != nil {
		return Receipt{}, replayErr
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, pr.receipt_prefix || '-' || lpad(p.receipt_no::text, 6, '0'), p.kind, p.date, p.amount
		FROM sale_payments p JOIN projects pr ON pr.id = p.project_id WHERE p.sale_id = $1 ORDER BY p.receipt_no DESC`, r.SaleID)
	if err != nil {
		return Receipt{}, fmt.Errorf("list receipts: %w", err)
	}
	if r.Others, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Brief]); err != nil {
		return Receipt{}, fmt.Errorf("list receipts: %w", err)
	}
	return r, nil
}

// replay works out, from the payments up to this receipt, what had been paid, which
// installments this payment covered, and what was due next.
func (s *Service) replay(ctx context.Context, projectID string, receiptNo int, r *Receipt) error {
	list, err := loadSales(ctx, s.pool, projectID, saleFilter{saleID: r.SaleID}, s.today())
	if err != nil || len(list) == 0 {
		return orInvalid(err, notFound())
	}
	sl := list[0]
	rows, err := s.pool.Query(ctx, `
		SELECT date, amount, receipt_no FROM sale_payments
		WHERE sale_id = $1 AND kind = 'INSTALLMENT' AND receipt_no <= $2 ORDER BY date, receipt_no`, r.SaleID, receiptNo)
	if err != nil {
		return fmt.Errorf("list payments: %w", err)
	}
	var before, upTo []Paid
	var paid int64
	err = func() error {
		defer rows.Close()
		for rows.Next() {
			var p Paid
			var no int
			if scanErr := rows.Scan(&p.Date, &p.Amount, &no); scanErr != nil {
				return scanErr
			}
			upTo = append(upTo, p)
			paid += p.Amount
			if no != receiptNo {
				before = append(before, p)
			}
		}
		return rows.Err()
	}()
	if err != nil {
		return fmt.Errorf("read payments: %w", err)
	}
	r.PaidSoFar = sl.Down + paid
	if sl.Installments == 0 || r.Kind == "DOWN" {
		r.PaidSoFar = sl.Down
	}
	after := Schedule(sl.terms(), upTo)
	if r.Kind == "INSTALLMENT" {
		was := Schedule(sl.terms(), before)
		for i := range after {
			if after[i].Paid > was[i].Paid {
				r.Covered = append(r.Covered, after[i])
			}
		}
	}
	if r.Kind == "DOWN" {
		after = Schedule(sl.terms(), nil)
	}
	r.Next = Stand(sl.terms(), after, r.Date).Next
	return nil
}

// Printed counts a print of a sale payment's receipt.
func (s *Service) Printed(ctx context.Context, userID, projectID, paymentID string) error {
	if !isID(paymentID) {
		return notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE sale_payments SET prints = prints + 1 WHERE id = $1 AND project_id = $2`, paymentID, projectID)
	if err != nil {
		return fmt.Errorf("count print: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notFound()
	}
	return nil
}
