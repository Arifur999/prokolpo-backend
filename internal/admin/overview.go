package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Overview is the admin dashboard (board AdminDashboard). Staff accounts aren't counted as users.
type Overview struct {
	Users      UserCounts    `json:"users"`
	Projects   ProjectCounts `json:"projects"`
	Entries30  int           `json:"entries30"`
	Weeks      []Count       `json:"weeks"`
	Months     []Count       `json:"months"`
	Recent     []Signup      `json:"recent"`
	Attention  Attention     `json:"attention"`
	AuditTrail []AuditLine   `json:"audit"`
}

// UserCounts are the user figures; Bangla and English are the languages they chose.
type UserCounts struct {
	Total    int `json:"total"`
	ThisWeek int `json:"thisWeek"`
	Active30 int `json:"active30"`
	Bangla   int `json:"bangla"`
	English  int `json:"english"`
}

// ProjectCounts are live projects (deleted ones aside) by status, ownership and kind.
type ProjectCounts struct {
	Total   int            `json:"total"`
	Running int            `json:"running"`
	Joint   int            `json:"joint"`
	ByKind  map[string]int `json:"byKind"`
}

// Count is sign-ups in one period, named by its first day (YYYY-MM-DD).
type Count struct {
	Start string `json:"start"`
	Count int    `json:"count"`
}

// Signup is a recent registration with what it has opened since.
type Signup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Company   *string   `json:"company"`
	Phone     string    `json:"phone"`
	HasEmail  bool      `json:"hasEmail"`
	Locale    string    `json:"locale"`
	CreatedAt time.Time `json:"createdAt"`
	Kinds     []string  `json:"kinds"`
}

// Attention is what wants a look: waiting support, projects about to be purged, suspended
// accounts, and staff without the second step.
type Attention struct {
	OpenSupport     int        `json:"openSupport"`
	OldestSupport   *time.Time `json:"oldestSupport"`
	PurgeSoon       int        `json:"purgeSoon"`
	FirstPurge      *time.Time `json:"firstPurge"`
	Suspended       int        `json:"suspended"`
	LastSuspended   *time.Time `json:"lastSuspended"`
	Staff           int        `json:"staff"`
	StaffWithout2FA int        `json:"staffWithout2fa"`
}

// AuditLine is one admin log entry as lists show it.
type AuditLine struct {
	ID        string            `json:"id"`
	ActorID   *string           `json:"actorId"`
	Actor     string            `json:"actor"`
	Action    string            `json:"action"`
	Summary   string            `json:"summary"`
	Details   map[string]string `json:"details"`
	CreatedAt time.Time         `json:"createdAt"`
}

// Overview gathers the dashboard; userID is the staff member asking (for nothing yet but the log).
func (s *Service) Overview(ctx context.Context, _ string) (Overview, error) {
	var o Overview
	now := s.now()
	err := s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE created_at > $1), count(*) FILTER (WHERE last_login_at > $2),
		       count(*) FILTER (WHERE locale = 'bn'), count(*) FILTER (WHERE locale = 'en')
		FROM users WHERE staff_role IS NULL`, now.AddDate(0, 0, -7), now.AddDate(0, 0, -30)).Scan(
		&o.Users.Total, &o.Users.ThisWeek, &o.Users.Active30, &o.Users.Bangla, &o.Users.English)
	if err != nil {
		return Overview{}, fmt.Errorf("count users: %w", err)
	}
	if o.Projects, err = projectCounts(ctx, s.pool); err != nil {
		return Overview{}, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM expenses WHERE created_at > $1) + (SELECT count(*) FROM payments WHERE created_at > $1)
		     + (SELECT count(*) FROM attendance WHERE updated_at > $1) + (SELECT count(*) FROM partner_deposits WHERE created_at > $1)
		     + (SELECT count(*) FROM sale_payments WHERE created_at > $1)`, now.AddDate(0, 0, -30)).Scan(&o.Entries30)
	if err != nil {
		return Overview{}, fmt.Errorf("count entries: %w", err)
	}
	if o.Weeks, o.Months, err = signupSeries(ctx, s.pool, now); err != nil {
		return Overview{}, err
	}
	if o.Recent, err = recentSignups(ctx, s.pool, 5); err != nil {
		return Overview{}, err
	}
	if o.Attention, err = attention(ctx, s.pool, now); err != nil {
		return Overview{}, err
	}
	o.AuditTrail, err = auditLines(ctx, s.pool, AuditFilter{Limit: 5})
	return o, err
}

