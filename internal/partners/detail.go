package partners

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Entry is one line of a partner's ledger: something they owe (their commitment, a call's
// part) or a deposit. Lines come oldest first; on a day, what is owed comes before deposits.
type Entry struct {
	Kind      string    `json:"kind"` // DUE or PAID
	Date      time.Time `json:"date"`
	Amount    int64     `json:"amount"`
	CallID    *string   `json:"callId"`
	CallDate  *string   `json:"callDate"`
	Reason    *string   `json:"reason"`
	CallTotal *int64    `json:"callTotal"`
	Account   *string   `json:"account"`
	Note      *string   `json:"note"`
	DepositID *string   `json:"depositId"`
	ReceiptNo *string   `json:"receiptNo"`
}

// Detail is one partner's page.
type Detail struct {
	Partner        Partner `json:"partner"`
	TotalCommitted int64   `json:"totalCommitted"`
	ProjectPaid    int64   `json:"projectPaid"`
	Entries        []Entry `json:"entries"`
	Calls          []Call  `json:"calls"`
}

// Detail returns a partner's ledger with their calls; any member may see it.
func (s *Service) Detail(ctx context.Context, userID, projectID, partnerID string) (Detail, error) {
	if !isID(partnerID) {
		return Detail{}, notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Detail{}, err
	}
	p, err := s.partner(ctx, projectID, partnerID)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Partner: p}
	err = s.pool.QueryRow(ctx, `
		SELECT coalesce((SELECT sum(committed) FROM partners WHERE project_id = $1), 0)::bigint,
		       coalesce((SELECT sum(amount) FROM partner_deposits WHERE project_id = $1), 0)::bigint`, projectID).Scan(&d.TotalCommitted, &d.ProjectPaid)
	if err != nil {
		return Detail{}, fmt.Errorf("project totals: %w", err)
	}
	if d.Entries, err = s.entries(ctx, projectID, p); err != nil {
		return Detail{}, err
	}
	if d.Calls, err = callsOf(ctx, s.pool, projectID, partnerID); err != nil {
		return Detail{}, err
	}
	return d, nil
}

func (s *Service) entries(ctx context.Context, projectID string, p Partner) ([]Entry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT 'DUE', p.joined_on, p.committed, NULL::text, NULL::text, NULL::text, NULL::bigint, NULL::text, NULL::text, NULL::text, NULL::text, 0 AS ord, p.created_at
		FROM partners p WHERE p.id = $2
		UNION ALL
		SELECT 'DUE', c.date, cp.amount, c.id::text, to_char(c.date, 'YYYY-MM-DD'), c.reason, c.total, NULL, NULL, NULL, NULL, 0, c.created_at
		FROM capital_call_parts cp JOIN capital_calls c ON c.id = cp.call_id WHERE cp.partner_id = $2
		UNION ALL
		SELECT 'PAID', d.date, d.amount, c.id::text, to_char(c.date, 'YYYY-MM-DD'), c.reason, c.total, a.name, d.note, d.id::text,
		       pr.receipt_prefix || '-' || lpad(d.receipt_no::text, 6, '0'), 1, d.created_at
		FROM partner_deposits d
		JOIN money_accounts a ON a.id = d.account_id
		JOIN projects pr ON pr.id = d.project_id
		LEFT JOIN capital_calls c ON c.id = d.call_id
		WHERE d.partner_id = $2 AND d.project_id = $1
		ORDER BY 2, 12, 13`, projectID, p.ID)
	if err != nil {
		return nil, fmt.Errorf("partner ledger: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		var e Entry
		var ord int
		var created time.Time
		err := row.Scan(&e.Kind, &e.Date, &e.Amount, &e.CallID, &e.CallDate, &e.Reason, &e.CallTotal, &e.Account, &e.Note, &e.DepositID, &e.ReceiptNo, &ord, &created)
		return e, err
	})
}
