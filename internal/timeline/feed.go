// Package timeline reads a project's activity log for the activity page.
package timeline

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/activity"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Members checks who may read a project's activity.
type Members interface {
	Member(ctx context.Context, userID, projectID string) (project.Membership, error)
}

// Service reads the activity log.
type Service struct {
	pool    *pgxpool.Pool
	members Members
	now     func() time.Time
}

// NewService returns a Service backed by pool.
func NewService(pool *pgxpool.Pool, members Members) *Service {
	return &Service{pool: pool, members: members, now: time.Now}
}

// Item is one entry with what the page needs to word it. The row an entry is about is
// joined in, so a bill's line shows its category, party and amount as they are now.
type Item struct {
	ID        string            `json:"id"`
	Action    string            `json:"action"`
	Entity    string            `json:"entity"`
	EntityID  string            `json:"entityId"`
	Summary   string            `json:"summary"`
	Changes   []activity.Change `json:"changes"`
	Reason    *string           `json:"reason"`
	Details   map[string]string `json:"details"`
	ActorID   string            `json:"actorId"`
	ActorName string            `json:"actorName"`
	CreatedAt time.Time         `json:"createdAt"`

	Title        *string `json:"title"`
	Quantity     *string `json:"quantity"`
	Unit         *string `json:"unit"`
	Amount       *int64  `json:"amount"`
	Party        *string `json:"party"`
	Account      *string `json:"account"`
	OtherAccount *string `json:"otherAccount"`
	Purpose      *string `json:"purpose"`
	Note         *string `json:"note"`
	Phase        *string `json:"phase"`
	PaidNow      bool    `json:"paidNow"`
	// Deleted is true while a deleted bill is still hidden, so the page offers to restore it.
	Deleted bool `json:"deleted"`
}

// Filter narrows the feed. Before pages back through older entries.
type Filter struct {
	ActorID string
	Action  string
	Since   *time.Time
	Before  *time.Time
	Limit   int
}

var actions = []string{"CREATE", "UPDATE", "DELETE", "RESTORE"}

// ValidAction reports whether a is an action the feed can filter on.
func ValidAction(a string) bool { return slices.Contains(actions, a) }

const feedSelect = `
	SELECT a.id, a.action, a.entity, a.entity_id, a.summary, a.changes, a.reason, a.details,
	       a.actor_id, u.first_name || ' ' || u.last_name, a.created_at,
	       coalesce(ec.name, x.title),
	       CASE WHEN e.quantity IS NULL THEN NULL ELSE trim_scale(e.quantity)::text END, e.unit,
	       coalesce(e.amount, p.amount, t.amount, x.amount),
	       coalesce(es.name, ecn.name, ps.name, pc.name, xc.name, ac.name),
	       coalesce(pa.name, ta.name), tb.name,
	       p.purpose, coalesce(p.note, t.note, e.description), ph.name,
	       EXISTS (SELECT 1 FROM payments pw WHERE pw.expense_id = e.id AND pw.paid_with_expense AND pw.deleted_at IS NULL),
	       a.entity = 'expense' AND a.action = 'DELETE' AND e.deleted_at IS NOT NULL
	FROM activity_log a
	JOIN users u ON u.id = a.actor_id
	LEFT JOIN expenses e ON a.entity = 'expense' AND e.id = a.entity_id
	LEFT JOIN expense_categories ec ON ec.id = e.category_id
	LEFT JOIN suppliers es ON es.id = e.supplier_id
	LEFT JOIN contractors ecn ON ecn.id = e.contractor_id
	LEFT JOIN project_phases ph ON ph.id = e.phase_id
	LEFT JOIN payments p ON a.entity = 'payment' AND p.id = a.entity_id
	LEFT JOIN suppliers ps ON ps.id = p.supplier_id
	LEFT JOIN contractors pc ON pc.id = p.contractor_id
	LEFT JOIN money_accounts pa ON pa.id = p.account_id
	LEFT JOIN transfers t ON a.entity = 'transfer' AND t.id = a.entity_id
	LEFT JOIN money_accounts ta ON ta.id = t.from_account_id
	LEFT JOIN money_accounts tb ON tb.id = t.to_account_id
	LEFT JOIN contractor_extras x ON a.entity = 'extra' AND x.id = a.entity_id
	LEFT JOIN contractors xc ON xc.id = x.contractor_id
	LEFT JOIN contractors ac ON a.entity = 'attendance' AND ac.id = a.entity_id
	WHERE a.project_id = $1
	  AND ($2::uuid IS NULL OR a.actor_id = $2)
	  AND ($3::text IS NULL OR a.action = $3 OR ($3 = 'CREATE' AND a.action = 'RESTORE'))
	  AND ($4::timestamptz IS NULL OR a.created_at >= $4)
	  AND ($5::timestamptz IS NULL OR a.created_at < $5)
	ORDER BY a.created_at DESC, a.id DESC
	LIMIT $6`

// Feed lists the project's activity, newest first.
func (s *Service) Feed(ctx context.Context, userID, projectID string, f Filter) ([]Item, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, err
	}
	var actor, action *string
	if f.ActorID != "" {
		actor = &f.ActorID
	}
	if f.Action != "" {
		action = &f.Action
	}
	rows, err := s.pool.Query(ctx, feedSelect, projectID, actor, action, f.Since, f.Before, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		err := row.Scan(&it.ID, &it.Action, &it.Entity, &it.EntityID, &it.Summary, &it.Changes, &it.Reason, &it.Details,
			&it.ActorID, &it.ActorName, &it.CreatedAt,
			&it.Title, &it.Quantity, &it.Unit, &it.Amount, &it.Party, &it.Account, &it.OtherAccount,
			&it.Purpose, &it.Note, &it.Phase, &it.PaidNow, &it.Deleted)
		return it, err
	})
}

// Count is how many entries one person made in a period.
type Count struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// Week counts each member's entries from Sunday to today in Bangladesh, the busiest
// first; members who made none are listed with zero.
func (s *Service) Week(ctx context.Context, userID, projectID string) ([]Count, time.Time, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, time.Time{}, err
	}
	start := weekStart(s.now())
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.first_name || ' ' || u.last_name, count(a.id)::int
		FROM project_members m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN activity_log a ON a.project_id = m.project_id AND a.actor_id = m.user_id AND a.created_at >= $2
		WHERE m.project_id = $1
		GROUP BY u.id, u.first_name, u.last_name, m.created_at
		ORDER BY count(a.id) DESC, m.created_at`, projectID, start)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("count week: %w", err)
	}
	counts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Count])
	return counts, start, err
}

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

// weekStart is the start of Sunday in Bangladesh for the week holding t.
func weekStart(t time.Time) time.Time {
	local := t.In(dhaka)
	y, m, d := local.Date()
	return time.Date(y, m, d-int(local.Weekday()), 0, 0, 0, 0, dhaka)
}

// isID reports whether s can be a row id.
func isID(s string) bool {
	var id pgtype.UUID
	return id.Scan(s) == nil
}
