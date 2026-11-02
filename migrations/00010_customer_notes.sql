-- +goose Up
-- A second number (often family) and a free note, as the add-customer dialog has them.
ALTER TABLE customers ADD COLUMN alt_phone text, ADD COLUMN note text;

-- +goose Down
ALTER TABLE customers DROP COLUMN note, DROP COLUMN alt_phone;
