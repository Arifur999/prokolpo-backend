package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// querier is a pool or a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AuditFilter narrows the admin log; Before pages back through older entries.
type AuditFilter struct {
	ActorID string
	Action  string
	Before  *time.Time
	Limit   int
}

func auditLines(ctx context.Context, q querier, f AuditFilter) ([]AuditLine, error) {
	var actor, action *string
	if f.ActorID != "" {
		actor = &f.ActorID
	}
	if f.Action != "" {
		action = &f.Action
	}
	rows, err := q.Query(ctx, `
		SELECT a.id, a.actor_id, coalesce(u.first_name || ' ' || u.last_name, ''), a.action, a.summary, a.details, a.created_at
		FROM admin_audit a LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($1::uuid IS NULL OR a.actor_id = $1) AND ($2::text IS NULL OR a.action LIKE $2 || '%')
		  AND ($3::timestamptz IS NULL OR a.created_at < $3)
		ORDER BY a.created_at DESC, a.id DESC LIMIT $4`, actor, action, f.Before, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list admin log: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[AuditLine])
}
