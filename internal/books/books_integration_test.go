//go:build integration

package books

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
	pool     *pgxpool.Pool
	owner    string
	project  string
	category string
	phase    string
	cash     string
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at)
		VALUES ('আব্দুল', 'করিম', '01711000099', 'x', now()) RETURNING id`).Scan(&owner); err != nil {
		t.Fatalf("add user: %v", err)
	}
	projects := project.NewService(pool)
	p, err := projects.Create(ctx, owner, "নগদ ক্যাশ", project.NewProject{
		Name: "মিরপুর ভবন", Kind: project.KindBuilding, Ownership: project.OwnershipPersonal, StartDate: time.Now(),
		Budget: new(int64(1_000_000)), Categories: []string{"রড", "সিমেন্ট"}, Phases: []string{"ফাউন্ডেশন"},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	f := fixture{svc: NewService(pool, projects, nil), pool: pool, owner: owner, project: p.ID, phase: p.Phases[0].ID}
	if err := pool.QueryRow(ctx, `SELECT id FROM expense_categories WHERE project_id = $1 AND name = 'রড'`, p.ID).Scan(&f.category); err != nil {
		t.Fatalf("category: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM money_accounts WHERE project_id = $1`, p.ID).Scan(&f.cash); err != nil {
		t.Fatalf("account: %v", err)
	}
	return f
}

func (f fixture) supplier(t *testing.T, name string, opening int64) Supplier {
	t.Helper()
	s, err := f.svc.CreateSupplier(context.Background(), f.owner, f.project, httpx.LangBangla, NewSupplier{Name: name, Supplies: []string{}, OpeningDue: opening})
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	return s
}

func (f fixture) bill(t *testing.T, amount int64, supplierID *string, paidNow bool) Expense {
	t.Helper()
	in := ExpenseInput{CategoryID: f.category, PhaseID: &f.phase, SupplierID: supplierID, Date: time.Now(), Amount: amount, PaidNow: paidNow}
	if paidNow {
		in.AccountID = &f.cash
	}
	e, err := f.svc.CreateExpense(context.Background(), f.owner, f.project, in)
	if err != nil {
		t.Fatalf("create expense: %v", err)
	}
	return e
}

func (f fixture) supplierDue(t *testing.T, id string) int64 {
	t.Helper()
	d, err := f.svc.SupplierLedger(context.Background(), f.owner, f.project, id)
	if err != nil {
		t.Fatalf("supplier ledger: %v", err)
	}
	return d.Supplier.Due
}

func (f fixture) cashBalance(t *testing.T) int64 {
	t.Helper()
	accounts, err := f.svc.Accounts(context.Background(), f.owner, f.project)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	return accounts[0].Balance
}

