// Package audit is the admin panel's log: who on the staff did what, and every sign-in and
// failed attempt. Writes happen inside the change they record.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Entry is one logged action. Target names what it was done to ("user", "project").
type Entry struct {
	ActorID    *string
	Action     string
	TargetType string
	TargetID   string
	Summary    string
	Details    map[string]string
	IP         string
}

// Execer is a pool or a transaction.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Log records e.
func Log(ctx context.Context, q Execer, e Entry) error {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	details, err := json.Marshal(e.Details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO admin_audit (actor_id, action, target_type, target_id, summary, details, ip)
		VALUES ($1, $2, nullif($3, ''), nullif($4, ''), $5, $6, nullif($7, ''))`,
		e.ActorID, e.Action, e.TargetType, e.TargetID, e.Summary, details, e.IP)
	if err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}
