package members

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// ChangeRole makes a member a manager or a viewer.
func (s *Service) ChangeRole(ctx context.Context, userID, projectID, memberID, role, lang string) error {
	if !slices.Contains(teamRoles, role) {
		return invalid(lang, "role", "role")
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		t, err := teammateFor(ctx, tx, projectID, memberID)
		if err != nil {
			return err
		}
		if t.role == role {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE project_members SET role = $3 WHERE project_id = $1 AND user_id = $2`, projectID, memberID, role); err != nil {
			return fmt.Errorf("change role: %w", err)
		}
		return logEntry(ctx, tx, projectID, userID, "UPDATE", "member", memberID, t.name, map[string]string{"role": role, "from": t.role})
	})
}

// Remove takes a member off the project. What they entered stays in the books, and any
// reset code they were given for it stops working.
func (s *Service) Remove(ctx context.Context, userID, projectID, memberID string) error {
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		t, err := teammateFor(ctx, tx, projectID, memberID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, memberID); err != nil {
			return fmt.Errorf("remove member: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE member_reset_codes SET voided_at = now()
			WHERE project_id = $1 AND user_id = $2 AND used_at IS NULL AND voided_at IS NULL`, projectID, memberID); err != nil {
			return fmt.Errorf("void reset codes: %w", err)
		}
		return logEntry(ctx, tx, projectID, userID, "DELETE", "member", memberID, t.name, map[string]string{"role": t.role, "phone": t.phone})
	})
}
