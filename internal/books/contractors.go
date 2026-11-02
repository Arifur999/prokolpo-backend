package books

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

var (
	// The kinds of work on board ContractorAdd.
	workKinds = []string{"রাজমিস্ত্রি", "রড মিস্ত্রি", "রং", "টাইলস", "স্যানিটারি", "ইলেকট্রিক", "কাঠমিস্ত্রি", "লেবার/যোগালি", "পাইলিং", "অন্যান্য"}
	statuses  = []string{"NOT_STARTED", "WORKING", "DONE"}
)

// Contractor is one row of the contractor list. Total is everything they have earned
// here: opening due, the contract, extra work, attendance wages and any bills.
type Contractor struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Phone          *string    `json:"phone"`
	Work           string     `json:"work"`
	PayType        string     `json:"payType"`
	ContractAmount *int64     `json:"contractAmount"`
	Scope          *string    `json:"scope"`
	DailyWage      *int64     `json:"dailyWage"`
	TeamSize       *int16     `json:"teamSize"`
	Status         string     `json:"status"`
	OpeningDue     int64      `json:"openingDue"`
	Extras         int64      `json:"extras"`
	Wages          int64      `json:"wages"`
	ManDays        float64    `json:"manDays"`
	Bills          int64      `json:"bills"`
	Total          int64      `json:"total"`
	Paid           int64      `json:"paid"`
	Due            int64      `json:"due"`
	LastPayDate    *time.Time `json:"lastPayDate"`
	LastPayAmount  *int64     `json:"lastPayAmount"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// The wage of one team-day: everyone at the rate, half of it on a half day.
const dayWage = `(a.headcount * a.rate / CASE WHEN a.half_day THEN 2 ELSE 1 END)`

const contractorSelect = `
	WITH wages AS (
		SELECT a.contractor_id, sum(` + dayWage + `)::bigint AS total,
		       sum(a.headcount * CASE WHEN a.half_day THEN 0.5 ELSE 1 END)::float8 AS days
		FROM attendance a WHERE a.project_id = $1 GROUP BY a.contractor_id
	), extras AS (
		SELECT contractor_id, sum(amount) AS total FROM contractor_extras WHERE project_id = $1 GROUP BY contractor_id
	), bills AS (
		SELECT contractor_id, sum(amount) AS total FROM expenses
		WHERE project_id = $1 AND contractor_id IS NOT NULL AND deleted_at IS NULL GROUP BY contractor_id
	), pays AS (
		SELECT contractor_id, sum(amount) AS total FROM payments
		WHERE project_id = $1 AND contractor_id IS NOT NULL AND deleted_at IS NULL GROUP BY contractor_id
	), totals AS (
		SELECT c.id, c.opening_due + coalesce(c.contract_amount, 0) + coalesce(x.total, 0) + coalesce(w.total, 0) + coalesce(b.total, 0) AS total
		FROM contractors c
		LEFT JOIN extras x ON x.contractor_id = c.id LEFT JOIN wages w ON w.contractor_id = c.id LEFT JOIN bills b ON b.contractor_id = c.id
		WHERE c.project_id = $1
	)
	SELECT c.id, c.name, c.phone, c.work, c.pay_type, c.contract_amount, c.scope, c.daily_wage, c.team_size, c.status, c.opening_due,
	       coalesce(x.total, 0), coalesce(w.total, 0), coalesce(w.days, 0), coalesce(b.total, 0),
	       t.total, coalesce(p.total, 0), t.total - coalesce(p.total, 0),
	       lp.date, lp.amount, c.created_at
	FROM contractors c JOIN totals t ON t.id = c.id
	LEFT JOIN extras x ON x.contractor_id = c.id LEFT JOIN wages w ON w.contractor_id = c.id
	LEFT JOIN bills b ON b.contractor_id = c.id LEFT JOIN pays p ON p.contractor_id = c.id
	LEFT JOIN LATERAL (SELECT date, amount FROM payments WHERE contractor_id = c.id AND deleted_at IS NULL
	                   ORDER BY date DESC, created_at DESC LIMIT 1) lp ON true
	WHERE c.project_id = $1`

func scanContractor(row pgx.Row) (Contractor, error) {
	var c Contractor
	err := row.Scan(&c.ID, &c.Name, &c.Phone, &c.Work, &c.PayType, &c.ContractAmount, &c.Scope, &c.DailyWage, &c.TeamSize, &c.Status,
		&c.OpeningDue, &c.Extras, &c.Wages, &c.ManDays, &c.Bills, &c.Total, &c.Paid, &c.Due, &c.LastPayDate, &c.LastPayAmount, &c.CreatedAt)
	return c, err
}

// Contractors lists the project's contractors, newest first.
func (s *Service) Contractors(ctx context.Context, userID, projectID string) ([]Contractor, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, contractorSelect+` ORDER BY c.created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list contractors: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Contractor, error) { return scanContractor(row) })
}

