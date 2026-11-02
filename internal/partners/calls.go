package partners

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/activity"
	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

const maxAmount = 1_000_000_000_000

// Call is one capital call with each partner's fixed part and what they have paid toward it.
type Call struct {
	ID      string     `json:"id"`
	Date    time.Time  `json:"date"`
	DueDate time.Time  `json:"dueDate"`
	Reason  string     `json:"reason"`
	Total   int64      `json:"total"`
	Parts   []CallPart `json:"parts"`
}

// CallPart is one partner's share of a call.
type CallPart struct {
	PartnerID string  `json:"partnerId"`
	Name      string  `json:"name"`
	Color     int     `json:"color"`
	Share     float64 `json:"share"`
	Amount    int64   `json:"amount"`
	Paid      int64   `json:"paid"`
}

// callsOf lists the project's calls, newest first; partnerID, when set, keeps only the
// calls that partner has a part in.
func callsOf(ctx context.Context, q db, projectID, partnerID string) ([]Call, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, c.date, c.due_date, c.reason, c.total FROM capital_calls c
		WHERE c.project_id = $1 AND ($2 = '' OR EXISTS (SELECT 1 FROM capital_call_parts cp WHERE cp.call_id = c.id AND cp.partner_id::text = $2))
		ORDER BY c.date DESC, c.created_at DESC`, projectID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("list calls: %w", err)
	}
	calls, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Call, error) {
		c := Call{Parts: []CallPart{}}
		scanErr := row.Scan(&c.ID, &c.Date, &c.DueDate, &c.Reason, &c.Total)
		return c, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("list calls: %w", err)
	}
	byCall := map[string]*Call{}
	for i := range calls {
		byCall[calls[i].ID] = &calls[i]
	}
	return calls, attachParts(ctx, q, projectID, byCall)
}

func attachParts(ctx context.Context, q db, projectID string, byCall map[string]*Call) error {
	rows, err := q.Query(ctx, `
		SELECT cp.call_id, cp.partner_id, p.name, p.color, cp.share::float8, cp.amount,
		       coalesce((SELECT sum(d.amount) FROM partner_deposits d WHERE d.call_id = cp.call_id AND d.partner_id = cp.partner_id), 0)::bigint
		FROM capital_call_parts cp JOIN partners p ON p.id = cp.partner_id
		WHERE p.project_id = $1 ORDER BY p.color`, projectID)
	if err != nil {
		return fmt.Errorf("list call parts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var callID string
		var part CallPart
		if err := rows.Scan(&callID, &part.PartnerID, &part.Name, &part.Color, &part.Share, &part.Amount, &part.Paid); err != nil {
			return fmt.Errorf("read call part: %w", err)
		}
		if c, ok := byCall[callID]; ok {
			c.Parts = append(c.Parts, part)
		}
	}
	return rows.Err()
}

// splitCall divides total by shares (in tenths of a percent): each part is rounded down
// and what is left goes to the biggest share, so the parts always add up to the total.
func splitCall(total int64, tenths []int64) []int64 {
	parts := make([]int64, len(tenths))
	var sum int64
	biggest := 0
	for i, t := range tenths {
		parts[i] = total * t / 1000
		sum += parts[i]
		if t > tenths[biggest] {
			biggest = i
		}
	}
	if len(parts) > 0 {
		parts[biggest] += total - sum
	}
	return parts
}

// toTenths turns a share like 12.5 into 125.
func toTenths(share float64) int64 { return int64(math.Round(share * 10)) }

type callBody struct {
	Total  int64  `json:"total"`
	Reason string `json:"reason"`
}

// NewCall is a validated capital call.
type NewCall struct {
	Total  int64
	Reason string
}

func (b callBody) validate(lang string) (NewCall, error) {
	reason := strings.TrimSpace(b.Reason)
	switch {
	case b.Total <= 0 || b.Total > maxAmount:
		return NewCall{}, invalid(lang, "total", "amount")
	case reason == "" || len([]rune(reason)) > 200:
		return NewCall{}, invalid(lang, "reason", "reason")
	}
	return NewCall{Total: b.Total, Reason: reason}, nil
}

// CreateCall asks every partner for their share of a sum, due by the end of the month.
func (s *Service) CreateCall(ctx context.Context, userID, projectID string, in NewCall) (Call, error) {
	today := s.today()
	due := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	var id string
	err := s.joint(ctx, func() error { _, err := s.access.Owner(ctx, userID, projectID); return err }, projectID, func(tx pgx.Tx) error {
		partners, err := partnersOf(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(partners) == 0 {
			return messages.Fail(apperr.KindInvalid, codeNoPartners)
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO capital_calls (project_id, date, due_date, reason, total, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, projectID, today, due, in.Reason, in.Total, userID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert call: %w", err)
		}
		tenths := make([]int64, len(partners))
		for i := range partners {
			tenths[i] = toTenths(partners[i].Share)
		}
		for i, amount := range splitCall(in.Total, tenths) {
			if _, err := tx.Exec(ctx, `INSERT INTO capital_call_parts (call_id, partner_id, share, amount) VALUES ($1, $2, $3, $4)`,
				id, partners[i].ID, partners[i].Share, amount); err != nil {
				return fmt.Errorf("insert call part: %w", err)
			}
		}
		return activity.Log(ctx, tx, activity.Entry{
			ProjectID: projectID, ActorID: userID, Action: "CREATE", Entity: "call", EntityID: id, Summary: in.Reason,
			Details: map[string]string{"total": strconv.FormatInt(in.Total, 10)},
		})
	})
	if err != nil {
		return Call{}, err
	}
	calls, err := callsOf(ctx, s.pool, projectID, "")
	if err != nil {
		return Call{}, err
	}
	for _, c := range calls {
		if c.ID == id {
			return c, nil
		}
	}
	return Call{}, notFound()
}
