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
| `internal/project` | projects, their starting books, membership checks, delete and restore |
| `internal/storage` | uploaded files on local disk under `UPLOAD_DIR` (default `data/uploads`) |
| `internal/books` | a project's money: expenses, suppliers, contractors, attendance, payments, accounts |
| `internal/partners` | a joint project's partners: shares, commitments, capital calls, deposits and their receipts |
| `internal/sales` | units, customers, sales on installments, collections and their receipts |
| `internal/produce` | a farm's sales by batch, what buyers owe, and batch dates and head counts |
| `internal/shop` | a shop's day book (one entry a day) and its credit book |
| `internal/members` | a project's team: invitations by phone, roles, removal, owner-issued reset codes |
| `internal/activity`, `internal/timeline` | writing the activity log inside each change; reading it for the activity page |
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
| `POST /auth/password/code` | `phone, code, password` | the 6-digit code a project's owner gave; 3 wrong tries spend it |
| `POST /auth/invite` | `phone` | the newest open invitation for the number (20 a minute per address) |

While registration is closed or invite-only, an invited number can still register. Registering joins every project
that invited the number.

A session is `{ user, accessToken, accessTokenExpiresAt, refreshToken, refreshTokenExpiresAt }`. Access tokens are
15-minute HS256 JWTs (`sub`, `role`: `user` or `staff`); refresh tokens are opaque and stored hashed.

## Projects

All routes need `Authorization: Bearer <accessToken>`. A project someone is not a member of answers 404.

| Endpoint | Body | Notes |
| --- | --- | --- |
| `GET /projects` | | the caller's projects with their `role` and `memberCount` |
| `POST /projects` | `name, kind, ownership, startDate, location?, description?, budget?` + kind details, `categories[]`, `phases[]` | 201; adds the owner, expense heads, phases and a cash account |
| `GET /projects/{id}` | | the project with its phases |
| `DELETE /projects/{id}` | | owner only; restorable for 30 days |
| `GET /projects/deleted` | | the owner's restorable projects with `purgeAt` |
| `POST /projects/{id}/restore` | | owner only, within 30 days |

`kind` is `BUILDING`, `FARM` (needs `farmType`), `LAND`, `BUSINESS`, `RENOVATION` or `OTHER`; `ownership` is `PERSONAL`
or `PARTNERSHIP` and cannot change later. Kind details (`floors`, `batches`, `plots`, `landArea`, `landUnit`,
`businessType`) are kept only for their kind. Categories and phases come worded by the client, in the creator's
language, exactly as its preview showed them.

## Books

Under `/projects/{id}`; members read, owners and managers write (viewers get 403 `READ_ONLY_ROLE`).

| Endpoint | Notes |
| --- | --- |
| `GET /overview` | cash, dues, this and last month's spending, 12 months, spending by head (labour as one row), recent entries |
| `GET, POST /categories` | expense heads in order; POST adds one at the end |
| `GET, POST /expenses` | list filters `from, to, category, phase, party, paid=credit\|cash, q, page, limit` (10 a page) with a summary; POST `paidNow` + `accountId` also records a payment of the full amount |
| `GET, PUT /expenses/{eid}` | PUT logs the changed fields and an optional `reason` |
| `POST /expenses/{eid}/delete`, `/restore` | soft delete with optional `reason`; the on-the-spot payment goes and comes back with it |
| `GET, POST /suppliers`, `GET /suppliers/{sid}` | dues are bills + opening − payments; a negative due is an advance; the detail has the running ledger |
| `POST /suppliers/{sid}/payments`, `/contractors/{cid}/payments` | `accountId, amount, date, expenseId?, purpose?, note?` |
| `GET, POST /contractors`, `GET /contractors/{cid}` | `payType` CONTRACT (`contractAmount`) or DAILY (`dailyWage`, `teamSize`); the ledger sums attendance per Sunday–Saturday week |
| `PATCH /contractors/{cid}/status`, `POST /contractors/{cid}/extras` | NOT_STARTED, WORKING, DONE; extra work raises what is owed |
| `GET /attendance?week=`, `PUT /attendance` | daily teams for the week holding the date; PUT `cells[{contractorId, day, headcount, halfDay}]`, 0 clears a day; a day keeps the wage it was first saved at |
| `GET, POST /accounts`, `GET /accounts/ledger`, `POST /transfers` | balances may go negative; transfers are not spending |
| `POST /payments/{pid}/delete` | not for a payment made with a bill |
| `PUT, GET /expenses/{eid}/memo` | the memo photo as the raw body, JPG/PNG/WebP up to 5MB, judged by its bytes |

Spending (`project_spent`) is every bill plus labour: daily wages, extra work, and contract payments up to the contract.

## Members and activity

Under `/projects/{id}`; members read, only the owner changes the team (others get 403 `OWNER_ONLY`).

