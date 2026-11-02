// Package activity is a project's record of who changed what: every write to the
// books or the team logs an entry in the same transaction, and the activity page reads them.
package activity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Change is one field that an edit changed, as the activity page shows it ("আগে → পরে").
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// Entry is one logged change. Details hold what the page needs to word it, for entries
// whose row it cannot join (a role change, an invitation).
type Entry struct {
	ProjectID, ActorID string
	Action, Entity     string
	EntityID, Summary  string
	Changes            []Change
	Reason             *string
	Details            map[string]string
}

// Execer is a pool or a transaction.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Log records e; call it inside the transaction that made the change.
func Log(ctx context.Context, q Execer, e Entry) error {
	if e.Changes == nil {
		e.Changes = []Change{}
	}
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	changes, err := json.Marshal(e.Changes)
	if err != nil {
		return fmt.Errorf("encode changes: %w", err)
	}
	details, err := json.Marshal(e.Details)
	if err != nil {
		return fmt.Errorf("encode details: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO activity_log (project_id, actor_id, action, entity, entity_id, summary, changes, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.ProjectID, e.ActorID, e.Action, e.Entity, e.EntityID, e.Summary, changes, e.Reason, details)
	if err != nil {
		return fmt.Errorf("log activity: %w", err)
	}
	return nil
}