func contractorByID(ctx context.Context, q db, projectID, id string) (Contractor, error) {
	if !isID(id) {
		return Contractor{}, notFound()
	}
	c, err := scanContractor(q.QueryRow(ctx, contractorSelect+` AND c.id = $2`, projectID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contractor{}, notFound()
	}
	return c, err
}

// NewContractor is a validated contractor.
type NewContractor struct {
	Name           string
	Phone          *string
	Work           string
	PayType        string
	ContractAmount *int64
	Scope          *string
	DailyWage      *int64
	TeamSize       *int16
	OpeningDue     int64
}

type contractorBody struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Work           string `json:"work"`
	PayType        string `json:"payType"`
	ContractAmount int64  `json:"contractAmount"`
	Scope          string `json:"scope"`
	DailyWage      int64  `json:"dailyWage"`
	TeamSize       int16  `json:"teamSize"`
	OpeningDue     int64  `json:"openingDue"`
}

func (b contractorBody) validate(lang string) (NewContractor, error) {
	f := &fields{lang: lang}
	nc := NewContractor{Work: b.Work, PayType: b.PayType, OpeningDue: b.OpeningDue}
	if name := f.text("name", b.Name, 80); name == nil {
		f.add("name", "contractorName")
	} else {
		nc.Name = *name
	}
	nc.Phone = f.text("phone", b.Phone, 30)
	if !slices.Contains(workKinds, b.Work) {
		f.add("work", "work")
	}
	switch b.PayType {
	case "CONTRACT":
		amount := f.amount("contractAmount", b.ContractAmount, "contract")
		nc.ContractAmount = &amount
		nc.Scope = f.text("scope", b.Scope, maxShortLen)
	case "DAILY":
		wage := f.amount("dailyWage", b.DailyWage, "wage")
		nc.DailyWage = &wage
		team := b.TeamSize
		if team < 1 || team > 50 {
			f.add("teamSize", "team")
		}
		nc.TeamSize = &team
	default:
		f.add("payType", "payType")
	}
	if b.OpeningDue < 0 || b.OpeningDue > maxAmount {
		f.add("openingDue", "amount")
	}
	return nc, f.err()
}

// CreateContractor adds a contractor or a daily team, not yet started.
func (s *Service) CreateContractor(ctx context.Context, userID, projectID, lang string, nc NewContractor) (Contractor, error) {
	var c Contractor
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO contractors (project_id, name, phone, work, pay_type, contract_amount, scope, daily_wage, team_size, opening_due, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			projectID, nc.Name, nc.Phone, nc.Work, nc.PayType, nc.ContractAmount, nc.Scope, nc.DailyWage, nc.TeamSize, nc.OpeningDue, userID).Scan(&id)
		if isUniqueViolation(err) {
			return nameTaken(lang, "contractorTaken")
		}
		if err != nil {
			return fmt.Errorf("insert contractor: %w", err)
		}
		err = logActivity(ctx, tx, activity{projectID: projectID, actorID: userID, action: "CREATE", entity: "contractor", entityID: id, summary: nc.Name})
		if err != nil {
			return err
		}
		c, err = contractorByID(ctx, tx, projectID, id)
		return err
	})
	return c, err
}

// SetContractorStatus marks the work as not started, under way or done.
func (s *Service) SetContractorStatus(ctx context.Context, userID, projectID, contractorID, status string) error {
	if !slices.Contains(statuses, status) {
		return messages.Fail(apperr.KindInvalid, codeInvalidInput)
	}
	if !isID(contractorID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		tag, err := tx.Exec(ctx, `UPDATE contractors SET status = $3 WHERE id = $1 AND project_id = $2`, contractorID, projectID, status)
		if err != nil {
			return fmt.Errorf("update contractor status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return notFound()
		}
		return nil
	})
}
