-- +goose Up
-- The admin panel asks staff for a second step: a code from an authenticator app (TOTP) or a
-- one-time backup code. The secret is stored encrypted; backup codes only as hashes.
ALTER TABLE users
  ADD COLUMN totp_secret        bytea,
  ADD COLUMN totp_pending       bytea,
  ADD COLUMN totp_enabled_at    timestamptz,
  -- The last 30-second step a code was accepted for, so a code can't be used twice.
  ADD COLUMN totp_last_step     bigint,
  ADD COLUMN totp_failures      smallint NOT NULL DEFAULT 0,
  ADD COLUMN totp_locked_until  timestamptz;

CREATE TABLE staff_backup_codes (
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  code_hash  bytea NOT NULL,
  used_at    timestamptz,
  PRIMARY KEY (user_id, code_hash)
);

-- An admin panel session after the second step. It ends after 30 idle minutes.
CREATE TABLE staff_sessions (
  token_hash  bytea PRIMARY KEY,
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  ip          text,
  user_agent  text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  last_seen   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX staff_sessions_user_idx ON staff_sessions (user_id);

-- What staff did in the panel, and every sign-in and failed attempt (board AdminAudit).
CREATE TABLE admin_audit (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  actor_id     uuid REFERENCES users (id) ON DELETE SET NULL,
  action       text NOT NULL,
  target_type  text,
  target_id    text,
  summary      text NOT NULL DEFAULT '',
  details      jsonb NOT NULL DEFAULT '{}',
  ip           text,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_created_idx ON admin_audit (created_at DESC);
CREATE INDEX admin_audit_actor_idx ON admin_audit (actor_id, created_at DESC);

-- +goose Down
DROP TABLE admin_audit;
DROP TABLE staff_sessions;
DROP TABLE staff_backup_codes;
ALTER TABLE users
  DROP COLUMN totp_secret, DROP COLUMN totp_pending, DROP COLUMN totp_enabled_at,
  DROP COLUMN totp_last_step, DROP COLUMN totp_failures, DROP COLUMN totp_locked_until;
