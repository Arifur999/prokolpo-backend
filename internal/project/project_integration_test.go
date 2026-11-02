//go:build integration

package project

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

func addUser(t *testing.T, pool *pgxpool.Pool, phone string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at)
		VALUES ('আব্দুল', 'করিম', $1, 'x', now()) RETURNING id`, phone).Scan(&id)
	if err != nil {
		t.Fatalf("add user: %v", err)
	}
	return id
}

func newBuilding(t *testing.T) NewProject {
	t.Helper()
	np, err := validBuilding().validate(httpx.LangBangla)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return np
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if e, ok := apperr.As(err); !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestCreateSeedsTheBooksAndListsForTheOwnerOnly(t *testing.T) {
	pool := testdb.Open(t)
	svc := NewService(pool)
	ctx := context.Background()
	owner := addUser(t, pool, "01711000001")
	stranger := addUser(t, pool, "01711000002")

	p, err := svc.Create(ctx, owner, "নগদ ক্যাশ", newBuilding(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Role != RoleOwner || p.MemberCount != 1 || p.AccountCount != 1 || len(p.Phases) != 2 || p.StartDate != "2026-10-03" {
		t.Errorf("created project = %+v", p)
	}
	var categories int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM expense_categories WHERE project_id = $1`, p.ID).Scan(&categories)
	if err != nil || categories != 2 {
		t.Errorf("categories = %d, %v", categories, err)
	}

	mine, err := svc.List(ctx, owner)
	if err != nil || len(mine) != 1 || mine[0].ID != p.ID {
		t.Fatalf("owner's list = %+v, %v", mine, err)
	}
	theirs, err := svc.List(ctx, stranger)
	if err != nil || len(theirs) != 0 {
		t.Fatalf("stranger's list = %+v, %v", theirs, err)
	}
	_, err = svc.Get(ctx, stranger, p.ID)
	wantCode(t, err, codeNotFound)
	_, err = svc.Get(ctx, owner, "not-a-uuid")
	wantCode(t, err, codeNotFound)
}

func TestDeleteAndRestoreAreOwnerOnlyAndTimeBound(t *testing.T) {
	pool := testdb.Open(t)
	svc := NewService(pool)
	ctx := context.Background()
	owner := addUser(t, pool, "01711000003")
	manager := addUser(t, pool, "01711000004")

	p, err := svc.Create(ctx, owner, "নগদ ক্যাশ", newBuilding(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'MANAGER')`, p.ID, manager); err != nil {
		t.Fatalf("add manager: %v", err)
	}

	wantCode(t, svc.Delete(ctx, manager, p.ID), codeOwnerOnly)
	if err = svc.Delete(ctx, owner, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = svc.Get(ctx, manager, p.ID); err == nil {
		t.Error("manager still sees a deleted project")
	}
	deleted, err := svc.Deleted(ctx, owner)
	if err != nil || len(deleted) != 1 || len(deleted[0].Members) != 1 || deleted[0].Members[0].Role != RoleManager {
		t.Fatalf("Deleted = %+v, %v", deleted, err)
	}
	wantCode(t, svc.Restore(ctx, manager, p.ID), codeNotFound)

	svc.now = func() time.Time { return time.Now().Add(RestoreWindow + time.Hour) }
	wantCode(t, svc.Restore(ctx, owner, p.ID), codeRestoreClosed)

	svc.now = time.Now
	if err := svc.Restore(ctx, owner, p.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := svc.Get(ctx, manager, p.ID); err != nil {
		t.Errorf("manager after restore: %v", err)
	}
}
