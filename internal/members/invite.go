package members

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

var teamRoles = []string{project.RoleManager, project.RoleViewer}

// What the invite dialog says about a number, as it is typed.
const (
	StatusMember  = "MEMBER"
	StatusPending = "PENDING"
	StatusFound   = "FOUND"
	StatusNew     = "NEW"
)

// Lookup is what the invite dialog shows under the number.
type Lookup struct {
	Status string  `json:"status"`
	Name   *string `json:"name"`
}

// Lookup tells the owner whether a number is on the team, invited, has an account or is new.
func (s *Service) Lookup(ctx context.Context, userID, projectID, rawPhone, lang string) (Lookup, error) {
	p, err := phone.Normalize(rawPhone)
	if err != nil {
		return Lookup{}, invalid(lang, "phone", "phone")
	}
	if _, ownerErr := s.access.Owner(ctx, userID, projectID); ownerErr != nil {
		return Lookup{}, ownerErr
	}
	var l Lookup
	err = s.pool.QueryRow(ctx, `
		SELECT CASE
		         WHEN EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = $1 AND m.user_id = u.id) THEN 'MEMBER'
		         WHEN u.id IS NOT NULL THEN 'FOUND'
		         WHEN EXISTS (SELECT 1 FROM project_invites i WHERE i.project_id = $1 AND i.phone = $2
		                      AND i.accepted_at IS NULL AND i.cancelled_at IS NULL) THEN 'PENDING'
		         ELSE 'NEW' END,
		       u.first_name || ' ' || u.last_name
		FROM (SELECT 1) one LEFT JOIN users u ON u.phone = $2`, projectID, p).Scan(&l.Status, &l.Name)
	if err != nil {
		return Lookup{}, fmt.Errorf("look up phone: %w", err)
	}
	return l, nil
}

// NewInvite is a validated invitation.
type NewInvite struct {
	Phone string
	Role  string
}

type inviteBody struct {
	Phone string `json:"phone"`
	Role  string `json:"role"`
}

func (b inviteBody) validate(lang string) (NewInvite, error) {
	p, err := phone.Normalize(b.Phone)
	if err != nil {
		return NewInvite{}, invalid(lang, "phone", "phone")
	}
	if !slices.Contains(teamRoles, b.Role) {
		return NewInvite{}, invalid(lang, "role", "role")
	}
	return NewInvite{Phone: p, Role: b.Role}, nil
}

// Added says what an invitation did: a number with an account joins at once, any other
// waits as an invitation.
type Added struct {
	Status string  `json:"status"`
	Name   *string `json:"name"`
	Phone  string  `json:"phone"`
}

// Invite adds a number to the team, or keeps an invitation for it until it registers.
func (s *Service) Invite(ctx context.Context, userID, projectID string, in NewInvite) (Added, error) {
	var added Added
	err := s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var err error
		added, err = Join(ctx, tx, userID, projectID, in)
		return err
	})
	return added, err
}

// Join puts a number on the team inside the caller's transaction: an account joins at
// once, any other number waits as an invitation. The caller has checked the actor may.
func Join(ctx context.Context, tx pgx.Tx, actorID, projectID string, in NewInvite) (Added, error) {
	var memberID, name string
	err := tx.QueryRow(ctx, `SELECT id, first_name || ' ' || last_name FROM users WHERE phone = $1`, in.Phone).Scan(&memberID, &name)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Added{Status: StatusPending, Phone: in.Phone}, keepInvite(ctx, tx, actorID, projectID, in)
	case err != nil:
		return Added{}, fmt.Errorf("find user: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id) DO NOTHING`, projectID, memberID, in.Role)
	if err != nil {
		return Added{}, fmt.Errorf("add member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Added{}, messages.Fail(apperr.KindConflict, codeAlreadyMember)
	}
	added := Added{Status: StatusMember, Name: &name, Phone: in.Phone}
	return added, logEntry(ctx, tx, projectID, actorID, "CREATE", "member", memberID, name, map[string]string{"role": in.Role, "phone": in.Phone})
}

func keepInvite(ctx context.Context, tx pgx.Tx, userID, projectID string, in NewInvite) error {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO project_invites (project_id, phone, role, invited_by) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING RETURNING id`, projectID, in.Phone, in.Role, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return messages.Fail(apperr.KindConflict, codeInviteExists)
	}
	if err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return logEntry(ctx, tx, projectID, userID, "CREATE", "invite", id, in.Phone, map[string]string{"role": in.Role, "phone": in.Phone})
}

// CancelInvite withdraws an open invitation.
func (s *Service) CancelInvite(ctx context.Context, userID, projectID, inviteID string) error {
	if !isID(inviteID) {
		return notFound()
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var p, role string
		err := tx.QueryRow(ctx, `
			UPDATE project_invites SET cancelled_at = now()
			WHERE id = $1 AND project_id = $2 AND accepted_at IS NULL AND cancelled_at IS NULL
			RETURNING phone, role`, inviteID, projectID).Scan(&p, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("cancel invite: %w", err)
		}
		return logEntry(ctx, tx, projectID, userID, "DELETE", "invite", inviteID, p, map[string]string{"role": role, "phone": p})
	})
}
