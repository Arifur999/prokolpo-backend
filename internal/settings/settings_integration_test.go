//go:build integration

package settings

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

type fixture struct {
	svc      *Service
	projects *project.Service
	pool     *pgxpool.Pool
	owner    string
	manager  string
	project  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	user := func(name, phone string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, company_name, phone, password_hash, terms_accepted_at)
			VALUES ($1, 'খান', 'করিম বিল্ডার্স', $2, 'x', now()) RETURNING id`, name, phone).Scan(&id); err != nil {
			t.Fatalf("add user: %v", err)
		}
		return id
	}
	f := fixture{pool: pool, owner: user("আব্দুল", "01712345678"), manager: user("রাশেদ", "01811000000")}
	f.projects = project.NewService(pool)
	p, err := f.projects.Create(ctx, f.owner, "নগদ ক্যাশ", project.NewProject{
		Name: "মিরপুর ৬ তলা ভবন", Kind: project.KindBuilding, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		Categories: []string{"রড", "সিমেন্ট"}, Phases: []string{"পাইলিং", "১ম তলা"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f.project = p.ID
	if _, err = pool.Exec(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'MANAGER')`, p.ID, f.manager); err != nil {
		t.Fatal(err)
	}
	f.svc = NewService(pool, f.projects)
	return f
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func (f fixture) load(t *testing.T) Settings {
	t.Helper()
	st, err := f.svc.Load(context.Background(), f.owner, f.project)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return st
}

