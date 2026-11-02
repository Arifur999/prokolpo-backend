package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// Membership is the caller's standing in one project, as the feature packages see it.
type Membership struct {
	ProjectID string
	UserID    string
	Role      string
	// Status is the project's: ACTIVE, ON_HOLD or DONE.
	Status string
}

// CanWrite reports whether the member may add or change entries. Viewers only read.
func (m Membership) CanWrite() bool { return m.Role == RoleOwner || m.Role == RoleManager }

// Member returns the caller's membership in a live project, or not found when they have
// none (another tenant's project looks the same as a missing one).
func (s *Service) Member(ctx context.Context, userID, projectID string) (Membership, error) {
	if !isID(projectID) {
		return Membership{}, notFound()
	}
	var role, status string
	err := s.pool.QueryRow(ctx, `
		SELECT m.role, p.status FROM project_members m JOIN projects p ON p.id = m.project_id
		WHERE m.project_id = $1 AND m.user_id = $2 AND p.deleted_at IS NULL`, projectID, userID).Scan(&role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, notFound()
	}
	if err != nil {
		return Membership{}, fmt.Errorf("load membership: %w", err)
	}
	return Membership{ProjectID: projectID, UserID: userID, Role: role, Status: status}, nil
}

// Writer is Member for changes: viewers get forbidden, and so does everyone on a project
// marked done until its owner sets it going again in settings.
func (s *Service) Writer(ctx context.Context, userID, projectID string) (Membership, error) {
	m, err := s.Member(ctx, userID, projectID)
	if err != nil {
		return Membership{}, err
	}
	if !m.CanWrite() {
		return Membership{}, messages.Fail(apperr.KindForbidden, codeReadOnly)
	}
	if m.Status == StatusDone {
		return Membership{}, messages.Fail(apperr.KindForbidden, codeProjectDone)
	}
	return m, nil
}

// Localize puts this package's coded errors (not found, read-only) into lang, for
// feature handlers whose services call Member or Writer.
func Localize(err error, lang string) error { return messages.Localize(err, lang) }

// Owner is Member for managing the project itself: anyone but the owner gets forbidden.
func (s *Service) Owner(ctx context.Context, userID, projectID string) (Membership, error) {
	m, err := s.Member(ctx, userID, projectID)
	if err != nil {
		return Membership{}, err
	}
	if m.Role != RoleOwner {
		return Membership{}, messages.Fail(apperr.KindForbidden, codeOwnerOnly)
	}
	return m, nil
}
