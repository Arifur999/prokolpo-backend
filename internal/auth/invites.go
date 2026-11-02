package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
)

// InviteCheck is what the closed registration page shows for an invited number.
type InviteCheck struct {
	Phone     string    `json:"phone"`
	Project   string    `json:"project"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// CheckInvite finds the newest open invitation for a number, so it can register even
// while registration is closed. Callers are limited per address: the answer names a project.
func (s *Service) CheckInvite(ctx context.Context, rawPhone, ip, lang string) (InviteCheck, error) {
	if !s.inviteChecks.Allow(ip) {
		return InviteCheck{}, messages.Fail(apperr.KindTooManyRequests, codeTooManyAttempts)
	}
	p, err := phone.Normalize(rawPhone)
	if err != nil {
		return InviteCheck{}, fieldFail(lang, "phone", "phoneBad")
	}
	ic := InviteCheck{Phone: p}
	err = s.pool.QueryRow(ctx, `
		SELECT pr.name, i.role, u.first_name || ' ' || u.last_name, i.created_at
		FROM project_invites i
		JOIN projects pr ON pr.id = i.project_id AND pr.deleted_at IS NULL
		JOIN users u ON u.id = i.invited_by
		WHERE i.phone = $1 AND i.accepted_at IS NULL AND i.cancelled_at IS NULL
		ORDER BY i.created_at DESC LIMIT 1`, p).Scan(&ic.Project, &ic.Role, &ic.InvitedBy, &ic.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return InviteCheck{}, messages.Fail(apperr.KindNotFound, codeNoInvite)
	}
	if err != nil {
		return InviteCheck{}, fmt.Errorf("check invite: %w", err)
	}
	return ic, nil
}

func hasInvite(ctx context.Context, q db, p string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM project_invites i JOIN projects pr ON pr.id = i.project_id AND pr.deleted_at IS NULL
		               WHERE i.phone = $1 AND i.accepted_at IS NULL AND i.cancelled_at IS NULL)`, p).Scan(&ok)
	return ok, err
}

// acceptInvites puts a new account on every project that invited its number.
func acceptInvites(ctx context.Context, tx pgx.Tx, user User) error {
	rows, err := tx.Query(ctx, `
		UPDATE project_invites SET accepted_at = now()
		WHERE phone = $1 AND accepted_at IS NULL AND cancelled_at IS NULL
		RETURNING project_id, role`, user.Phone)
	if err != nil {
		return fmt.Errorf("accept invites: %w", err)
	}
	type joined struct{ ProjectID, Role string }
	invites, err := pgx.CollectRows(rows, pgx.RowToStructByPos[joined])
	if err != nil {
		return fmt.Errorf("accept invites: %w", err)
	}
	for _, j := range invites {
		if _, err := tx.Exec(ctx, `
			INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, j.ProjectID, user.ID, j.Role); err != nil {
			return fmt.Errorf("join project: %w", err)
		}
		err := activity.Log(ctx, tx, activity.Entry{
			ProjectID: j.ProjectID, ActorID: user.ID, Action: "CREATE", Entity: "member", EntityID: user.ID,
			Summary: user.FirstName + " " + user.LastName, Details: map[string]string{"role": j.Role, "phone": user.Phone, "joined": "true"},
		})
		if err != nil {
			return err
		}
	}
	return nil
}
