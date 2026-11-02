package staff

import (
	"context"
	"fmt"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// Touch accepts a panel session that belongs to userID, was used in the last 30 minutes and
// is younger than MaxSessionAge, while the account is still active staff, and marks it used
// now. Anything else ends with codeSessionEnded.
func (s *Service) Touch(ctx context.Context, userID, raw string) error {
	if raw == "" {
		return messages.Fail(apperr.KindUnauthorized, codeSessionEnded)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE staff_sessions s SET last_seen = now()
		FROM users u
		WHERE s.token_hash = $1 AND s.user_id = $2 AND s.last_seen > $3 AND s.created_at > $4
			AND u.id = s.user_id AND u.staff_role IS NOT NULL AND u.status = 'active'`,
		token.Hash(raw), userID, s.now().Add(-IdleLimit), s.now().Add(-MaxSessionAge))
	if err != nil {
		return fmt.Errorf("touch staff session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return messages.Fail(apperr.KindUnauthorized, codeSessionEnded)
	}
	return nil
}

// End closes a panel session (logout).
func (s *Service) End(ctx context.Context, raw string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM staff_sessions WHERE token_hash = $1`, token.Hash(raw)); err != nil {
		return fmt.Errorf("end staff session: %w", err)
	}
	return nil
}
