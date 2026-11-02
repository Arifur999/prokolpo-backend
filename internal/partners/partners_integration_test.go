//go:build integration

package partners

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/books"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

type fixture struct {
	svc     *Service
	books   *books.Service
	pool    *pgxpool.Pool
	owner   string
	project string
	bank    string
}

func setup(t *testing.T, ownership string) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at)
		VALUES ('আব্দুল', 'করিম', '01712345678', 'x', now()) RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("add user: %v", err)
	}
	projects := project.NewService(pool)
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "মিরপুর ভবন", Kind: project.KindBuilding, Ownership: ownership, StartDate: time.Now(), Categories: []string{"রড"}, Phases: []string{},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f := fixture{svc: NewService(pool, projects), books: books.NewService(pool, projects, nil), pool: pool, owner: owner, project: p.ID}
	a, err := f.books.CreateAccount(ctx, owner, p.ID, httpx.LangBangla, books.NewAccount{Name: "ডাচ্-বাংলা ব্যাংক", Kind: "BANK"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	f.bank = a.ID
	return f
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func (f fixture) add(t *testing.T, name, phone string, share float64, committed int64, shares map[string]float64) Partner {
	t.Helper()
	in, err := addBody{Phone: phone, Name: name, Share: share, Committed: committed, JoinedOn: "2026-01-15", Shares: shares}.validate(httpx.LangBangla, time.Now())
	if err != nil {
		t.Fatalf("validate %s: %v", name, err)
	}
	p, err := f.svc.AddPartner(context.Background(), f.owner, f.project, httpx.LangBangla, in)
	if err != nil {
		t.Fatalf("add %s: %v", name, err)
	}
	return p
}

func TestSharesStayAtAHundredAndCallsSplitByShare(t *testing.T) {
	f := setup(t, project.OwnershipPartnership)
	ctx := context.Background()
	karim := f.add(t, "আব্দুল করিম", "01712345678", 100, 8_000_000, map[string]float64{})
	_, err := addBody{
		Phone: "01819556677", Name: "সাইফুল ইসলাম", Share: 25, Committed: 5_000_000, JoinedOn: "2026-01-15",
		Shares: map[string]float64{karim.ID: 80},
	}.validate(httpx.LangBangla, time.Now())
	wantCode(t, err, codeInvalidInput)

	saiful := f.add(t, "সাইফুল ইসলাম", "01819556677", 33.3, 5_000_000, map[string]float64{karim.ID: 66.7})
	board, err := f.svc.Board(ctx, f.owner, f.project)
	if err != nil || len(board.Partners) != 2 || board.Partners[0].Share != 66.7 || board.Partners[1].Color != 1 {
		t.Fatalf("board = %+v, %v", board.Partners, err)
	}
	if board.Partners[0].Role == nil || *board.Partners[0].Role != project.RoleOwner {
		t.Fatalf("the owner partner's role = %v", board.Partners[0].Role)
	}

	call, err := f.svc.CreateCall(ctx, f.owner, f.project, NewCall{Total: 1_000_001, Reason: "ছাদ ঢালাই"})
	if err != nil {
		t.Fatalf("create call: %v", err)
	}
	if call.Parts[0].Amount+call.Parts[1].Amount != 1_000_001 || call.Parts[0].Amount != 667_001 || call.Parts[1].PartnerID != saiful.ID {
		t.Fatalf("parts = %+v", call.Parts)
	}
	again := f.add(t, "মাহবুব আলম", "01922445566", 10, 0, map[string]float64{karim.ID: 60, saiful.ID: 30})
	board, _ = f.svc.Board(ctx, f.owner, f.project)
	if board.Calls[0].Parts[0].Share != 66.7 || len(board.Calls[0].Parts) != 2 || board.Partners[2].ID != again.ID {
		t.Fatal("a later share change rewrote an earlier call")
	}
}

func TestDepositsGetReceiptsAndFillTheAccount(t *testing.T) {
	f := setup(t, project.OwnershipPartnership)
	ctx := context.Background()
	tania := f.add(t, "তানিয়া রহমান", "01755889900", 100, 4_000_000, map[string]float64{})
	call, err := f.svc.CreateCall(ctx, f.owner, f.project, NewCall{Total: 400_000, Reason: "ফাউন্ডেশন"})
	if err != nil {
		t.Fatalf("create call: %v", err)
	}

	// The call is dated by the Bangladesh day; deposits use the same day, or late in the
	// UTC evening the call would land a day after its deposit.
	today := f.svc.today()
	first, err := f.svc.Deposit(ctx, f.owner, f.project, tania.ID, httpx.LangBangla, NewDeposit{Amount: 1_500_000, AccountID: f.bank, Date: today})
	if err != nil || first.ReceiptNo != "PRK-000001" {
		t.Fatalf("first deposit = %+v, %v", first, err)
	}
	second, err := f.svc.Deposit(ctx, f.owner, f.project, tania.ID, httpx.LangBangla, NewDeposit{Amount: 400_000, CallID: &call.ID, AccountID: f.bank, Date: today})
	if err != nil || second.ReceiptNo != "PRK-000002" {
		t.Fatalf("second deposit = %+v, %v", second, err)
	}
	_, err = f.svc.Deposit(ctx, f.owner, f.project, tania.ID, httpx.LangBangla, NewDeposit{Amount: 1, AccountID: "00000000-0000-0000-0000-000000000000", Date: today})
	wantCode(t, err, codeInvalidInput)

	d, err := f.svc.Detail(ctx, f.owner, f.project, tania.ID)
	if err != nil || len(d.Entries) != 4 || d.Partner.Paid != 1_900_000 || d.Partner.CallShare != 400_000 || len(d.Calls) != 1 || d.Calls[0].Parts[0].Paid != 400_000 {
		t.Fatalf("detail = %+v, %v", d, err)
	}

	r, err := f.svc.Receipt(ctx, f.owner, f.project, second.ID)
	if err != nil || r.Should != 4_400_000 || r.PaidSoFar != 1_900_000 || r.Reason == nil || len(r.Others) != 2 || r.Company != "আব্দুল করিম" {
		t.Fatalf("receipt = %+v, %v", r, err)
	}
	if err = f.svc.Printed(ctx, f.owner, f.project, second.ID); err != nil {
		t.Fatalf("printed: %v", err)
	}

	accounts, err := f.books.Accounts(ctx, f.owner, f.project)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	for _, a := range accounts {
		if a.ID == f.bank && a.Balance != 1_900_000 {
			t.Fatalf("bank balance = %d, want the deposits", a.Balance)
		}
	}
}

func TestOnlyJointProjectsHavePartners(t *testing.T) {
	f := setup(t, project.OwnershipPersonal)
	in, _ := addBody{Phone: "01712345678", Name: "আব্দুল করিম", Share: 100, JoinedOn: "2026-01-15", Shares: map[string]float64{}}.validate(httpx.LangBangla, time.Now())
	_, err := f.svc.AddPartner(context.Background(), f.owner, f.project, httpx.LangBangla, in)
	wantCode(t, err, codeNotPartnership)
}

func TestANewPartnerCanJoinTheTeamAsAViewer(t *testing.T) {
	f := setup(t, project.OwnershipPartnership)
	ctx := context.Background()
	karim := f.add(t, "আব্দুল করিম", "01712345678", 100, 0, map[string]float64{})
	in, _ := addBody{
		Phone: "01633221100", Name: "নাসরিন আক্তার", Share: 20, JoinedOn: "2026-01-15",
		Shares: map[string]float64{karim.ID: 80}, Viewer: true,
	}.validate(httpx.LangBangla, time.Now())
	p, err := f.svc.AddPartner(ctx, f.owner, f.project, httpx.LangBangla, in)
	if err != nil || !p.Invited || p.HasAccount {
		t.Fatalf("partner = %+v, %v; want an open invitation", p, err)
	}
	l, err := f.svc.Lookup(ctx, f.owner, f.project, "01633221100", httpx.LangBangla)
	if err != nil || l.Status != LookupPartner {
		t.Fatalf("lookup = %+v, %v", l, err)
	}
}

func TestSplitCallGivesTheRemainderToTheBiggestShare(t *testing.T) {
	got := splitCall(100, []int64{333, 333, 334})
	if got[0]+got[1]+got[2] != 100 || got[2] != 34 {
		t.Fatalf("split = %v", got)
	}
}
