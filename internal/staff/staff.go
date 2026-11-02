// Package staff guards the admin panel: the second sign-in step (an authenticator app code or a
// backup code), lockout after repeated mistakes, and the idle-limited panel session.
package staff

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/audit"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

const (
	maxFailures = 5
	lockFor     = 15 * time.Minute
	// IdleLimit is how long a panel session lasts without a request.
	IdleLimit = 30 * time.Minute
	// MaxSessionAge ends even a busy panel session: the app code is asked again after a shift.
	MaxSessionAge = 12 * time.Hour
	backupCount   = 10
)

// Service runs the second step and the panel sessions.
type Service struct {
	pool *pgxpool.Pool
	seal sealer
	now  func() time.Time
}

// NewService returns a Service; serverSecret keys the encryption of TOTP secrets.
func NewService(pool *pgxpool.Pool, serverSecret []byte) (*Service, error) {
	s, err := newSealer(serverSecret)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, seal: s, now: time.Now}, nil
}

// Meta is where a request came from, for the log and the session list.
type Meta struct {
	IP        string
	UserAgent string
}

// Status is what the second-step pages show about the signed-in staff member.
type Status struct {
	Enabled     bool       `json:"enabled"`
	Name        string     `json:"name"`
	Phone       string     `json:"phone"`
	StaffRole   string     `json:"staffRole"`
	BackupLeft  int        `json:"backupLeft"`
	LockedUntil *time.Time `json:"lockedUntil"`
}

type member struct {
	Status
	secret, pending []byte
	lastStep        *int64
	failures        int
}

// load reads a staff member, refusing anyone without a staff role.
func load(ctx context.Context, q pgx.Tx, userID string, forUpdate bool) (member, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF u"
	}
	var m member
	var role *string
	var enabled *time.Time
	err := q.QueryRow(ctx, `
		SELECT u.first_name || ' ' || u.last_name, u.phone, u.staff_role, u.totp_enabled_at, u.totp_secret, u.totp_pending,
		       u.totp_last_step, u.totp_failures, u.totp_locked_until,
		       (SELECT count(*) FROM staff_backup_codes b WHERE b.user_id = u.id AND b.used_at IS NULL)
		FROM users u WHERE u.id = $1`+lock, userID).Scan(
		&m.Name, &m.Phone, &role, &enabled, &m.secret, &m.pending, &m.lastStep, &m.failures, &m.LockedUntil, &m.BackupLeft)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role == nil) {
		return member{}, messages.Fail(apperr.KindForbidden, codeNotStaff)
	}
	if err != nil {
		return member{}, fmt.Errorf("load staff: %w", err)
	}
	m.StaffRole, m.Enabled = *role, enabled != nil
	return m, nil
}

// Status reports whether the second step is set up, and whether it is locked.
func (s *Service) Status(ctx context.Context, userID string) (Status, error) {
	var st Status
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := load(ctx, tx, userID, false)
		st = m.Status
		if st.LockedUntil != nil && !st.LockedUntil.After(s.now()) {
			st.LockedUntil = nil
		}
		return err
	})
	return st, err
}

// Setup is what the authenticator app needs: the QR code's link and the key to type.
type Setup struct {
	URI string `json:"uri"`
	Key string `json:"key"`
}

// Begin starts setting up the second step with a fresh secret; it can't replace one in use.
func (s *Service) Begin(ctx context.Context, userID string) (Setup, error) {
	var out Setup
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := load(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if m.Enabled {
			return messages.Fail(apperr.KindConflict, codeAlreadyOn)
		}
		secret, err := newSecret()
		if err != nil {
			return err
		}
		sealed, err := s.seal.seal(secret)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET totp_pending = $2 WHERE id = $1`, userID, sealed); err != nil {
			return fmt.Errorf("save pending totp: %w", err)
		}
		out = Setup{URI: otpauthURI(secret, m.Phone), Key: groupedKey(secret)}
		return nil
	})
	return out, err
}

// Enabled is the result of finishing setup: the backup codes, shown once, and a session.
type Enabled struct {
	Codes   []string `json:"codes"`
	Session string   `json:"session"`
}

// Enable checks a code against the secret being set up, turns the second step on, issues
// ten backup codes and opens a panel session.
func (s *Service) Enable(ctx context.Context, userID, code string, meta Meta) (Enabled, error) {
	var out Enabled
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := load(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if m.Enabled {
			return messages.Fail(apperr.KindConflict, codeAlreadyOn)
		}
		if m.pending == nil {
			return messages.Fail(apperr.KindInvalid, codeNoSetup)
		}
		secret, err := s.seal.open(m.pending)
		if err != nil {
			return err
		}
		step := matchStep(secret, digitsOnly(code), s.now())
		if step == 0 {
			return messages.Fail(apperr.KindInvalid, codeWrongSetup)
		}
		_, err = tx.Exec(ctx, `
			UPDATE users SET totp_secret = totp_pending, totp_pending = NULL, totp_enabled_at = now(), totp_last_step = $2,
			       totp_failures = 0, totp_locked_until = NULL
			WHERE id = $1`, userID, step)
		if err != nil {
			return fmt.Errorf("enable totp: %w", err)
		}
		if out.Codes, err = replaceBackupCodes(ctx, tx, userID); err != nil {
			return err
		}
		if out.Session, err = openSession(ctx, tx, userID, meta); err != nil {
			return err
		}
		return audit.Log(ctx, tx, audit.Entry{ActorID: &userID, Action: "2fa.enable", TargetType: "user", TargetID: userID, Summary: m.Name, IP: meta.IP})
	})
	return out, err
}

func replaceBackupCodes(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM staff_backup_codes WHERE user_id = $1`, userID); err != nil {
		return nil, fmt.Errorf("clear backup codes: %w", err)
	}
	codes := make([]string, 0, backupCount)
	for len(codes) < backupCount {
		code, err := newBackupCode()
		if err != nil {
			return nil, err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO staff_backup_codes (user_id, code_hash) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, hashBackup(code))
		if err != nil {
			return nil, fmt.Errorf("save backup code: %w", err)
		}
		if tag.RowsAffected() == 1 {
			codes = append(codes, code)
		}
	}
	return codes, nil
}

func openSession(ctx context.Context, tx pgx.Tx, userID string, meta Meta) (string, error) {
	raw, hash := token.NewOpaque()
	_, err := tx.Exec(ctx, `INSERT INTO staff_sessions (token_hash, user_id, ip, user_agent) VALUES ($1, $2, nullif($3, ''), nullif($4, ''))`,
		hash, userID, meta.IP, truncate(meta.UserAgent, 300))
	if err != nil {
		return "", fmt.Errorf("open staff session: %w", err)
	}
	return raw, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// digitsOnly keeps the digits of a typed code, Bangla ones as Latin.
func digitsOnly(s string) string {
	var out []rune
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r >= '০' && r <= '৯':
			out = append(out, '0'+(r-'০'))
		}
	}
	return string(out)
}
