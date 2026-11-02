package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/audit"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// codeOwner is who can give a member a reset code: the owner of the project the member
// joined most recently.
type codeOwner struct {
	OwnerName   string
	ProjectName string
}

func ownerForCode(ctx context.Context, q db, userID string) (*codeOwner, error) {
	var o codeOwner
	err := q.QueryRow(ctx, `
		SELECT ou.first_name || ' ' || ou.last_name, p.name
		FROM project_members m
		JOIN projects p ON p.id = m.project_id AND p.deleted_at IS NULL
		JOIN project_members om ON om.project_id = p.id AND om.role = 'OWNER'
		JOIN users ou ON ou.id = om.user_id
		WHERE m.user_id = $1 AND m.role <> 'OWNER'
		ORDER BY m.created_at DESC LIMIT 1`, userID).Scan(&o.OwnerName, &o.ProjectName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no project to ask is an answer, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("find code owner: %w", err)
	}
	return &o, nil
}

// codeRow is the newest live code issued to a member.
type codeRow struct {
	ID        string
	ProjectID *string
	Hash      []byte
	TriesLeft int
	ExpiresAt time.Time
}

// ResetWithCode sets a new password with the code the project's owner gave, and signs
// the account out everywhere. A wrong code spends one of its three tries.
func (s *Service) ResetWithCode(ctx context.Context, rawPhone, rawCode, newPassword, lang string) error {
	code := strings.ReplaceAll(toASCIIDigits(rawCode), " ", "")
	if len(code) != 6 {
		return fieldFail(lang, "code", "codeShort")
	}
	if password.Validate(newPassword) != nil {
		return fieldFail(lang, "password", "passwordWeak")
	}
	user, err := s.userForReset(ctx, rawPhone, lang)
	if err != nil {
		return err
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}

	// A wrong try has to be saved, so the outcome is decided inside and refused after commit.
	var refusal error
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var c codeRow
		findErr := tx.QueryRow(ctx, `
			SELECT id, project_id, code_hash, tries_left, expires_at FROM member_reset_codes
			WHERE user_id = $1 AND used_at IS NULL AND voided_at IS NULL
			ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, user.ID).Scan(&c.ID, &c.ProjectID, &c.Hash, &c.TriesLeft, &c.ExpiresAt)
		switch {
		case errors.Is(findErr, pgx.ErrNoRows):
			refusal = messages.Fail(apperr.KindInvalid, codeResetCodeInvalid)
			return nil
		case findErr != nil:
			return fmt.Errorf("find reset code: %w", findErr)
		case c.TriesLeft <= 0:
			refusal = messages.Fail(apperr.KindInvalid, codeResetCodeLocked)
			return nil
		case !time.Now().Before(c.ExpiresAt):
			refusal = messages.Fail(apperr.KindInvalid, codeResetCodeExpired)
			return nil
		}
		if subtle.ConstantTimeCompare(c.Hash, token.Hash(code)) != 1 {
			refusal = wrongCode(lang, c.TriesLeft-1)
			_, triesErr := tx.Exec(ctx, `UPDATE member_reset_codes SET tries_left = tries_left - 1 WHERE id = $1`, c.ID)
			return triesErr
		}
		return s.applyCodeReset(ctx, tx, user, c, hash)
	})
	if err != nil {
		return fmt.Errorf("reset with code: %w", err)
	}
	return refusal
}

func (s *Service) applyCodeReset(ctx context.Context, tx pgx.Tx, user User, c codeRow, hash string) error {
	if err := updatePassword(ctx, tx, user.ID, hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE member_reset_codes SET used_at = now() WHERE id = $1`, c.ID); err != nil {
		return fmt.Errorf("spend reset code: %w", err)
	}
	if err := voidResetLinks(ctx, tx, user.ID); err != nil {
		return err
	}
	if err := revokeAllForUser(ctx, tx, user.ID); err != nil {
		return err
	}
	name := user.FirstName + " " + user.LastName
	if c.ProjectID == nil {
		// A code from support: the admin log shows it was used.
		return audit.Log(ctx, tx, audit.Entry{ActorID: &user.ID, Action: "user.code_used", TargetType: "user", TargetID: user.ID, Summary: name})
	}
	// The owner sees on the activity page that the code was used.
	return activity.Log(ctx, tx, activity.Entry{
		ProjectID: *c.ProjectID, ActorID: user.ID, Action: "UPDATE", Entity: "password", EntityID: user.ID, Summary: name,
	})
}

func wrongCode(lang string, triesLeft int) error {
	if triesLeft <= 0 {
		return messages.Fail(apperr.KindInvalid, codeResetCodeLocked)
	}
	return messages.Fail(apperr.KindInvalid, codeResetCodeWrong,
		apperr.FieldError{Path: "code", Message: fmt.Sprintf(fieldMessages["codeWrong"].In(lang), triesLeft)})
}

func fieldFail(lang, path, key string) error {
	return messages.Fail(apperr.KindInvalid, codeInvalidInput, apperr.FieldError{Path: path, Message: fieldMessages[key].In(lang)})
}

// toASCIIDigits lets a code typed in Bangla digits work.
func toASCIIDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '০' && r <= '৯' {
			return '0' + (r - '০')
		}
		return r
	}, s)
}
