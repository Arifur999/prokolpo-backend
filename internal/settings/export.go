package settings

import (
	"context"
	"fmt"
)

// Sheet is one tab of the backup workbook ("সব ডেটা ডাউনলোড"). Key and Columns are message
// keys the page words in the reader's language; money stays a number.
type Sheet struct {
	Key     string   `json:"key"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Each sheet is a query of the project's ($1) live entries, oldest first. Every value is
// text except the trailing money columns.
var sheets = []struct {
	key     string
	columns []string
	sql     string
}{
	{"expenses", []string{"date", "category", "phase", "party", "description", "quantity", "amount"}, `
		SELECT to_char(e.date, 'DD/MM/YYYY'), c.name, coalesce(ph.name, ''), coalesce(s.name, k.name, ''),
		       coalesce(e.description, ''), coalesce(trim(trailing '.' from trim(trailing '0' from e.quantity::text)) || ' ' || coalesce(e.unit, ''), ''), e.amount
		FROM expenses e JOIN expense_categories c ON c.id = e.category_id
		LEFT JOIN project_phases ph ON ph.id = e.phase_id
		LEFT JOIN suppliers s ON s.id = e.supplier_id LEFT JOIN contractors k ON k.id = e.contractor_id
		WHERE e.project_id = $1 AND e.deleted_at IS NULL ORDER BY e.date, e.created_at`},
	{"payments", []string{"date", "party", "account", "note", "amount"}, `
		SELECT to_char(p.date, 'DD/MM/YYYY'), coalesce(s.name, k.name, ''), a.name, coalesce(p.note, ''), p.amount
		FROM payments p JOIN money_accounts a ON a.id = p.account_id
		LEFT JOIN suppliers s ON s.id = p.supplier_id LEFT JOIN contractors k ON k.id = p.contractor_id
		WHERE p.project_id = $1 AND p.deleted_at IS NULL ORDER BY p.date, p.created_at`},
	{"sales", []string{"date", "receipt", "customer", "unit", "account", "amount"}, `
		SELECT to_char(sp.date, 'DD/MM/YYYY'), pr.receipt_prefix || '-' || lpad(sp.receipt_no::text, 6, '0'), c.name, u.name, a.name, sp.amount
		FROM sale_payments sp JOIN sales sa ON sa.id = sp.sale_id JOIN customers c ON c.id = sa.customer_id
		JOIN units u ON u.id = sa.unit_id JOIN money_accounts a ON a.id = sp.account_id JOIN projects pr ON pr.id = sp.project_id
		WHERE sp.project_id = $1 ORDER BY sp.date, sp.receipt_no`},
	{"deposits", []string{"date", "receipt", "partner", "account", "amount"}, `
		SELECT to_char(d.date, 'DD/MM/YYYY'), pr.receipt_prefix || '-' || lpad(d.receipt_no::text, 6, '0'), pa.name, a.name, d.amount
		FROM partner_deposits d JOIN partners pa ON pa.id = d.partner_id JOIN money_accounts a ON a.id = d.account_id
		JOIN projects pr ON pr.id = d.project_id
		WHERE d.project_id = $1 ORDER BY d.date, d.receipt_no`},
	{"produce", []string{"date", "product", "quantity", "party", "phase", "amount"}, `
		SELECT to_char(x.date, 'DD/MM/YYYY'), x.product, trim(trailing '.' from trim(trailing '0' from x.quantity::text)) || ' ' || x.unit,
		       b.name, coalesce(ph.name, ''), x.amount
		FROM produce_sales x JOIN produce_buyers b ON b.id = x.buyer_id LEFT JOIN project_phases ph ON ph.id = x.phase_id
		WHERE x.project_id = $1 ORDER BY x.date, x.created_at`},
	{"shop", []string{"date", "note", "cash", "mobile", "other"}, `
		SELECT to_char(d.date, 'DD/MM/YYYY'), coalesce(d.note, ''), d.cash, d.mobile, d.other
		FROM shop_days d WHERE d.project_id = $1 ORDER BY d.date`},
	{"transfers", []string{"date", "from", "to", "note", "amount"}, `
		SELECT to_char(t.date, 'DD/MM/YYYY'), f.name, o.name, coalesce(t.note, ''), t.amount
		FROM transfers t JOIN money_accounts f ON f.id = t.from_account_id JOIN money_accounts o ON o.id = t.to_account_id
		WHERE t.project_id = $1 ORDER BY t.date, t.created_at`},
}

// Export reads every entry of the project for the owner's backup, one sheet per book. Sheets
// with no rows are left out.
func (s *Service) Export(ctx context.Context, userID, projectID string) ([]Sheet, error) {
	if _, err := s.members.Owner(ctx, userID, projectID); err != nil {
		return nil, err
	}
	out := []Sheet{}
	for _, def := range sheets {
		rows, err := s.pool.Query(ctx, def.sql, projectID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", def.key, err)
		}
		sheet := Sheet{Key: def.key, Columns: def.columns, Rows: [][]any{}}
		for rows.Next() {
			values, valuesErr := rows.Values()
			if valuesErr != nil {
				rows.Close()
				return nil, fmt.Errorf("export %s: %w", def.key, valuesErr)
			}
			sheet.Rows = append(sheet.Rows, values)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("export %s: %w", def.key, err)
		}
		if len(sheet.Rows) > 0 {
			out = append(out, sheet)
		}
	}
	return out, nil
}
