package settings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

const maxPhases = 60

// PhaseInput is one row of the saved phase list, in its new order. A blank ID is a new phase.
type PhaseInput struct {
	ID     string
	Name   string
	Budget *int64
}

type phaseBody struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Budget *int64 `json:"budget"`
}

func validatePhases(body []phaseBody, lang string) ([]PhaseInput, error) {
	f := &fields{lang: lang}
	if len(body) > maxPhases {
		f.add("phases", "phases")
		return nil, f.err()
	}
	seen := map[string]bool{}
	out := make([]PhaseInput, 0, len(body))
	for i, b := range body {
		path := "phases." + strconv.Itoa(i)
		name := strings.TrimSpace(b.Name)
		key := strings.ToLower(name)
		if name == "" || utf8.RuneCountInString(name) > 60 || seen[key] {
			f.add(path+".name", "phaseName")
		}
		seen[key] = true
		if b.ID != "" && !isID(b.ID) {
			f.add(path, "phases")
		}
		out = append(out, PhaseInput{ID: b.ID, Name: name, Budget: f.budget(path+".budget", b.Budget)})
	}
	return out, f.err()
}

// SavePhases writes the phase list as the owner left it: renamed, re-budgeted, reordered,
// new rows added and missing ones deleted, unless they already carry entries. When any phase
// has a budget, their sum becomes the project's budget, which the overview measures against.
func (s *Service) SavePhases(ctx context.Context, userID, projectID string, list []PhaseInput) error {
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		kept := make([]string, 0, len(list))
		for _, p := range list {
			if p.ID != "" {
				kept = append(kept, p.ID)
			}
		}
		if err := dropPhases(ctx, tx, projectID, kept); err != nil {
			return err
		}
		// Park every position out of the way first, so the new order can't collide.
		if _, err := tx.Exec(ctx, `UPDATE project_phases SET position = position + 10000 WHERE project_id = $1`, projectID); err != nil {
			return fmt.Errorf("park phases: %w", err)
		}
		var total int64
		budgeted := false
		for i, p := range list {
			if err := writePhase(ctx, tx, projectID, i+1, p); err != nil {
				return err
			}
			if p.Budget != nil {
				total += *p.Budget
				budgeted = true
			}
		}
		if budgeted {
			if _, err := tx.Exec(ctx, `UPDATE projects SET budget = $2 WHERE id = $1`, projectID, total); err != nil {
				return fmt.Errorf("set budget: %w", err)
			}
		}
		return logSettings(ctx, tx, projectID, userID, "phases", strconv.Itoa(len(list)), nil)
	})
}

// dropPhases deletes the phases left out of the list, refusing any that has entries.
func dropPhases(ctx context.Context, tx pgx.Tx, projectID string, kept []string) error {
	var locked bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM project_phases ph
			WHERE ph.project_id = $1 AND NOT (ph.id::text = ANY($2))
			  AND (ph.setup OR EXISTS (SELECT 1 FROM expenses e WHERE e.phase_id = ph.id)
			                OR EXISTS (SELECT 1 FROM produce_sales x WHERE x.phase_id = ph.id)))`, projectID, kept).Scan(&locked)
	if err != nil {
		return fmt.Errorf("check phases: %w", err)
	}
	if locked {
		return messages.Fail(apperr.KindConflict, codePhaseLocked)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM project_phases WHERE project_id = $1 AND NOT (id::text = ANY($2))`, projectID, kept); err != nil {
		return fmt.Errorf("delete phases: %w", err)
	}
	return nil
}

func writePhase(ctx context.Context, tx pgx.Tx, projectID string, position int, p PhaseInput) error {
	if p.ID == "" {
		_, err := tx.Exec(ctx, `INSERT INTO project_phases (project_id, name, position, budget) VALUES ($1, $2, $3, $4)`, projectID, p.Name, position, p.Budget)
		if isUniqueViolation(err) {
			return messages.Fail(apperr.KindInvalid, codeInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("add phase: %w", err)
		}
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE project_phases SET name = $3, position = $4, budget = $5 WHERE id = $1 AND project_id = $2`,
		p.ID, projectID, p.Name, position, p.Budget)
	if isUniqueViolation(err) {
		return messages.Fail(apperr.KindInvalid, codeInvalidInput)
	}
	if err != nil {
		return fmt.Errorf("save phase: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notFound()
	}
	return nil
}
