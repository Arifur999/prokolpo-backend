package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// CategoryInput is a head's name, optional budget and whether the expense form shows it.
type CategoryInput struct {
	Name   string
	Budget *int64
	Hidden bool
}

type categoryBody struct {
	Name   string `json:"name"`
	Budget *int64 `json:"budget"`
	Hidden bool   `json:"hidden"`
}

func (b categoryBody) validate(lang string) (CategoryInput, error) {
	f := &fields{lang: lang}
	in := CategoryInput{Name: strings.TrimSpace(b.Name), Hidden: b.Hidden}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 60 {
		f.add("name", "category")
	}
	in.Budget = f.budget("budget", b.Budget)
	return in, f.err()
}

// AddCategory puts a new head at the end of the list.
func (s *Service) AddCategory(ctx context.Context, userID, projectID, lang string, in CategoryInput) (Category, error) {
	c := Category{Name: in.Name, Budget: in.Budget}
	err := s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO expense_categories (project_id, name, position, budget)
			SELECT $1, $2, coalesce(max(position), 0) + 1, $3 FROM expense_categories WHERE project_id = $1 RETURNING id`,
			projectID, in.Name, in.Budget).Scan(&c.ID)
		if isUniqueViolation(err) {
			return nameTaken(lang)
		}
		if err != nil {
			return fmt.Errorf("add category: %w", err)
		}
		return logSettings(ctx, tx, projectID, userID, "categoryAdd", in.Name, nil)
	})
	return c, err
}

// UpdateCategory renames a head, sets its budget, or hides it from the expense form (old
// expenses and reports keep it).
func (s *Service) UpdateCategory(ctx context.Context, userID, projectID, categoryID, lang string, in CategoryInput) error {
	if !isID(categoryID) {
		return notFound()
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var before string
		var wasHidden bool
		err := tx.QueryRow(ctx, `SELECT name, hidden FROM expense_categories WHERE id = $1 AND project_id = $2 FOR UPDATE`, categoryID, projectID).Scan(&before, &wasHidden)
		if err != nil {
			return notFoundOr(err, "load category")
		}
		_, err = tx.Exec(ctx, `UPDATE expense_categories SET name = $3, budget = $4, hidden = $5 WHERE id = $1 AND project_id = $2`,
			categoryID, projectID, in.Name, in.Budget, in.Hidden)
		if isUniqueViolation(err) {
			return nameTaken(lang)
		}
		if err != nil {
			return fmt.Errorf("save category: %w", err)
		}
		section := "category"
		if wasHidden != in.Hidden {
			section = map[bool]string{true: "categoryHide", false: "categoryShow"}[in.Hidden]
		}
		details := map[string]string{}
		if before != in.Name {
			details["from"] = before
		}
		return logSettings(ctx, tx, projectID, userID, section, in.Name, details)
	})
}

// DeleteCategory removes a head that no expense has used; a used one can only be hidden.
func (s *Service) DeleteCategory(ctx context.Context, userID, projectID, categoryID string) error {
	if !isID(categoryID) {
		return notFound()
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		var used bool
		err := tx.QueryRow(ctx, `
			SELECT c.name, EXISTS (SELECT 1 FROM expenses e WHERE e.category_id = c.id)
			FROM expense_categories c WHERE c.id = $1 AND c.project_id = $2 FOR UPDATE`, categoryID, projectID).Scan(&name, &used)
		if err != nil {
			return notFoundOr(err, "load category")
		}
		if used {
			return messages.Fail(apperr.KindConflict, codeCategoryInUse)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM expense_categories WHERE id = $1`, categoryID); err != nil {
			return fmt.Errorf("delete category: %w", err)
		}
		return logSettings(ctx, tx, projectID, userID, "categoryDelete", name, nil)
	})
}

func nameTaken(lang string) error {
	f := &fields{lang: lang}
	f.add("name", "catTaken")
	return f.err()
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}
	return fmt.Errorf("%s: %w", what, err)
}
