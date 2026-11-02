-- +goose Up
-- The owner's settings page (board Settings): heads can be hidden from the expense form and
-- carry a budget; receipts print the project's own name, address, phone and footer in the
-- chosen language and paper; and the owner can let platform support look in for a while.
ALTER TABLE expense_categories
  ADD COLUMN hidden boolean NOT NULL DEFAULT false,
  ADD COLUMN budget bigint CHECK (budget >= 0);

ALTER TABLE projects
  ADD COLUMN receipt_company text CHECK (char_length(receipt_company) <= 80),
  ADD COLUMN receipt_address text CHECK (char_length(receipt_address) <= 160),
  ADD COLUMN receipt_phone text CHECK (char_length(receipt_phone) <= 30),
  ADD COLUMN receipt_lang text NOT NULL DEFAULT 'bn' CHECK (receipt_lang IN ('bn', 'en', 'both')),
  ADD COLUMN receipt_paper text NOT NULL DEFAULT 'A4' CHECK (receipt_paper IN ('A4', 'A5')),
  ADD COLUMN receipt_footer text CHECK (char_length(receipt_footer) <= 160);

-- A window in which support may read (never change) the project. Ending it early sets
-- revoked_at; the newest row is the one that counts.
CREATE TABLE support_grants (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id      uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  granted_by      uuid NOT NULL REFERENCES users (id),
  starts_at       timestamptz NOT NULL DEFAULT now(),
  ends_at         timestamptz NOT NULL,
  revoked_at      timestamptz,
  last_opened_by  uuid REFERENCES users (id),
  last_opened_at  timestamptz,
  CHECK (ends_at > starts_at)
);

CREATE INDEX support_grants_project_idx ON support_grants (project_id, starts_at DESC);

-- +goose Down
DROP TABLE support_grants;
ALTER TABLE projects
  DROP COLUMN receipt_company, DROP COLUMN receipt_address, DROP COLUMN receipt_phone,
  DROP COLUMN receipt_lang, DROP COLUMN receipt_paper, DROP COLUMN receipt_footer;
ALTER TABLE expense_categories DROP COLUMN hidden, DROP COLUMN budget;
