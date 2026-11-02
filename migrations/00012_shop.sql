-- +goose Up
-- A shop's day book: one entry per day when the shop closes, with what was sold for
-- cash, by bKash and on credit, and the credit book ("বাকির খাতা") of who owes what.
CREATE TABLE shop_days (
  id                 uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id         uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  date               date NOT NULL,
  cash               bigint NOT NULL DEFAULT 0 CHECK (cash >= 0),
  mobile             bigint NOT NULL DEFAULT 0 CHECK (mobile >= 0),
  other              bigint NOT NULL DEFAULT 0 CHECK (other >= 0),
  note               text,
  -- What the drawer held when counted, if it was.
  counted            bigint CHECK (counted >= 0),
  cash_account_id    uuid NOT NULL REFERENCES money_accounts (id),
  mobile_account_id  uuid NOT NULL REFERENCES money_accounts (id),
  created_by         uuid NOT NULL REFERENCES users (id),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, date)
);

CREATE TRIGGER shop_days_set_updated_at BEFORE UPDATE ON shop_days
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE khata_people (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX khata_people_name_idx ON khata_people (project_id, (lower(name)));

-- Goods taken on credit, or credit paid back. Lines written in a day's entry belong to
-- that day and are replaced when the day is edited; a payment taken on its own has no day.
CREATE TABLE khata_entries (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  person_id   uuid NOT NULL REFERENCES khata_people (id),
  day_id      uuid REFERENCES shop_days (id) ON DELETE CASCADE,
  kind        text NOT NULL CHECK (kind IN ('CREDIT', 'PAYMENT')),
  date        date NOT NULL,
  amount      bigint NOT NULL CHECK (amount > 0),
  account_id  uuid REFERENCES money_accounts (id),
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((kind = 'PAYMENT') = (account_id IS NOT NULL))
);

CREATE INDEX khata_entries_person_idx ON khata_entries (person_id, date);
CREATE INDEX khata_entries_project_date_idx ON khata_entries (project_id, date);

-- +goose Down
DROP TABLE khata_entries;
DROP TABLE khata_people;
DROP TABLE shop_days;
