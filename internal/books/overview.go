package books

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Figures are the key numbers on the overview.
type Figures struct {
	Cash            int64 `json:"cash"`
	SupplierDue     int64 `json:"supplierDue"`
	SupplierCount   int   `json:"supplierCount"`
	ContractorDue   int64 `json:"contractorDue"`
	ContractorCount int   `json:"contractorCount"`
	MonthSpent      int64 `json:"monthSpent"`
	LastMonthSpent  int64 `json:"lastMonthSpent"`
}

// MonthTotal is one month's spending ("2026-09").
type MonthTotal struct {
	Month  string `json:"month"`
	Amount int64  `json:"amount"`
}

// CategoryTotal is spending under one head. Labour (daily wages, extra work, contract
// payments) is one row with Labour set, since it has no expense head.
type CategoryTotal struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
	Labour bool   `json:"labour"`
}

// RecentEntry is one line of "সাম্প্রতিক এন্ট্রি".
type RecentEntry struct {
	Ref       string  `json:"ref"`
	Kind      string  `json:"kind"`
	Date      string  `json:"date"`
	Title     *string `json:"title"`
	Detail    *string `json:"detail"`
	Party     *string `json:"party"`
	Phase     *string `json:"phase"`
	Account   *string `json:"account"`
	Quantity  *string `json:"quantity"`
	Unit      *string `json:"unit"`
	Amount    int64   `json:"amount"`
	ExpenseID *string `json:"expenseId"`
}

// Overview is everything the overview page shows beyond the project itself.
type Overview struct {
	Figures    Figures         `json:"figures"`
	Months     []MonthTotal    `json:"months"`
	Categories []CategoryTotal `json:"categories"`
	Recent     []RecentEntry   `json:"recent"`
}

// contractPaidSQL dates contract work by its payments ($1 is the project), counting each
// payment only up to what is left of the contract, as project_labour() does: extra work is
// counted on its own and an advance past the contract is not spending yet.
const contractPaidSQL = `SELECT date, CASE WHEN contract_amount IS NULL THEN amount
		ELSE least(amount, greatest(contract_amount - (paid - amount), 0)) END
	FROM (
		SELECT p.date, p.amount, c.contract_amount,
			sum(p.amount) OVER (PARTITION BY p.contractor_id ORDER BY p.date, p.id) AS paid
		FROM payments p JOIN contractors c ON c.id = p.contractor_id
		WHERE p.project_id = $1 AND p.deleted_at IS NULL AND NOT p.paid_with_expense AND c.pay_type = 'CONTRACT'
	) contract_paid`

// spendingCTE is every taka spent, dated ($1 is the project): bills, team-day wages, extra
// work and payments to contract workers.
const spendingCTE = `
WITH spending AS (
	SELECT date, amount FROM expenses WHERE project_id = $1 AND deleted_at IS NULL
	UNION ALL
	SELECT day, headcount * rate / CASE WHEN half_day THEN 2 ELSE 1 END FROM attendance WHERE project_id = $1
	UNION ALL
	SELECT date, amount FROM contractor_extras WHERE project_id = $1
	UNION ALL
	` + contractPaidSQL + `
)`

