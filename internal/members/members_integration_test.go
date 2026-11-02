//go:build integration

package members

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/auth"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/mail"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
	"github.com/Arifur999/prokolpo-backend/internal/timeline"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

type noMail struct{}

func (noMail) Send(context.Context, mail.Message) error { return nil }

type fixture struct {
	svc     *Service
	auth    *auth.Service
	feed    *timeline.Service
	pool    *pgxpool.Pool
	owner   string
	project string
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	owner := addUser(t, pool, "আব্দুল", "01711000099")
	projects := project.NewService(pool)
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "মিরপুর ভবন", Kind: project.KindBuilding, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		Categories: []string{"রড"}, Phases: []string{},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	issuer := token.NewIssuer([]byte("members-integration-secret-32-byte"), 15*time.Minute)
	return fixture{
		svc:  NewService(pool, projects),
		auth: auth.NewService(pool, issuer, noMail{}, "http://localhost:3000", slog.New(slog.DiscardHandler)),
		feed: timeline.NewService(pool, projects),
		pool: pool, owner: owner, project: p.ID,
	}
}

func addUser(t *testing.T, pool *pgxpool.Pool, first, phone string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at)
		VALUES ($1, 'খান', $2, 'x', now()) RETURNING id`, first, phone).Scan(&id); err != nil {
		t.Fatalf("add user: %v", err)
	}
	return id
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func (f fixture) team(t *testing.T) Team {
	t.Helper()
	team, err := f.svc.Team(context.Background(), f.owner, f.project)
	if err != nil {
		t.Fatalf("team: %v", err)
	}
	return team
}

func TestInviteAddsAccountsAndKeepsNewNumbers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	rashed := addUser(t, f.pool, "রাশেদ", "01911334455")

	l, err := f.svc.Lookup(ctx, f.owner, f.project, "+8801911334455", httpx.LangBangla)
	if err != nil || l.Status != StatusFound || *l.Name != "রাশেদ খান" {
		t.Fatalf("lookup = %+v, %v; want FOUND রাশেদ খান", l, err)
	}
	added, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01911334455", Role: project.RoleManager})
	if err != nil || added.Status != StatusMember {
		t.Fatalf("invite account = %+v, %v; want MEMBER", added, err)
	}
	_, err = f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01911334455", Role: project.RoleViewer})
	wantCode(t, err, codeAlreadyMember)

	added, err = f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01633221100", Role: project.RoleViewer})
	if err != nil || added.Status != StatusPending {
		t.Fatalf("invite new number = %+v, %v; want PENDING", added, err)
	}
	_, err = f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01633221100", Role: project.RoleViewer})
	wantCode(t, err, codeInviteExists)

	team := f.team(t)
	if len(team.Members) != 2 || team.Members[0].Role != project.RoleOwner || !team.Members[0].IsYou || team.Members[1].UserID != rashed {
		t.Fatalf("members = %+v", team.Members)
	}
	if len(team.Invites) != 1 || team.Invites[0].Phone != "01633221100" {
		t.Fatalf("invites = %+v", team.Invites)
	}

	// Only the owner manages the team.
	_, err = f.svc.Invite(ctx, rashed, f.project, NewInvite{Phone: "01755889900", Role: project.RoleViewer})
	wantCode(t, err, "OWNER_ONLY")

	if err := f.svc.CancelInvite(ctx, f.owner, f.project, team.Invites[0].ID); err != nil {
		t.Fatalf("cancel invite: %v", err)
	}
	if got := f.team(t).Invites; len(got) != 0 {
		t.Fatalf("invites after cancel = %+v", got)
	}
}

func TestRolesAndRemoval(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tania := addUser(t, f.pool, "তানিয়া", "01755889900")
	if _, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01755889900", Role: project.RoleManager}); err != nil {
		t.Fatalf("invite: %v", err)
	}

	if err := f.svc.ChangeRole(ctx, f.owner, f.project, tania, project.RoleViewer, httpx.LangBangla); err != nil {
		t.Fatalf("change role: %v", err)
	}
	if got := f.team(t).Members[1].Role; got != project.RoleViewer {
		t.Fatalf("role = %s, want VIEWER", got)
	}
	wantCode(t, f.svc.ChangeRole(ctx, f.owner, f.project, f.owner, project.RoleViewer, httpx.LangBangla), codeOwnerFixed)
	wantCode(t, f.svc.ChangeRole(ctx, f.owner, f.project, tania, project.RoleOwner, httpx.LangBangla), codeInvalidInput)

	if err := f.svc.Remove(ctx, f.owner, f.project, tania); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := f.team(t).Members; len(got) != 1 {
		t.Fatalf("members after removal = %+v", got)
	}
	if _, err := f.svc.Team(ctx, tania, f.project); err == nil {
		t.Fatal("a removed member can still see the team")
	}

	items, err := f.feed.Feed(ctx, f.owner, f.project, timeline.Filter{Limit: 10})
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(items) != 3 || items[0].Action != "DELETE" || items[1].Details["from"] != project.RoleManager {
		t.Fatalf("feed = %+v", items)
	}
}

func TestOwnerCodeResetsAMembersPassword(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tania := addUser(t, f.pool, "তানিয়া", "01755889900")
	if _, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01755889900", Role: project.RoleViewer}); err != nil {
		t.Fatalf("invite: %v", err)
	}

	opts, err := f.auth.ResetOptions(ctx, "01755889900", httpx.LangBangla)
	if err != nil || !opts.CanUseOwnerCode || *opts.ProjectName != "মিরপুর ভবন" {
		t.Fatalf("reset options = %+v, %v", opts, err)
	}
	if opts, _ := f.auth.ResetOptions(ctx, "01711000099", httpx.LangBangla); opts.CanUseOwnerCode {
		t.Fatal("the owner was offered an owner's code")
	}

	_, err = f.svc.IssueResetCode(ctx, f.owner, f.project, f.owner)
	wantCode(t, err, codeOwnerFixed)
	first, err := f.svc.IssueResetCode(ctx, f.owner, f.project, tania)
	if err != nil || len(first.Code) != 6 {
		t.Fatalf("issue code = %+v, %v", first, err)
	}
	rc, err := f.svc.IssueResetCode(ctx, f.owner, f.project, tania)
	if err != nil {
		t.Fatalf("issue second code: %v", err)
	}

	wrong := "000000"
	if rc.Code == wrong {
		wrong = "111111"
	}
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", wrong, "tania2026", httpx.LangBangla), "RESET_CODE_WRONG")
	if first.Code != rc.Code {
		wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", first.Code, "tania2026", httpx.LangBangla), "RESET_CODE_WRONG")
	}
	if err = f.auth.ResetWithCode(ctx, "01755889900", rc.Code[:3]+" "+rc.Code[3:], "tania2026", httpx.LangBangla); err != nil {
		t.Fatalf("reset with code: %v", err)
	}
	if _, err = f.auth.Login(ctx, "01755889900", "tania2026", false, auth.ClientMeta{}); err != nil {
		t.Fatalf("login with the new password: %v", err)
	}
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", rc.Code, "tania2027", httpx.LangBangla), "RESET_CODE_INVALID")

	items, err := f.feed.Feed(ctx, f.owner, f.project, timeline.Filter{ActorID: tania, Limit: 10})
	if err != nil || len(items) != 1 || items[0].Entity != "password" {
		t.Fatalf("feed = %+v, %v; want the password change", items, err)
	}
}

func TestOwnerCodeLocksAfterThreeWrongTries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tania := addUser(t, f.pool, "তানিয়া", "01755889900")
	if _, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01755889900", Role: project.RoleViewer}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	rc, err := f.svc.IssueResetCode(ctx, f.owner, f.project, tania)
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	wrong := "000000"
	if rc.Code == wrong {
		wrong = "111111"
	}
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", wrong, "tania2026", httpx.LangBangla), "RESET_CODE_WRONG")
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", wrong, "tania2026", httpx.LangBangla), "RESET_CODE_WRONG")
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", wrong, "tania2026", httpx.LangBangla), "RESET_CODE_LOCKED")
	wantCode(t, f.auth.ResetWithCode(ctx, "01755889900", rc.Code, "tania2026", httpx.LangBangla), "RESET_CODE_LOCKED")
}

func TestInvitedNumberRegistersWhileClosedAndJoins(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE app_settings SET value = '"closed"' WHERE key = 'registration_mode'`); err != nil {
		t.Fatalf("close registration: %v", err)
	}
	if _, err := f.svc.Invite(ctx, f.owner, f.project, NewInvite{Phone: "01633221100", Role: project.RoleViewer}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	stranger := auth.RegisterInput{FirstName: "অচেনা", LastName: "লোক", Phone: "01633221199", Password: "pass2026", Locale: "bn"}
	_, err := f.auth.Register(ctx, stranger, auth.ClientMeta{})
	wantCode(t, err, "REGISTRATION_CLOSED")
	_, err = f.auth.CheckInvite(ctx, "01633221199", "127.0.0.1", httpx.LangBangla)
	wantCode(t, err, "NO_INVITE")

	check, err := f.auth.CheckInvite(ctx, "01633-221100", "127.0.0.1", httpx.LangBangla)
	if err != nil || check.Project != "মিরপুর ভবন" || check.Role != project.RoleViewer {
		t.Fatalf("check invite = %+v, %v", check, err)
	}
	invited := auth.RegisterInput{FirstName: "নাসরিন", LastName: "আক্তার", Phone: "01633221100", Password: "pass2026", Locale: "bn"}
	if _, err := f.auth.Register(ctx, invited, auth.ClientMeta{}); err != nil {
		t.Fatalf("register invited number: %v", err)
	}
	team := f.team(t)
	if len(team.Members) != 2 || team.Members[1].Name != "নাসরিন আক্তার" || len(team.Invites) != 0 {
		t.Fatalf("team = %+v", team)
	}
}
