# prokolpo-backend

HTTP API for Prokolpo, the project accounts book (প্রকল্প — প্রজেক্টের হিসাব খাতা). Go 1.27, PostgreSQL 18.
The Next.js frontend lives in [Arifur999/prokolpo](https://github.com/Arifur999/prokolpo) and calls this API from its server only.

## Run locally

Requires Go 1.27 and Docker.

```bash
cp .env.example .env
docker compose up -d db        # PostgreSQL 18 on localhost:5433
go run ./cmd/migrate up        # apply migrations
go run ./cmd/api               # http://localhost:5000/api/v1/health
```

Or everything in containers: `docker compose --profile app up --build`.

## Checks

```bash
go vet ./...
golangci-lint run ./...
go test ./...                                     # unit tests
TEST_DATABASE_URL=postgres://prokolpo:prokolpo@localhost:5433/prokolpo?sslmode=disable \
  go test -tags=integration -count=1 ./...        # plus tests that need the database
```

On Linux, macOS or WSL the Makefile wraps these: `make help`. CI runs the same steps plus `govulncheck` and a
production image build.

## Layout

| Path | What lives there |
| --- | --- |
| `cmd/api` | the API server |
| `cmd/migrate` | `up`, `down`, `status`, `version` for the embedded migrations |
| `cmd/healthcheck` | probe the container's HEALTHCHECK runs (the image has no shell) |
| `internal/app` | wiring and graceful shutdown |
| `internal/config` | environment variables, validated at startup |
| `internal/server` | routes and middleware under `/api/v1` |
| `internal/httpx` | response envelope, JSON decoding, request id, logging, panic recovery |
| `internal/apperr` | errors services return; the HTTP layer maps them to statuses |
| `internal/database` | PostgreSQL pool |
| `internal/health` | `GET /api/v1/health` |
| `internal/auth` | registration, login, rotating sessions, password reset |
| `internal/token`, `internal/password`, `internal/phone` | access/opaque tokens, argon2id, BD mobile numbers |
| `internal/ratelimit`, `internal/mail` | in-memory limits (single instance), SMTP and log mailers |
| `internal/testdb` | migrated, emptied database for integration tests (name must end in `_test`) |
| `migrations` | goose SQL migrations, embedded in the binary |

Feature packages (auth, projects, expenses, ...) are added next to these as each feature is built.

## API contract

Every response uses one envelope.

```jsonc
// success
{ "success": true, "message": "Project created", "data": { } }
// list
{ "success": true, "message": "", "data": [ ], "meta": { "page": 1, "limit": 20, "total": 41, "totalPages": 3 } }
// error
{ "success": false, "message": "Check the highlighted fields.", "errorSource": [{ "path": "budget", "message": "Must be positive." }] }
```

| Status | When |
| --- | --- |
| 401 | missing, expired or wrong credentials |
| 403 | signed in but not allowed |
| 404 | not found, or belongs to another tenant |
| 409 | conflicts with current state (duplicates) |
| 422 | invalid input; `errorSource` names the fields |
| 429 | rate limited |
| 500 | unexpected; logged with the request id, details never sent |

Every response carries `X-Request-ID`; send one to correlate frontend and API logs. Errors the client must branch on
carry a stable `code` (`ACCOUNT_SUSPENDED`, `RESET_LINK_INVALID`, ...). Messages follow `Accept-Language` (`bn` default,
`en`).

## Auth

| Endpoint | Body | Notes |
| --- | --- | --- |
| `GET /auth/registration` | | `{ mode: open \| invite_only \| closed }` |
| `POST /auth/register` | `firstName, lastName, companyName?, email?, phone, password, acceptTerms` | 201 with a session |
| `POST /auth/login` | `identifier` (phone or email), `password`, `remember` | 5 wrong passwords lock 15 minutes |
| `POST /auth/refresh` | `refreshToken` | rotates; a reused token revokes its family |
| `POST /auth/logout` | `refreshToken` | |
| `GET /auth/me` | | `Authorization: Bearer <accessToken>` |
| `POST /auth/password/options` | `phone` | name, masked email, which ways can reset |
| `POST /auth/password/email` | `phone` | emails a 30-minute single-use link, once a minute |
| `POST /auth/password/check` | `token` | is the link still usable |
| `POST /auth/password/reset` | `token, password` | signs out every session |

A session is `{ user, accessToken, accessTokenExpiresAt, refreshToken, refreshTokenExpiresAt }`. Access tokens are
15-minute HS256 JWTs (`sub`, `role`: `user` or `staff`); refresh tokens are opaque and stored hashed.

## Database

- Ids are `uuid` with PostgreSQL 18's built-in `uuidv7()` default (time-ordered).
- Instants are `timestamptz`; money is `numeric(14, 2)`.
- Every tenant-owned table carries its tenant key and an index that leads with it.
- Tables with `updated_at` use the shared `set_updated_at()` trigger.
- Migrations are additive; destructive changes go expand, backfill, switch, contract across releases.
