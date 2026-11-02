// Package members manages who belongs to a project: the member list, invitations by
// phone number, role changes, removal, and the reset codes owners give members.
package members

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Access checks the caller's standing in a project.
type Access interface {
	Member(ctx context.Context, userID, projectID string) (project.Membership, error)
	Owner(ctx context.Context, userID, projectID string) (project.Membership, error)
}

// Service reads and changes a project's team.
type Service struct {
	pool   *pgxpool.Pool
	access Access
	now    func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool, access Access) *Service {
	return &Service{pool: pool, access: access, now: time.Now}
}

// Member is one person on the project's team.
type Member struct {
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	Role       string     `json:"role"`
	IsYou      bool       `json:"isYou"`
	JoinedAt   time.Time  `json:"joinedAt"`
	LastActive *time.Time `json:"lastActive"`
	// Entries is how many changes they made to the project this month.
	Entries int `json:"entries"`
}

// Invite is an invitation waiting for its number to open an account.
type Invite struct {
	ID        string    `json:"id"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// Team is the members page: the owner first, then everyone in the order they joined.
type Team struct {
	Members []Member `json:"members"`
	Invites []Invite `json:"invites"`
}

// Team lists the project's members and open invitations; any member may see them.
func (s *Service) Team(ctx context.Context, userID, projectID string) (Team, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Team{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, u.phone, m.role, u.id = $2, m.created_at,
		       greatest(u.last_login_at, (SELECT max(r.created_at) FROM refresh_tokens r WHERE r.user_id = u.id)),
		       (SELECT count(*)::int FROM activity_log a WHERE a.project_id = m.project_id AND a.actor_id = u.id AND a.created_at >= $3)
		FROM project_members m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1
		ORDER BY m.role <> 'OWNER', m.created_at`, projectID, userID, monthStart(s.now()))
	if err != nil {
		return Team{}, fmt.Errorf("list members: %w", err)
	}
	members, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
	if err != nil {
		return Team{}, fmt.Errorf("list members: %w", err)
	}
	rows, err = s.pool.Query(ctx, `
		SELECT i.id, i.phone, i.role, u.first_name || ' ' || u.last_name, i.created_at
		FROM project_invites i JOIN users u ON u.id = i.invited_by
		WHERE i.project_id = $1 AND i.accepted_at IS NULL AND i.cancelled_at IS NULL
		ORDER BY i.created_at DESC`, projectID)
	if err != nil {
		return Team{}, fmt.Errorf("list invites: %w", err)
	}
	invites, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Invite])
	if err != nil {
		return Team{}, fmt.Errorf("list invites: %w", err)
	}
	return Team{Members: members, Invites: invites}, nil
}

// owned runs fn in a transaction once the caller is the project's owner.
func (s *Service) owned(ctx context.Context, userID, projectID string, fn func(tx pgx.Tx) error) error {
	if _, err := s.access.Owner(ctx, userID, projectID); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, fn)
}

// teammate is a member the owner is acting on.
type teammate struct {
	name, phone, role string
}

// teammateFor loads a member of the project the owner may manage: anyone but the owner.
func teammateFor(ctx context.Context, tx pgx.Tx, projectID, memberID string) (teammate, error) {
	if !isID(memberID) {
		return teammate{}, notFound()
	}
	var t teammate
	err := tx.QueryRow(ctx, `
		SELECT u.first_name || ' ' || u.last_name, u.phone, m.role
		FROM project_members m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.user_id = $2 FOR UPDATE OF m`, projectID, memberID).Scan(&t.name, &t.phone, &t.role)
	if errors.Is(err, pgx.ErrNoRows) {
		return teammate{}, notFound()
	}
	if err != nil {
		return teammate{}, fmt.Errorf("load member: %w", err)
	}
	if t.role == project.RoleOwner {
		return teammate{}, messages.Fail(apperr.KindForbidden, codeOwnerFixed)
	}
	return t, nil
}

func logEntry(ctx context.Context, tx pgx.Tx, projectID, actorID, action, entity, entityID, summary string, details map[string]string) error {
	return activity.Log(ctx, tx, activity.Entry{
		ProjectID: projectID, ActorID: actorID, Action: action, Entity: entity, EntityID: entityID, Summary: summary, Details: details,
	})
}

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// monthStart is the first moment of t's month in Bangladesh.
func monthStart(t time.Time) time.Time {
	y, m, _ := t.In(dhaka).Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, dhaka)
}

func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}
