//go:build integration

package sales

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
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, company_name, phone, password_hash, terms_accepted_at)
		VALUES ('আব্দুল', 'করিম', 'করিম বিল্ডার্স', '01712345678', 'x', now()) RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("add user: %v", err)
	}
	projects := project.NewService(pool)
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "মিরপুর ভবন", Kind: project.KindBuilding, Ownership: project.OwnershipPersonal, StartDate: time.Now(), Categories: []string{"রড"}, Phases: []string{},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f := fixture{svc: NewService(pool, projects), books: books.NewService(pool, projects, nil), owner: owner, project: p.ID}
	f.svc.now = func() time.Time { return time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC) }
	if err := pool.QueryRow(ctx, `SELECT id FROM money_accounts WHERE project_id = $1`, p.ID).Scan(&f.cash); err != nil {
		t.Fatalf("account: %v", err)
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

func (f fixture) units(t *testing.T) []Unit {
	t.Helper()
	list, err := f.svc.Units(context.Background(), f.owner, f.project)
	if err != nil {
		t.Fatalf("units: %v", err)
	}
	return list
}

func TestABlockOfFlatsSkipsNamesAlreadyUsed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in, err := unitsBody{Units: []unitBody{{Name: "ফ্ল্যাট ৩-এ", Type: "FLAT", Floor: 3, Size: 1050, Price: 6_200_000}}}.validate(httpx.LangBangla)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err = f.svc.AddUnits(ctx, f.owner, f.project, in); err != nil {
		t.Fatalf("add one: %v", err)
	}
	_, err = f.svc.AddUnits(ctx, f.owner, f.project, in)
	wantCode(t, err, codeUnitTaken)

	block, _ := unitsBody{Units: []unitBody{
		{Name: "ফ্ল্যাট ৩-এ", Type: "FLAT", Floor: 3, Price: 6_200_000},
		{Name: "ফ্ল্যাট ৩-বি", Type: "FLAT", Floor: 3, Price: 5_500_000},
		{Name: "দোকান ১", Type: "SHOP", Floor: 1, Price: 3_000_000},
	}}.validate(httpx.LangBangla)
	added, err := f.svc.AddUnits(ctx, f.owner, f.project, block)
	if err != nil || added != 2 {
		t.Fatalf("block added %d, %v; want the two new names", added, err)
	}
	list := f.units(t)
	if len(list) != 3 || list[0].Floor != 3 || list[1].Position != 2 || list[2].Type != "SHOP" {
		t.Fatalf("units = %+v", list)
	}
}

