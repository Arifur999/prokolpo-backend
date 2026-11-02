package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// Active refuses a token whose account is gone, deleted or suspended, whose login was signed
// out (from the profile's device list, a reset or "log out everywhere"), or that was issued
// before the password last changed: each takes effect at once, not when the token expires.
func (s *Service) Active(ctx context.Context, claims token.Claims) error {
	var status string
	var changed *time.Time
	var loggedIn bool
	err := s.pool.QueryRow(ctx, `
		SELECT status, password_changed_at,
			$2 = '' OR EXISTS (
				SELECT 1 FROM refresh_tokens
				WHERE user_id = $1 AND family_id::text = $2 AND revoked_at IS NULL)
		FROM users WHERE id = $1`, claims.UserID, claims.SessionID).Scan(&status, &changed, &loggedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return messages.Fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	if err != nil {
		return fmt.Errorf("check account: %w", err)
	}
	switch {
	case status == statusSuspended:
		return messages.Fail(apperr.KindForbidden, codeAccountSuspended)
	case status != statusActive, !loggedIn:
		return messages.Fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	// JWT times are whole seconds.
	if changed != nil && changed.Truncate(time.Second).After(claims.IssuedAt) {
		return messages.Fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	return nil
}
