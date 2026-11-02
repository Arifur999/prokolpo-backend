package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// confirmPassword checks the account's own password before a change to how it signs in.
// Wrong guesses count against the same 5-in-15-minutes limit as the login form.
func (s *Service) confirmPassword(user User, pw, path, lang string) error {
	key := "account:" + user.ID
	if s.loginFailures.Exceeded(key) {
		return messages.Fail(apperr.KindTooManyRequests, codeTooManyAttempts)
	}
	match, err := password.Verify(pw, user.PasswordHash)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !match {
		s.loginFailures.Hit(key)
		return fieldFail(lang, path, "passwordWrong")
	}
	s.loginFailures.Reset(key)
	return nil
}

// ChangePassword sets a new password after checking the current one. Every other access
// token stops working (they predate the change); this device gets a fresh session, and the
// other devices are signed out too when logoutOthers is set.
func (s *Service) ChangePassword(ctx context.Context, claims token.Claims, current, next string, logoutOthers bool, meta ClientMeta, lang string) (Session, error) {
	user, err := userByID(ctx, s.pool, claims.UserID)
	if err != nil {
		return Session{}, fmt.Errorf("load user: %w", err)
	}
	if current == "" {
		return Session{}, fieldFail(lang, "currentPassword", "required")
	}
	if pwErr := s.confirmPassword(user, current, "currentPassword", lang); pwErr != nil {
		return Session{}, pwErr
	}
	if next == current {
		return Session{}, fieldFail(lang, "password", "passwordSame")
	}
	if password.Validate(next) != nil {
		return Session{}, fieldFail(lang, "password", "passwordWeak")
	}
	hash, err := password.Hash(next)
	if err != nil {
		return Session{}, err
	}
	var session Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if txErr := updatePassword(ctx, tx, user.ID, hash); txErr != nil {
			return fmt.Errorf("update password: %w", txErr)
		}
		remember, txErr := familyRemembered(ctx, tx, claims.SessionID)
		if txErr != nil {
			return txErr
		}
		if logoutOthers {
			txErr = revokeAllForUser(ctx, tx, user.ID)
		} else if claims.SessionID != "" {
			txErr = revokeFamily(ctx, tx, claims.SessionID)
		}
		if txErr != nil {
			return fmt.Errorf("end sessions: %w", txErr)
		}
		session, txErr = s.issueSession(ctx, tx, user, newFamily(), remember, meta)
		return txErr
	})
	if err != nil {
		return Session{}, fmt.Errorf("change password: %w", err)
	}
	return session, nil
}

func familyRemembered(ctx context.Context, q db, family string) (bool, error) {
	if family == "" {
		return false, nil
	}
	var remember *bool
	if err := q.QueryRow(ctx, `SELECT bool_or(remember) FROM refresh_tokens WHERE family_id::text = $1`, family).Scan(&remember); err != nil {
		return false, fmt.Errorf("read session: %w", err)
	}
	return remember != nil && *remember, nil
}

// ChangePhone moves the account to a new login number after checking its password.
func (s *Service) ChangePhone(ctx context.Context, userID, rawPhone, pw, lang string) (UserView, error) {
	user, err := userByID(ctx, s.pool, userID)
	if err != nil {
		return UserView{}, fmt.Errorf("load user: %w", err)
	}
	next, err := phone.Normalize(rawPhone)
	switch {
	case err != nil:
		return UserView{}, fieldFail(lang, "phone", "phoneBad")
	case next == user.Phone:
		return UserView{}, fieldFail(lang, "phone", "phoneSame")
	case pw == "":
		return UserView{}, fieldFail(lang, "password", "required")
	}
	if pwErr := s.confirmPassword(user, pw, "password", lang); pwErr != nil {
		return UserView{}, pwErr
	}
	updated, err := scanUser(s.pool.QueryRow(ctx, `UPDATE users SET phone = $2 WHERE id = $1 RETURNING `+userColumns, userID, next))
	if isUniqueViolation(err) {
		return UserView{}, duplicateError(true, false, lang)
	}
	if err != nil {
		return UserView{}, fmt.Errorf("change phone: %w", err)
	}
	return updated.view(), nil
}

// DeleteAccount closes an account that owns no project: it leaves every project, every
// device is signed out, and its number and email are freed. Staff accounts are removed by
// a super admin, not from here.
func (s *Service) DeleteAccount(ctx context.Context, userID, pw, lang string) error {
	user, err := userByID(ctx, s.pool, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	if user.StaffRole != nil {
		return messages.Fail(apperr.KindForbidden, codeStaffKeep)
	}
	if pw == "" {
		return fieldFail(lang, "password", "required")
	}
	if pwErr := s.confirmPassword(user, pw, "password", lang); pwErr != nil {
		return pwErr
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var owns bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE owner_id = $1 AND deleted_at IS NULL)`, userID).Scan(&owns); err != nil {
			return fmt.Errorf("check owned projects: %w", err)
		}
		if owns {
			return messages.Fail(apperr.KindConflict, codeOwnsProjects)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM project_members WHERE user_id = $1`, userID); err != nil {
			return fmt.Errorf("leave projects: %w", err)
		}
		if err := revokeAllForUser(ctx, tx, userID); err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		_, err := tx.Exec(ctx, `
			UPDATE users SET status = $2, deleted_at = now(), phone = 'deleted:' || id::text, email = NULL, password_hash = '!'
			WHERE id = $1`, userID, statusDeleted)
		if err != nil {
			return fmt.Errorf("delete account: %w", err)
		}
		return nil
	})
}
