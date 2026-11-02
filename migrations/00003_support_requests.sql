-- +goose Up
-- Readable request numbers (S-1001, S-1002, ...) that people quote on WhatsApp.
CREATE SEQUENCE support_request_number_seq START 1001;

CREATE TABLE support_requests (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  number      integer NOT NULL UNIQUE DEFAULT nextval('support_request_number_seq'),
  -- Set when the sender was signed in; kept if the account is later deleted.
  user_id     uuid REFERENCES users (id) ON DELETE SET NULL,
  name        text NOT NULL,
  -- Normalised 01XXXXXXXXX; support calls this number.
  phone       text NOT NULL,
  email       citext,
  topic       text NOT NULL CHECK (topic IN ('login', 'project', 'members', 'other')),
  -- Which screen the request came from: the public page, forgot password, a suspended login, or inside the app.
  source      text NOT NULL CHECK (source IN ('public', 'forgot', 'suspended', 'app')),
  message     text NOT NULL,
  status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
  closed_at   timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

ALTER SEQUENCE support_request_number_seq OWNED BY support_requests.number;

CREATE TRIGGER support_requests_set_updated_at BEFORE UPDATE ON support_requests
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The admin inbox lists open requests newest first; repeat requests are found by phone.
CREATE INDEX support_requests_inbox_idx ON support_requests (status, created_at DESC);
CREATE INDEX support_requests_phone_idx ON support_requests (phone);
CREATE INDEX support_requests_user_idx ON support_requests (user_id);

-- +goose Down
DROP TABLE support_requests;
