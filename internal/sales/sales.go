// Package sales keeps what a project sells: units, customers, sales on installments, the
// payments that come in, and their receipts.
package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Access checks the caller's standing in a project.
type Access interface {
	Member(ctx context.Context, userID, projectID string) (project.Membership, error)
	Writer(ctx context.Context, userID, projectID string) (project.Membership, error)
}

// Service reads and writes the sales books.
type Service struct {
	pool   *pgxpool.Pool
	access Access
	now    func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool, access Access) *Service {
	return &Service{pool: pool, access: access, now: time.Now}
}

// db is a pool or a transaction.
type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// write runs fn in a transaction once the caller may change the books.
func (s *Service) write(ctx context.Context, userID, projectID string, fn func(tx pgx.Tx) error) error {
	if _, err := s.access.Writer(ctx, userID, projectID); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, fn)
}

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// today is the date in Bangladesh.
func (s *Service) today() time.Time {
	y, m, d := s.now().In(dhaka).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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
	if err != nil {
		return false, fmt.Errorf("check %s: %w", table, err)
	}
	return ok, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