// spend writes a bill on the head and phase straight into the books.
func (f fixture) spend(t *testing.T, categoryID, phaseID string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO expenses (project_id, category_id, phase_id, date, amount, created_by)
		VALUES ($1, $2, $3, now(), 5000, $4)`, f.project, categoryID, phaseID, f.owner); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyTheOwnerChangesSettings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	st, err := f.svc.Load(ctx, f.manager, f.project)
	if err != nil || st.Role != "MANAGER" || st.Receipt.Company != "করিম বিল্ডার্স" || st.Receipt.Next != 1 {
		t.Fatalf("manager's view = %+v, %v", st, err)
	}
	in := GeneralInput{Name: "নতুন নাম", StartDate: time.Now(), Status: "ACTIVE"}
	wantCode(t, f.svc.SaveGeneral(ctx, f.manager, f.project, in), "OWNER_ONLY")
}

func TestADoneProjectTakesNoNewEntries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := generalBody{Name: "ab", StartDate: "2026-13-01", Status: "X"}.validate(httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	in, err := generalBody{Name: "মিরপুর ভবন", Location: "মিরপুর-১০", StartDate: "2026-01-05", Status: "DONE"}.validate(httpx.LangBangla)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.SaveGeneral(ctx, f.owner, f.project, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	if g := f.load(t).General; g.Name != "মিরপুর ভবন" || g.Status != "DONE" || *g.Location != "মিরপুর-১০" || g.StartDate != "2026-01-05" {
		t.Fatalf("general = %+v", g)
	}
	_, err = f.projects.Writer(ctx, f.manager, f.project)
	wantCode(t, err, "PROJECT_DONE")
	in.Status = "ACTIVE"
	if err = f.svc.SaveGeneral(ctx, f.owner, f.project, in); err != nil {
		t.Fatal(err)
	}
	if _, err = f.projects.Writer(ctx, f.manager, f.project); err != nil {
		t.Fatalf("writer after reopening: %v", err)
	}
}

func TestPhasesAreRenamedReorderedAndOnlyEmptyOnesDeleted(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	st := f.load(t)
	piling, first := st.Phases[0], st.Phases[1]
	f.spend(t, st.Categories[0].ID, piling.ID)

	budget := int64(3_200_000)
	list := []PhaseInput{{ID: first.ID, Name: "১ম তলা ছাদ"}, {ID: piling.ID, Name: "পাইলিং", Budget: &budget}, {Name: "ফিনিশিং"}}
	if err := f.svc.SavePhases(ctx, f.owner, f.project, list); err != nil {
		t.Fatalf("save phases: %v", err)
	}
	st = f.load(t)
	if len(st.Phases) != 3 || st.Phases[0].Name != "১ম তলা ছাদ" || st.Phases[1].ID != piling.ID || !st.Phases[1].Locked || st.Phases[2].Name != "ফিনিশিং" {
		t.Fatalf("phases = %+v", st.Phases)
	}
	var projectBudget int64
	if err := f.pool.QueryRow(ctx, `SELECT budget FROM projects WHERE id = $1`, f.project).Scan(&projectBudget); err != nil || projectBudget != budget {
		t.Fatalf("project budget = %d, %v", projectBudget, err)
	}
	wantCode(t, f.svc.SavePhases(ctx, f.owner, f.project, []PhaseInput{{ID: first.ID, Name: "১ম তলা"}}), codePhaseLocked)

	_, err := validatePhases([]phaseBody{{Name: "ছাদ"}, {Name: " ছাদ "}}, httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
}

func TestHeadsAreHiddenRenamedAndOnlyUnusedOnesDeleted(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	st := f.load(t)
	rod, cement := st.Categories[0], st.Categories[1]
	f.spend(t, rod.ID, st.Phases[0].ID)

	lift, err := f.svc.AddCategory(ctx, f.owner, f.project, httpx.LangBangla, CategoryInput{Name: "লিফট"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AddCategory(ctx, f.owner, f.project, httpx.LangBangla, CategoryInput{Name: "লিফট"})
	wantCode(t, err, codeInvalidInput)

	budget := int64(4_500_000)
	if err = f.svc.UpdateCategory(ctx, f.owner, f.project, rod.ID, httpx.LangBangla, CategoryInput{Name: "রড", Budget: &budget, Hidden: true}); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.DeleteCategory(ctx, f.owner, f.project, rod.ID), codeCategoryInUse)
	if err = f.svc.DeleteCategory(ctx, f.owner, f.project, cement.ID); err != nil {
		t.Fatal(err)
	}
	cats := f.load(t).Categories
	if len(cats) != 2 || !cats[0].Hidden || *cats[0].Budget != budget || cats[0].Entries != 1 || cats[0].Spent != 5000 || cats[1].ID != lift.ID {
		t.Fatalf("categories = %+v", cats)
	}
}

func TestReceiptSettingsAndSupportAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := receiptBody{Prefix: "toolong", Lang: "fr", Paper: "A3"}.validate(httpx.LangBangla)
	wantCode(t, err, codeInvalidInput)
	in, err := receiptBody{Company: "করিম ডেভেলপার্স", Address: "মিরপুর, ঢাকা", Phone: "01712-345678", Prefix: "MRP", Lang: "both", Paper: "A5", Footer: "ধন্যবাদ"}.validate(httpx.LangBangla)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.SaveReceipt(ctx, f.owner, f.project, in); err != nil {
		t.Fatal(err)
	}
	if r := f.load(t).Receipt; r.Company != "করিম ডেভেলপার্স" || r.Prefix != "MRP" || r.Lang != "both" || r.Paper != "A5" || r.Footer != "ধন্যবাদ" {
		t.Fatalf("receipt = %+v", r)
	}

	wantCode(t, f.svc.GrantSupport(ctx, f.owner, f.project, httpx.LangBangla, 5), codeInvalidInput)
	if err = f.svc.GrantSupport(ctx, f.owner, f.project, httpx.LangBangla, 72); err != nil {
		t.Fatal(err)
	}
	g := f.load(t).Support
	if g == nil || g.EndsAt.Sub(g.StartsAt) != 72*time.Hour || g.LastOpenedBy != nil {
		t.Fatalf("grant = %+v", g)
	}
	if err = f.svc.RevokeSupport(ctx, f.owner, f.project); err != nil {
		t.Fatal(err)
	}
	if g = f.load(t).Support; g != nil {
		t.Fatalf("grant after revoke = %+v", g)
	}
}

func TestHandingTheProjectToAManager(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if m := f.load(t).Members; len(m) != 1 || m[0].UserID != f.manager {
		t.Fatalf("members = %+v", m)
	}
	wantCode(t, f.svc.TransferOwnership(ctx, f.owner, f.project, f.owner), codeNotMember)
	if err := f.svc.TransferOwnership(ctx, f.owner, f.project, f.manager); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if _, err := f.projects.Owner(ctx, f.manager, f.project); err != nil {
		t.Fatalf("new owner refused: %v", err)
	}
	m, err := f.projects.Member(ctx, f.owner, f.project)
	if err != nil || m.Role != "MANAGER" {
		t.Fatalf("old owner = %+v, %v", m, err)
	}
}

func TestTheBackupHasASheetPerBookWithEntries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	st := f.load(t)
	f.spend(t, st.Categories[0].ID, st.Phases[0].ID)
	sheets, err := f.svc.Export(ctx, f.owner, f.project)
	if err != nil || len(sheets) != 1 || sheets[0].Key != "expenses" || len(sheets[0].Rows) != 1 || sheets[0].Rows[0][1] != "রড" {
		t.Fatalf("sheets = %+v, %v", sheets, err)
	}
	_, err = f.svc.Export(ctx, f.manager, f.project)
	wantCode(t, err, "OWNER_ONLY")
}
