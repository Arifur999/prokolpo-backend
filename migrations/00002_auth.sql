-- +goose Up
CREATE TABLE users (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  first_name        text NOT NULL,
  last_name         text NOT NULL,
  company_name      text,
  -- Always the normalised 11-digit form (01XXXXXXXXX); see internal/phone.
  phone             text NOT NULL UNIQUE,
  email             citext UNIQUE,
  password_hash     text NOT NULL,
  locale            text NOT NULL DEFAULT 'bn' CHECK (locale IN ('bn', 'en')),
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
  suspended_at      timestamptz,
  suspended_reason  text,
  -- Platform staff only; NULL for every ordinary user. Never set from a request body.
  staff_role        text CHECK (staff_role IN ('super_admin', 'support', 'billing')),
  terms_accepted_at timestamptz NOT NULL,
  last_login_at     timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One row per issued refresh token. Rotation links a token to its successor;
-- presenting a token that was already rotated revokes its whole family.
CREATE TABLE refresh_tokens (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  family_id   uuid NOT NULL,
  token_hash  bytea NOT NULL UNIQUE,
  remember    boolean NOT NULL,
  user_agent  text,
  ip          inet,
  expires_at  timestamptz NOT NULL,
  rotated_at  timestamptz,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id, revoked_at);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);

-- Emailed reset links: valid 30 minutes, usable once; a new request voids older ones.
CREATE TABLE password_reset_tokens (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  voided_at   timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id, created_at DESC);

-- Platform-wide switches the admin panel edits.
CREATE TABLE app_settings (
  key         text PRIMARY KEY,
  value       jsonb NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER app_settings_set_updated_at BEFORE UPDATE ON app_settings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO app_settings (key, value) VALUES ('registration_mode', '"open"');

-- +goose Down
DROP TABLE app_settings;
DROP TABLE password_reset_tokens;
DROP TABLE refresh_tokens;
DROP TABLE users;
