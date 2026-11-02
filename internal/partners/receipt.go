package partners

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// Receipt is everything a printed partner receipt shows (board ReceiptPartner).
type Receipt struct {
	DepositID   string         `json:"depositId"`
	No          string         `json:"no"`
	Date        time.Time      `json:"date"`
	Amount      int64          `json:"amount"`
	PartnerID   string         `json:"partnerId"`
	Payer       string         `json:"payer"`
	PayerPhone  string         `json:"payerPhone"`
	Share       float64        `json:"share"`
	Project     string         `json:"project"`
	Location    *string        `json:"location"`
	Company     string         `json:"company"`
	Print       project.Print  `json:"print"`
	OwnerPhone  string         `json:"ownerPhone"`
	CallDate    *string        `json:"callDate"`
	Reason      *string        `json:"reason"`
	CallTotal   *int64         `json:"callTotal"`
	TotalBase   int64          `json:"totalBase"`
	Account     string         `json:"account"`
	AccountKind string         `json:"accountKind"`
	Note        *string        `json:"note"`
	TakenBy     string         `json:"takenBy"`
	TakenByRole *string        `json:"takenByRole"`
	WrittenAt   time.Time      `json:"writtenAt"`
	Prints      int            `json:"prints"`
	Should      int64          `json:"should"`
	PaidSoFar   int64          `json:"paidSoFar"`
	Others      []ReceiptBrief `json:"others"`
}

// ReceiptBrief is one line of "all this partner's receipts".
type ReceiptBrief struct {
	DepositID string    `json:"depositId"`
	No        string    `json:"no"`
	Date      time.Time `json:"date"`
	Amount    int64     `json:"amount"`
	CallDate  *string   `json:"callDate"`
}

// Receipt returns a deposit's receipt; what was owed and paid is as of that receipt.
func (s *Service) Receipt(ctx context.Context, userID, projectID, depositID string) (Receipt, error) {
	if !isID(depositID) {
		return Receipt{}, notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return Receipt{}, err
	}
	var r Receipt
	err := s.pool.QueryRow(ctx, `
		SELECT d.id, pr.receipt_prefix || '-' || lpad(d.receipt_no::text, 6, '0'), d.date, d.amount,
		       p.id, p.name, p.phone, p.share::float8, pr.name,
		       `+project.ReceiptHeadingSQL+`,
		       to_char(c.date, 'YYYY-MM-DD'), c.reason, c.total,
		       coalesce(c.total, (SELECT sum(committed) FROM partners WHERE project_id = pr.id))::bigint,
		       a.name, a.kind, d.note, t.first_name || ' ' || t.last_name,
		       (SELECT m.role FROM project_members m WHERE m.project_id = pr.id AND m.user_id = t.id),
		       d.created_at, d.prints,
		       p.committed + coalesce((SELECT sum(cp.amount) FROM capital_call_parts cp JOIN capital_calls cc ON cc.id = cp.call_id
		                               WHERE cp.partner_id = p.id AND cc.date <= d.date), 0)::bigint,
		       (SELECT sum(x.amount) FROM partner_deposits x WHERE x.partner_id = p.id AND x.receipt_no <= d.receipt_no)::bigint
		FROM partner_deposits d
		JOIN partners p ON p.id = d.partner_id
		JOIN projects pr ON pr.id = d.project_id
		JOIN users o ON o.id = pr.owner_id
		JOIN users t ON t.id = d.created_by
		JOIN money_accounts a ON a.id = d.account_id
		LEFT JOIN capital_calls c ON c.id = d.call_id
		WHERE d.id = $1 AND d.project_id = $2`, depositID, projectID).Scan(
		&r.DepositID, &r.No, &r.Date, &r.Amount, &r.PartnerID, &r.Payer, &r.PayerPhone, &r.Share, &r.Project,
		&r.Company, &r.Location, &r.OwnerPhone, &r.Print.Lang, &r.Print.Paper, &r.Print.Footer, &r.CallDate, &r.Reason, &r.CallTotal, &r.TotalBase, &r.Account, &r.AccountKind, &r.Note,
		&r.TakenBy, &r.TakenByRole, &r.WrittenAt, &r.Prints, &r.Should, &r.PaidSoFar)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, notFound()
	}
	if err != nil {
		return Receipt{}, fmt.Errorf("load receipt: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, pr.receipt_prefix || '-' || lpad(d.receipt_no::text, 6, '0'), d.date, d.amount, to_char(c.date, 'YYYY-MM-DD')
		FROM partner_deposits d JOIN projects pr ON pr.id = d.project_id LEFT JOIN capital_calls c ON c.id = d.call_id
		WHERE d.partner_id = $1 ORDER BY d.receipt_no DESC`, r.PartnerID)
	if err != nil {
		return Receipt{}, fmt.Errorf("list receipts: %w", err)
	}
	if r.Others, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ReceiptBrief]); err != nil {
		return Receipt{}, fmt.Errorf("list receipts: %w", err)
	}
	return r, nil
}

// Printed counts a print of a deposit's receipt.
func (s *Service) Printed(ctx context.Context, userID, projectID, depositID string) error {
	if !isID(depositID) {
		return notFound()
	}
	if _, err := s.access.Member(ctx, userID, projectID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE partner_deposits SET prints = prints + 1 WHERE id = $1 AND project_id = $2`, depositID, projectID)
	if err != nil {
		return fmt.Errorf("count print: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notFound()
	}
	return nil
}