// Overview gathers the overview page's figures, the last 12 months, the heads and the
// latest entries.
func (s *Service) Overview(ctx context.Context, userID, projectID string) (Overview, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return Overview{}, err
	}
	var o Overview
	today := s.today()
	monthStart := today.AddDate(0, 0, 1-today.Day())

	accounts, err := accountsOf(ctx, s.pool, projectID)
	if err != nil {
		return Overview{}, err
	}
	for _, a := range accounts {
		o.Figures.Cash += a.Balance
	}

	f := &o.Figures
	err = s.pool.QueryRow(ctx, `
		SELECT coalesce(sum(due) FILTER (WHERE due > 0), 0), count(*) FILTER (WHERE due > 0) FROM (`+supplierSelect+`) x(id, name, phone, address, supplies, note, opening_due, billed, paid, due, bills, pays, last_date, oldest, created_at)`, projectID).Scan(&f.SupplierDue, &f.SupplierCount)
	if err != nil {
		return Overview{}, fmt.Errorf("supplier dues: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT coalesce(sum(due) FILTER (WHERE due > 0), 0), count(*) FILTER (WHERE due > 0)
		FROM (`+contractorSelect+`) x(id, name, phone, work, pay_type, contract_amount, scope, daily_wage, team_size, status, opening_due,
		      extras, wages, days, bills, total, paid, due, last_date, last_amount, created_at)`, projectID).Scan(&f.ContractorDue, &f.ContractorCount)
	if err != nil {
		return Overview{}, fmt.Errorf("contractor dues: %w", err)
	}

	rows, err := s.pool.Query(ctx, spendingCTE+`
		SELECT to_char(m, 'YYYY-MM'), coalesce((SELECT sum(amount) FROM spending WHERE date_trunc('month', date) = m), 0)::bigint
		FROM generate_series(date_trunc('month', $2::date) - interval '11 months', date_trunc('month', $2::date), interval '1 month') m
		ORDER BY m`, projectID, monthStart)
	if err != nil {
		return Overview{}, fmt.Errorf("monthly spending: %w", err)
	}
	o.Months, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (MonthTotal, error) {
		var m MonthTotal
		scanErr := row.Scan(&m.Month, &m.Amount)
		return m, scanErr
	})
	if err != nil {
		return Overview{}, err
	}
	if n := len(o.Months); n >= 2 {
		f.MonthSpent, f.LastMonthSpent = o.Months[n-1].Amount, o.Months[n-2].Amount
	}

	if o.Categories, err = categoryTotals(ctx, s.pool, projectID); err != nil {
		return Overview{}, err
	}
	o.Recent, err = recentEntries(ctx, s.pool, projectID, 6)
	return o, err
}

func categoryTotals(ctx context.Context, q db, projectID string) ([]CategoryTotal, error) {
	rows, err := q.Query(ctx, `
		SELECT name, amount, labour FROM (
			SELECT c.name, sum(e.amount)::bigint AS amount, false AS labour
			FROM expenses e JOIN expense_categories c ON c.id = e.category_id
			WHERE e.project_id = $1 AND e.deleted_at IS NULL GROUP BY c.name
			UNION ALL
			SELECT '', project_labour($1), true
		) t WHERE amount > 0 ORDER BY amount DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("category totals: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (CategoryTotal, error) {
		var c CategoryTotal
		err := row.Scan(&c.Name, &c.Amount, &c.Labour)
		return c, err
	})
}

func recentEntries(ctx context.Context, q db, projectID string, limit int) ([]RecentEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT * FROM (
			SELECT e.id::text, 'EXPENSE', to_char(e.date, 'YYYY-MM-DD'), c.name, e.description, coalesce(s.name, k.name), ph.name, NULL::text,
			       e.quantity::text, e.unit, e.amount, e.id::text, e.created_at
			FROM expenses e JOIN expense_categories c ON c.id = e.category_id
			LEFT JOIN suppliers s ON s.id = e.supplier_id LEFT JOIN contractors k ON k.id = e.contractor_id
			LEFT JOIN project_phases ph ON ph.id = e.phase_id
			WHERE e.project_id = $1 AND e.deleted_at IS NULL
			UNION ALL
			SELECT p.id::text, 'PAYMENT', to_char(p.date, 'YYYY-MM-DD'), NULL, p.note, coalesce(s.name, k.name), NULL, a.name,
			       NULL, NULL, p.amount, NULL, p.created_at
			FROM payments p JOIN money_accounts a ON a.id = p.account_id
			LEFT JOIN suppliers s ON s.id = p.supplier_id LEFT JOIN contractors k ON k.id = p.contractor_id
			WHERE p.project_id = $1 AND p.deleted_at IS NULL AND NOT p.paid_with_expense
		) r ORDER BY 3 DESC, 13 DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent entries: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (RecentEntry, error) {
		var r RecentEntry
		var created any
		err := row.Scan(&r.Ref, &r.Kind, &r.Date, &r.Title, &r.Detail, &r.Party, &r.Phase, &r.Account, &r.Quantity, &r.Unit, &r.Amount, &r.ExpenseID, &created)
		return r, err
	})
}
