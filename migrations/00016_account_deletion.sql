-- +goose Up
-- Someone who owns no project can delete their account from the profile page. The row stays
-- (names on old entries still read right) but its number and email are freed for reuse.
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended', 'deleted'));
ALTER TABLE users ADD COLUMN deleted_at timestamptz;

-- +goose Down
ALTER TABLE users DROP COLUMN deleted_at;
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended'));
