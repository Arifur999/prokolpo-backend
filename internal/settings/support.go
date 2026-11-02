package settings

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// GrantHours are the windows the owner can open for support: 24 hours, 3 days, 7 days.
var GrantHours = []int{24, 72, 168}

func (s *Service) loadSupport(ctx context.Context, projectID string, st *Settings) error {
	var g Grant
	err := s.pool.QueryRow(ctx, `
		SELECT sg.starts_at, sg.ends_at, gu.first_name || ' ' || gu.last_name,
		       ou.first_name || ' ' || ou.last_name, sg.last_opened_at
		FROM support_grants sg
		JOIN users gu ON gu.id = sg.granted_by
		LEFT JOIN users ou ON ou.id = sg.last_opened_by
		WHERE sg.project_id = $1 AND sg.revoked_at IS NULL AND sg.ends_at > $2
		ORDER BY sg.starts_at DESC LIMIT 1`, projectID, s.now()).Scan(&g.StartsAt, &g.EndsAt, &g.GrantedBy, &g.LastOpenedBy, &g.LastOpenedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load support grant: %w", err)
	}
	st.Support = &g
	return nil
}

// GrantSupport lets platform support read the project for hours, replacing any open window.
func (s *Service) GrantSupport(ctx context.Context, userID, projectID, lang string, hours int) error {
	if !slices.Contains(GrantHours, hours) {
		f := &fields{lang: lang}
		f.add("hours", "hours")
		return f.err()
	}
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		now := s.now()
		if err := endGrants(ctx, tx, projectID, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO support_grants (project_id, granted_by, starts_at, ends_at) VALUES ($1, $2, $3, $4)`,
			projectID, userID, now, now.Add(time.Duration(hours)*time.Hour))
		if err != nil {
			return fmt.Errorf("grant support: %w", err)
		}
		return logSettings(ctx, tx, projectID, userID, "supportOn", strconv.Itoa(hours), map[string]string{"hours": strconv.Itoa(hours)})
	})
}

// RevokeSupport closes support's window early.
func (s *Service) RevokeSupport(ctx context.Context, userID, projectID string) error {
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		if err := endGrants(ctx, tx, projectID, s.now()); err != nil {
			return err
		}
		return logSettings(ctx, tx, projectID, userID, "supportOff", "", nil)
	})
}

func endGrants(ctx context.Context, tx pgx.Tx, projectID string, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE support_grants SET revoked_at = $2 WHERE project_id = $1 AND revoked_at IS NULL AND ends_at > $2`, projectID, now)
	if err != nil {
		return fmt.Errorf("end support grant: %w", err)
	}
	return nil
}
