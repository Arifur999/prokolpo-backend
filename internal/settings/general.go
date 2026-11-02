package settings

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// GeneralInput is the "প্রজেক্টের তথ্য" form after validation.
type GeneralInput struct {
	Name        string
	Location    *string
	Description *string
	StartDate   time.Time
	Status      string
}

type generalBody struct {
	Name        string `json:"name"`
	Location    string `json:"location"`
	Description string `json:"description"`
	StartDate   string `json:"startDate"`
	Status      string `json:"status"`
}

var statuses = []string{"ACTIVE", "ON_HOLD", "DONE"}

func (b generalBody) validate(lang string) (GeneralInput, error) {
	f := &fields{lang: lang}
	in := GeneralInput{Name: strings.TrimSpace(b.Name), Status: b.Status}
	switch n := utf8.RuneCountInString(in.Name); {
	case n < 3:
		f.add("name", "nameShort")
	case n > 80:
		f.add("name", "nameLong")
	}
	in.Location = f.text("location", b.Location, 120, "location")
	in.Description = f.text("description", b.Description, 500, "description")
	d, err := time.Parse(time.DateOnly, b.StartDate)
	if err != nil || d.Year() < 1990 || d.Year() > 2100 {
		f.add("startDate", "date")
	}
	in.StartDate = d
	if !slices.Contains(statuses, b.Status) {
		f.add("status", "status")
	}
	return in, f.err()
}

// SaveGeneral changes the project's name, place, start, description and status. A project
// marked done takes no new entries until it is set going again.
func (s *Service) SaveGeneral(ctx context.Context, userID, projectID string, in GeneralInput) error {
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		var before string
		if err := tx.QueryRow(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&before); err != nil {
			return fmt.Errorf("read status: %w", err)
		}
		_, err := tx.Exec(ctx, `
			UPDATE projects SET name = $2, location = $3, description = $4, start_date = $5, status = $6 WHERE id = $1`,
			projectID, in.Name, in.Location, in.Description, in.StartDate, in.Status)
		if err != nil {
			return fmt.Errorf("save project: %w", err)
		}
		details := map[string]string{}
		if before != in.Status {
			details["status"], details["from"] = in.Status, before
		}
		return logSettings(ctx, tx, projectID, userID, "general", in.Name, details)
	})
}
