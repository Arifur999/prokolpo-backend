//go:build integration

package auth

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/mail"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// outbox keeps sent emails so tests can follow reset links.
type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (o *outbox) Send(_ context.Context, msg mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, msg)
	return nil
}

func (o *outbox) lastLinkToken(t *testing.T) string {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.sent) == 0 {
		t.Fatal("no email was sent")
	}
	body := o.sent[len(o.sent)-1].Body
	_, after, found := strings.Cut(body, "token=")
	if !found {
		t.Fatalf("email has no reset link:\n%s", body)
	}
	raw := strings.Fields(after)[0]
	tok, err := url.QueryUnescape(raw)
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return tok
}

func newTestService(t *testing.T) (*Service, *pgxpool.Pool, *outbox) {
	t.Helper()
	pool := testdb.Open(t)
	box := &outbox{}
	issuer := token.NewIssuer([]byte("auth-integration-secret-32-bytes!!"), 15*time.Minute)
	return NewService(pool, issuer, box, "http://localhost:3000", slog.New(slog.DiscardHandler)), pool, box
}

func karim() RegisterInput {
	email := "abdul.karim@example.com"
	company := "করিম বিল্ডার্স"
	return RegisterInput{FirstName: "আব্দুল", LastName: "করিম", CompanyName: &company, Email: &email, Phone: "01712345678", Password: "karim2026", Locale: "bn"}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestRegisterLoginAndMe(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.Register(ctx, karim(), ClientMeta{UserAgent: "test", IP: "103.112.54.18"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if session.User.Name != "আব্দুল করিম" || session.User.Role != token.RoleUser || session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("unexpected session: %+v", session.User)
	}

	for _, id := range []string{"01712345678", "+880 1712-345678", "Abdul.Karim@example.com"} {
		if _, loginErr := svc.Login(ctx, id, "karim2026", false, ClientMeta{}); loginErr != nil {
			t.Errorf("Login(%q): %v", id, loginErr)
		}
	}

	me, err := svc.Me(ctx, session.User.ID)
	if err != nil || me.Phone != "01712345678" {
		t.Fatalf("Me = %+v, %v", me, err)
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, karim(), ClientMeta{}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := svc.Register(ctx, karim(), ClientMeta{})
	wantCode(t, err, codePhoneTaken)
	if e, _ := apperr.As(err); len(e.Fields) != 2 {
		t.Errorf("fields = %+v, want phone and email both flagged", e.Fields)
	}
}

func TestRegisterWhenClosed(t *testing.T) {
	svc, pool, _ := newTestService(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE app_settings SET value = '"closed"' WHERE key = 'registration_mode'`); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Register(ctx, karim(), ClientMeta{})
	wantCode(t, err, codeRegistrationClosed)
}

func TestLoginLocksAfterFiveFailures(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, karim(), ClientMeta{}); err != nil {
		t.Fatal(err)
	}

	for range maxLoginFailures {
		_, err := svc.Login(ctx, "01712345678", "wrong-pass1", false, ClientMeta{})
		wantCode(t, err, codeInvalidCredentials)
	}
	_, err := svc.Login(ctx, "01712345678", "karim2026", false, ClientMeta{})
	wantCode(t, err, codeTooManyAttempts)
}

func TestUnknownAccountLooksLikeWrongPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Login(context.Background(), "01999999999", "karim2026", false, ClientMeta{})
	wantCode(t, err, codeInvalidCredentials)
}

func TestSuspendedAccount(t *testing.T) {
	svc, pool, _ := newTestService(t)
	ctx := context.Background()
	session, err := svc.Register(ctx, karim(), ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := pool.Exec(ctx, `UPDATE users SET status = 'suspended', suspended_at = now() WHERE id = $1`, session.User.ID); execErr != nil {
		t.Fatal(execErr)
	}

	_, err = svc.Login(ctx, "01712345678", "karim2026", false, ClientMeta{})
	wantCode(t, err, codeAccountSuspended)
	_, err = svc.Refresh(ctx, session.RefreshToken, ClientMeta{})
	wantCode(t, err, codeAccountSuspended)
}

func TestRefreshRotatesAndDetectsReuse(t *testing.T) {
	svc, pool, _ := newTestService(t)
	ctx := context.Background()
	first, err := svc.Register(ctx, karim(), ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Refresh(ctx, first.RefreshToken, ClientMeta{})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	// Reusing the first token inside the grace period fails but keeps the new session.
	_, err = svc.Refresh(ctx, first.RefreshToken, ClientMeta{})
	wantCode(t, err, codeSessionInvalid)

	// Past the grace period, reuse revokes the whole family, including the new token.
	if _, execErr := pool.Exec(ctx, `UPDATE refresh_tokens SET rotated_at = now() - interval '1 minute' WHERE rotated_at IS NOT NULL`); execErr != nil {
		t.Fatal(execErr)
	}
	_, err = svc.Refresh(ctx, first.RefreshToken, ClientMeta{})
	wantCode(t, err, codeSessionInvalid)
	_, err = svc.Refresh(ctx, second.RefreshToken, ClientMeta{})
	wantCode(t, err, codeSessionInvalid)
}

func TestLogoutEndsTheSession(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	session, err := svc.Register(ctx, karim(), ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if logoutErr := svc.Logout(ctx, session.RefreshToken); logoutErr != nil {
		t.Fatalf("Logout: %v", logoutErr)
	}
	_, err = svc.Refresh(ctx, session.RefreshToken, ClientMeta{})
	wantCode(t, err, codeSessionInvalid)
	if err := svc.Logout(ctx, "never-issued"); err != nil {
		t.Errorf("Logout(unknown token) = %v, want nil", err)
	}
}

func TestPasswordResetByEmail(t *testing.T) {
	svc, _, box := newTestService(t)
	ctx := context.Background()
	session, err := svc.Register(ctx, karim(), ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}

	opts, err := svc.ResetOptions(ctx, "০১৭১২ ৩৪৫৬৭৮", "bn")
	if err != nil || !opts.CanUseEmail || opts.MaskedEmail == nil || *opts.MaskedEmail != "ab•••@example.com" {
		t.Fatalf("ResetOptions = %+v, %v", opts, err)
	}

	if sendErr := svc.SendResetLink(ctx, "01712345678", "bn"); sendErr != nil {
		t.Fatalf("SendResetLink: %v", sendErr)
	}
	wantCode(t, svc.SendResetLink(ctx, "01712345678", "bn"), codeResetThrottled)

	link := box.lastLinkToken(t)
	info, checkErr := svc.CheckResetLink(ctx, link)
	if checkErr != nil || info.MaskedEmail == nil || *info.MaskedEmail != "ab•••@example.com" {
		t.Fatalf("CheckResetLink = %+v, %v", info, checkErr)
	}
	wantCode(t, svc.ResetPassword(ctx, link, "short", "bn"), codeInvalidInput)
	if resetErr := svc.ResetPassword(ctx, link, "notun2026", "bn"); resetErr != nil {
		t.Fatalf("ResetPassword: %v", resetErr)
	}

	// The link is spent, the old session is gone, and only the new password works.
	_, err = svc.CheckResetLink(ctx, link)
	wantCode(t, err, codeResetLinkUsed)
	wantCode(t, svc.ResetPassword(ctx, link, "abar2026", "bn"), codeResetLinkUsed)
	_, err = svc.Refresh(ctx, session.RefreshToken, ClientMeta{})
	wantCode(t, err, codeSessionInvalid)
	_, err = svc.Login(ctx, "01712345678", "karim2026", false, ClientMeta{})
	wantCode(t, err, codeInvalidCredentials)
	if _, err := svc.Login(ctx, "01712345678", "notun2026", false, ClientMeta{}); err != nil {
		t.Fatalf("Login with new password: %v", err)
	}
}

func TestResetLinkStates(t *testing.T) {
	svc, pool, box := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, karim(), ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SendResetLink(ctx, "01712345678", "bn"); err != nil {
		t.Fatal(err)
	}
	link := box.lastLinkToken(t)

	if _, err := pool.Exec(ctx, `UPDATE password_reset_tokens SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CheckResetLink(ctx, link)
	wantCode(t, err, codeResetLinkExpired)
	wantCode(t, svc.ResetPassword(ctx, link, "notun2026", "bn"), codeResetLinkExpired)

	_, err = svc.CheckResetLink(ctx, "never-issued")
	wantCode(t, err, codeResetLinkInvalid)
}

func TestResetOptionsForUnknownNumber(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.ResetOptions(context.Background(), "01888888888", "bn")
	wantCode(t, err, codeAccountNotFound)
}

// A token stops working the moment its account is suspended or its password is reset.
func TestAccessTokenFollowsTheAccount(t *testing.T) {
	svc, pool, _ := newTestService(t)
	ctx := context.Background()
	session, err := svc.Register(ctx, karim(), ClientMeta{})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.issuer.Parse(session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Active(ctx, claims); err != nil {
		t.Fatalf("fresh token refused: %v", err)
	}

	if _, err = pool.Exec(ctx, `UPDATE users SET password_changed_at = now() + interval '2 seconds' WHERE id = $1`, claims.UserID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.Active(ctx, claims), codeUnauthorized)

	if _, err = pool.Exec(ctx, `UPDATE users SET status = 'suspended', suspended_at = now() WHERE id = $1`, claims.UserID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.Active(ctx, claims), codeAccountSuspended)
}
