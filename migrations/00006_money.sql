-- +goose Up
-- The money books of a project. Money is whole taka (bigint). Entries soft-delete so
-- the owner or a manager can bring them back from the activity page.

-- Every change to the books, for the activity page and for partners to see.
CREATE TABLE activity_log (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  actor_id    uuid NOT NULL REFERENCES users (id),
  action      text NOT NULL CHECK (action IN ('CREATE', 'UPDATE', 'DELETE', 'RESTORE')),
  entity      text NOT NULL,
  entity_id   uuid NOT NULL,
  summary     text NOT NULL,
  changes     jsonb NOT NULL DEFAULT '[]',
  reason      text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX activity_log_project_idx ON activity_log (project_id, created_at DESC);

ALTER TABLE money_accounts ADD COLUMN number text;

CREATE TABLE suppliers (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id   uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name         text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 80),
  phone        text,
  address      text,
  supplies     text[] NOT NULL DEFAULT '{}',
  note         text,
  -- What was owed before the books moved here ("আগের জের").
  opening_due  bigint NOT NULL DEFAULT 0 CHECK (opening_due >= 0),
  created_by   uuid NOT NULL REFERENCES users (id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);

CREATE TRIGGER suppliers_set_updated_at BEFORE UPDATE ON suppliers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE contractors (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  phone            text,
  work             text NOT NULL,
  pay_type         text NOT NULL CHECK (pay_type IN ('CONTRACT', 'DAILY')),
  contract_amount  bigint CHECK (contract_amount > 0),
  scope            text,
  daily_wage       bigint CHECK (daily_wage > 0),
  team_size        smallint CHECK (team_size BETWEEN 1 AND 50),
  status           text NOT NULL DEFAULT 'NOT_STARTED' CHECK (status IN ('NOT_STARTED', 'WORKING', 'DONE')),
  opening_due      bigint NOT NULL DEFAULT 0 CHECK (opening_due >= 0),
  created_by       uuid NOT NULL REFERENCES users (id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name),
  CHECK ((pay_type = 'CONTRACT') = (contract_amount IS NOT NULL)),
  CHECK ((pay_type = 'DAILY') = (daily_wage IS NOT NULL AND team_size IS NOT NULL))
);

CREATE TRIGGER contractors_set_updated_at BEFORE UPDATE ON contractors
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Work outside a contract, agreed later ("অতিরিক্ত কাজ").
CREATE TABLE contractor_extras (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id     uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  contractor_id  uuid NOT NULL REFERENCES contractors (id) ON DELETE CASCADE,
  date           date NOT NULL,
  title          text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
  amount         bigint NOT NULL CHECK (amount > 0),
  created_by     uuid NOT NULL REFERENCES users (id),
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contractor_extras_contractor_idx ON contractor_extras (contractor_id);

-- How many of a daily team worked on a day. The wage is kept per day, so changing the
-- team's rate never rewrites weeks already worked.
CREATE TABLE attendance (
  project_id     uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  contractor_id  uuid NOT NULL REFERENCES contractors (id) ON DELETE CASCADE,
  day            date NOT NULL,
  headcount      smallint NOT NULL CHECK (headcount BETWEEN 1 AND 99),
  half_day       boolean NOT NULL DEFAULT false,
  rate           bigint NOT NULL CHECK (rate > 0),
  updated_by     uuid NOT NULL REFERENCES users (id),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (contractor_id, day)
);

CREATE INDEX attendance_project_day_idx ON attendance (project_id, day);

-- One bill: something bought or a piece of work, from a supplier, a contractor, or nobody.
CREATE TABLE expenses (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id     uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  category_id    uuid NOT NULL REFERENCES expense_categories (id),
  phase_id       uuid REFERENCES project_phases (id) ON DELETE SET NULL,
  supplier_id    uuid REFERENCES suppliers (id),
  contractor_id  uuid REFERENCES contractors (id),
  date           date NOT NULL,
  quantity       numeric(14, 3) CHECK (quantity > 0),
  unit           text,
  rate           bigint CHECK (rate >= 0),
  amount         bigint NOT NULL CHECK (amount > 0),
  description    text,
  memo_path      text,
  created_by     uuid NOT NULL REFERENCES users (id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  deleted_by     uuid REFERENCES users (id),
  CHECK (supplier_id IS NULL OR contractor_id IS NULL)
);

CREATE TRIGGER expenses_set_updated_at BEFORE UPDATE ON expenses
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX expenses_project_date_idx ON expenses (project_id, date DESC) WHERE deleted_at IS NULL;
CREATE INDEX expenses_supplier_idx ON expenses (supplier_id) WHERE deleted_at IS NULL;
CREATE INDEX expenses_contractor_idx ON expenses (contractor_id) WHERE deleted_at IS NULL;

-- Money paid out of an account: to a supplier, to a contractor, or for a bill paid on the spot.
CREATE TABLE payments (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id     uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  account_id     uuid NOT NULL REFERENCES money_accounts (id),
  supplier_id    uuid REFERENCES suppliers (id),
  contractor_id  uuid REFERENCES contractors (id),
  -- The bill it was for, if any. A bill paid in full on the spot sets paid_with_expense.
  expense_id         uuid REFERENCES expenses (id),
  paid_with_expense  boolean NOT NULL DEFAULT false,
  date           date NOT NULL,
  amount         bigint NOT NULL CHECK (amount > 0),
  purpose        text CHECK (purpose IN ('WAGE', 'ADVANCE', 'INSTALL', 'EXTRA', 'FINAL', 'OTHER')),
  note           text,
  created_by     uuid NOT NULL REFERENCES users (id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  deleted_at     timestamptz,
  deleted_by     uuid REFERENCES users (id),
  CHECK (supplier_id IS NULL OR contractor_id IS NULL),
  CHECK (NOT paid_with_expense OR expense_id IS NOT NULL)
);

CREATE INDEX payments_project_date_idx ON payments (project_id, date DESC) WHERE deleted_at IS NULL;
CREATE INDEX payments_supplier_idx ON payments (supplier_id) WHERE deleted_at IS NULL;
CREATE INDEX payments_contractor_idx ON payments (contractor_id) WHERE deleted_at IS NULL;
CREATE INDEX payments_account_idx ON payments (account_id) WHERE deleted_at IS NULL;

-- Moving money between two accounts of the same project. Not spending.
CREATE TABLE transfers (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  from_account_id  uuid NOT NULL REFERENCES money_accounts (id),
  to_account_id    uuid NOT NULL REFERENCES money_accounts (id),
  date             date NOT NULL,
  amount           bigint NOT NULL CHECK (amount > 0),
  note             text,
  created_by       uuid NOT NULL REFERENCES users (id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (from_account_id <> to_account_id)
);

CREATE INDEX transfers_project_idx ON transfers (project_id, date DESC);

-- What the project has spent: every bill, the wages of daily teams, extra work, and
-- what contract workers have been paid up to their contract. Contract money is counted
-- as it is paid, since an unpaid contract is a promise, not spending yet.
-- +goose StatementBegin
CREATE FUNCTION project_labour(p uuid) RETURNS bigint LANGUAGE sql STABLE AS $$
  SELECT coalesce((SELECT sum(headcount * rate / CASE WHEN half_day THEN 2 ELSE 1 END) FROM attendance WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(amount) FROM contractor_extras WHERE project_id = p), 0)::bigint
       + coalesce((SELECT sum(least(c.contract_amount, coalesce((
             SELECT sum(x.amount) FROM payments x WHERE x.contractor_id = c.id AND x.deleted_at IS NULL AND NOT x.paid_with_expense), 0)))
           FROM contractors c WHERE c.project_id = p AND c.pay_type = 'CONTRACT'), 0)::bigint
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION project_spent(p uuid) RETURNS bigint LANGUAGE sql STABLE AS $$
  SELECT coalesce((SELECT sum(amount) FROM expenses WHERE project_id = p AND deleted_at IS NULL), 0)::bigint + project_labour(p)
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION project_spent(uuid);
DROP FUNCTION project_labour(uuid);
DROP TABLE transfers;
DROP TABLE payments;
DROP TABLE expenses;
DROP TABLE attendance;
DROP TABLE contractor_extras;
DROP TABLE contractors;
DROP TABLE suppliers;
ALTER TABLE money_accounts DROP COLUMN number;
DROP TABLE activity_log;
