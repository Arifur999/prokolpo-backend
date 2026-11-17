//go:build integration

package project

import (
	"context"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/password"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

func TestCreatingAProjectOpensTheManagersLogin(t *testing.T) {
	pool := testdb.Open(t)
	svc := NewService(pool)
	ctx := context.Background()
	owner := addUser(t, pool, "01711000001")

	np := newBuilding(t)
	np.Manager = &NewManager{Name: "জসিম উদ্দিন", Phone: "01811223344", Password: "abc123", CanAddStaff: true}
	p, err := svc.Create(ctx, owner, "নগদ ক্যাশ", np)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Manager == nil || p.Manager.Name != "জসিম উদ্দিন" || p.Manager.Phone != "01811223344" || p.MemberCount != 2 {
		t.Fatalf("project = %+v", p.Summary)
	}

	var hash string
	var locked, staff bool
	var managedBy *string
	err = pool.QueryRow(ctx, `
		SELECT u.password_hash, u.password_locked, u.managed_by, m.can_add_staff
		FROM users u JOIN project_members m ON m.user_id = u.id AND m.project_id = $1 AND m.role = 'MANAGER'
		WHERE u.phone = '01811223344'`, p.ID).Scan(&hash, &locked, &managedBy, &staff)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := password.Verify("abc123", hash); !ok || !locked || managedBy == nil || *managedBy != owner || !staff {
		t.Errorf("manager account: verified=%v locked=%v managedBy=%v staff=%v", ok, locked, managedBy, staff)
	}
	var logged int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM activity_log WHERE project_id = $1 AND entity = 'member'`, p.ID).Scan(&logged); err != nil || logged != 1 {
		t.Errorf("activity entries = %d, %v", logged, err)
	}
}

// A number that already has an account is refused on the phone field, and nothing is created.
func TestATakenManagerNumberCreatesNothing(t *testing.T) {
	pool := testdb.Open(t)
	svc := NewService(pool)
	ctx := context.Background()
	owner := addUser(t, pool, "01711000001")
	addUser(t, pool, "01811223344")

	np := newBuilding(t)
	np.Manager = &NewManager{Name: "জসিম উদ্দিন", Phone: "01811223344", Password: "abc123"}
	_, err := svc.Create(ctx, owner, "নগদ ক্যাশ", np)
	wantCode(t, err, codePhoneTaken)
	if e, _ := apperr.As(err); len(e.Fields) != 1 || e.Fields[0].Path != "manager.phone" {
		t.Errorf("fields = %+v", e.Fields)
	}
	list, err := svc.List(ctx, owner)
	if err != nil || len(list) != 0 {
		t.Fatalf("projects after a refused create = %+v, %v", list, err)
	}
}
