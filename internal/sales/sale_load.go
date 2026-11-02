package sales

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Sale is one sale with where it stands today.
type Sale struct {
	ID             string     `json:"id"`
	UnitID         string     `json:"unitId"`
	UnitName       string     `json:"unitName"`
	UnitType       string     `json:"unitType"`
	Floor          int        `json:"floor"`
	Size           *int       `json:"size"`
	ListPrice      int64      `json:"listPrice"`
	CustomerID     string     `json:"customerId"`
	Customer       string     `json:"customer"`
	Phone          string     `json:"phone"`
	Date           time.Time  `json:"date"`
	Price          int64      `json:"price"`
	Down           int64      `json:"down"`
	Installments   int        `json:"installments"`
	IntervalMonths int        `json:"intervalMonths"`
	FirstDue       *time.Time `json:"firstDue"`
	Installment    int64      `json:"installment"`
	Standing       Standing   `json:"standing"`
	Rows           []Row      `json:"rows"`
}

func (sl *Sale) terms() Terms {
	t := Terms{Price: sl.Price, Down: sl.Down, Installments: sl.Installments, IntervalMonths: sl.IntervalMonths}
	if sl.FirstDue != nil {
		t.FirstDue = *sl.FirstDue
	}
	return t
}

// saleFilter picks which sales to load: all of a project's, or by sale, unit or customer.
type saleFilter struct {
	saleID, customerID string
}

const saleSelect = `
	SELECT sl.id, u.id, u.name, u.type, u.floor, u.size, u.price, c.id, c.name, c.phone,
	       sl.date, sl.price, sl.down, sl.installments, sl.interval_months, sl.first_due, sl.installment
	FROM sales sl JOIN units u ON u.id = sl.unit_id JOIN customers c ON c.id = sl.customer_id
	WHERE sl.project_id = $1 AND ($2 = '' OR sl.id::text = $2) AND ($3 = '' OR c.id::text = $3)
	ORDER BY sl.date DESC, sl.created_at DESC`

// loadSales reads sales and works out each one's schedule and standing as of today.
func loadSales(ctx context.Context, q db, projectID string, f saleFilter, today time.Time) ([]Sale, error) {
	rows, err := q.Query(ctx, saleSelect, projectID, f.saleID, f.customerID)
	if err != nil {
		return nil, fmt.Errorf("list sales: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Sale, error) {
		var sl Sale
		scanErr := row.Scan(&sl.ID, &sl.UnitID, &sl.UnitName, &sl.UnitType, &sl.Floor, &sl.Size, &sl.ListPrice, &sl.CustomerID, &sl.Customer, &sl.Phone,
			&sl.Date, &sl.Price, &sl.Down, &sl.Installments, &sl.IntervalMonths, &sl.FirstDue, &sl.Installment)
		return sl, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("list sales: %w", err)
	}
	paid, err := installmentPayments(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		sl := &list[i]
		sl.Rows = Schedule(sl.terms(), paid[sl.ID])
		sl.Standing = Stand(sl.terms(), sl.Rows, today)
	}
	return list, nil
}

// installmentPayments groups a project's installment payments by sale, oldest first.
func installmentPayments(ctx context.Context, q db, projectID string) (map[string][]Paid, error) {
	rows, err := q.Query(ctx, `
		SELECT sale_id, date, amount FROM sale_payments
		WHERE project_id = $1 AND kind = 'INSTALLMENT' ORDER BY date, receipt_no`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list installment payments: %w", err)
	}
	defer rows.Close()
	out := map[string][]Paid{}
	for rows.Next() {
		var saleID string
		var p Paid
		if err := rows.Scan(&saleID, &p.Date, &p.Amount); err != nil {
			return nil, fmt.Errorf("read payment: %w", err)
		}
		out[saleID] = append(out[saleID], p)
	}
	return out, rows.Err()
}
