// Package books keeps a project's money: expenses, suppliers, contractors and their
// attendance, payments, money accounts and transfers, and the activity log of changes.
package books

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Members checks who may read and write a project's books.
type Members interface {
	Member(ctx context.Context, userID, projectID string) (project.Membership, error)
	Writer(ctx context.Context, userID, projectID string) (project.Membership, error)
}

// Service reads and writes the books.
type Service struct {
	pool    *pgxpool.Pool
	members Members
	files   Files
	now     func() time.Time
}

// NewService returns a Service backed by pool. files keeps memo photos; nil turns them off.
func NewService(pool *pgxpool.Pool, members Members, files Files) *Service {
	return &Service{pool: pool, members: members, files: files, now: time.Now}
}

// db is what the queries need from a pool or a transaction.
type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// isID reports whether s can be a row id; anything else is simply not found.
func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}

// write runs fn in a transaction once the caller may change the project's books.
func (s *Service) write(ctx context.Context, userID, projectID string, fn func(tx pgx.Tx, m project.Membership) error) error {
	m, err := s.members.Writer(ctx, userID, projectID)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return fn(tx, m) })
}

// exists reports whether a row of table with id belongs to the project. table is a
// constant from this package, never input.
func exists(ctx context.Context, q db, table, projectID, id string) (bool, error) {
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

// today is the date in Bangladesh, where every project's books are kept.
func (s *Service) today() time.Time {
	dhaka := time.FixedZone("Asia/Dhaka", 6*60*60)
	y, m, d := s.now().In(dhaka).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
