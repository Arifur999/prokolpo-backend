-- +goose Up
-- An owner's invitation for a phone number that has no account yet. Registering with
-- the number joins every project that invited it.
CREATE TABLE project_invites (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  project_id    uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  phone         text NOT NULL,
  role          text NOT NULL CHECK (role IN ('MANAGER', 'VIEWER')),
  invited_by    uuid NOT NULL REFERENCES users (id),
  accepted_at   timestamptz,
  cancelled_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX project_invites_open_idx ON project_invites (project_id, phone)
  WHERE accepted_at IS NULL AND cancelled_at IS NULL;
CREATE INDEX project_invites_phone_idx ON project_invites (phone)
  WHERE accepted_at IS NULL AND cancelled_at IS NULL;

-- A six-digit code an owner gives a member who forgot their password: 30 minutes, once,
-- three wrong tries. A new code voids the older ones.
CREATE TABLE member_reset_codes (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  project_id  uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  issued_by   uuid NOT NULL REFERENCES users (id),
  code_hash   bytea NOT NULL,
  tries_left  smallint NOT NULL DEFAULT 3,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  voided_at   timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX member_reset_codes_user_idx ON member_reset_codes (user_id, created_at DESC);

-- What the activity page needs to write a sentence: parties, accounts, amounts.
ALTER TABLE activity_log ADD COLUMN details jsonb NOT NULL DEFAULT '{}';
CREATE INDEX activity_log_actor_idx ON activity_log (project_id, actor_id, created_at DESC);

-- +goose Down
DROP INDEX activity_log_actor_idx;
ALTER TABLE activity_log DROP COLUMN details;
DROP TABLE member_reset_codes;
DROP TABLE project_invites;
