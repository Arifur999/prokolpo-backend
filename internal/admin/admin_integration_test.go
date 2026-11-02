//go:build integration

package admin

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/audit"
	"github.com/Arifur999/prokolpo-backend/internal/project"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

type fixture struct {
	svc   *Service
	pool  *pgxpool.Pool
	staff string
	owner string
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	f := fixture{svc: NewService(pool, nil, nil), pool: pool}
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at, staff_role)
		VALUES ('মাহমুদ', 'হাসান', '01700112233', 'x', now(), 'super_admin') RETURNING id`).Scan(&f.staff); err != nil {
		t.Fatalf("add staff: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (first_name, last_name, company_name, phone, email, password_hash, terms_accepted_at, last_login_at)
		VALUES ('সুমাইয়া', 'আক্তার', 'সুমাইয়া এগ্রো', '01933450128', 'sumaiya@example.com', 'x', now(), now()) RETURNING id`).Scan(&f.owner); err != nil {
		t.Fatalf("add user: %v", err)
	}
	return f
}

func TestTheOverviewCountsUsersProjectsAndWhatNeedsALook(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	poultry := "POULTRY"
	if _, err := project.NewService(f.pool).Create(ctx, f.owner, "নগদ ক্যাশ", project.NewProject{
		Name: "সাভার খামার", Kind: project.KindFarm, Ownership: project.OwnershipPartnership, StartDate: time.Now(),
		FarmType: &poultry, Categories: []string{"খাবার"}, Phases: []string{"শেড তৈরি"},
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO support_requests (name, phone, topic, source, message) VALUES ('ক', '01711000000', 'login', 'public', 'লগইন হচ্ছে না কেন?')`); err != nil {
		t.Fatal(err)
	}
	if err := audit.Log(ctx, f.pool, audit.Entry{ActorID: &f.staff, Action: "user.suspend", Summary: "কামরুল হাসান", Details: map[string]string{"reason": "প্রতারণার অভিযোগ"}}); err != nil {
		t.Fatal(err)
	}

	o, err := f.svc.Overview(ctx, f.staff)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if o.Users.Total != 1 || o.Users.ThisWeek != 1 || o.Users.Active30 != 1 || o.Users.Bangla != 1 {
		t.Fatalf("users = %+v; staff must not count", o.Users)
	}
	if o.Projects.Total != 1 || o.Projects.Joint != 1 || o.Projects.ByKind["FARM"] != 1 {
		t.Fatalf("projects = %+v", o.Projects)
	}
	if len(o.Weeks) != 12 || o.Weeks[11].Count != 1 || len(o.Months) != 6 || o.Months[5].Count != 1 {
		t.Fatalf("weeks %+v months %+v", o.Weeks, o.Months)
	}
	if len(o.Recent) != 1 || !o.Recent[0].HasEmail || len(o.Recent[0].Kinds) != 1 {
		t.Fatalf("recent = %+v", o.Recent)
	}
	if o.Attention.OpenSupport != 1 || o.Attention.Staff != 1 || o.Attention.StaffWithout2FA != 1 {
		t.Fatalf("attention = %+v", o.Attention)
	}
	if len(o.AuditTrail) != 1 || o.AuditTrail[0].Actor != "মাহমুদ হাসান" || o.AuditTrail[0].Details["reason"] == "" {
		t.Fatalf("audit = %+v", o.AuditTrail)
	}
}
