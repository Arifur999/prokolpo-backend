package settings

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// TransferOwnership makes another member the owner. They get settings, members and delete;
// the old owner stays on as a manager.
func (s *Service) TransferOwnership(ctx context.Context, userID, projectID, newOwnerID string) error {
	if !isID(newOwnerID) || newOwnerID == userID {
		return messages.Fail(apperr.KindInvalid, codeNotMember)
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `
			SELECT u.first_name || ' ' || u.last_name FROM project_members m JOIN users u ON u.id = m.user_id
			WHERE m.project_id = $1 AND m.user_id = $2 AND u.status = 'active' FOR UPDATE OF m`, projectID, newOwnerID).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return messages.Fail(apperr.KindInvalid, codeNotMember)
		}
		if err != nil {
			return fmt.Errorf("load new owner: %w", err)
		}
		steps := []struct {
			sql  string
			args []any
		}{
			{`UPDATE project_members SET role = 'MANAGER' WHERE project_id = $1 AND user_id = $2`, []any{projectID, userID}},
			{`UPDATE project_members SET role = 'OWNER' WHERE project_id = $1 AND user_id = $2`, []any{projectID, newOwnerID}},
			{`UPDATE projects SET owner_id = $2 WHERE id = $1`, []any{projectID, newOwnerID}},
		}
		for _, step := range steps {
			if _, err = tx.Exec(ctx, step.sql, step.args...); err != nil {
				return fmt.Errorf("transfer ownership: %w", err)
			}
		}
		return logSettings(ctx, tx, projectID, userID, "owner", name, nil)
	})
}
