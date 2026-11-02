package sales

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Sales lists every sale, newest first, with where each stands.
func (s *Service) Sales(ctx context.Context, userID, projectID string) ([]Sale, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	return loadSales(ctx, s.pool, projectID, saleFilter{}, s.today())
}

// Payment is money that came in on a sale.
type Payment struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Date        time.Time `json:"date"`
	Amount      int64     `json:"amount"`
	Account     string    `json:"account"`
	AccountKind string    `json:"accountKind"`
	Note        *string   `json:"note"`
	ReceiptNo   string    `json:"receiptNo"`
	TakenBy     string    `json:"takenBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SaleDetail is one sale's page: the customer, the installments and every payment.
type SaleDetail struct {
	Sale     Sale      `json:"sale"`
	NID      *string   `json:"nid"`
	Address  *string   `json:"address"`
	Rows     []Row     `json:"rows"`
	Payments []Payment `json:"payments"`
}

// Sale returns one sale with its schedule and payments.
func (s *Service) Sale(ctx context.Context, userID, projectID, saleID string) (SaleDetail, error) {
	if !isID(saleID) {
		return SaleDetail{}, notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return SaleDetail{}, err
	}
	list, err := loadSales(ctx, s.pool, projectID, saleFilter{saleID: saleID}, s.today())
	if err != nil {
		return SaleDetail{}, err
	}
	if len(list) == 0 {
		return SaleDetail{}, notFound()
	}
	d := SaleDetail{Sale: list[0], Rows: list[0].rows}
	if d.Rows == nil {
		d.Rows = []Row{}
	}
	err = s.pool.QueryRow(ctx, `SELECT nid, address FROM customers WHERE id = $1`, d.Sale.CustomerID).Scan(&d.NID, &d.Address)
	if err != nil {
		return SaleDetail{}, fmt.Errorf("load customer: %w", err)
	}
	if d.Payments, err = paymentsOf(ctx, s.pool, saleID); err != nil {
		return SaleDetail{}, err
	}
	return d, nil
}

func paymentsOf(ctx context.Context, q db, saleID string) ([]Payment, error) {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.kind, p.date, p.amount, a.name, a.kind, p.note, pr.receipt_prefix || '-' || lpad(p.receipt_no::text, 6, '0'),
		       u.first_name || ' ' || u.last_name, p.created_at
		FROM sale_payments p
		JOIN money_accounts a ON a.id = p.account_id
		JOIN projects pr ON pr.id = p.project_id
		JOIN users u ON u.id = p.created_by
		WHERE p.sale_id = $1 ORDER BY p.date, p.receipt_no`, saleID)
	if err != nil {
		return nil, fmt.Errorf("list sale payments: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Payment])
}
