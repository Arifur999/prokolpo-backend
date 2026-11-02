//go:build integration

package produce

import (
	"context"
	"testing"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/books"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

type fixture struct {
	svc     *Service
	books   *books.Service
	owner   string
	project string
	cash    string
	batch   string
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, company_name, phone, password_hash, terms_accepted_at)
		VALUES ('আব্দুল', 'করিম', '', '01712345678', 'x', now()) RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("add user: %v", err)
	}
	projects := project.NewService(pool)
	poultry := "POULTRY"
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "সাভার পোল্ট্রি খামার", Kind: project.KindFarm, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		FarmType: &poultry, Categories: []string{"বাচ্চা", "খাবার"}, Phases: []string{"শেড তৈরি", "১ম ব্যাচ", "২য় ব্যাচ"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f := fixture{svc: NewService(pool, projects), books: books.NewService(pool, projects, nil), owner: owner, project: p.ID}
	f.svc.now = func() time.Time { return time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC) }
	if err := pool.QueryRow(ctx, `SELECT id FROM money_accounts WHERE project_id = $1`, p.ID).Scan(&f.cash); err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM project_phases WHERE project_id = $1 AND NOT setup ORDER BY position LIMIT 1`, p.ID).Scan(&f.batch); err != nil {
		t.Fatalf("batch: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO expenses (project_id, category_id, phase_id, date, amount, created_by)
		SELECT $1, c.id, $2, '2026-09-01', 412000, $3 FROM expense_categories c WHERE c.project_id = $1 LIMIT 1`, p.ID, f.batch, owner); err != nil {
		t.Fatalf("batch expense: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO expenses (project_id, category_id, phase_id, date, amount, created_by)
		SELECT $1, c.id, ph.id, '2026-08-01', 650000, $2 FROM expense_categories c, project_phases ph
		WHERE c.project_id = $1 AND ph.project_id = $1 AND ph.setup LIMIT 1`, p.ID, owner); err != nil {
		t.Fatalf("shed expense: %v", err)
	}
	return f
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func (f fixture) sell(t *testing.T, b saleBody) Saved {
	t.Helper()
	in, err := b.validate(httpx.LangBangla, f.svc.today())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	saved, err := f.svc.CreateSale(context.Background(), f.owner, f.project, httpx.LangBangla, in)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	return saved
}

func TestABatchEarnsAgainstWhatItCost(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.sell(t, saleBody{Product: "BROILER", PhaseID: f.batch, Quantity: 1280, Unit: "KG", Rate: 160, Buyer: "হানিফ ট্রেডার্স", Date: "2026-10-01", Received: 204_800, AccountID: f.cash})
	saved := f.sell(t, saleBody{Product: "BROILER", PhaseID: f.batch, Quantity: 1760, Unit: "KG", Rate: 168, Buyer: "মামুন পোল্ট্রি", Date: "2026-10-02", Received: 283_680, AccountID: f.cash})
	if saved.Amount != 295_680 || saved.Left != 12_000 {
		t.Fatalf("saved = %+v, want 2,95,680 with 12,000 left", saved)
	}
	f.sell(t, saleBody{Product: "LITTER", PhaseID: f.batch, Quantity: 30, Unit: "SACK", Rate: 150, Buyer: "আজিজুল হক", Date: "2026-10-02", Received: 4500, AccountID: f.cash})

	ended, err := batchBody{StartedOn: "2026-09-01", EndedOn: "2026-10-02", HeadCount: new(1500), Losses: 48}.validate(httpx.LangBangla, f.svc.today())
	if err != nil {
		t.Fatalf("validate batch: %v", err)
	}
	if err = f.svc.UpdateBatch(ctx, f.owner, f.project, f.batch, ended); err != nil {
		t.Fatalf("end batch: %v", err)
	}

	book, err := f.svc.Book(ctx, f.owner, f.project)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	if book.SetupCost != 650_000 || book.SetupName != "শেড তৈরি" || len(book.Batches) != 2 {
		t.Fatalf("setup %d with batches %+v; want the shed apart from two batches", book.SetupCost, book.Batches)
	}
	b := book.Batches[0]
	if b.Cost != 412_000 || b.Sales != 504_980 || b.Weight != 3040 || b.WeightSales != 500_480 || b.EndedOn == nil || b.Losses != 48 {
		t.Fatalf("batch = %+v", b)
	}
	if len(book.Dues) != 1 || book.Dues[0].Buyer != "মামুন পোল্ট্রি" || book.Dues[0].Amount != 12_000 {
		t.Fatalf("dues = %+v", book.Dues)
	}
	if book.Sales[0].Product != "LITTER" || book.Buyers[0].Name != "আজিজুল হক" {
		t.Fatalf("newest sale %+v, newest buyer %+v", book.Sales[0], book.Buyers[0])
	}
}

func TestABuyerPaysOffWhatTheyOweIntoAnAccount(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.sell(t, saleBody{Product: "BROILER", Quantity: 900, Unit: "KG", Rate: 175, Buyer: "হানিফ ট্রেডার্স", Date: "2026-10-02", Received: 120_000, AccountID: f.cash})
	book, _ := f.svc.Book(ctx, f.owner, f.project)
	buyer := book.Dues[0].BuyerID

	err := f.svc.Collect(ctx, f.owner, f.project, buyer, httpx.LangBangla, CollectInput{Amount: 37_501, AccountID: f.cash})
	wantCode(t, err, codeOverpaid)
	if err = f.svc.Collect(ctx, f.owner, f.project, buyer, httpx.LangBangla, CollectInput{Amount: 37_500, AccountID: f.cash}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	book, _ = f.svc.Book(ctx, f.owner, f.project)
	if len(book.Dues) != 0 || book.Sales[0].Left != 0 {
		t.Fatalf("after paying: dues %+v, sale %+v", book.Dues, book.Sales[0])
	}
	accounts, err := f.books.Accounts(ctx, f.owner, f.project)
	if err != nil || accounts[0].Balance != 157_500 {
		t.Fatalf("cash = %+v, %v; want 1,57,500 in", accounts, err)
	}
}

func TestAFarmOnlySellsWhatItMakes(t *testing.T) {
	f := setup(t)
	in, _ := saleBody{Product: "MILK", Quantity: 10, Unit: "LITRE", Rate: 80, Buyer: "দুধের দোকান", Date: "2026-10-02"}.validate(httpx.LangBangla, f.svc.today())
	_, err := f.svc.CreateSale(context.Background(), f.owner, f.project, httpx.LangBangla, in)
	wantCode(t, err, codeInvalidInput)

	_, err = saleBody{Product: "BROILER", Quantity: 10, Unit: "KG", Rate: 100, Buyer: "ক", Date: "2026-10-02", Received: 1001, AccountID: f.cash}.validate(httpx.LangBangla, f.svc.today())
	wantCode(t, err, codeInvalidInput)
}
