-- +goose Up
-- Case-insensitive text for emails, so Foo@x.com and foo@x.com are one account.
CREATE EXTENSION IF NOT EXISTS citext;

-- Every table with an updated_at column gets this trigger instead of trusting callers to set it.
-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION set_updated_at();
DROP EXTENSION IF EXISTS citext;
