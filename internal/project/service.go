package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// Service manages projects and membership checks.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

func notFound() error { return messages.Fail(apperr.KindNotFound, codeNotFound) }

// Create opens a project owned by userID with its starting books, and returns it.
// cashName is the starting account's name in the creator's language.
func (s *Service) Create(ctx context.Context, userID, cashName string, np NewProject) (Project, error) {
	var id string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if id, err = insertProject(ctx, tx, userID, np); err != nil {
			return err
		}
		return seedBooks(ctx, tx, id, userID, cashName, np)
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return s.Get(ctx, userID, id)
}

// List returns the projects userID is a member of, newest first, finished ones last.
func (s *Service) List(ctx context.Context, userID string) ([]Summary, error) {
	return listSummaries(ctx, s.pool, userID)
}

// Get returns one project, or not found when userID is not a member. Another
// tenant's project and a missing one look the same.
func (s *Service) Get(ctx context.Context, userID, projectID string) (Project, error) {
	if !isID(projectID) {
		return Project{}, notFound()
	}
	p, err := projectFor(ctx, s.pool, userID, projectID)
	if errors.Is(err, errNotMember) {
		return Project{}, notFound()
	}
	return p, err
}

// Deleted lists the owner's deleted projects that can still be restored.
func (s *Service) Deleted(ctx context.Context, userID string) ([]Deleted, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.kind, p.ownership, p.farm_type, p.business_type, p.deleted_at,
		       coalesce((SELECT json_agg(json_build_object('name', u.first_name || ' ' || u.last_name, 'role', m.role) ORDER BY m.created_at)
		                 FROM project_members m JOIN users u ON u.id = m.user_id
		                 WHERE m.project_id = p.id AND m.role <> 'OWNER'), '[]')
		FROM projects p
		WHERE p.owner_id = $1 AND p.deleted_at > $2
		ORDER BY p.deleted_at`, userID, s.now().Add(-RestoreWindow))
	if err != nil {
		return nil, fmt.Errorf("list deleted projects: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Deleted, error) {
		var d Deleted
		err := row.Scan(&d.ID, &d.Name, &d.Kind, &d.Ownership, &d.FarmType, &d.BusinessType, &d.DeletedAt, &d.Members)
		d.PurgeAt = d.DeletedAt.Add(RestoreWindow)
		return d, err
	})
}

// Delete moves the project to the owner's restore list. Members lose access at once.
func (s *Service) Delete(ctx context.Context, userID, projectID string) error {
	return s.ownerChange(ctx, userID, projectID, func(tx pgx.Tx, a access) error {
		if a.deletedAt != nil {
			return notFound()
		}
		_, err := tx.Exec(ctx, `UPDATE projects SET deleted_at = $3, deleted_by = $2 WHERE id = $1`, projectID, userID, s.now())
		return err
	})
}

// Restore brings a deleted project back within the restore window, as it was.
func (s *Service) Restore(ctx context.Context, userID, projectID string) error {
	return s.ownerChange(ctx, userID, projectID, func(tx pgx.Tx, a access) error {
		if a.deletedAt == nil {
			return nil
		}
		if s.now().Sub(*a.deletedAt) > RestoreWindow {
			return messages.Fail(apperr.KindConflict, codeRestoreClosed)
		}
		_, err := tx.Exec(ctx, `UPDATE projects SET deleted_at = NULL, deleted_by = NULL, status = 'ACTIVE' WHERE id = $1`, projectID)
		return err
	})
}

// ownerChange runs change with the project locked, once the caller is known to own it.
func (s *Service) ownerChange(ctx context.Context, userID, projectID string, change func(pgx.Tx, access) error) error {
	if !isID(projectID) {
		return notFound()
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := lockProject(ctx, tx, userID, projectID)
		switch {
		case errors.Is(err, errNotMember):
			return notFound()
		case err != nil:
			return err
		case !a.isOwner && (!a.isMember || a.deletedAt != nil):
			return notFound()
		case !a.isOwner:
			return messages.Fail(apperr.KindForbidden, codeOwnerOnly)
		}
		return change(tx, a)
	})
}