func TestBillsOnCreditRaiseTheDueAndPaymentsLowerIt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	s := f.supplier(t, "করিম রড স্টোর", 20_000)

	onCredit := f.bill(t, 475_000, &s.ID, false)
	d, err := f.svc.SupplierLedger(context.Background(), f.owner, f.project, s.ID)
	if err != nil || d.Lines[0].Kind != "OPENING" {
		t.Fatalf("first ledger line = %+v, %v; want the opening due first", d.Lines, err)
	}
	f.bill(t, 10_000, &s.ID, true)
	if got := f.supplierDue(t, s.ID); got != 495_000 {
		t.Fatalf("due = %d, want opening + credit bill 495000", got)
	}
	if got := f.cashBalance(t); got != -10_000 {
		t.Fatalf("cash = %d, want -10000 after the bill paid on the spot", got)
	}

	if err := f.svc.Pay(ctx, f.owner, f.project, Party{Table: "suppliers", ID: s.ID}, NewPayment{AccountID: f.cash, Amount: 155_000, Date: time.Now(), ExpenseID: &onCredit.ID}); err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if got := f.supplierDue(t, s.ID); got != 340_000 {
		t.Fatalf("due after payment = %d, want 340000", got)
	}

	// Deleting the bill keeps the payment, which becomes an advance.
	if err := f.svc.DeleteExpense(ctx, f.owner, f.project, onCredit.ID, new("দুইবার লেখা")); err != nil {
		t.Fatalf("DeleteExpense: %v", err)
	}
	if got := f.supplierDue(t, s.ID); got != -135_000 {
		t.Fatalf("due after delete = %d, want an advance of 135000", got)
	}
	if err := f.svc.RestoreExpense(ctx, f.owner, f.project, onCredit.ID); err != nil {
		t.Fatalf("RestoreExpense: %v", err)
	}
	if got := f.supplierDue(t, s.ID); got != 340_000 {
		t.Fatalf("due after restore = %d, want 340000", got)
	}

	var logged int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM activity_log WHERE entity_id = $1`, onCredit.ID).Scan(&logged); err != nil || logged != 3 {
		t.Errorf("activity rows for the bill = %d, %v; want create, delete, restore", logged, err)
	}
}

func TestEditingABillLogsWhatChanged(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	e := f.bill(t, 100_000, nil, true)

	in := ExpenseInput{CategoryID: f.category, Date: time.Now(), Amount: 120_000, Description: new("ছাদ ঢালাই"), Reason: new("ভুল টাকা")}
	after, err := f.svc.UpdateExpense(ctx, f.owner, f.project, e.ID, in)
	if err != nil {
		t.Fatalf("UpdateExpense: %v", err)
	}
	if after.Amount != 120_000 || !after.PaidNow || f.cashBalance(t) != -120_000 {
		t.Fatalf("after edit: %+v, cash %d", after, f.cashBalance(t))
	}
	var changes int
	if err := f.pool.QueryRow(ctx, `SELECT jsonb_array_length(changes) FROM activity_log WHERE entity_id = $1 AND action = 'UPDATE'`, e.ID).Scan(&changes); err != nil || changes != 3 {
		t.Errorf("changes logged = %d, %v; want amount, phase and description", changes, err)
	}
}

func TestDailyWagesKeepTheRateOfTheDayWorked(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	team, err := f.svc.CreateContractor(ctx, f.owner, f.project, httpx.LangBangla, NewContractor{
		Name: "জামাল মিস্ত্রির দল", Work: "রাজমিস্ত্রি", PayType: "DAILY", DailyWage: new(int64(1_000)), TeamSize: new(int16(6)),
	})
	if err != nil {
		t.Fatalf("CreateContractor: %v", err)
	}
	monday := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	if err = f.svc.SaveAttendance(ctx, f.owner, f.project, []AttendanceCell{{ContractorID: team.ID, Day: monday, Headcount: 6}, {ContractorID: team.ID, Day: monday.AddDate(0, 0, 1), Headcount: 4, HalfDay: true}}); err != nil {
		t.Fatalf("SaveAttendance: %v", err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE contractors SET daily_wage = 1500 WHERE id = $1`, team.ID); err != nil {
		t.Fatalf("raise wage: %v", err)
	}
	if err = f.svc.SaveAttendance(ctx, f.owner, f.project, []AttendanceCell{{ContractorID: team.ID, Day: monday, Headcount: 5}, {ContractorID: team.ID, Day: monday.AddDate(0, 0, 2), Headcount: 2}}); err != nil {
		t.Fatalf("SaveAttendance again: %v", err)
	}

	d, err := f.svc.ContractorLedger(ctx, f.owner, f.project, team.ID)
	if err != nil {
		t.Fatalf("ContractorLedger: %v", err)
	}
	// 5×1000 (kept rate) + 4×1000/2 + 2×1500 (new day, new rate).
	if d.Contractor.Status != "WORKING" {
		t.Errorf("status = %s, want WORKING once attendance is in", d.Contractor.Status)
	}
	if d.Contractor.Wages != 10_000 || d.Contractor.ManDays != 9 || d.Contractor.Due != 10_000 {
		t.Fatalf("wages %d, man-days %v, due %d", d.Contractor.Wages, d.Contractor.ManDays, d.Contractor.Due)
	}
	if len(d.Lines) != 1 || d.Lines[0].Kind != "WEEK" || d.Lines[0].Date != "2026-09-27" {
		t.Fatalf("ledger lines = %+v, want one week starting Sunday 27/09", d.Lines)
	}
}