func TestASaleOnInstallmentsFillsOldestFirstWithReceipts(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in, _ := unitsBody{Units: []unitBody{{Name: "ফ্ল্যাট ৩-বি", Type: "FLAT", Floor: 3, Size: 950, Price: 5_500_000}}}.validate(httpx.LangBangla)
	if _, err := f.svc.AddUnits(ctx, f.owner, f.project, in); err != nil {
		t.Fatalf("add unit: %v", err)
	}
	unit := f.units(t)[0]

	sale, err := saleBody{
		UnitID: unit.ID, Customer: &customerBody{Name: "নাসরিন আক্তার", Phone: "01712-998877"},
		Date: "2026-04-10", Price: 5_400_000, Down: 3_360_000, Installments: 24, IntervalMonths: 1, FirstDue: "2026-04-25", AccountID: f.cash,
	}.validate(httpx.LangBangla, f.svc.today())
	if err != nil {
		t.Fatalf("validate sale: %v", err)
	}
	sold, err := f.svc.CreateSale(ctx, f.owner, f.project, httpx.LangBangla, sale)
	if err != nil || sold.ReceiptNo == nil || *sold.ReceiptNo != "PRK-000001" {
		t.Fatalf("sale = %+v, %v", sold, err)
	}
	_, err = f.svc.CreateSale(ctx, f.owner, f.project, httpx.LangBangla, sale)
	wantCode(t, err, codeUnitSold)

	pay := func(amount int64) Sold {
		t.Helper()
		in, vErr := collectBody{Amount: amount, AccountID: f.cash}.validate(httpx.LangBangla, f.svc.today())
		if vErr != nil {
			t.Fatalf("validate collect: %v", vErr)
		}
		out, cErr := f.svc.Collect(ctx, f.owner, f.project, sold.SaleID, httpx.LangBangla, in)
		if cErr != nil {
			t.Fatalf("collect %d: %v", amount, cErr)
		}
		return out
	}
	pay(85_000)
	second := pay(127_500)

	d, err := f.svc.Sale(ctx, f.owner, f.project, sold.SaleID)
	if err != nil {
		t.Fatalf("sale detail: %v", err)
	}
	st := d.Sale.Standing
	if st.Received != 3_572_500 || st.PaidCount != 2 || st.Next.No != 3 || st.Next.Amount != 42_500 || st.Late == nil || st.Late.No != 3 {
		t.Fatalf("standing = %+v", st)
	}
	if len(d.Payments) != 3 || d.Rows[1].PaidOn == nil {
		t.Fatalf("payments = %d, rows[1] = %+v", len(d.Payments), d.Rows[1])
	}

	r, err := f.svc.Receipt(ctx, f.owner, f.project, *second.PaymentID)
	if err != nil || len(r.Covered) != 2 || r.Covered[0].No != 2 || r.PaidSoFar != 3_572_500 || r.Next.No != 3 || r.Company != "করিম বিল্ডার্স" || len(r.Others) != 3 {
		t.Fatalf("receipt = %+v, %v", r, err)
	}

	in2, _ := collectBody{Amount: 99_000_000, AccountID: f.cash}.validate(httpx.LangBangla, f.svc.today())
	_, err = f.svc.Collect(ctx, f.owner, f.project, sold.SaleID, httpx.LangBangla, in2)
	wantCode(t, err, codeOverpaid)

	wantCode(t, f.svc.DeleteUnit(ctx, f.owner, f.project, unit.ID), codeUnitHasSale)
	if got := f.units(t)[0].Sale; got == nil || got.Standing.Status != StatusBooked {
		t.Fatalf("unit sale = %+v", got)
	}

	accounts, _ := f.books.Accounts(ctx, f.owner, f.project)
	if accounts[0].Balance != 3_572_500 {
		t.Fatalf("cash = %d, want everything the customer paid", accounts[0].Balance)
	}
	customers, err := f.svc.Customers(ctx, f.owner, f.project)
	if err != nil || len(customers) != 1 || len(customers[0].Sales) != 1 || customers[0].Phone != "01712998877" {
		t.Fatalf("customers = %+v, %v", customers, err)
	}
}

func TestAFullPriceSaleIsSoldOnTheDay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in, _ := unitsBody{Units: []unitBody{{Name: "দোকান ২", Type: "SHOP", Floor: 1, Price: 2_800_000}}}.validate(httpx.LangBangla)
	if _, err := f.svc.AddUnits(ctx, f.owner, f.project, in); err != nil {
		t.Fatalf("add unit: %v", err)
	}
	unit := f.units(t)[0]
	c, err := f.svc.AddCustomer(ctx, f.owner, f.project, CustomerInput{Name: "সোহেল রানা", Phone: "01624889911"})
	if err != nil {
		t.Fatalf("add customer: %v", err)
	}
	sale, err := saleBody{UnitID: unit.ID, CustomerID: c.ID, Date: "2026-02-05", Price: 2_800_000, AccountID: f.cash}.validate(httpx.LangBangla, f.svc.today())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err = f.svc.CreateSale(ctx, f.owner, f.project, httpx.LangBangla, sale); err != nil {
		t.Fatalf("sale: %v", err)
	}
	if got := f.units(t)[0].Sale.Standing; got.Status != StatusSold || got.Due != 0 {
		t.Fatalf("standing = %+v", got)
	}
	_, err = saleBody{UnitID: unit.ID, CustomerID: c.ID, Date: "2026-02-05", Price: 1_000_000, Down: 1_000_000, Installments: 12, IntervalMonths: 1, FirstDue: "2026-03-01", AccountID: f.cash}.
		validate(httpx.LangBangla, f.svc.today())
	wantCode(t, err, codeInvalidInput)
}
