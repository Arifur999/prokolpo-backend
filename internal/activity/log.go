package books

import (
	"context"
	"encoding/json"
	"fmt"
)

// Change is one field that an edit changed, as the activity page shows it ("আগে → পরে").
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type activity struct {
	projectID, actorID string
	action, entity     string
	entityID, summary  string
	changes            []Change
	reason             *string
}

// logActivity records a change to the books in the same transaction as the change.
func logActivity(ctx context.Context, q db, a activity) error {
	if a.changes == nil {
		a.changes = []Change{}
	}
	changes, err := json.Marshal(a.changes)
	if err != nil {
		return fmt.Errorf("encode changes: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO activity_log (project_id, actor_id, action, entity, entity_id, summary, changes, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		a.projectID, a.actorID, a.action, a.entity, a.entityID, a.summary, changes, a.reason)
	if err != nil {
		return fmt.Errorf("log activity: %w", err)
	}
	return nil
}
