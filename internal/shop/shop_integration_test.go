//go:build integration

package shop

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
	bkash   string
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
	kind := "হার্ডওয়্যার দোকান"
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "করিম হার্ডওয়্যার", Kind: project.KindBusiness, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		BusinessType: &kind, Categories: []string{"মাল কেনা"}, Phases: []string{},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f := fixture{svc: NewService(pool, projects), books: books.NewService(pool, projects, nil), owner: owner, project: p.ID}
	f.svc.now = func() time.Time { return time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC) }
	if err := pool.QueryRow(ctx, `SELECT id FROM money_accounts WHERE project_id = $1`, p.ID).Scan(&f.cash); err != nil {
		t.Fatalf("account: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO money_accounts (project_id, name, kind) VALUES ($1, 'বিকাশ', 'MOBILE') RETURNING id`, p.ID).Scan(&f.bkash); err != nil {
		t.Fatalf("bkash: %v", err)
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

func (f fixture) save(t *testing.T, b dayBody) error {
	t.Helper()
	b.CashAccountID, b.MobileAccountID = f.cash, f.bkash
	in, err := b.validate(httpx.LangBangla, f.svc.today())
	if err != nil {
		return err
	}
	return f.svc.SaveDay(context.Background(), f.owner, f.project, httpx.LangBangla, in)
}

func (f fixture) book(t *testing.T) Book {
	t.Helper()
	b, err := f.svc.Book(context.Background(), f.owner, f.project)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	return b
}

func TestADayBookSplitsTakingsIntoAccountsAndTheCreditBook(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := f.save(t, dayBody{Date: "2026-10-02", Cash: 7800, Mobile: 1600, Credits: []lineBody{{Name: "রহিম কনস্ট্রাকশন", Amount: 3400}}}); err != nil {
		t.Fatalf("day one: %v", err)
	}
	err := f.save(t, dayBody{
		Date: "2026-10-03", Cash: 14250, Mobile: 2600, Other: 1200,
		Credits:  []lineBody{{Name: "সেলিম মিস্ত্রি", Amount: 1400}, {}},
		Collects: []lineBody{{Name: "রহিম কনস্ট্রাকশন", Amount: 3000}},
	})
	if err != nil {
		t.Fatalf("day two: %v", err)
	}

	b := f.book(t)
	today := b.Days[0]
	if today.Date != "2026-10-03" || today.Credit != 1400 || today.Collect != 3000 || len(today.Collects) != 1 || today.By != "আব্দুল করিম" {
		t.Fatalf("today = %+v", today)
	}
	if len(b.Khata) != 2 || b.Khata[0].Name != "সেলিম মিস্ত্রি" || b.Khata[1].Due != 400 {
		t.Fatalf("khata = %+v", b.Khata)
	}
	accounts, err := f.books.Accounts(ctx, f.owner, f.project)
	if err != nil || accounts[0].Balance != 7800+14250+1200+3000 || accounts[1].Balance != 4200 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
}

func TestEditingADayRewritesItsOwnCreditLines(t *testing.T) {
	f := setup(t)
	day := dayBody{Date: "2026-10-03", Cash: 5000, Credits: []lineBody{{Name: "জামাল প্লাম্বার", Amount: 5500}}}
	if err := f.save(t, day); err != nil {
		t.Fatalf("save: %v", err)
	}
	day.Credits[0].Amount = 2500
	if err := f.save(t, day); err != nil {
		t.Fatalf("edit: %v", err)
	}
	b := f.book(t)
	if len(b.Days) != 1 || b.Days[0].Credit != 2500 || b.Khata[0].Due != 2500 {
		t.Fatalf("after edit: days %+v, khata %+v", b.Days, b.Khata)
	}
}

func TestPaybacksCannotPassWhatIsOwed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := f.save(t, dayBody{Date: "2026-10-02", Credits: []lineBody{{Name: "নূর মসজিদ কমিটি", Amount: 6400}}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	err := f.save(t, dayBody{Date: "2026-10-03", Cash: 100, Collects: []lineBody{{Name: "অচেনা", Amount: 100}}})
	wantCode(t, err, codeInvalidInput)
	err = f.save(t, dayBody{Date: "2026-10-03"})
	wantCode(t, err, codeEmptyDay)

	person := f.book(t).Khata[0].ID
	err = f.svc.Collect(ctx, f.owner, f.project, person, httpx.LangBangla, CollectInput{Amount: 6401, AccountID: f.cash})
	wantCode(t, err, codeOverpaid)
	if err = f.svc.Collect(ctx, f.owner, f.project, person, httpx.LangBangla, CollectInput{Amount: 6400, AccountID: f.cash}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if b := f.book(t); len(b.Khata) != 0 {
		t.Fatalf("khata after paying = %+v", b.Khata)
	}
}
