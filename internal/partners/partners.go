// Package partners keeps a joint project's partners: their shares and commitments,
// capital calls, the money they put in, and its receipts.
package partners

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Access checks the caller's standing in a project.
type Access interface {
	Member(ctx context.Context, userID, projectID string) (project.Membership, error)
	Writer(ctx context.Context, userID, projectID string) (project.Membership, error)
	Owner(ctx context.Context, userID, projectID string) (project.Membership, error)
}

// Service reads and writes the partner books.
type Service struct {
	pool   *pgxpool.Pool
	access Access
	now    func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool, access Access) *Service {
	return &Service{pool: pool, access: access, now: time.Now}
}

// Partner is one partner with what they owe: their commitment plus their part of every
// capital call, less what they have put in.
type Partner struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Share     float64   `json:"share"`
	Committed int64     `json:"committed"`
	JoinedOn  time.Time `json:"joinedOn"`
	Color     int       `json:"color"`
	CallShare int64     `json:"callShare"`
	Paid      int64     `json:"paid"`
	// Role is their place on the team, if the number has joined it.
	Role       *string `json:"role"`
	HasAccount bool    `json:"hasAccount"`
	Invited    bool    `json:"invited"`
}

const partnerSelect = `
	SELECT p.id, p.name, p.phone, p.share::float8, p.committed, p.joined_on, p.color,
	       coalesce((SELECT sum(cp.amount) FROM capital_call_parts cp WHERE cp.partner_id = p.id), 0)::bigint,
	       coalesce((SELECT sum(d.amount) FROM partner_deposits d WHERE d.partner_id = p.id), 0)::bigint,
	       (SELECT m.role FROM project_members m JOIN users u ON u.id = m.user_id WHERE m.project_id = p.project_id AND u.phone = p.phone),
	       EXISTS (SELECT 1 FROM users u WHERE u.phone = p.phone),
	       EXISTS (SELECT 1 FROM project_invites i WHERE i.project_id = p.project_id AND i.phone = p.phone
	               AND i.accepted_at IS NULL AND i.cancelled_at IS NULL)
	FROM partners p`

func partnersOf(ctx context.Context, q db, projectID string) ([]Partner, error) {
	rows, err := q.Query(ctx, partnerSelect+` WHERE p.project_id = $1 ORDER BY p.color`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list partners: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Partner])
}

// Board is the partners page: everyone's books and the capital calls.
type Board struct {
	Partners []Partner `json:"partners"`
	Calls    []Call    `json:"calls"`
}

// Board lists the partners and calls; any member may see them.
func (s *Service) Board(ctx context.Context, userID, projectID string) (Board, error) {
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Board{}, err
	}
	partners, err := partnersOf(ctx, s.pool, projectID)
	if err != nil {
		return Board{}, err
	}
	calls, err := callsOf(ctx, s.pool, projectID, "")
	if err != nil {
		return Board{}, err
	}
	return Board{Partners: partners, Calls: calls}, nil
}

// db is a pool or a transaction.
type db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// joint runs fn in a transaction once access allows and the project is a joint one.
func (s *Service) joint(ctx context.Context, check func() error, projectID string, fn func(tx pgx.Tx) error) error {
	if err := check(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var ownership string
		err := tx.QueryRow(ctx, `SELECT ownership FROM projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&ownership)
		if err != nil {
			return fmt.Errorf("load project: %w", err)
		}
		if ownership != project.OwnershipPartnership {
			return messages.Fail(apperr.KindInvalid, codeNotPartnership)
		}
		return fn(tx)
	})
}

func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}

// inProject reports whether a row of table belongs to the project; table is a constant.
func inProject(ctx context.Context, q db, table, projectID, id string) (bool, error) {
	if !isID(id) {
		return false, nil
	}
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1 AND project_id = $2)`, id, projectID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ok, err
}

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// today is the date in Bangladesh.
func (s *Service) today() time.Time {
	y, m, d := s.now().In(dhaka).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
