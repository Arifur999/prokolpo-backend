package sales

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
)

// Customer is a buyer with what they bought and owe across their sales.
type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	NID       *string   `json:"nid"`
	Address   *string   `json:"address"`
	AltPhone  *string   `json:"altPhone"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
	AddedBy   string    `json:"addedBy"`
	Sales     []Sale    `json:"sales"`
	// Payments, newest first, are filled only on a single customer's profile.
	Payments []CustomerPayment `json:"payments,omitempty"`
}

// CustomerPayment is a payment on one of the customer's sales.
type CustomerPayment struct {
	Payment
	SaleID   string `json:"saleId"`
	UnitName string `json:"unitName"`
}

func customersOf(ctx context.Context, q db, projectID, customerID string) ([]Customer, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, c.name, c.phone, c.nid, c.address, c.alt_phone, c.note, c.created_at, u.first_name || ' ' || u.last_name
		FROM customers c JOIN users u ON u.id = c.created_by
		WHERE c.project_id = $1 AND ($2 = '' OR c.id::text = $2) ORDER BY c.created_at DESC`, projectID, customerID)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Customer, error) {
		c := Customer{Sales: []Sale{}}
		scanErr := row.Scan(&c.ID, &c.Name, &c.Phone, &c.NID, &c.Address, &c.AltPhone, &c.Note, &c.CreatedAt, &c.AddedBy)
		return c, scanErr
	})
}

// Customers lists the buyers, newest first, each with their sales.
func (s *Service) Customers(ctx context.Context, userID, projectID string) ([]Customer, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	return s.customersWithSales(ctx, projectID, "")
}

// Customer returns one buyer's profile.
func (s *Service) Customer(ctx context.Context, userID, projectID, customerID string) (Customer, error) {
	if !isID(customerID) {
		return Customer{}, notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Customer{}, err
	}
	list, err := s.customersWithSales(ctx, projectID, customerID)
	if err != nil {
		return Customer{}, err
	}
	if len(list) == 0 {
		return Customer{}, notFound()
	}
	c := list[0]
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.kind, p.date, p.amount, a.name, a.kind, p.note, pr.receipt_prefix || '-' || lpad(p.receipt_no::text, 6, '0'),
		       u.first_name || ' ' || u.last_name, p.created_at, sl.id, un.name
		FROM sale_payments p
		JOIN sales sl ON sl.id = p.sale_id
		JOIN units un ON un.id = sl.unit_id
		JOIN money_accounts a ON a.id = p.account_id
		JOIN projects pr ON pr.id = p.project_id
		JOIN users u ON u.id = p.created_by
		WHERE sl.customer_id = $1 ORDER BY p.date DESC, p.receipt_no DESC`, customerID)
	if err != nil {
		return Customer{}, fmt.Errorf("list customer payments: %w", err)
	}
	c.Payments, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (CustomerPayment, error) {
		var p CustomerPayment
		scanErr := row.Scan(&p.ID, &p.Kind, &p.Date, &p.Amount, &p.Account, &p.AccountKind, &p.Note, &p.ReceiptNo, &p.TakenBy, &p.CreatedAt, &p.SaleID, &p.UnitName)
		return p, scanErr
	})
	if err != nil {
		return Customer{}, fmt.Errorf("list customer payments: %w", err)
	}
	return c, nil
}

func (s *Service) customersWithSales(ctx context.Context, projectID, customerID string) ([]Customer, error) {
	list, err := customersOf(ctx, s.pool, projectID, customerID)
	if err != nil {
		return nil, err
	}
	sold, err := loadSales(ctx, s.pool, projectID, saleFilter{customerID: customerID}, s.today())
	if err != nil {
		return nil, err
	}
	byCustomer := map[string]*Customer{}
	for i := range list {
		byCustomer[list[i].ID] = &list[i]
	}
	for i := range sold {
		if c, ok := byCustomer[sold[i].CustomerID]; ok {
			c.Sales = append(c.Sales, sold[i])
		}
	}
	return list, nil
}

// CustomerInput is a validated buyer.
type CustomerInput struct {
	Name     string
	Phone    string
	NID      *string
	Address  *string
	AltPhone *string
	Note     *string
}

type customerBody struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	NID      string `json:"nid"`
	Address  string `json:"address"`
	AltPhone string `json:"altPhone"`
	Note     string `json:"note"`
}

func (b customerBody) validate(f *fields, prefix string) CustomerInput {
	c := CustomerInput{Name: strings.TrimSpace(b.Name)}
	if n := len([]rune(c.Name)); n < 2 || n > 60 {
		f.add(prefix+"name", "name")
	}
	p, err := phone.Normalize(b.Phone)
	if err != nil {
		f.add(prefix+"phone", "phone")
	}
	c.Phone = p
	c.NID = optional(f, prefix+"nid", b.NID, 30)
	c.Address = optional(f, prefix+"address", b.Address, 200)
	c.Note = optional(f, prefix+"note", b.Note, 300)
	if alt := strings.TrimSpace(b.AltPhone); alt != "" {
		p, err := phone.Normalize(alt)
		if err != nil {
			f.add(prefix+"altPhone", "phone")
		}
		c.AltPhone = &p
	}
	return c
}

// optional trims s; blank is nil, too long is a field error.
func optional(f *fields, path, s string, limit int) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len([]rune(s)) > limit {
		f.add(path, "text")
	}
	return &s
}

// AddCustomer adds a buyer, or returns the one already using that number.
func (s *Service) AddCustomer(ctx context.Context, userID, projectID string, in CustomerInput) (Customer, error) {
	var id string
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var err error
		id, err = upsertCustomer(ctx, tx, userID, projectID, in)
		return err
	})
	if err != nil {
		return Customer{}, err
	}
	return s.Customer(ctx, userID, projectID, id)
}

// upsertCustomer adds a buyer inside tx, or finds the one with the same number.
func upsertCustomer(ctx context.Context, tx pgx.Tx, userID, projectID string, in CustomerInput) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO customers (project_id, name, phone, nid, address, alt_phone, note, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, phone) DO NOTHING RETURNING id`, projectID, in.Name, in.Phone, in.NID, in.Address, in.AltPhone, in.Note, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM customers WHERE project_id = $1 AND phone = $2`, projectID, in.Phone).Scan(&id)
		return id, err
	}
	if err != nil {
		return "", fmt.Errorf("insert customer: %w", err)
	}
	return id, activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "customer", EntityID: id, Summary: in.Name})
}
