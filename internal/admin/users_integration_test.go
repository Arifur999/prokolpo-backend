//go:build integration

package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/staff"
)

// okCode accepts "123456" and refuses anything else, like the staff app-code check.
type okCode struct{}

func (okCode) Confirm(_ context.Context, _, code string, _ staff.Meta) error {
	if code != "123456" {
		return errors.New("wrong code")
	}
	return nil
}

type sentLinks struct{ phones []string }

func (s *sentLinks) SendResetLink(_ context.Context, phone, _ string) error {
	s.phones = append(s.phones, phone)
	return nil
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestStaffFindAndHelpAnAccount(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	links := &sentLinks{}
	f.svc.confirm, f.svc.links = okCode{}, links
	a := Actor{ID: f.staff, Lang: httpx.LangBangla}

	p, err := f.svc.Users(ctx, UserFilter{Kind: "ALL", Query: "০১৯৩৩"})
	if err != nil || p.Total != 1 || p.Rows[0].ID != f.owner || p.Counts["STAFF"] != 1 || p.Counts["ALL"] != 2 {
		t.Fatalf("search = %+v, %v", p, err)
	}
	if p, _ = f.svc.Users(ctx, UserFilter{Kind: "STAFF"}); p.Total != 1 || p.Rows[0].ID != f.staff {
		t.Fatalf("staff chip = %+v", p.Rows)
	}

	if err = f.svc.SendResetLink(ctx, a, f.owner); err != nil || len(links.phones) != 1 {
		t.Fatalf("reset link: %v, sent %v", err, links.phones)
	}
	code, err := f.svc.IssueResetCode(ctx, a, f.owner, "নিবন্ধিত নম্বরে ফোন করে পরিচয় যাচাই")
	if err != nil || len(code.Code) != 6 {
		t.Fatalf("reset code = %+v, %v", code, err)
	}
	if err = f.svc.AddNote(ctx, a, f.owner, "ফোনে কথা হয়েছে, পরিচয় মিলেছে"); err != nil {
		t.Fatalf("note: %v", err)
	}

	err = f.svc.SetSuspended(ctx, a, f.owner, true, "ছোট", "123456")
	wantCode(t, err, codeInvalidInput)
	if err = f.svc.SetSuspended(ctx, a, f.owner, true, "প্রতারণার অভিযোগ — তদন্ত চলছে", "000000"); err == nil {
		t.Fatal("a wrong app code suspended the account")
	}
	if err = f.svc.SetSuspended(ctx, a, f.owner, true, "প্রতারণার অভিযোগ — তদন্ত চলছে", "123456"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	err = f.svc.SetSuspended(ctx, a, f.staff, true, "নিজেকে বন্ধ করার চেষ্টা", "123456")
	wantCode(t, err, codeStaffAccount)

	err = f.svc.ChangePhone(ctx, a, f.owner, "01700112233", "সিম হারিয়েছে — পরিচয় যাচাই করা", "123456")
	wantCode(t, err, codeInvalidInput)
	if err = f.svc.ChangePhone(ctx, a, f.owner, "01933-450129", "সিম হারিয়েছে — পরিচয় যাচাই করা", "123456"); err != nil {
		t.Fatalf("change phone: %v", err)
	}

	d, err := f.svc.User(ctx, f.owner)
	if err != nil || d.Status != "suspended" || d.SuspendedBy == nil || d.Phone != "01933450129" || len(d.Notes) != 1 {
		t.Fatalf("detail = %+v, %v", d, err)
	}
	actions := map[string]bool{}
	for _, e := range d.History {
		if e.Kind == "admin" {
			actions[e.Text] = true
		}
	}
	for _, want := range []string{"user.reset_link", "user.reset_code", "user.suspend", "user.phone"} {
		if !actions[want] {
			t.Errorf("history is missing %s: %+v", want, d.History)
		}
	}
}

func TestOnlySuperAdminsSuspend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.svc.confirm = okCode{}
	if _, err := f.pool.Exec(ctx, `UPDATE users SET staff_role = 'support' WHERE id = $1`, f.staff); err != nil {
		t.Fatal(err)
	}
	err := f.svc.SetSuspended(ctx, Actor{ID: f.staff, Lang: httpx.LangBangla}, f.owner, true, "প্রতারণার অভিযোগ — তদন্ত চলছে", "123456")
	wantCode(t, err, codeNotAllowed)
}
