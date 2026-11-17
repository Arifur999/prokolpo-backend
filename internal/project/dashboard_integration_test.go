//go:build integration

package project

import (
	"context"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

// The dashboard card numbers an owner's projects, names the owner and the first manager,
// and counts the money that came in apart from transfers.
func TestTheDashboardCardCarriesNumberOwnerManagerAndDeposits(t *testing.T) {
	pool := testdb.Open(t)
	svc := NewService(pool)
	ctx := context.Background()
	owner := addUser(t, pool, "01711000001")
	manager := addUser(t, pool, "01711000002")

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`UPDATE users SET company_name = 'করিম বিল্ডার্স' WHERE id = $1`, owner)

	first, err := svc.Create(ctx, owner, "নগদ ক্যাশ", newBuilding(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, owner, "নগদ ক্যাশ", newBuilding(t))
	if err != nil {
		t.Fatal(err)
	}
	if first.ProjectNo != 1 || second.ProjectNo != 2 {
		t.Fatalf("numbers = %d, %d", first.ProjectNo, second.ProjectNo)
	}
	if !first.IsNew || first.Manager != nil || first.OwnerName != "আব্দুল করিম" || first.OwnerCompany == nil || *first.OwnerCompany != "করিম বিল্ডার্স" {
		t.Fatalf("new card = %+v", first.Summary)
	}

	exec(`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'MANAGER')`, first.ID, manager)
	exec(`UPDATE money_accounts SET opening_balance = 50000 WHERE project_id = $1`, first.ID)
	exec(`INSERT INTO money_accounts (project_id, name, kind) VALUES ($1, 'ব্যাংক', 'BANK')`, first.ID)

	list, err := svc.List(ctx, owner)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	card := list[1] // newest first
	if card.ID != first.ID || card.Deposited != 50000 || card.IsNew {
		t.Fatalf("card = %+v", card)
	}
	if card.Manager == nil || card.Manager.ID != manager || card.Manager.Phone != "01711000002" {
		t.Fatalf("manager = %+v", card.Manager)
	}

	// A deleted project keeps its number; the next one doesn't reuse it.
	exec(`UPDATE projects SET deleted_at = now() WHERE id = $1`, second.ID)
	third, err := svc.Create(ctx, owner, "নগদ ক্যাশ", newBuilding(t))
	if err != nil || third.ProjectNo != 3 {
		t.Fatalf("third = %d, %v", third.ProjectNo, err)
	}
}
