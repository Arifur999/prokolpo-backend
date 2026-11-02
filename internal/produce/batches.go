package produce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
)

type batchBody struct {
	StartedOn string `json:"startedOn"`
	EndedOn   string `json:"endedOn"`
	HeadCount *int   `json:"headCount"`
	Losses    int    `json:"losses"`
	Note      string `json:"note"`
}

// BatchInput is a batch's validated details. A batch without an end date is still running.
type BatchInput struct {
	StartedOn *time.Time
	EndedOn   *time.Time
	HeadCount *int
	Losses    int
	Note      *string
}

func (b batchBody) validate(lang string, today time.Time) (BatchInput, error) {
	f := &fields{lang: lang}
	in := BatchInput{HeadCount: b.HeadCount, Losses: b.Losses}
	in.StartedOn = optionalDate(f, "startedOn", b.StartedOn)
	in.EndedOn = optionalDate(f, "endedOn", b.EndedOn)
	if in.StartedOn != nil && in.StartedOn.After(today) {
		f.add("startedOn", "date")
	}
	if in.EndedOn != nil && in.StartedOn != nil && in.EndedOn.Before(*in.StartedOn) {
		f.add("endedOn", "ended")
	}
	if b.HeadCount != nil && (*b.HeadCount <= 0 || *b.HeadCount > 10_000_000) {
		f.add("headCount", "count")
	}
	if b.Losses < 0 || (b.HeadCount != nil && b.Losses > *b.HeadCount) {
		f.add("losses", "count")
	}
	in.Note = optional(f, "note", b.Note, 120)
	return in, f.err()
}

// UpdateBatch saves a batch's dates, head count and losses.
func (s *Service) UpdateBatch(ctx context.Context, userID, projectID, phaseID string, in BatchInput) error {
	if !isID(phaseID) {
		return notFound()
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `
			UPDATE project_phases SET started_on = $3, ended_on = $4, head_count = $5, losses = $6, note = $7
			WHERE id = $1 AND project_id = $2 RETURNING name`,
			phaseID, projectID, in.StartedOn, in.EndedOn, in.HeadCount, in.Losses, in.Note).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return fmt.Errorf("update batch: %w", err)
		}
		details := map[string]string{}
		if in.EndedOn != nil {
			details["ended"] = in.EndedOn.Format(time.DateOnly)
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "UPDATE", Entity: "batch", EntityID: phaseID, Summary: name, Details: details,
		})
	})
}
