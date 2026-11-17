//go:build integration

package members

import (
	"context"
	"testing"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

// A manager login opened with the project belongs to the owner: the manager signs in with
// it, but only the owner sets its password, and doing so signs the manager out.
func TestTheOwnerSetsAManagedLoginsPassword(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p, err := project.NewService(f.pool).Create(ctx, f.owner, "নগদ ক্যাশ", project.NewProject{
		Name: "উত্তরা ভবন", Kind: project.KindBuilding, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		Categories: []string{"রড"}, Phases: []string{},
		Manager: &project.NewManager{Name: "জসিম উদ্দিন", Phone: "01811223344", Password: "abc123"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	session, err := f.auth.Login(ctx, "01811223344", "abc123", true, auth.ClientMeta{})
	if err != nil {
		t.Fatalf("manager login: %v", err)
	}
	if !session.User.PasswordManaged {
		t.Error("the manager's profile doesn't say the owner sets the password")
	}
	issuer := token.NewIssuer([]byte("members-integration-secret-32-byte"), 15*time.Minute)
	claims, err := issuer.Parse(session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	managerID := claims.UserID

	_, err = f.auth.ChangePassword(ctx, claims, "abc123", "xyz789ab", false, auth.ClientMeta{}, httpx.LangBangla)
	wantCode(t, err, "PASSWORD_MANAGED")
	_, err = f.auth.ResetOptions(ctx, "01811223344", httpx.LangBangla)
	wantCode(t, err, "PASSWORD_MANAGED")
	_, err = f.svc.IssueResetCode(ctx, f.owner, p.ID, managerID)
	wantCode(t, err, codePasswordSet)

	wantCode(t, f.svc.SetPassword(ctx, managerID, p.ID, managerID, "xyz789", httpx.LangBangla), "OWNER_ONLY")
	wantCode(t, f.svc.SetPassword(ctx, f.owner, p.ID, managerID, "xyz", httpx.LangBangla), codeInvalidInput)
	if err = f.svc.SetPassword(ctx, f.owner, p.ID, managerID, "xyz789", httpx.LangBangla); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if _, err = f.auth.Refresh(ctx, session.RefreshToken, auth.ClientMeta{}); err == nil {
		t.Error("the manager's old session still refreshes")
	}
	if _, err = f.auth.Login(ctx, "01811223344", "xyz789", false, auth.ClientMeta{}); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
}

func TestSetPasswordRefusesAccountsTheOwnerDidNotOpen(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	rahim := addUser(t, f.pool, "রহিম", "01911000001")
	if _, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01911000001", Role: project.RoleManager}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.SetPassword(ctx, f.owner, f.project, rahim, "xyz789", httpx.LangBangla), codeNotManaged)
}
