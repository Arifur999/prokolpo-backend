package books

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// What a supplier can be marked as supplying (board SupplierAdd).
var supplyKinds = []string{"রড", "সিমেন্ট", "ইট", "বালু", "পাথর/খোয়া", "টাইলস", "রং", "স্যানিটারি", "ইলেকট্রিক", "কাঠ/দরজা-জানালা", "গ্রিল/থাই গ্লাস", "হার্ডওয়্যার", "অন্যান্য"}

// Supplier is one row of the supplier list: what was billed, paid, and is still due.
// A negative due is an advance the supplier holds.
type Supplier struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Phone      *string    `json:"phone"`
	Address    *string    `json:"address"`
	Supplies   []string   `json:"supplies"`
	Note       *string    `json:"note"`
	OpeningDue int64      `json:"openingDue"`
	Billed     int64      `json:"billed"`
	Paid       int64      `json:"paid"`
	Due        int64      `json:"due"`
	BillCount  int        `json:"billCount"`
	PayCount   int        `json:"paymentCount"`
	LastDate   *time.Time `json:"lastDate"`
	// The oldest bill still not covered by payments, for "28 দিন ধরে বাকি".
	OldestDueDate *time.Time `json:"oldestDueDate"`
	CreatedAt     time.Time  `json:"createdAt"`
}

const supplierSelect = `
	WITH bills AS (
		SELECT supplier_id, sum(amount) AS total, count(*) AS n, max(date) AS last FROM expenses
		WHERE project_id = $1 AND supplier_id IS NOT NULL AND deleted_at IS NULL GROUP BY supplier_id
	), pays AS (
		SELECT supplier_id, sum(amount) AS total, count(*) AS n, max(date) AS last FROM payments
		WHERE project_id = $1 AND supplier_id IS NOT NULL AND deleted_at IS NULL GROUP BY supplier_id
	)
	SELECT s.id, s.name, s.phone, s.address, s.supplies, s.note, s.opening_due,
	       s.opening_due + coalesce(b.total, 0), coalesce(p.total, 0),
	       s.opening_due + coalesce(b.total, 0) - coalesce(p.total, 0),
	       coalesce(b.n, 0), coalesce(p.n, 0), greatest(b.last, p.last),
	       (SELECT min(e.date) FROM expenses e WHERE e.supplier_id = s.id AND e.deleted_at IS NULL
	          AND s.opening_due + (SELECT coalesce(sum(x.amount), 0) FROM expenses x WHERE x.supplier_id = s.id AND x.deleted_at IS NULL
	                               AND (x.date, x.created_at) <= (e.date, e.created_at)) > coalesce(p.total, 0)),
	       s.created_at
	FROM suppliers s LEFT JOIN bills b ON b.supplier_id = s.id LEFT JOIN pays p ON p.supplier_id = s.id
	WHERE s.project_id = $1`

func scanSupplier(row pgx.Row) (Supplier, error) {
	var s Supplier
	err := row.Scan(&s.ID, &s.Name, &s.Phone, &s.Address, &s.Supplies, &s.Note, &s.OpeningDue, &s.Billed, &s.Paid, &s.Due,
		&s.BillCount, &s.PayCount, &s.LastDate, &s.OldestDueDate, &s.CreatedAt)
	return s, err
}

