package books

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// AttendanceDay is one team-day of a daily team.
type AttendanceDay struct {
	Day       string `json:"day"`
	Headcount int16  `json:"headcount"`
	HalfDay   bool   `json:"halfDay"`
	Rate      int64  `json:"rate"`
}

// ContractorDetail is the contractor ledger page.
type ContractorDetail struct {
	Contractor Contractor      `json:"contractor"`
	Lines      []LedgerLine    `json:"lines"`
	Days       []AttendanceDay `json:"days"`
}

// ContractorLedger returns a contractor with every charge and payment, oldest first.
// Attendance is summed into one line per week (Sunday to Saturday); the days come too,
// for the week rows that open up.
func (s *Service) ContractorLedger(ctx context.Context, userID, projectID, contractorID string) (ContractorDetail, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return ContractorDetail{}, err
	}
	c, err := contractorByID(ctx, s.pool, projectID, contractorID)
	if err != nil {
		return ContractorDetail{}, err
	}
	rows, err := s.pool.Query(ctx, `
		WITH lines AS (
			SELECT c.id::text AS ref, 'OPENING' AS kind, (c.created_at AT TIME ZONE 'Asia/Dhaka')::date AS date, c.created_at,
			       NULL::text AS title, NULL::text AS detail, c.opening_due AS charge, 0::bigint AS payment, NULL::text AS account, NULL::text AS purpose
			FROM contractors c WHERE c.id = $1 AND c.opening_due > 0
			UNION ALL
			SELECT c.id::text || '-contract', 'CONTRACT', (c.created_at AT TIME ZONE 'Asia/Dhaka')::date, c.created_at + interval '1 microsecond',
			       c.scope, NULL, c.contract_amount, 0, NULL, NULL
			FROM contractors c WHERE c.id = $1 AND c.contract_amount IS NOT NULL
			UNION ALL
			SELECT x.id::text, 'EXTRA', x.date, x.created_at, x.title, NULL, x.amount, 0, NULL, NULL
			FROM contractor_extras x WHERE x.contractor_id = $1
			UNION ALL
			SELECT 'week-' || to_char(w.start, 'YYYY-MM-DD'), 'WEEK', w.start, w.start::timestamptz,
			       to_char(w.start + 6, 'YYYY-MM-DD'), w.days::text, w.total, 0, NULL, NULL
			FROM (
				SELECT (a.day - extract(dow FROM a.day)::int) AS start, sum(`+dayWage+`)::bigint AS total,
				       sum(a.headcount * CASE WHEN a.half_day THEN 0.5 ELSE 1 END) AS days
				FROM attendance a WHERE a.contractor_id = $1 GROUP BY 1
			) w
			UNION ALL
			SELECT e.id::text, 'BILL', e.date, e.created_at, cat.name, e.description, e.amount, 0, NULL, NULL
			FROM expenses e JOIN expense_categories cat ON cat.id = e.category_id WHERE e.contractor_id = $1 AND e.deleted_at IS NULL
			UNION ALL
			SELECT p.id::text, 'PAYMENT', p.date, p.created_at, NULL, p.note, 0, p.amount, a.name, p.purpose
			FROM payments p JOIN money_accounts a ON a.id = p.account_id WHERE p.contractor_id = $1 AND p.deleted_at IS NULL
		)
		SELECT ref, kind, to_char(date, 'YYYY-MM-DD'), title, detail, charge, payment,
		       sum(charge - payment) OVER (ORDER BY kind <> 'OPENING', date, created_at, ref ROWS UNBOUNDED PRECEDING), account, purpose
		FROM lines ORDER BY kind <> 'OPENING', date, created_at, ref`, contractorID)
	if err != nil {
		return ContractorDetail{}, fmt.Errorf("contractor ledger: %w", err)
	}
	lines, err := pgx.CollectRows(rows, scanLedgerLine)
	if err != nil {
		return ContractorDetail{}, err
	}
	days, err := attendanceOf(ctx, s.pool, contractorID, time.Time{}, time.Time{})
	return ContractorDetail{Contractor: c, Lines: lines, Days: days}, err
}

func attendanceOf(ctx context.Context, q db, contractorID string, from, to time.Time) ([]AttendanceDay, error) {
	rows, err := q.Query(ctx, `
		SELECT to_char(day, 'YYYY-MM-DD'), headcount, half_day, rate FROM attendance
		WHERE contractor_id = $1 AND ($2::date = '0001-01-01' OR day BETWEEN $2 AND $3) ORDER BY day`, contractorID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list attendance: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AttendanceDay, error) {
		var d AttendanceDay
		err := row.Scan(&d.Day, &d.Headcount, &d.HalfDay, &d.Rate)
		return d, err
	})
}

// NewExtra is work agreed outside a contract.
type NewExtra struct {
	Title  string
	Amount int64
	Date   time.Time
}

type extraBody struct {
	Title  string `json:"title"`
	Amount int64  `json:"amount"`
	Date   string `json:"date"`
}

func (b extraBody) validate(lang string, today time.Time) (NewExtra, error) {
	f := &fields{lang: lang}
	ne := NewExtra{Amount: f.amount("amount", b.Amount, "amount"), Date: f.date("date", b.Date, today)}
	if title := f.text("title", b.Title, maxShortLen); title == nil {
		f.add("title", "title")
	} else {
		ne.Title = *title
	}
	return ne, f.err()
}

// AddExtra adds extra work to a contractor's account, raising what they are owed.
func (s *Service) AddExtra(ctx context.Context, userID, projectID, contractorID string, ne NewExtra) error {
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		ok, err := exists(ctx, tx, "contractors", projectID, contractorID)
		if err != nil {
			return fmt.Errorf("check contractor: %w", err)
		}
		if !ok {
			return notFound()
		}
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO contractor_extras (project_id, contractor_id, date, title, amount, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, projectID, contractorID, ne.Date, ne.Title, ne.Amount, userID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert extra: %w", err)
		}
		if err = markWorking(ctx, tx, contractorID); err != nil {
			return err
		}
		return logActivity(ctx, tx, activity{projectID: projectID, actorID: userID, action: "CREATE", entity: "extra", entityID: id, summary: ne.Title})
	})
}
