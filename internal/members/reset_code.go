package members

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// ResetCodeTTL is how long an owner's reset code works.
const ResetCodeTTL = 30 * time.Minute

// ResetCode is shown to the owner once; only its hash is kept.
type ResetCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// IssueResetCode makes a six-digit code a member can reset their password with, voiding
// any code they were given before.
func (s *Service) IssueResetCode(ctx context.Context, userID, projectID, memberID string) (ResetCode, error) {
	code, err := sixDigits()
	if err != nil {
		return ResetCode{}, err
	}
	rc := ResetCode{Code: code, ExpiresAt: s.now().Add(ResetCodeTTL)}
	err = s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		if _, findErr := teammateFor(ctx, tx, projectID, memberID); findErr != nil {
			return findErr
		}
		if _, voidErr := tx.Exec(ctx, `
			UPDATE member_reset_codes SET voided_at = now()
			WHERE user_id = $1 AND used_at IS NULL AND voided_at IS NULL`, memberID); voidErr != nil {
			return fmt.Errorf("void reset codes: %w", voidErr)
		}
		_, insertErr := tx.Exec(ctx, `
			INSERT INTO member_reset_codes (user_id, project_id, issued_by, code_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5)`, memberID, projectID, userID, token.Hash(code), rc.ExpiresAt)
		if insertErr != nil {
			return fmt.Errorf("insert reset code: %w", insertErr)
		}
		return nil
	})
	if err != nil {
		return ResetCode{}, err
	}
	return rc, nil
}

func sixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("draw reset code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
