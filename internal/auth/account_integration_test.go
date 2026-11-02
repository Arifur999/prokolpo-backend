//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

func signIn(t *testing.T, svc *Service, password string) (Session, token.Claims) {
	t.Helper()
	s, err := svc.Login(context.Background(), "01712345678", password, true, ClientMeta{UserAgent: "test"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	c, err := svc.issuer.Parse(s.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func registered(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	svc, pool, _ := newTestService(t)
	if _, err := svc.Register(context.Background(), karim(), ClientMeta{}); err != nil {
		t.Fatal(err)
	}
	return svc, pool
}

func TestProfileEditsNameCompanyAndEmail(t *testing.T) {
	svc, _ := registered(t)
	ctx := context.Background()
	_, me := signIn(t, svc, "karim2026")

	_, err := profileRequest{FirstName: "", Email: "not-an-email"}.validate(httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	in, err := profileRequest{FirstName: " আব্দুল ", LastName: "", CompanyName: "করিম ট্রেডার্স", Email: "Karim@Example.com"}.validate(httpx.LangBangla)
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.UpdateProfile(ctx, me.UserID, in, httpx.LangBangla)
	if err != nil || user.FirstName != "আব্দুল" || user.LastName != "" || *user.CompanyName != "করিম ট্রেডার্স" || *user.Email != "karim@example.com" {
		t.Fatalf("updated = %+v, %v", user, err)
	}
	p, err := svc.Profile(ctx, me.UserID)
	if err != nil || p.Name != "আব্দুল " || len(p.Projects) != 0 {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	if err = svc.SetLocale(ctx, me.UserID, "en"); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.SetLocale(ctx, me.UserID, "fr"), codeInvalidInput)
}

// A new password signs every other access token out at once; this device gets a new session.
func TestChangingThePasswordKeepsOnlyThisDevice(t *testing.T) {
	svc, _ := registered(t)
	ctx := context.Background()
	_, other := signIn(t, svc, "karim2026")
	_, me := signIn(t, svc, "karim2026")

	_, err := svc.ChangePassword(ctx, me, "wrong-pass1", "karim2027", true, ClientMeta{}, httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	_, err = svc.ChangePassword(ctx, me, "karim2026", "karim2026", true, ClientMeta{}, httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	_, err = svc.ChangePassword(ctx, me, "karim2026", "short", true, ClientMeta{}, httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)

	time.Sleep(1100 * time.Millisecond) // JWT times are whole seconds
	fresh, err := svc.ChangePassword(ctx, me, "karim2026", "karim2027", true, ClientMeta{}, httpx.LangBangla)
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	wantCode(t, svc.Active(ctx, other), codeUnauthorized)
	wantCode(t, svc.Active(ctx, me), codeUnauthorized)
	now, err := svc.issuer.Parse(fresh.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Active(ctx, now); err != nil {
		t.Fatalf("the new session was refused: %v", err)
	}
	devices, err := svc.Devices(ctx, now.UserID, now.SessionID)
	if err != nil || len(devices) != 1 || !devices[0].Current {
		t.Fatalf("devices = %+v, %v", devices, err)
	}
	signIn(t, svc, "karim2027")
}

func TestSigningOutOtherDevices(t *testing.T) {
	svc, _ := registered(t)
	ctx := context.Background()
	_, phone := signIn(t, svc, "karim2026")
	_, laptop := signIn(t, svc, "karim2026")
	_, me := signIn(t, svc, "karim2026")

	devices, err := svc.Devices(ctx, me.UserID, me.SessionID)
	if err != nil || len(devices) != 4 { // registration opened one too
		t.Fatalf("devices = %+v, %v", devices, err)
	}
	wantCode(t, svc.EndDevice(ctx, me.UserID, me.SessionID, me.SessionID), codeInvalidInput)
	if err = svc.EndDevice(ctx, me.UserID, me.SessionID, phone.SessionID); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.Active(ctx, phone), codeUnauthorized)
	wantCode(t, svc.EndDevice(ctx, me.UserID, me.SessionID, phone.SessionID), codeDeviceGone)

	ended, err := svc.EndOtherDevices(ctx, me.UserID, me.SessionID)
	if err != nil || ended != 2 {
		t.Fatalf("ended %d, %v", ended, err)
	}
	wantCode(t, svc.Active(ctx, laptop), codeUnauthorized)
	if err = svc.Active(ctx, me); err != nil {
		t.Fatalf("this device was signed out: %v", err)
	}
}

func TestChangingThePhoneNumber(t *testing.T) {
	svc, pool := registered(t)
	ctx := context.Background()
	_, me := signIn(t, svc, "karim2026")
	if _, err := pool.Exec(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at) VALUES ('রহিম', 'খান', '01811000000', 'x', now())`); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ChangePhone(ctx, me.UserID, "01712345678", "karim2026", httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	_, err = svc.ChangePhone(ctx, me.UserID, "01933-450129", "wrong-pass1", httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	_, err = svc.ChangePhone(ctx, me.UserID, "01811000000", "karim2026", httpx.LangBangla)
	wantCode(t, err, codePhoneTaken)
	user, err := svc.ChangePhone(ctx, me.UserID, "০১৯৩৩-৪৫০১২৯", "karim2026", httpx.LangBangla)
	if err != nil || user.Phone != "01933450129" {
		t.Fatalf("changed = %+v, %v", user, err)
	}
	if _, err = svc.Login(ctx, "01933450129", "karim2026", false, ClientMeta{}); err != nil {
		t.Fatalf("login with the new number: %v", err)
	}
}

func TestOnlyAnAccountWithoutProjectsCanBeDeleted(t *testing.T) {
	svc, pool := registered(t)
	ctx := context.Background()
	_, me := signIn(t, svc, "karim2026")
	if _, err := pool.Exec(ctx, `INSERT INTO projects (owner_id, name, kind, ownership, start_date) VALUES ($1, 'মিরপুর ভবন', 'BUILDING', 'PERSONAL', now())`, me.UserID); err != nil {
		t.Fatal(err)
	}

	wantCode(t, svc.DeleteAccount(ctx, me.UserID, "wrong-pass1", httpx.LangBangla), codeInvalidInput)
	wantCode(t, svc.DeleteAccount(ctx, me.UserID, "karim2026", httpx.LangBangla), codeOwnsProjects)
	if _, err := pool.Exec(ctx, `UPDATE projects SET deleted_at = now() WHERE owner_id = $1`, me.UserID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAccount(ctx, me.UserID, "karim2026", httpx.LangBangla); err != nil {
		t.Fatalf("delete: %v", err)
	}
	wantCode(t, svc.Active(ctx, me), codeUnauthorized)
	_, err := svc.Login(ctx, "01712345678", "karim2026", false, ClientMeta{})
	wantCode(t, err, codeInvalidCredentials)
	// The number is free for a new account.
	if _, err = svc.Register(ctx, karim(), ClientMeta{}); err != nil {
		t.Fatalf("register again: %v", err)
	}
}
