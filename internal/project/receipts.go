package project

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// NextReceipt takes the project's next receipt number inside tx. Numbers are never reused:
// a deleted entry keeps its receipt, marked cancelled.
func NextReceipt(ctx context.Context, tx pgx.Tx, projectID string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `UPDATE projects SET receipt_seq = receipt_seq + 1 WHERE id = $1 RETURNING receipt_seq`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("next receipt: %w", err)
	}
	return n, nil
}

// Print is how the owner set receipts up in settings ("রশিদ ও প্রিন্ট"): the language
// (bn, en or both), the usual paper and a line printed at the bottom.
type Print struct {
	Lang   string  `json:"lang"`
	Paper  string  `json:"paper"`
	Footer *string `json:"footer"`
}

// ReceiptHeadingSQL is the receipt heading columns, for a query that joins projects as pr and the owner as o: the
// name, address and phone a receipt prints, then its Print settings.
const ReceiptHeadingSQL = `coalesce(pr.receipt_company, nullif(o.company_name, ''), o.first_name || ' ' || o.last_name),
	coalesce(pr.receipt_address, pr.location), coalesce(pr.receipt_phone, o.phone),
	pr.receipt_lang, pr.receipt_paper, pr.receipt_footer`

// ReceiptNo writes a receipt number the way it is printed: "PRK-000148".
func ReceiptNo(prefix string, n int) string { return fmt.Sprintf("%s-%06d", prefix, n) }
