-- +goose Up
-- What a farm sells: birds, eggs, litter, milk, fish, crops. A batch is one of the
-- project's phases; its dates and head count let the sales page show profit per batch.
ALTER TABLE project_phases
  ADD COLUMN started_on  date,
  ADD COLUMN ended_on    date,
  ADD COLUMN head_count  integer CHECK (head_count > 0),
  ADD COLUMN losses      integer NOT NULL DEFAULT 0 CHECK (losses >= 0),
  ADD COLUMN note        text,
  ADD CHECK (ended_on IS NULL OR started_on IS NULL OR ended_on >= started_on);

-- Wholesalers and shops that buy from the farm, by name.
CREATE TABLE produce_buyers (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX produce_buyers_name_idx ON produce_buyers (project_id, (lower(name)));

-- One sale: quantity × rate, rounded to whole taka.
CREATE TABLE produce_sales (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  phase_id    uuid REFERENCES project_phases (id) ON DELETE SET NULL,
  buyer_id    uuid NOT NULL REFERENCES produce_buyers (id),
  date        date NOT NULL,
  product     text NOT NULL,
  quantity    numeric(14, 3) NOT NULL CHECK (quantity > 0),
  unit        text NOT NULL,
  rate        bigint NOT NULL CHECK (rate > 0),
  amount      bigint NOT NULL CHECK (amount > 0),
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX produce_sales_project_idx ON produce_sales (project_id, date);

-- Money a buyer paid: on the sale itself (sale_id set) or later against what they owe.
CREATE TABLE produce_receipts (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  buyer_id    uuid NOT NULL REFERENCES produce_buyers (id),
  sale_id     uuid REFERENCES produce_sales (id) ON DELETE CASCADE,
  account_id  uuid NOT NULL REFERENCES money_accounts (id),
  date        date NOT NULL,
  amount      bigint NOT NULL CHECK (amount > 0),
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX produce_receipts_buyer_idx ON produce_receipts (buyer_id);

-- +goose Down
DROP TABLE produce_receipts;
DROP TABLE produce_sales;
DROP TABLE produce_buyers;
ALTER TABLE project_phases
  DROP COLUMN started_on, DROP COLUMN ended_on, DROP COLUMN head_count, DROP COLUMN losses, DROP COLUMN note;
