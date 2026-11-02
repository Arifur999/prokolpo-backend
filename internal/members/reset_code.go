package members

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
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
		if ownErr := ownersToReset(ctx, tx, projectID, memberID); ownErr != nil {
			return ownErr
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

// ownersToReset refuses a code for an account that is not the owner's to look after: staff,
// anyone on another project, and anyone whose account existed before they joined this one.
// Otherwise adding a stranger's number and issuing a code would hand over their account.
func ownersToReset(ctx context.Context, tx pgx.Tx, projectID, memberID string) error {
	var own bool
	err := tx.QueryRow(ctx, `
		SELECT u.staff_role IS NOT NULL
			OR u.created_at < m.created_at - interval '1 minute'
			OR EXISTS (
				SELECT 1 FROM project_members o JOIN projects p ON p.id = o.project_id
				WHERE o.user_id = u.id AND o.project_id <> m.project_id AND p.deleted_at IS NULL)
		FROM users u JOIN project_members m ON m.user_id = u.id AND m.project_id = $1
		WHERE u.id = $2`, projectID, memberID).Scan(&own)
	if err != nil {
		return fmt.Errorf("check reset code target: %w", err)
	}
	if own {
		return messages.Fail(apperr.KindForbidden, codeOwnAccount)
	}
	return nil
}

func sixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("draw reset code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
