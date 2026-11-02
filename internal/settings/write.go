package settings

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
)

// owned runs change in a transaction once the caller is known to own the project, with the
// project row locked so two settings saves don't interleave.
func (s *Service) owned(ctx context.Context, userID, projectID string, change func(pgx.Tx) error) error {
	if _, err := s.members.Owner(ctx, userID, projectID); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM projects WHERE id = $1 FOR UPDATE`, projectID); err != nil {
			return fmt.Errorf("lock project: %w", err)
		}
		return change(tx)
	})
}

// logSettings records a settings change in the project's activity, under "settings".
func logSettings(ctx context.Context, tx pgx.Tx, projectID, userID, section, summary string, details map[string]string) error {
	if details == nil {
		details = map[string]string{}
	}
	details["section"] = section
	return activity.Log(ctx, tx, activity.Entry{
		ProjectID: projectID, ActorID: userID, Action: "UPDATE", Entity: "settings", EntityID: projectID, Summary: summary, Details: details,
	})
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isID(s string) bool { return uuidPattern.MatchString(s) }

// text trims s and checks its length; blank is nil.
func (f *fields) text(path, s string, limit int, key string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if utf8.RuneCountInString(s) > limit {
		f.add(path, key)
	}
	return &s
}

// maxAmount matches the books: thirteen digits of taka.
const maxAmount = 9_999_999_999_999

func (f *fields) budget(path string, n *int64) *int64 {
	if n == nil || *n == 0 {
		return nil
	}
	if *n < 0 || *n > maxAmount {
		f.add(path, "budget")
	}
	return n
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
