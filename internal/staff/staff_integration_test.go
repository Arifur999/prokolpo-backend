//go:build integration

package staff

import (
	"context"
	"testing"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

type fixture struct {
	svc   *Service
	staff string
	clock time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	svc, err := NewService(pool, []byte("a-test-secret-that-is-long-enough-32"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{svc: svc, clock: time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)}
	svc.now = func() time.Time { return f.clock }
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at, staff_role)
		VALUES ('মাহমুদ', 'হাসান', '01700112233', 'x', now(), 'super_admin') RETURNING id`).Scan(&f.staff); err != nil {
		t.Fatalf("add staff: %v", err)
	}
	return f
}

// appCode is what the app would show for the secret being set up or in use.
func (f *fixture) appCode(t *testing.T, column string, at time.Time) string {
	t.Helper()
	var sealed []byte
	if err := f.svc.pool.QueryRow(context.Background(), `SELECT `+column+` FROM users WHERE id = $1`, f.staff).Scan(&sealed); err != nil {
		t.Fatalf("read secret: %v", err)
	}
	secret, err := f.svc.seal.open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	return codeAt(secret, stepOf(at))
}

func TestStaffSetUpTheSecondStepAndGetBackupCodes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if st, err := f.svc.Status(ctx, f.staff); err != nil || st.Enabled || st.StaffRole != "super_admin" {
		t.Fatalf("status = %+v, %v", st, err)
	}
	s, err := f.svc.Begin(ctx, f.staff)
	if err != nil || s.Key == "" {
		t.Fatalf("begin = %+v, %v", s, err)
	}
	_, err = f.svc.Enable(ctx, f.staff, "000000", Meta{})
	wantCode(t, err, codeWrongSetup)

	on, err := f.svc.Enable(ctx, f.staff, f.appCode(t, "totp_pending", f.clock), Meta{IP: "203.0.113.5"})
	if err != nil || len(on.Codes) != 10 || on.Session == "" {
		t.Fatalf("enable = %+v, %v", on, err)
	}
	if err = f.svc.Touch(ctx, f.staff, on.Session); err != nil {
		t.Fatalf("fresh session refused: %v", err)
	}
	_, err = f.svc.Begin(ctx, f.staff)
	wantCode(t, err, codeAlreadyOn)

	// The same code can't be used again; the next step's can.
	_, err = f.svc.Verify(ctx, f.staff, f.appCode(t, "totp_secret", f.clock), false, Meta{})
	wantCode(t, err, codeWrongCode)
	f.clock = f.clock.Add(30 * time.Second)
	v, err := f.svc.Verify(ctx, f.staff, f.appCode(t, "totp_secret", f.clock), false, Meta{})
	if err != nil || v.Session == "" || v.UsedBackup {
		t.Fatalf("verify = %+v, %v", v, err)
	}

	b, err := f.svc.Verify(ctx, f.staff, on.Codes[3], true, Meta{})
	if err != nil || !b.UsedBackup || b.BackupLeft != 9 {
		t.Fatalf("backup = %+v, %v", b, err)
	}
	_, err = f.svc.Verify(ctx, f.staff, on.Codes[3], true, Meta{})
	wantCode(t, err, codeWrongBackup)
}

func TestFiveWrongCodesLockTheStepAndSessionsGoIdle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.Begin(ctx, f.staff); err != nil {
		t.Fatal(err)
	}
	on, err := f.svc.Enable(ctx, f.staff, f.appCode(t, "totp_pending", f.clock), Meta{})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		_, err = f.svc.Verify(ctx, f.staff, "000000", false, Meta{})
		wantCode(t, err, codeWrongCode)
	}
	_, err = f.svc.Verify(ctx, f.staff, "000000", false, Meta{})
	wantCode(t, err, codeLocked)
	f.clock = f.clock.Add(time.Minute)
	_, err = f.svc.Verify(ctx, f.staff, f.appCode(t, "totp_secret", f.clock), false, Meta{})
	wantCode(t, err, codeLocked)

	var logged int
	if err = f.svc.pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit WHERE actor_id = $1 AND action LIKE 'login.%'`, f.staff).Scan(&logged); err != nil || logged != 5 {
		t.Fatalf("logged %d failures, %v", logged, err)
	}

	// The session from setup ends after 30 idle minutes (the database clock decides).
	if _, err = f.svc.pool.Exec(ctx, `UPDATE staff_sessions SET last_seen = now() - interval '31 minutes'`); err != nil {
		t.Fatal(err)
	}
	f.svc.now = time.Now
	wantCode(t, f.svc.Touch(ctx, f.staff, on.Session), codeSessionEnded)
}

// A panel session ends after a shift however busy it is, and as soon as the account stops
// being active staff.
func TestPanelSessionsEndWithTheShiftAndTheRole(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.svc.now = time.Now
	start := func() string {
		t.Helper()
		if _, err := f.svc.pool.Exec(ctx, `UPDATE users SET totp_secret = NULL, totp_enabled_at = NULL, totp_pending = NULL WHERE id = $1`, f.staff); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Begin(ctx, f.staff); err != nil {
			t.Fatal(err)
		}
		on, err := f.svc.Enable(ctx, f.staff, f.appCode(t, "totp_pending", time.Now()), Meta{})
		if err != nil {
			t.Fatal(err)
		}
		return on.Session
	}

	long := start()
	if _, err := f.svc.pool.Exec(ctx, `UPDATE staff_sessions SET created_at = now() - interval '13 hours'`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.Touch(ctx, f.staff, long), codeSessionEnded)

	removed := start()
	if _, err := f.svc.pool.Exec(ctx, `UPDATE users SET staff_role = NULL WHERE id = $1`, f.staff); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.Touch(ctx, f.staff, removed), codeSessionEnded)
}
