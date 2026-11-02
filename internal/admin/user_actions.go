package admin

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/audit"
	"github.com/Arifur999/prokolpo-backend/internal/phone"
	"github.com/Arifur999/prokolpo-backend/internal/staff"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// supportCodeLife is how long a reset code from support works.
const supportCodeLife = 30 * time.Minute

// Actor is the staff member doing something, and where from.
type Actor struct {
	ID   string
	Lang string
	Meta staff.Meta
}

// account is the user an action is about.
type account struct {
	name, phone, status string
	email, role         *string
}

// target loads that account, locked for the change.
func target(ctx context.Context, tx pgx.Tx, userID string) (account, error) {
	var u account
	if !isID(userID) {
		return u, notFound()
	}
	err := tx.QueryRow(ctx, `SELECT first_name || ' ' || last_name, phone, status, email::text, staff_role FROM users WHERE id = $1 FOR UPDATE`, userID).
		Scan(&u.name, &u.phone, &u.status, &u.email, &u.role)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, notFound()
	}
	if err != nil {
		return u, fmt.Errorf("load user: %w", err)
	}
	return u, nil
}

// requireRole refuses an actor whose staff role is not one of roles.
func (s *Service) requireRole(ctx context.Context, q querier, actorID string, roles ...string) error {
	var role *string
	if err := q.QueryRow(ctx, `SELECT staff_role FROM users WHERE id = $1`, actorID).Scan(&role); err != nil {
		return fmt.Errorf("load staff role: %w", err)
	}
	for _, r := range roles {
		if role != nil && *role == r {
			return nil
		}
	}
	return messages.Fail(apperr.KindForbidden, codeNotAllowed)
}

// SendResetLink emails the account its own password-reset link.
func (s *Service) SendResetLink(ctx context.Context, a Actor, userID string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireRole(ctx, tx, a.ID, "super_admin", "support"); err != nil {
			return err
		}
		u, err := target(ctx, tx, userID)
		if err != nil {
			return err
		}
		if u.status != "active" || u.email == nil {
			return messages.Fail(apperr.KindInvalid, codeNoLink)
		}
		if err := s.links.SendResetLink(ctx, u.phone, a.Lang); err != nil {
			return err
		}
		return audit.Log(ctx, tx, audit.Entry{
			ActorID: &a.ID, Action: "user.reset_link", TargetType: "user", TargetID: userID, Summary: u.name,
			Details: map[string]string{"email": *u.email}, IP: a.Meta.IP,
		})
	})
}

// IssuedCode is a support reset code, shown once to the staff member who reads it out.
type IssuedCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// IssueResetCode gives a six-digit code the account can set a new password with, after staff
// checked who is calling. Older unused codes stop working.
func (s *Service) IssueResetCode(ctx context.Context, a Actor, userID, note string) (IssuedCode, error) {
	var out IssuedCode
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireRole(ctx, tx, a.ID, "super_admin", "support"); err != nil {
			return err
		}
		u, err := target(ctx, tx, userID)
		if err != nil {
			return err
		}
		if u.status != "active" {
			return messages.Fail(apperr.KindInvalid, codeSuspendedUser)
		}
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return fmt.Errorf("draw reset code: %w", err)
		}
		out = IssuedCode{Code: fmt.Sprintf("%06d", n.Int64()), ExpiresAt: s.now().Add(supportCodeLife)}
		if _, err = tx.Exec(ctx, `UPDATE member_reset_codes SET voided_at = now() WHERE user_id = $1 AND used_at IS NULL AND voided_at IS NULL`, userID); err != nil {
			return fmt.Errorf("void old codes: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO member_reset_codes (user_id, issued_by, code_hash, expires_at) VALUES ($1, $2, $3, $4)`,
			userID, a.ID, token.Hash(out.Code), out.ExpiresAt); err != nil {
			return fmt.Errorf("save reset code: %w", err)
		}
		details := map[string]string{"until": out.ExpiresAt.Format(time.RFC3339)}
		if note = strings.TrimSpace(note); note != "" {
			details["reason"] = truncate(note, 200)
		}
		return audit.Log(ctx, tx, audit.Entry{ActorID: &a.ID, Action: "user.reset_code", TargetType: "user", TargetID: userID, Summary: u.name, Details: details, IP: a.Meta.IP})
	})
	return out, err
}

