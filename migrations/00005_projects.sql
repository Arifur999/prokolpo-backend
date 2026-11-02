-- +goose Up
-- A project is one set of books: a building, a farm, a shop. Money columns are
-- whole taka (bigint); nothing in the app deals in paisa.
CREATE TABLE projects (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  owner_id       uuid NOT NULL REFERENCES users (id),
  name           text NOT NULL CHECK (char_length(name) BETWEEN 3 AND 80),
  kind           text NOT NULL CHECK (kind IN ('BUILDING', 'FARM', 'LAND', 'BUSINESS', 'RENOVATION', 'OTHER')),
  -- Fixed at creation: the joint-project menus and partner books depend on it.
  ownership      text NOT NULL CHECK (ownership IN ('PERSONAL', 'PARTNERSHIP')),
  status         text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ON_HOLD', 'DONE')),
  location       text,
  description    text,
  start_date     date NOT NULL,
  budget         bigint CHECK (budget >= 0),
  -- Kind-specific details; NULL where they do not apply.
  farm_type      text CHECK (farm_type IN ('POULTRY', 'CATTLE', 'FISH', 'CROP')),
  floors         smallint CHECK (floors BETWEEN 0 AND 30),
  batches        smallint CHECK (batches BETWEEN 0 AND 20),
  plots          smallint CHECK (plots BETWEEN 0 AND 50),
  land_area      numeric(12, 2) CHECK (land_area >= 0),
  land_unit      text CHECK (land_unit IN ('SHOTANGSHO', 'KATHA', 'BIGHA')),
  business_type  text,
  -- Soft delete: the owner can restore for 30 days, then a job purges it.
  deleted_at     timestamptz,
  deleted_by     uuid REFERENCES users (id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  CHECK ((kind = 'FARM') = (farm_type IS NOT NULL))
);

CREATE TRIGGER projects_set_updated_at BEFORE UPDATE ON projects
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX projects_owner_idx ON projects (owner_id) WHERE deleted_at IS NULL;

-- Who can open a project and what they may do there. The owner is a member too.
CREATE TABLE project_members (
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role        text NOT NULL CHECK (role IN ('OWNER', 'MANAGER', 'VIEWER')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, user_id)
);

CREATE INDEX project_members_user_idx ON project_members (user_id);

-- Expense heads ("রড", "সিমেন্ট"), seeded from the project kind and editable later.
CREATE TABLE expense_categories (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
  position    smallint NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);

-- Stages of the work (floors, batches); each may carry its own budget.
CREATE TABLE project_phases (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
  position    smallint NOT NULL,
  budget      bigint CHECK (budget >= 0),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);

-- Where the project's money sits: cash in hand, a bank account, bKash.
CREATE TABLE money_accounts (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
  kind             text NOT NULL CHECK (kind IN ('CASH', 'BANK', 'MOBILE')),
  opening_balance  bigint NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);

CREATE TRIGGER money_accounts_set_updated_at BEFORE UPDATE ON money_accounts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE money_accounts;
DROP TABLE project_phases;
DROP TABLE expense_categories;
DROP TABLE project_members;
DROP TABLE projects;
