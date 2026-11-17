package members

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/password"
)

type passwordBody struct {
	Password string `json:"password"`
}

// SetPassword gives a login the owner opened (a manager made on the new-project page) a new
// password, and signs it out everywhere. Anyone who runs their own account is refused: they
// reset it themselves or with a reset code.
func (s *Service) SetPassword(ctx context.Context, userID, projectID, memberID, pw, lang string) error {
	if password.ValidateManaged(pw) != nil {
		return invalid(lang, "password", "password")
	}
	hash, err := password.Hash(pw)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		t, err := teammateFor(ctx, tx, projectID, memberID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE users SET password_hash = $3, password_changed_at = now()
			WHERE id = $1 AND password_locked AND managed_by = $2`, memberID, userID, hash)
		if err != nil {
			return fmt.Errorf("set password: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return messages.Fail(apperr.KindConflict, codeNotManaged)
		}
		if _, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, memberID); err != nil {
			return fmt.Errorf("sign member out: %w", err)
		}
		return logEntry(ctx, tx, projectID, userID, "UPDATE", "member", memberID, t.name, map[string]string{"password": "changed"})
	})
}
