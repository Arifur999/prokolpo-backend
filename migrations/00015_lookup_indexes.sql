-- +goose Up
-- Columns the list screens look up once per row (a bill's payment, a sale's receipts, a
-- buyer's sales, a batch's sales and bills, a day's credit lines).
CREATE INDEX payments_expense_idx ON payments (expense_id) WHERE expense_id IS NOT NULL;
CREATE INDEX produce_receipts_sale_idx ON produce_receipts (sale_id);
CREATE INDEX produce_sales_buyer_idx ON produce_sales (buyer_id);
CREATE INDEX produce_sales_phase_idx ON produce_sales (phase_id);
CREATE INDEX expenses_phase_idx ON expenses (phase_id) WHERE deleted_at IS NULL;
CREATE INDEX khata_entries_day_idx ON khata_entries (day_id) WHERE day_id IS NOT NULL;

-- +goose Down
DROP INDEX khata_entries_day_idx;
DROP INDEX expenses_phase_idx;
DROP INDEX produce_sales_phase_idx;
DROP INDEX produce_sales_buyer_idx;
DROP INDEX produce_receipts_sale_idx;
DROP INDEX payments_expense_idx;
