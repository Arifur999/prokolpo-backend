package books

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PhaseTotal is spending in one phase (a floor, a batch) against its budget.
type PhaseTotal struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
	Budget *int64 `json:"budget"`
}

// SupplierTotal is what one supplier billed and was paid in the period.
type SupplierTotal struct {
	Name   string `json:"name"`
	Billed int64  `json:"billed"`
	Paid   int64  `json:"paid"`
}

// ExpenseReport is the spending between From and To, grouped four ways. Every head,
// phase and supplier is listed, the unused ones with 0. Labour has no head or phase, so
// it is its own head and falls under Unphased.
type ExpenseReport struct {
	From       string          `json:"from"`
	To         string          `json:"to"`
	Budget     *int64          `json:"budget"`
	Categories []CategoryTotal `json:"categories"`
	Phases     []PhaseTotal    `json:"phases"`
	Unphased   int64           `json:"unphased"`
	Months     []MonthTotal    `json:"months"`
	Suppliers  []SupplierTotal `json:"suppliers"`
}

// labourCTE is labour paid for between $2 and $3: team-day wages, extra work and payments
// to contract workers.
const labourCTE = `
WITH labour AS (
	SELECT day AS date, headcount * rate / CASE WHEN half_day THEN 2 ELSE 1 END AS amount FROM attendance WHERE project_id = $1
	UNION ALL
	SELECT date, amount FROM contractor_extras WHERE project_id = $1
	UNION ALL
	SELECT p.date, p.amount FROM payments p JOIN contractors c ON c.id = p.contractor_id
	WHERE p.project_id = $1 AND p.deleted_at IS NULL AND NOT p.paid_with_expense AND c.pay_type = 'CONTRACT'
), period_labour AS (SELECT coalesce(sum(amount), 0)::bigint AS amount FROM labour WHERE date BETWEEN $2 AND $3)`

// ExpenseReport reports spending between from and to; a nil from means since the
// project started, a nil to means today.
func (s *Service) ExpenseReport(ctx context.Context, userID, projectID string, from, to *time.Time) (ExpenseReport, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return ExpenseReport{}, err
	}
	var start time.Time
	r := ExpenseReport{}
	err := s.pool.QueryRow(ctx, `SELECT start_date, budget FROM projects WHERE id = $1`, projectID).Scan(&start, &r.Budget)
	if err != nil {
		return ExpenseReport{}, fmt.Errorf("load project: %w", err)
	}
	end := s.today()
	if from != nil {
		start = *from
	}
	if to != nil {
		end = *to
	}
	r.From, r.To = start.Format(time.DateOnly), end.Format(time.DateOnly)
	steps := []func(context.Context, string, time.Time, time.Time, *ExpenseReport) error{
		s.reportCategories, s.reportPhases, s.reportMonths, s.reportSuppliers,
	}
	for _, step := range steps {
		if err := step(ctx, projectID, start, end, &r); err != nil {
			return ExpenseReport{}, err
		}
	}
	return r, nil
}

func (s *Service) reportCategories(ctx context.Context, projectID string, from, to time.Time, r *ExpenseReport) error {
	rows, err := s.pool.Query(ctx, labourCTE+`
		SELECT name, amount, labour FROM (
			SELECT c.name, coalesce(sum(e.amount), 0)::bigint AS amount, false AS labour
			FROM expense_categories c
			LEFT JOIN expenses e ON e.category_id = c.id AND e.deleted_at IS NULL AND e.date BETWEEN $2 AND $3
			WHERE c.project_id = $1 GROUP BY c.id, c.name
			UNION ALL
			SELECT '', amount, true FROM period_labour
		) t WHERE amount > 0 OR NOT labour ORDER BY amount DESC, name`, projectID, from, to)
	if err != nil {
		return fmt.Errorf("report heads: %w", err)
	}
	r.Categories, err = pgx.CollectRows(rows, pgx.RowToStructByPos[CategoryTotal])
	return err
}

func (s *Service) reportPhases(ctx context.Context, projectID string, from, to time.Time, r *ExpenseReport) error {
	rows, err := s.pool.Query(ctx, `
		SELECT ph.name, coalesce(sum(e.amount), 0)::bigint, ph.budget
		FROM project_phases ph
		LEFT JOIN expenses e ON e.phase_id = ph.id AND e.deleted_at IS NULL AND e.date BETWEEN $2 AND $3
		WHERE ph.project_id = $1 GROUP BY ph.id ORDER BY ph.position, ph.created_at`, projectID, from, to)
	if err != nil {
		return fmt.Errorf("report phases: %w", err)
	}
	if r.Phases, err = pgx.CollectRows(rows, pgx.RowToStructByPos[PhaseTotal]); err != nil {
		return fmt.Errorf("report phases: %w", err)
	}
	err = s.pool.QueryRow(ctx, labourCTE+`
		SELECT coalesce((SELECT sum(amount) FROM expenses WHERE project_id = $1 AND phase_id IS NULL AND deleted_at IS NULL
		                 AND date BETWEEN $2 AND $3), 0)::bigint + (SELECT amount FROM period_labour)`, projectID, from, to).Scan(&r.Unphased)
	if err != nil {
		return fmt.Errorf("report unphased: %w", err)
	}
	return nil
}

// reportMonths totals each month of the period, the empty ones too.
func (s *Service) reportMonths(ctx context.Context, projectID string, from, to time.Time, r *ExpenseReport) error {
	rows, err := s.pool.Query(ctx, spendingCTE+`
		SELECT to_char(m, 'YYYY-MM'), coalesce((SELECT sum(amount) FROM spending
		       WHERE date_trunc('month', date) = m AND date BETWEEN $2 AND $3), 0)::bigint
		FROM generate_series(date_trunc('month', $2::date), date_trunc('month', $3::date), interval '1 month') m
		ORDER BY m`, projectID, from, to)
	if err != nil {
		return fmt.Errorf("report months: %w", err)
	}
	r.Months, err = pgx.CollectRows(rows, pgx.RowToStructByPos[MonthTotal])
	return err
}

func (s *Service) reportSuppliers(ctx context.Context, projectID string, from, to time.Time, r *ExpenseReport) error {
	rows, err := s.pool.Query(ctx, `
		SELECT s.name,
		       coalesce((SELECT sum(amount) FROM expenses e WHERE e.supplier_id = s.id AND e.deleted_at IS NULL AND e.date BETWEEN $2 AND $3), 0)::bigint AS billed,
		       coalesce((SELECT sum(amount) FROM payments p WHERE p.supplier_id = s.id AND p.deleted_at IS NULL AND p.date BETWEEN $2 AND $3), 0)::bigint
		FROM suppliers s WHERE s.project_id = $1 ORDER BY billed DESC, s.name`, projectID, from, to)
	if err != nil {
		return fmt.Errorf("report suppliers: %w", err)
	}
	r.Suppliers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[SupplierTotal])
	return err
}
