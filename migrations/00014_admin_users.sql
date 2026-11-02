-- +goose Up
-- What the admin panel's user pages need: support can issue a reset code outside any project,
-- the account remembers who suspended it and when its password last changed, staff keep
-- private notes, and sign-ins (and wrong passwords) are recorded for the security history.
ALTER TABLE member_reset_codes ALTER COLUMN project_id DROP NOT NULL;

ALTER TABLE users
  ADD COLUMN password_changed_at  timestamptz,
  ADD COLUMN suspended_by         uuid REFERENCES users (id) ON DELETE SET NULL;

CREATE TABLE staff_notes (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  author_id   uuid REFERENCES users (id) ON DELETE SET NULL,
  body        text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX staff_notes_user_idx ON staff_notes (user_id, created_at DESC);

CREATE TABLE login_events (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  ok          boolean NOT NULL,
  ip          text,
  user_agent  text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX login_events_user_idx ON login_events (user_id, created_at DESC);

-- +goose Down
DROP TABLE login_events;
DROP TABLE staff_notes;
ALTER TABLE users DROP COLUMN password_changed_at, DROP COLUMN suspended_by;
DELETE FROM member_reset_codes WHERE project_id IS NULL;
ALTER TABLE member_reset_codes ALTER COLUMN project_id SET NOT NULL;