// LogoutEverywhere ends every session the account has.
func (s *Service) LogoutEverywhere(ctx context.Context, a Actor, userID string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireRole(ctx, tx, a.ID, "super_admin", "support"); err != nil {
			return err
		}
		u, err := target(ctx, tx, userID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
		if err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			ActorID: &a.ID, Action: "user.logout_all", TargetType: "user", TargetID: userID, Summary: u.name,
			Details: map[string]string{"count": strconv.FormatInt(tag.RowsAffected(), 10)}, IP: a.Meta.IP,
		})
	})
}

// sensitive is a super-admin action: a reason for the log and the actor's own app code.
func (s *Service) sensitive(ctx context.Context, a Actor, reason, code string) (string, error) {
	f := &fields{lang: a.Lang}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 10 {
		f.add("reason", "reason")
	}
	if len([]rune(reason)) > 300 {
		f.add("reason", "text")
	}
	if err := f.err(); err != nil {
		return "", err
	}
	if err := s.requireRole(ctx, s.pool, a.ID, "super_admin"); err != nil {
		return "", err
	}
	return reason, s.confirm.Confirm(ctx, a.ID, code, a.Meta)
}

// ChangePhone moves an account to a new number (a lost SIM), signing it out everywhere.
func (s *Service) ChangePhone(ctx context.Context, a Actor, userID, rawPhone, reason, code string) error {
	ph, err := phone.Normalize(rawPhone)
	if err != nil {
		f := &fields{lang: a.Lang}
		f.add("phone", "phone")
		return f.err()
	}
	if reason, err = s.sensitive(ctx, a, reason, code); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		u, err := target(ctx, tx, userID)
		if err != nil {
			return err
		}
		f := &fields{lang: a.Lang}
		if u.phone == ph {
			f.add("phone", "same")
			return f.err()
		}
		var taken bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE phone = $1)`, ph).Scan(&taken); err != nil {
			return fmt.Errorf("check phone: %w", err)
		}
		if taken {
			f.add("phone", "taken")
			return f.err()
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET phone = $2 WHERE id = $1`, userID, ph); err != nil {
			return fmt.Errorf("change phone: %w", err)
		}
		if _, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			ActorID: &a.ID, Action: "user.phone", TargetType: "user", TargetID: userID, Summary: u.name,
			Details: map[string]string{"from": u.phone, "to": ph, "reason": reason}, IP: a.Meta.IP,
		})
	})
}

// SetSuspended suspends an account (signing it out everywhere) or opens it again. Staff
// accounts and the actor's own can't be suspended here.
func (s *Service) SetSuspended(ctx context.Context, a Actor, userID string, suspend bool, reason, code string) error {
	reason, err := s.sensitive(ctx, a, reason, code)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		u, err := target(ctx, tx, userID)
		if err != nil {
			return err
		}
		if suspend && (u.role != nil || userID == a.ID) {
			return messages.Fail(apperr.KindInvalid, codeStaffAccount)
		}
		if (u.status == "suspended") == suspend {
			return nil
		}
		action := "user.unsuspend"
		if suspend {
			action = "user.suspend"
			_, err = tx.Exec(ctx, `UPDATE users SET status = 'suspended', suspended_at = now(), suspended_reason = $2, suspended_by = $3 WHERE id = $1`, userID, reason, a.ID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
			}
		} else {
			_, err = tx.Exec(ctx, `UPDATE users SET status = 'active', suspended_at = NULL, suspended_reason = NULL, suspended_by = NULL WHERE id = $1`, userID)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", action, err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			ActorID: &a.ID, Action: action, TargetType: "user", TargetID: userID, Summary: u.name,
			Details: map[string]string{"reason": reason}, IP: a.Meta.IP,
		})
	})
}

// AddNote keeps a private staff note on the account.
func (s *Service) AddNote(ctx context.Context, a Actor, userID, body string) error {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 1000 {
		f := &fields{lang: a.Lang}
		f.add("body", "text")
		return f.err()
	}
	if !isID(userID) {
		return notFound()
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO staff_notes (user_id, author_id, body) SELECT id, $2, $3 FROM users WHERE id = $1`, userID, a.ID, body)
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notFound()
	}
	return nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
