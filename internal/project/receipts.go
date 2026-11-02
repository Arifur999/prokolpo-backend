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

// ReceiptNo writes a receipt number the way it is printed: "PRK-000148".
func ReceiptNo(prefix string, n int) string { return fmt.Sprintf("%s-%06d", prefix, n) }