func projectCounts(ctx context.Context, q querier) (ProjectCounts, error) {
	c := ProjectCounts{ByKind: map[string]int{}}
	rows, err := q.Query(ctx, `
		SELECT kind, count(*), count(*) FILTER (WHERE status = 'ACTIVE'), count(*) FILTER (WHERE ownership = 'PARTNERSHIP')
		FROM projects WHERE deleted_at IS NULL GROUP BY kind`)
	if err != nil {
		return c, fmt.Errorf("count projects: %w", err)
	}
	var kind string
	var n, running, joint int
	_, err = pgx.ForEachRow(rows, []any{&kind, &n, &running, &joint}, func() error {
		c.ByKind[kind] = n
		c.Total += n
		c.Running += running
		c.Joint += joint
		return nil
	})
	if err != nil {
		return c, fmt.Errorf("count projects: %w", err)
	}
	return c, nil
}

// signupSeries counts registrations in the last 12 weeks (from Saturday, the Bangladesh week)
// and the last 6 months, the current period last.
func signupSeries(ctx context.Context, q querier, now time.Time) (weeks, months []Count, err error) {
	local := now.In(dhaka)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := day.AddDate(0, 0, -((int(day.Weekday()) + 1) % 7))
	monthStart := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	series := func(first time.Time, n int, step func(time.Time) time.Time) ([]Count, error) {
		rows, qErr := q.Query(ctx, `
			SELECT b.start::date::text, count(u.id)
			FROM unnest($1::date[], $2::date[]) AS b(start, finish)
			LEFT JOIN users u ON u.staff_role IS NULL AND (u.created_at AT TIME ZONE 'Asia/Dhaka')::date >= b.start
			                 AND (u.created_at AT TIME ZONE 'Asia/Dhaka')::date < b.finish
			GROUP BY b.start ORDER BY b.start`, starts(first, n, step), starts(step(first), n, step))
		if qErr != nil {
			return nil, fmt.Errorf("count signups: %w", qErr)
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[Count])
	}
	weekly := func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }
	monthly := func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	if weeks, err = series(weekStart.AddDate(0, 0, -7*11), 12, weekly); err != nil {
		return nil, nil, err
	}
	months, err = series(monthStart.AddDate(0, -5, 0), 6, monthly)
	return weeks, months, err
}

func starts(first time.Time, n int, step func(time.Time) time.Time) []time.Time {
	out := make([]time.Time, n)
	for i := range out {
		out[i] = first
		first = step(first)
	}
	return out
}

func recentSignups(ctx context.Context, q querier, limit int) ([]Signup, error) {
	rows, err := q.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, nullif(u.company_name, ''), u.phone, u.email IS NOT NULL, u.locale, u.created_at,
		       coalesce((SELECT array_agg(p.kind ORDER BY p.created_at) FROM projects p WHERE p.owner_id = u.id AND p.deleted_at IS NULL), '{}')
		FROM users u WHERE u.staff_role IS NULL ORDER BY u.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent signups: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Signup])
}

func attention(ctx context.Context, q querier, now time.Time) (Attention, error) {
	var a Attention
	err := q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM support_requests WHERE status = 'open'),
		       (SELECT min(created_at) FROM support_requests WHERE status = 'open'),
		       (SELECT count(*) FROM projects WHERE deleted_at IS NOT NULL AND deleted_at > $1 AND deleted_at <= $2),
		       (SELECT min(deleted_at) FROM projects WHERE deleted_at IS NOT NULL AND deleted_at > $1),
		       (SELECT count(*) FROM users WHERE status = 'suspended'),
		       (SELECT max(suspended_at) FROM users WHERE status = 'suspended'),
		       (SELECT count(*) FROM users WHERE staff_role IS NOT NULL),
		       (SELECT count(*) FROM users WHERE staff_role IS NOT NULL AND totp_enabled_at IS NULL)`,
		now.Add(-project.RestoreWindow), now.Add(-project.RestoreWindow+7*24*time.Hour)).Scan(
		&a.OpenSupport, &a.OldestSupport, &a.PurgeSoon, &a.FirstPurge, &a.Suspended, &a.LastSuspended, &a.Staff, &a.StaffWithout2FA)
	if err != nil {
		return Attention{}, fmt.Errorf("attention: %w", err)
	}
	if a.FirstPurge != nil {
		purge := a.FirstPurge.Add(project.RestoreWindow)
		a.FirstPurge = &purge
	}
	return a, nil
}
