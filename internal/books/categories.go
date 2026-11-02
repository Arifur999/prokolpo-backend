package books

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Category is an expense head of the project.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Hidden heads stay on old expenses and in reports; the expense form leaves them out.
	Hidden bool `json:"hidden"`
}

// Categories lists the project's expense heads in their order.
func (s *Service) Categories(ctx context.Context, userID, projectID string) ([]Category, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, name, hidden FROM expense_categories WHERE project_id = $1 ORDER BY position, created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Category])
}

type categoryBody struct {
	Name string `json:"name"`
}

// CreateCategory adds a head at the end ("নতুন খাত" on the expense form).
func (s *Service) CreateCategory(ctx context.Context, userID, projectID, lang string, b categoryBody) (Category, error) {
	f := &fields{lang: lang}
	name := f.text("name", b.Name, 60)
	if name == nil {
		f.add("name", "category")
	}
	if err := f.err(); err != nil {
		return Category{}, err
	}
	c := Category{Name: *name}
	err := s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO expense_categories (project_id, name, position)
			SELECT $1, $2, coalesce(max(position), 0) + 1 FROM expense_categories WHERE project_id = $1 RETURNING id`, projectID, c.Name).Scan(&c.ID)
		if isUniqueViolation(err) {
			return nameTaken(lang, "categoryTaken")
		}
		if err != nil {
			return fmt.Errorf("insert category: %w", err)
		}
		return activity.Log(ctx, tx, activity.Entry{ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "category", EntityID: c.ID, Summary: c.Name})
	})
	return c, err
}
