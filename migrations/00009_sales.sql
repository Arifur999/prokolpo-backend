-- +goose Up
-- What a project sells: flats, shops, parking, plots. Floor 1 is the ground floor.
CREATE TABLE units (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
  type        text NOT NULL CHECK (type IN ('FLAT', 'SHOP', 'PARKING', 'PLOT', 'OTHER')),
  floor       smallint NOT NULL CHECK (floor BETWEEN 0 AND 60),
  position    smallint NOT NULL,
  size        integer CHECK (size > 0),
  price       bigint NOT NULL CHECK (price > 0),
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX units_name_idx ON units (project_id, lower(name));

CREATE TABLE customers (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 60),
  phone       text NOT NULL,
  nid         text,
  address     text,
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, phone)
);

-- A unit sold to a customer: paid in full on the day, or a down payment and then N equal
-- installments every interval months from first_due (the last one takes the rounding).
CREATE TABLE sales (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  unit_id          uuid NOT NULL UNIQUE REFERENCES units (id),
  customer_id      uuid NOT NULL REFERENCES customers (id),
  date             date NOT NULL,
  price            bigint NOT NULL CHECK (price > 0),
  down             bigint NOT NULL CHECK (down >= 0 AND down <= price),
  installments     smallint NOT NULL CHECK (installments BETWEEN 0 AND 240),
  interval_months  smallint NOT NULL CHECK (interval_months IN (1, 3, 6)),
  first_due        date,
  installment      bigint NOT NULL CHECK (installment >= 0),
  created_by       uuid NOT NULL REFERENCES users (id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  CHECK ((installments = 0 AND down = price) OR (installments > 0 AND first_due > date AND down < price))
);

CREATE INDEX sales_customer_idx ON sales (customer_id);

-- Money a customer paid: the down payment on the sale day, then installments. Each takes
-- the project's next receipt number and goes into an account.
CREATE TABLE sale_payments (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  sale_id     uuid NOT NULL REFERENCES sales (id) ON DELETE CASCADE,
  account_id  uuid NOT NULL REFERENCES money_accounts (id),
  kind        text NOT NULL CHECK (kind IN ('DOWN', 'INSTALLMENT')),
  date        date NOT NULL,
  amount      bigint NOT NULL CHECK (amount > 0),
  note        text,
  receipt_no  integer NOT NULL,
  prints      integer NOT NULL DEFAULT 0,
  created_by  uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, receipt_no)
);

CREATE INDEX sale_payments_sale_idx ON sale_payments (sale_id, date);

-- +goose Down
DROP TABLE sale_payments;
DROP TABLE sales;
DROP TABLE customers;
DROP TABLE units;
