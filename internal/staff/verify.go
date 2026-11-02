package staff

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/audit"
)

// Verified is a passed second step: the panel session, and what is left of the backup codes.
type Verified struct {
	Session    string `json:"session"`
	UsedBackup bool   `json:"usedBackup"`
	BackupLeft int    `json:"backupLeft"`
}

// Verify checks an app code, or a backup code when backup is set, and opens a panel session.
// Five wrong codes in a row lock the step for 15 minutes; every attempt is logged.
func (s *Service) Verify(ctx context.Context, userID, code string, backup bool, meta Meta) (Verified, error) {
	var out Verified
	var refusal error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := load(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if !m.Enabled {
			return messages.Fail(apperr.KindInvalid, codeNoSetup)
		}
		now := s.now()
		if m.LockedUntil != nil && m.LockedUntil.After(now) {
			return messages.Fail(apperr.KindTooManyRequests, codeLocked)
		}
		ok, err := s.check(ctx, tx, userID, m, code, backup)
		if err != nil {
			return err
		}
		if !ok {
			// The failure is counted and logged; the refusal goes out after the commit.
			refusal, err = s.fail(ctx, tx, userID, m, backup, meta)
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET totp_failures = 0, totp_locked_until = NULL WHERE id = $1`, userID); err != nil {
			return fmt.Errorf("reset failures: %w", err)
		}
		if out.Session, err = openSession(ctx, tx, userID, meta); err != nil {
			return err
		}
		out.UsedBackup, out.BackupLeft = backup, m.BackupLeft
		if backup {
			out.BackupLeft--
		}
		action := "login"
		if backup {
			action = "login.backup"
		}
		return audit.Log(ctx, tx, audit.Entry{ActorID: &userID, Action: action, TargetType: "user", TargetID: userID, Summary: m.Name, IP: meta.IP})
	})
	if err != nil {
		return Verified{}, err
	}
	return out, refusal
}

// check tests the code; a matching app code also moves the last used step forward.
func (s *Service) check(ctx context.Context, tx pgx.Tx, userID string, m member, code string, backup bool) (bool, error) {
	if backup {
		tag, err := tx.Exec(ctx, `UPDATE staff_backup_codes SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
			userID, hashBackup(code))
		if err != nil {
			return false, fmt.Errorf("use backup code: %w", err)
		}
		return tag.RowsAffected() == 1, nil
	}
	secret, err := s.seal.open(m.secret)
	if err != nil {
		return false, err
	}
	step := matchStep(secret, digitsOnly(code), s.now())
	if step == 0 || (m.lastStep != nil && step <= *m.lastStep) {
		return false, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET totp_last_step = $2 WHERE id = $1`, userID, step); err != nil {
		return false, fmt.Errorf("save totp step: %w", err)
	}
	return true, nil
}

// fail counts a wrong code, locks after the fifth, and returns the refusal to send.
func (s *Service) fail(ctx context.Context, tx pgx.Tx, userID string, m member, backup bool, meta Meta) (error, error) {
	failures := m.failures + 1
	var locked any
	if failures >= maxFailures {
		locked = s.now().Add(lockFor)
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET totp_failures = $2, totp_locked_until = $3 WHERE id = $1`,
		userID, failures%maxFailures, locked); err != nil {
		return nil, fmt.Errorf("count failure: %w", err)
	}
	action := "login.failed"
	if locked != nil {
		action = "login.locked"
	}
	err := audit.Log(ctx, tx, audit.Entry{
		ActorID: &userID, Action: action, TargetType: "user", TargetID: userID, Summary: m.Name, IP: meta.IP,
		Details: map[string]string{"backup": strconv.FormatBool(backup)},
	})
	if err != nil {
		return nil, err
	}
	if locked != nil {
		return messages.Fail(apperr.KindTooManyRequests, codeLocked), nil
	}
	return wrongCode(backup, maxFailures-failures), nil
}

// Confirm checks an app code before a sensitive action (changing a number, suspending an
// account); wrong codes count toward the same lockout as signing in.
func (s *Service) Confirm(ctx context.Context, userID, code string, meta Meta) error {
	var refusal error
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := load(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if m.LockedUntil != nil && m.LockedUntil.After(s.now()) {
			refusal = messages.Fail(apperr.KindTooManyRequests, codeLocked)
			return nil
		}
		ok, err := s.check(ctx, tx, userID, m, code, false)
		if err != nil || ok {
			return err
		}
		refusal, err = s.fail(ctx, tx, userID, m, false, meta)
		return err
	})
	if err != nil {
		return err
	}
	return refusal
}