| Endpoint | Notes |
| --- | --- |
| `GET /members` | members (owner first, last active, entries this month) and open invitations |
| `GET /members/lookup?phone=` | `MEMBER`, `PENDING`, `FOUND` (with the name) or `NEW` |
| `POST /members` | `phone, role` (MANAGER or VIEWER): an account joins now, any other number waits as an invitation |
| `PATCH /members/{uid}`, `DELETE /members/{uid}` | change role, remove; the owner is fixed (`OWNER_FIXED`); entries stay |
| `POST /members/{uid}/reset-code` | a 6-digit code, 30 minutes, once; a new code voids the old one |
| `DELETE /invites/{iid}` | cancels an invitation |
| `GET /activity?actor=&action=&days=&before=` | 50 entries a page, newest first, with the bill, payment or transfer joined in |
| `GET /activity/week` | each member's entries since Sunday |

## Partners

Under `/projects/{id}` of a joint project (`NOT_PARTNERSHIP` otherwise); members read, the owner adds partners
and capital calls, owners and managers take deposits.

| Endpoint | Notes |
| --- | --- |
| `GET /partners` | partners (share, commitment, call parts, paid, team role) and capital calls with each part and what was paid |
| `GET /partners/lookup?phone=` | `PARTNER`, `MEMBER` (with role), `ACCOUNT` or `NONE` |
| `POST /partners` | `phone, name, share, committed, joinedOn, shares{partnerId: share}, viewer`; all shares must add up to 100 |
| `GET /partners/{pid}` | the partner's ledger (owed and paid, oldest first) and their calls |
| `POST /partners/{pid}/deposits` | `amount, accountId, callId?, date?, note?`; takes the next receipt number, money goes into the account |
| `POST /capital-calls` | `total, reason`; split by today's shares, remainder to the biggest; due at the month's end |
| `GET /partner-deposits/{did}/receipt`, `POST .../printed` | the receipt as printed, with owed and paid as of it |

Receipt numbers (`PRK-000148`) run per project and are never reused.

## Sales

Under `/projects/{id}`; members read, owners and managers write.

| Endpoint | Notes |
| --- | --- |
| `GET /units`, `POST /units` | units by floor with their sale and its standing; POST `units[]` adds one or a block (used names skipped in a block) |
| `PUT, DELETE /units/{uid}` | a sold unit can't be deleted (`UNIT_HAS_SALE`) |
| `GET, POST /customers`, `GET /customers/{cid}` | buyers with their sales; a number already used returns that buyer |
| `GET /sales`, `GET /sales/{sid}` | sales with standing (received, due, next, oldest late); the detail has the schedule and payments |
| `POST /sales` | `unitId, customerId \| customer{name, phone, nid?, address?}, date, price, down, installments (0 = full price), intervalMonths (1, 3, 6), firstDue, accountId` |
| `POST /sales/{sid}/payments` | `amount, accountId, date?, note?`; fills the oldest installments first, never past what is owed |
| `GET /sale-payments/{pid}/receipt`, `POST .../printed` | the receipt, with the installments it covered and the next one |

Installments are even parts rounded down to ৳100; the last one takes the rest. Payments are kept, the schedule is
worked out from them.

## Farm and shop sales

Under `/projects/{id}`; members read, owners and managers write.

| Endpoint | Notes |
| --- | --- |
| `GET /produce` | batches (cost, sales, kilos sold), the setup phase's cost, sales with what is still owed, buyer dues, buyers |
| `POST /produce` | `product, phaseId?, quantity, unit, rate, buyer, date, received, accountId`; the product must suit the farm type |
| `POST /produce/buyers/{bid}/payments` | `amount, accountId`; clears the buyer's oldest sales first, never past their due |
| `PUT /batches/{phid}` | `startedOn, endedOn (empty = running), headCount, losses, note` |
| `GET /shop` | the last year of days, the credit book (due, oldest unpaid credit, last taken), monthly spending |
| `PUT /shop/days` | `date, cash, mobile, other, note, counted, cashAccountId, mobileAccountId, credits[], collects[]`; writes or rewrites that day |
| `POST /shop/khata/{pid}/payments` | `amount, accountId`; credit paid back today |

A farm's first phase is its setup (the shed, the pond); its spending is what batch profits pay back. Paybacks in a
day's entry go into its cash account.

## Reports

`GET /projects/{id}/reports/expenses?from=&to=` gives a period's spending by head (labour as its own head), by
phase against budget (plus what fell outside any phase), by month, and by supplier (billed and paid). Installment,
profit and dues reports are worked out on the page from the sales, partner and books data.

## Database

- Ids are `uuid` with PostgreSQL 18's built-in `uuidv7()` default (time-ordered).
- Instants are `timestamptz`; money is whole taka in `bigint` (`int64` in Go, a plain number in JSON). The books
  never deal in paisa.
- Every tenant-owned table carries its tenant key and an index that leads with it.
- Tables with `updated_at` use the shared `set_updated_at()` trigger.
- Migrations are additive; destructive changes go expand, backfill, switch, contract across releases.
