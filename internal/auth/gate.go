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

// Active refuses a token whose account is gone or suspended, or that was issued before the
// password last changed (a reset signs every device out at once, not 15 minutes later).
func (s *Service) Active(ctx context.Context, claims token.Claims) error {
	var status string
	var changed *time.Time
	err := s.pool.QueryRow(ctx, `SELECT status, password_changed_at FROM users WHERE id = $1`, claims.UserID).Scan(&status, &changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return messages.Fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	if err != nil {
		return fmt.Errorf("check account: %w", err)
	}
	if status == statusSuspended {
		return messages.Fail(apperr.KindForbidden, codeAccountSuspended)
	}
	// JWT times are whole seconds.
	if changed != nil && changed.Truncate(time.Second).After(claims.IssuedAt) {
		return messages.Fail(apperr.KindUnauthorized, codeUnauthorized)
	}
	return nil
}