// Suppliers lists the project's suppliers, most owed first.
func (s *Service) Suppliers(ctx context.Context, userID, projectID string) ([]Supplier, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, supplierSelect+` ORDER BY 10 DESC, s.name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Supplier, error) { return scanSupplier(row) })
}

func supplierByID(ctx context.Context, q db, projectID, id string) (Supplier, error) {
	if !isID(id) {
		return Supplier{}, notFound()
	}
	sup, err := scanSupplier(q.QueryRow(ctx, supplierSelect+` AND s.id = $2`, projectID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Supplier{}, notFound()
	}
	return sup, err
}

// NewSupplier is a validated supplier.
type NewSupplier struct {
	Name       string
	Phone      *string
	Address    *string
	Supplies   []string
	Note       *string
	OpeningDue int64
}

type supplierBody struct {
	Name       string   `json:"name"`
	Phone      string   `json:"phone"`
	Address    string   `json:"address"`
	Supplies   []string `json:"supplies"`
	Note       string   `json:"note"`
	OpeningDue int64    `json:"openingDue"`
}

func (b supplierBody) validate(lang string) (NewSupplier, error) {
	f := &fields{lang: lang}
	ns := NewSupplier{OpeningDue: b.OpeningDue, Supplies: []string{}}
	switch name := f.text("name", b.Name, 80); {
	case name == nil:
		f.add("name", "supplierName")
	case utf8.RuneCountInString(*name) < 2:
		f.add("name", "supplierShort")
	default:
		ns.Name = *name
	}
	if b.Phone != "" {
		p, err := phone.Normalize(b.Phone)
		if err != nil {
			f.add("phone", "phone")
		}
		ns.Phone = &p
	}
	ns.Address = f.text("address", b.Address, maxShortLen)
	ns.Note = f.text("note", b.Note, maxNoteLen)
	for _, item := range b.Supplies {
		if slices.Contains(supplyKinds, item) && !slices.Contains(ns.Supplies, item) {
			ns.Supplies = append(ns.Supplies, item)
		}
	}
	if b.OpeningDue < 0 || b.OpeningDue > maxAmount {
		f.add("openingDue", "amount")
	}
	return ns, f.err()
}

// CreateSupplier adds a supplier; an opening due becomes the first line of their ledger.
func (s *Service) CreateSupplier(ctx context.Context, userID, projectID, lang string, ns NewSupplier) (Supplier, error) {
	var sup Supplier
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO suppliers (project_id, name, phone, address, supplies, note, opening_due, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			projectID, ns.Name, ns.Phone, ns.Address, ns.Supplies, ns.Note, ns.OpeningDue, userID).Scan(&id)
		if isUniqueViolation(err) {
			return nameTaken(lang, "supplierTaken")
		}
		if err != nil {
			return fmt.Errorf("insert supplier: %w", err)
		}
		err = logActivity(ctx, tx, activity{projectID: projectID, actorID: userID, action: "CREATE", entity: "supplier", entityID: id, summary: ns.Name})
		if err != nil {
			return err
		}
		sup, err = supplierByID(ctx, tx, projectID, id)
		return err
	})
	return sup, err
}

// LedgerLine is one row of a supplier's or contractor's ledger, with the due after it.
type LedgerLine struct {
	Ref     string  `json:"ref"`
	Kind    string  `json:"kind"`
	Date    string  `json:"date"`
	Title   *string `json:"title"`
	Detail  *string `json:"detail"`
	Charge  int64   `json:"charge"`
	Payment int64   `json:"payment"`
	Due     int64   `json:"due"`
	Account *string `json:"account"`
	Purpose *string `json:"purpose"`
}

// SupplierDetail is the supplier ledger page: the totals and every bill and payment.
type SupplierDetail struct {
	Supplier Supplier     `json:"supplier"`
	Lines    []LedgerLine `json:"lines"`
}

// SupplierLedger returns a supplier with their bills and payments, oldest first.
func (s *Service) SupplierLedger(ctx context.Context, userID, projectID, supplierID string) (SupplierDetail, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return SupplierDetail{}, err
	}
	sup, err := supplierByID(ctx, s.pool, projectID, supplierID)
	if err != nil {
		return SupplierDetail{}, err
	}
	rows, err := s.pool.Query(ctx, `
		WITH lines AS (
			SELECT s.id::text AS ref, 'OPENING' AS kind, (s.created_at AT TIME ZONE 'Asia/Dhaka')::date AS date, s.created_at,
			       NULL::text AS title, s.note AS detail, s.opening_due AS charge, 0::bigint AS payment, NULL::text AS account, NULL::text AS purpose
			FROM suppliers s WHERE s.id = $1 AND s.opening_due > 0
			UNION ALL
			SELECT e.id::text, 'BILL', e.date, e.created_at, c.name, e.description, e.amount, 0, NULL, NULL
			FROM expenses e JOIN expense_categories c ON c.id = e.category_id WHERE e.supplier_id = $1 AND e.deleted_at IS NULL
			UNION ALL
			SELECT p.id::text, 'PAYMENT', p.date, p.created_at, NULL, p.note, 0, p.amount, a.name, p.purpose
			FROM payments p JOIN money_accounts a ON a.id = p.account_id WHERE p.supplier_id = $1 AND p.deleted_at IS NULL
		)
		SELECT ref, kind, to_char(date, 'YYYY-MM-DD'), title, detail, charge, payment,
		       sum(charge - payment) OVER (ORDER BY date, created_at, ref ROWS UNBOUNDED PRECEDING), account, purpose
		FROM lines ORDER BY date, created_at, ref`, supplierID)
	if err != nil {
		return SupplierDetail{}, fmt.Errorf("supplier ledger: %w", err)
	}
	lines, err := pgx.CollectRows(rows, scanLedgerLine)
	return SupplierDetail{Supplier: sup, Lines: lines}, err
}

func scanLedgerLine(row pgx.CollectableRow) (LedgerLine, error) {
	var l LedgerLine
	err := row.Scan(&l.Ref, &l.Kind, &l.Date, &l.Title, &l.Detail, &l.Charge, &l.Payment, &l.Due, &l.Account, &l.Purpose)
	return l, err
}