func TestContractPaymentsCountAsSpendingUpToTheContract(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, err := f.svc.CreateContractor(ctx, f.owner, f.project, httpx.LangBangla, NewContractor{
		Name: "মনির ইলেকট্রিক", Work: "ইলেকট্রিক", PayType: "CONTRACT", ContractAmount: new(int64(100_000)),
	})
	if err != nil {
		t.Fatalf("CreateContractor: %v", err)
	}
	if err = f.svc.AddExtra(ctx, f.owner, f.project, c.ID, NewExtra{Title: "বাড়তি পয়েন্ট", Amount: 12_000, Date: time.Now()}); err != nil {
		t.Fatalf("AddExtra: %v", err)
	}
	if err = f.svc.Pay(ctx, f.owner, f.project, Party{Table: "contractors", ID: c.ID}, NewPayment{AccountID: f.cash, Amount: 130_000, Date: time.Now()}); err != nil {
		t.Fatalf("Pay: %v", err)
	}
	var spent int64
	if err = f.pool.QueryRow(ctx, `SELECT project_spent($1)`, f.project).Scan(&spent); err != nil || spent != 112_000 {
		t.Fatalf("spent = %d, %v; want contract 100000 + extra 12000", spent, err)
	}
	o, err := f.svc.Overview(ctx, f.owner, f.project)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if o.Figures.Cash != -130_000 || o.Figures.ContractorDue != 0 || len(o.Months) != 12 {
		t.Fatalf("overview figures %+v, months %d", o.Figures, len(o.Months))
	}
}

func TestTransfersMoveMoneyWithoutSpendingIt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	bank, err := f.svc.CreateAccount(ctx, f.owner, f.project, httpx.LangBangla, NewAccount{Name: "ডাচ্-বাংলা ব্যাংক", Kind: "BANK", Opening: 500_000})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err = f.svc.Transfer(ctx, f.owner, f.project, NewTransfer{From: bank.ID, To: f.cash, Amount: 50_000, Date: time.Now()}); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if got := f.cashBalance(t); got != 50_000 {
		t.Fatalf("cash = %d, want 50000", got)
	}
	ledger, err := f.svc.Ledger(ctx, f.owner, f.project)
	if err != nil || len(ledger) != 3 {
		t.Fatalf("ledger = %+v, %v; want the opening and both legs", ledger, err)
	}
	_, err = f.svc.CreateAccount(ctx, f.owner, f.project, httpx.LangBangla, NewAccount{Name: "ডাচ্-বাংলা ব্যাংক", Kind: "BANK"})
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindInvalid || e.Fields[0].Path != "name" {
		t.Fatalf("duplicate account err = %v", err)
	}
}

func TestViewersCannotWrite(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var viewer string
	if err := f.pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at)
		VALUES ('রফিক', 'হাসান', '01711000098', 'x', now()) RETURNING id`).Scan(&viewer); err != nil {
		t.Fatalf("add viewer: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'VIEWER')`, f.project, viewer); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := f.svc.Suppliers(ctx, viewer, f.project); err != nil {
		t.Fatalf("viewer reading suppliers: %v", err)
	}
	_, err := f.svc.CreateSupplier(ctx, viewer, f.project, httpx.LangBangla, NewSupplier{Name: "নতুন", Supplies: []string{}})
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindForbidden {
		t.Fatalf("viewer create err = %v, want forbidden", err)
	}
}
