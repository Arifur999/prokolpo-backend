-- +goose Up
-- Receipt numbers ("PRK-000148") run per project across partner deposits and sale payments.
ALTER TABLE projects
  ADD COLUMN receipt_prefix text NOT NULL DEFAULT 'PRK' CHECK (receipt_prefix ~ '^[A-Z]{2,6}$'),
  ADD COLUMN receipt_seq integer NOT NULL DEFAULT 0;

-- The partners of a joint project. Shares always add up to 100; a new partner's share
-- comes out of the others'. The phone links them to an account and the team, if any.
CREATE TABLE partners (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 60),
  phone       text NOT NULL,
  share       numeric(4, 1) NOT NULL CHECK (share > 0 AND share <= 100),
  committed   bigint NOT NULL CHECK (committed >= 0),
  joined_on   date NOT NULL,
  -- Position in the partner palette, fixed so a partner keeps their colour.
  color       smallint NOT NULL,
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, phone)
);

-- Extra money asked of every partner by share. Each partner's part is fixed when the call
-- is made, so later share changes leave it alone.
CREATE TABLE capital_calls (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  date        date NOT NULL,
  due_date    date NOT NULL,
  reason      text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 200),
  total       bigint NOT NULL CHECK (total > 0),
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE capital_call_parts (
  call_id     uuid NOT NULL REFERENCES capital_calls (id) ON DELETE CASCADE,
  partner_id  uuid NOT NULL REFERENCES partners (id) ON DELETE CASCADE,
  share       numeric(4, 1) NOT NULL,
  amount      bigint NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (call_id, partner_id)
);

-- Money a partner put in, toward their commitment or a call, into one of the accounts.
CREATE TABLE partner_deposits (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  partner_id  uuid NOT NULL REFERENCES partners (id) ON DELETE CASCADE,
  call_id     uuid REFERENCES capital_calls (id),
  account_id  uuid NOT NULL REFERENCES money_accounts (id),
  date        date NOT NULL,
  amount      bigint NOT NULL CHECK (amount > 0),
  note        text,
  receipt_no  integer NOT NULL,
  prints      integer NOT NULL DEFAULT 0,
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, receipt_no)
);

CREATE INDEX partner_deposits_partner_idx ON partner_deposits (partner_id, date);

-- +goose Down
DROP TABLE partner_deposits;
DROP TABLE capital_call_parts;
DROP TABLE capital_calls;
DROP TABLE partners;
ALTER TABLE projects DROP COLUMN receipt_seq, DROP COLUMN receipt_prefix;
