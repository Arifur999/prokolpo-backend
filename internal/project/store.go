package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// db is what the read queries need from a pool or a transaction.
type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// summarySelect lists projects with the caller's role ($1 is the caller).
const summarySelect = `
	SELECT p.id, p.name, p.kind, p.ownership, p.status, p.location, p.budget, project_spent(p.id),
	       p.farm_type, p.floors, p.business_type, m.role,
	       (SELECT count(*) FROM project_members x WHERE x.project_id = p.id), p.created_at`

func scanSummary(row pgx.Row, s *Summary) error {
	return row.Scan(&s.ID, &s.Name, &s.Kind, &s.Ownership, &s.Status, &s.Location, &s.Budget, &s.Spent,
		&s.FarmType, &s.Floors, &s.BusinessType, &s.Role, &s.MemberCount, &s.CreatedAt)
}

func insertProject(ctx context.Context, tx pgx.Tx, ownerID string, np NewProject) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO projects (owner_id, name, kind, ownership, location, description, start_date, budget,
		                      farm_type, floors, batches, plots, land_area, land_unit, business_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id`,
		ownerID, np.Name, np.Kind, np.Ownership, np.Location, np.Description, np.StartDate, np.Budget,
		np.FarmType, np.Floors, np.Batches, np.Plots, np.LandArea, np.LandUnit, np.BusinessType).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}
	return id, nil
}

// seedBooks gives a new project its owner, expense heads, phases and cash account.
func seedBooks(ctx context.Context, tx pgx.Tx, projectID, ownerID, cashName string, np NewProject) error {
	if _, err := tx.Exec(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'OWNER')`, projectID, ownerID); err != nil {
		return fmt.Errorf("add owner: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO expense_categories (project_id, name, position)
		SELECT $1, name, ord FROM unnest($2::text[]) WITH ORDINALITY AS t(name, ord)`, projectID, np.Categories); err != nil {
		return fmt.Errorf("add categories: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_phases (project_id, name, position, setup)
		SELECT $1, name, ord, $3 AND ord = 1 FROM unnest($2::text[]) WITH ORDINALITY AS t(name, ord)`, projectID, np.Phases, np.Kind == KindFarm); err != nil {
		return fmt.Errorf("add phases: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO money_accounts (project_id, name, kind) VALUES ($1, $2, 'CASH')`, projectID, cashName); err != nil {
		return fmt.Errorf("add cash account: %w", err)
	}
	return nil
}

func listSummaries(ctx context.Context, q db, userID string) ([]Summary, error) {
	rows, err := q.Query(ctx, summarySelect+`
		FROM projects p JOIN project_members m ON m.project_id = p.id AND m.user_id = $1
		WHERE p.deleted_at IS NULL
		ORDER BY p.status = 'DONE', p.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var s Summary
		if err := scanSummary(rows, &s); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var errNotMember = errors.New("not a member")

// isID reports whether s can be a project id; anything else is simply not found,
// rather than a database error about uuid syntax.
func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}

func projectFor(ctx context.Context, q db, userID, projectID string) (Project, error) {
	var p Project
	row := q.QueryRow(ctx, summarySelect+`,
		       p.description, to_char(p.start_date, 'YYYY-MM-DD'), p.batches, p.plots, p.land_area::float8, p.land_unit,
		       (SELECT count(*) FROM money_accounts a WHERE a.project_id = p.id)
		FROM projects p JOIN project_members m ON m.project_id = p.id AND m.user_id = $1
		WHERE p.id = $2 AND p.deleted_at IS NULL`, userID, projectID)
	err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.Ownership, &p.Status, &p.Location, &p.Budget, &p.Spent,
		&p.FarmType, &p.Floors, &p.BusinessType, &p.Role, &p.MemberCount, &p.CreatedAt,
		&p.Description, &p.StartDate, &p.Batches, &p.Plots, &p.LandArea, &p.LandUnit, &p.AccountCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, errNotMember
	}
	if err != nil {
		return Project{}, fmt.Errorf("load project: %w", err)
	}
	p.Phases, err = phasesOf(ctx, q, projectID)
	return p, err
}

func phasesOf(ctx context.Context, q db, projectID string) ([]Phase, error) {
	rows, err := q.Query(ctx, `SELECT ph.id, ph.name, ph.budget,
		       coalesce((SELECT sum(e.amount) FROM expenses e WHERE e.phase_id = ph.id AND e.deleted_at IS NULL), 0)::bigint
		FROM project_phases ph WHERE ph.project_id = $1 ORDER BY ph.position`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list phases: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Phase, error) {
		var ph Phase
		err := row.Scan(&ph.ID, &ph.Name, &ph.Budget, &ph.Spent)
		return ph, err
	})
}

// access is the caller's standing on a project, read with the row locked.
type access struct {
	isOwner   bool
	isMember  bool
	deletedAt *time.Time
}

func lockProject(ctx context.Context, tx pgx.Tx, userID, projectID string) (access, error) {
	var a access
	err := tx.QueryRow(ctx, `
		SELECT p.owner_id = $1,
		       EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = $1),
		       p.deleted_at
		FROM projects p WHERE p.id = $2 FOR UPDATE`, userID, projectID).Scan(&a.isOwner, &a.isMember, &a.deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return access{}, errNotMember
	}
	if err != nil {
		return access{}, fmt.Errorf("lock project: %w", err)
	}
	return a, nil
}
