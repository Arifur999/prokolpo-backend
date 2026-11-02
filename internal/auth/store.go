package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// db is what the store needs from a pool or a transaction.
type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var errNotFound = errors.New("not found")

const userColumns = `id, first_name, last_name, company_name, phone, email, password_hash, locale, status, staff_role, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.CompanyName, &u.Phone, &u.Email, &u.PasswordHash, &u.Locale, &u.Status, &u.StaffRole, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, errNotFound
	}
	return u, err
}

type newUser struct {
	FirstName, LastName, Phone, PasswordHash, Locale string
	CompanyName, Email                               *string
}

func insertUser(ctx context.Context, q db, nu newUser) (User, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, company_name, phone, email, password_hash, locale, terms_accepted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		RETURNING `+userColumns,
		nu.FirstName, nu.LastName, nu.CompanyName, nu.Phone, nu.Email, nu.PasswordHash, nu.Locale)
	return scanUser(row)
}

func userByID(ctx context.Context, q db, id string) (User, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func userByPhone(ctx context.Context, q db, phone string) (User, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE phone = $1`, phone))
}

func userByEmail(ctx context.Context, q db, email string) (User, error) {
	return scanUser(q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

func phoneOrEmailTaken(ctx context.Context, q db, phone string, email *string) (phoneTaken, emailTaken bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE phone = $1),
		       EXISTS (SELECT 1 FROM users WHERE $2::citext IS NOT NULL AND email = $2::citext)`,
		phone, email).Scan(&phoneTaken, &emailTaken)
	return phoneTaken, emailTaken, err
}

func touchLastLogin(ctx context.Context, q db, userID string) error {
	_, err := q.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

func updatePassword(ctx context.Context, q db, userID, hash string) error {
	if _, err := q.Exec(ctx, `UPDATE users SET password_hash = $2, password_changed_at = now() WHERE id = $1`, userID, hash); err != nil {
		return err
	}
	// A new password closes any staff panel session too.
	_, err := q.Exec(ctx, `DELETE FROM staff_sessions WHERE user_id = $1`, userID)
	return err
}

// recordLogin keeps a sign-in or a wrong password for the account's security history.
func recordLogin(ctx context.Context, q db, userID string, ok bool, meta ClientMeta) error {
	_, err := q.Exec(ctx, `INSERT INTO login_events (user_id, ok, ip, user_agent) VALUES ($1, $2, nullif($3, ''), nullif($4, ''))`,
		userID, ok, meta.IP, meta.UserAgent)
	if err != nil {
		return fmt.Errorf("record login: %w", err)
	}
	return nil
}

func registrationMode(ctx context.Context, q db) (string, error) {
	var mode string
	err := q.QueryRow(ctx, `SELECT value #>> '{}' FROM app_settings WHERE key = 'registration_mode'`).Scan(&mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return modeOpen, nil
	}
	return mode, err
}

// ---- refresh tokens

type refreshRow struct {
	ID        string
	UserID    string
	FamilyID  string
	Remember  bool
	ExpiresAt time.Time
	RotatedAt *time.Time
	RevokedAt *time.Time
}

type newRefresh struct {
	UserID, FamilyID, UserAgent, IP string
	Hash                            []byte
	Remember                        bool
	ExpiresAt                       time.Time
}

func insertRefresh(ctx context.Context, q db, r newRefresh) error {
	_, err := q.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, family_id, token_hash, remember, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, '')::inet, $7)`,
		r.UserID, r.FamilyID, r.Hash, r.Remember, r.UserAgent, r.IP, r.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// refreshForUpdate locks the token row so two concurrent refreshes cannot both rotate it.
func refreshForUpdate(ctx context.Context, q db, hash []byte) (refreshRow, error) {
	var r refreshRow
	err := q.QueryRow(ctx, `
		SELECT id, user_id, family_id, remember, expires_at, rotated_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, hash).
		Scan(&r.ID, &r.UserID, &r.FamilyID, &r.Remember, &r.ExpiresAt, &r.RotatedAt, &r.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return refreshRow{}, errNotFound
	}
	return r, err
}

func markRotated(ctx context.Context, q db, id string) error {
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET rotated_at = now() WHERE id = $1`, id)
	return err
}

func revokeFamily(ctx context.Context, q db, familyID string) error {
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}

func revokeAllForUser(ctx context.Context, q db, userID string) error {
	_, err := q.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

// ---- password reset links

func voidResetLinks(ctx context.Context, q db, userID string) error {
	_, err := q.Exec(ctx, `UPDATE password_reset_tokens SET voided_at = now() WHERE user_id = $1 AND used_at IS NULL AND voided_at IS NULL`, userID)
	return err
}

func insertResetLink(ctx context.Context, q db, userID string, hash []byte, expires time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, hash, expires)
	return err
}

type resetLinkRow struct {
	ID        string
	UserID    string
	Email     *string
	ExpiresAt time.Time
	UsedAt    *time.Time
	VoidedAt  *time.Time
}

// resetLinkByHash finds a link whatever its state; lock holds it for a reset.
func resetLinkByHash(ctx context.Context, q db, hash []byte, lock bool) (resetLinkRow, error) {
	sql := `SELECT t.id, t.user_id, u.email, t.expires_at, t.used_at, t.voided_at
	        FROM password_reset_tokens t JOIN users u ON u.id = t.user_id
	        WHERE t.token_hash = $1`
	if lock {
		sql += ` FOR UPDATE OF t`
	}
	var r resetLinkRow
	err := q.QueryRow(ctx, sql, hash).Scan(&r.ID, &r.UserID, &r.Email, &r.ExpiresAt, &r.UsedAt, &r.VoidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return resetLinkRow{}, errNotFound
	}
	return r, err
}

func markResetLinkUsed(ctx context.Context, q db, id string) error {
	_, err := q.Exec(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`, id)
	return err
}
