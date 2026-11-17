package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/password"
)

// addManager opens the manager's login inside the create transaction and puts them on the
// project. The account is the owner's to look after: it speaks the owner's language, and its
// password stays locked to what the owner set. A number that already has an account is
// refused here; that person is invited from the members page instead.
func addManager(ctx context.Context, tx pgx.Tx, projectID, ownerID string, m NewManager, lang string) error {
	hash, err := password.Hash(m.Password)
	if err != nil {
		return fmt.Errorf("hash manager password: %w", err)
	}
	var managerID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, phone, password_hash, terms_accepted_at, managed_by, password_locked, locale)
		SELECT $1, '', $2, $3, now(), id, true, locale FROM users WHERE id = $4
		ON CONFLICT (phone) DO NOTHING
		RETURNING id`, m.Name, m.Phone, hash, ownerID).Scan(&managerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return messages.Fail(apperr.KindConflict, codePhoneTaken,
			apperr.FieldError{Path: "manager.phone", Message: fieldMessages["managerPhoneTaken"].In(lang)})
	}
	if err != nil {
		return fmt.Errorf("add manager account: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role, can_add_staff) VALUES ($1, $2, 'MANAGER', $3)`,
		projectID, managerID, m.CanAddStaff); err != nil {
		return fmt.Errorf("add manager: %w", err)
	}
	return activity.Log(ctx, tx, activity.Entry{
		ProjectID: projectID, ActorID: ownerID, Action: "CREATE", Entity: "member", EntityID: managerID, Summary: m.Name,
		Details: map[string]string{"role": RoleManager, "phone": m.Phone, "managed": "true"},
	})
}
