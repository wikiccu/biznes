# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 2 — Money Owed & Obligations.**

The product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). Phase 0's foundation, developer commands, API conventions, and Go CI are implemented. Phase 1 implements global identity, persistent bearer sessions, organizations, scoped contacts, income/expense transaction categories, cash/bank accounts, exact IRR transaction recording, linked full reversals, and per-account recorded net activity with account/occurred-period filters and membership/role authorization. Phase 2 now records tenant-scoped customer receivables and supplier payables with exact IRR amounts, date-only due dates, provenance, and persistent retry identity. Opening/current cash balances and further correction/payment workflows remain planned. Hosted validation status is available in GitHub Actions.

The blueprint is the living source of truth. Update it whenever a significant product or architecture decision changes.

## Planned stack

- Go and Gin for a versioned REST API.
- PostgreSQL as the primary source of truth, with explicit migrations.
- Docker and Docker Compose for the local environment.
- A modular monolith; add infrastructure and domain modules when an implemented requirement needs them.

The Go module uses `github.com/wikiccu/biznes` and requires Go 1.27.1 or newer. Gin is pinned to [v1.12.0](https://github.com/gin-gonic/gin/releases/tag/v1.12.0), the PostgreSQL driver/pool is [pgx v5.11.0](https://github.com/jackc/pgx/releases/tag/v5.11.0), and migrations use [Goose v3.26.0](https://github.com/pressly/goose/releases/tag/v3.26.0). Goose is pinned to this stable version to preserve existing API dependency versions. Additional libraries will be selected during their respective implementation steps after checking current stable releases. Redis, AI providers, object storage, and background workers are future capabilities.

## Local development

Install [Go 1.27.1 or newer](https://go.dev/dl/) and Git. Start PostgreSQL using the local development instructions below, then set `BIZNES_DATABASE_URL` in the process environment with the matching database, user, password, and host port. Apply migrations explicitly before running the API. From the repository root, run:

```text
go run ./cmd/migrate up
go run ./cmd/api
```

It loads and validates configuration, creates and verifies the PostgreSQL pool, and then serves HTTP on port `8080` by default. Failed database initialization exits with code `1` before the HTTP listener opens. Stop it with Ctrl+C; deployments can send SIGTERM. HTTP shutdown stops accepting connections and allows in-flight requests to finish within the configured deadline, then closes remaining HTTP connections if the deadline expires. The database pool closes after HTTP drains, including on binding or shutdown failure. pgx cleanup can take up to approximately 15 additional seconds when PostgreSQL is unresponsive; the HTTP shutdown timeout applies to HTTP draining. A shutdown failure exits with code `1`.

Check the running server with:

```text
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
```

Both return JSON with an `X-Request-ID` response header and `Cache-Control: no-store`. `/ready` checks the configured database connection; `/health` remains a process liveness check.

### Local PostgreSQL

Install Docker with the Compose v2 plugin and start its Linux container engine (for example, Docker Desktop). [compose.yaml](compose.yaml) pins the [official PostgreSQL image](https://github.com/docker-library/docs/blob/master/postgres/README.md) to `18.6-trixie`, with a named volume and a TCP readiness check. PostgreSQL is published only on `127.0.0.1`, using host port `5432` by default.

Copy `.env.example` to `.env` if the file does not already exist. Set `BIZNES_POSTGRES_PASSWORD` to a local development password before running Compose; an absent or empty password fails configuration. Keep `.env` private. The image creates a development superuser, so these settings are for local development only.

| Compose variable | Default when absent | Purpose |
| --- | --- | --- |
| `BIZNES_POSTGRES_PORT` | `5432` | Host port on `127.0.0.1`; change it if another database uses this port. |
| `BIZNES_POSTGRES_DB` | `biznes` | Database created on first initialization. |
| `BIZNES_POSTGRES_USER` | `biznes` | Development superuser created on first initialization. |
| `BIZNES_POSTGRES_PASSWORD` | Required, non-empty | Password set on first initialization. |

Compose reads `.env` automatically; process environment values take precedence. These initialization settings are separate from the API's `BIZNES_DATABASE_URL`, which must identify the same database for local development. Use `docker compose config --quiet` to validate without printing the resolved password.

From the repository root:

```text
docker compose config --quiet
docker compose up -d --wait --wait-timeout 60 postgres
docker compose ps
docker compose logs --tail 50 postgres
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
docker compose down
```

Inside `psql`, run `SELECT current_database(), current_user, version();` and quit with `\q`. Host clients connect to `127.0.0.1`, the configured host port, and the database/user/password above. The health check verifies that PostgreSQL accepts connections; it does not verify application schema or credentials.

The `biznes_postgres_data` volume retains data across container recreation and `docker compose down`. It is mounted at `/var/lib/postgresql`, as required by the PostgreSQL 18 image layout. Changing initialization credentials in `.env` does not change an existing database; use SQL to update an existing role. Major version upgrades require an explicit data migration. Do not use `down --volumes` unless you intend to delete the local database.

### Database migrations

The [migration workflow](migrations/README.md) uses the pinned Goose library through `cmd/migrate`, reusing pgx and the same environment configuration. It provides `up`, one-step `down`, and `status`, with PostgreSQL advisory locking, transactional SQL, a work deadline, and Ctrl+C/SIGTERM cancellation. It never runs automatically on API startup.

```text
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
```

The migrations create the `biznes` schema, global users, durable sessions, organizations/memberships, organization-owned contacts, transaction categories, financial accounts, immutable API transaction records, linked full reversals, immutable customer receivables, and supplier payables. Goose tracks versions in `public.goose_db_version`. Rollbacks lock affected tables and refuse to remove stored rows; the schema rollback refuses a non-empty schema. Rollbacks remain explicit. See the workflow for flags, naming, adding migrations, permissions, failure recovery, and rollback validation.

### User persistence

Users are global identities; memberships grant access to organizations. [The migration](migrations/00002_create_users.sql) supplies native PostgreSQL UUIDv4 IDs, unique canonical email addresses, an opaque password verifier, and `timestamptz` creation/update defaults. [The Go model](internal/identity/user.go) mirrors these fields and excludes `PasswordHash` from JSON; handlers should still use explicit response types.

Stored emails are lowercase printable ASCII, 3–254 bytes, with one `@` and non-empty local/domain parts. Registration trims surrounding email whitespace, validates a plain address and DNS domain, normalizes casing, and preserves dots/plus tags. Internationalized addresses and phone login remain future choices. Password verifiers must be non-empty printable ASCII, at most 1024 bytes; these storage checks do not establish cryptographic strength. Registration writes only a server-generated Argon2id verifier. Future update statements must maintain `updated_at`; there is no timestamp trigger or automatic schema migration.

### Registration

`POST /api/v1/auth/register` accepts one JSON object with `email` and `password` string fields. Send `Content-Type: application/json`, optionally with `charset=utf-8`; query parameters are rejected. The body limit is 8 KiB. Unknown/duplicate fields, malformed JSON, invalid UTF-8, lone surrogate escapes, wrong types, and trailing values are rejected. Missing or unacceptable field values use `422` with safe field/code details. See [the endpoint contract](docs/API_CONVENTIONS.md#registration) for examples and error mappings.

Passwords contain 15–128 Unicode code points. Whitespace and Unicode are preserved exactly; there are no composition rules, normalization, or truncation. [OWASP's password storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) informs the Argon2id policy: 19 MiB, two passes, one lane, a fresh random 16-byte salt, and a 32-byte verifier in PHC format. The implementation imports the existing `golang.org/x/crypto v0.48.0` directly without changing dependency versions.

Successful registration returns `201` with `data.id`, canonical `data.email`, and UTC `data.created_at`/`data.updated_at`. It grants no session or organization access. Database uniqueness protects concurrent creation; duplicates return a safe `409` without changing the account. Each API process shares one password operation at a time and one start per second between registration and login, with `429`/`Retry-After: 1` for excess work and five-second database operation deadlines. Responses use `Cache-Control: no-store`; passwords, verifiers, and emails are omitted from application logs.

Before public deployment, add email ownership verification, common/breached password screening, and client-aware limits at the trusted ingress. The process-wide limit bounds hashing work but can be exhausted by one caller. The distinct `201`/`409` responses reveal email availability; an ownership-verification flow should provide indistinguishable registration responses. Deploy credential endpoints over HTTPS.

### Authentication and sessions

`POST /api/v1/auth/login` accepts the same strict email/password JSON contract as registration. Login passwords must be non-empty and at most 128 Unicode code points; signup strength rules do not reject a short login candidate. It normalizes email casing/outer whitespace and preserves passwords exactly. Unknown accounts, wrong passwords, and unsupported stored verifiers return the same safe `401`; unknown/unsupported verifiers still run the fixed-cost Argon2id derivation. Stored cost parameters cannot select arbitrary hashing workloads.

Login returns `200` with `data.user`, a fresh `data.access_token`, `data.token_type: "Bearer"`, and UTC `data.expires_at`. Tokens contain 32 cryptographically random bytes encoded as 43 unpadded Base64URL characters. [The sessions migration](migrations/00003_create_sessions.sql) stores only the SHA-256 digest of the decoded token bytes, the user reference, and creation/last-use/expiry instants. Each login creates an independent session with an eight-hour absolute lifetime and a fifteen-minute idle timeout. Login does not grant organization access.

Send the token in one `Authorization: Bearer <token>` header to `GET /api/v1/auth/me` or `POST /api/v1/auth/logout`. Both accept an empty body and no query string. `me` returns the public user DTO; logout returns an empty `204` and removes the current session. Missing, malformed, unknown, expired, or revoked tokens return `401` with a Bearer challenge. Credentials are accepted only from the authorization header for these routes; no session cookie is set. See [the authentication contract](docs/API_CONVENTIONS.md#authentication-and-sessions).

Session checks use PostgreSQL on every authorized request, refresh idle activity without extending the absolute expiry, and fail with safe `503` errors when storage is unavailable. Sessions and revocation are shared across API processes and survive API restarts. Requests already authenticated may finish after logout. Expiry requires a new login; there is no refresh endpoint. Tokens, passwords, verifiers, and credential input are omitted from application logs, and credential responses use `Cache-Control: no-store`.

Expired rows remain unusable and can be removed through explicit [session maintenance](migrations/README.md#sessions); schedule this maintenance in deployments to bound table growth. Recovery, MFA, device/session management, and stronger policies for sensitive financial operations remain future work. This increment adds no environment settings, signing keys, or dependencies.

### Organizations

Apply [migration 00004](migrations/00004_create_organizations.sql) before using these authenticated routes:

| Route | Behavior |
| --- | --- |
| `POST /api/v1/organizations` | Create an organization and the caller's owner membership atomically; return `201` and its retrieval `Location`. |
| `GET /api/v1/organizations` | List only the caller's memberships, using bounded `page`/`limit` pagination. |
| `GET /api/v1/organizations/:organization_id` | Read an organization only when the caller is a member. |
| `PATCH /api/v1/organizations/:organization_id` | Rename an organization when the caller is an owner or administrator. |

Create/rename accept a strict UTF-8 JSON object with only a `name` string, capped at 8 KiB. Names are trimmed, contain 1–120 Unicode code points, and reject control characters; Persian and zero-width non-joiners are preserved. Responses contain `id`, `name`, the caller's `role`, and UTC timestamps. Names need not be unique. Every lookup checks database membership using the authenticated user; client user IDs, roles, and tenant headers cannot grant access. Outsiders receive the same `404` as an unknown organization, while accountant/staff members receive `403` when renaming. Rename holds the membership row through commit to serialize concurrent role changes/removal. There are no invitations, membership-management routes, ownership transfer, or deletion yet. See [the full contract](docs/API_CONVENTIONS.md#organizations).

### Contacts

Apply [migration 00005](migrations/00005_create_contacts.sql) before using `/api/v1/organizations/:organization_id/contacts`. `POST` creates a contact; `GET` lists that organization's contacts with `page`/`limit`; `GET /:contact_id` retrieves one; `PUT /:contact_id` replaces its editable fields. All require a bearer session and current organization membership. All members can read; owner/admin members can create or replace. Both organization and contact IDs scope every lookup/write.

Send a strict JSON object with required `name` and `kind` (`customer`, `supplier`, or `both`), and optional `email`, `phone`, and `notes` strings. Names support Persian and contain 1–120 code points. Phone numbers are recorded as free-form text up to 64 code points; email uses plain mailbox syntax up to 254 UTF-8 bytes, with case preserved. Multiline notes allow up to 2000 code points. The body cap is 8 KiB. `PUT` requires name/kind and clears omitted optional fields. Names need not be unique. There is no deletion, contact search/filter, tags, activity history, or financial balance yet. See [the endpoint contract](docs/API_CONVENTIONS.md#contacts) for validation, permissions, and concurrency behavior.

### Transaction categories

Apply [migration 00006](migrations/00006_create_transaction_categories.sql) before using `/api/v1/organizations/:organization_id/transaction-categories`. `POST` creates a category from a `name` and `kind` (`income` or `expense`); `GET` lists categories with `page`/`limit`; `GET /:category_id` retrieves one; `PATCH /:category_id` renames it using only `name`. Kind stays fixed through the API. Owner/admin/accountant members can create/rename; every member can read. All operations enforce the selected organization and current membership.

Names contain 1–120 Unicode code points after trimming, reject control characters, and preserve Persian text. Exact, case-sensitive names are unique within organization/kind; duplicate creation or rename returns safe `409` without changing existing categories. The same name in another organization or the other kind is allowed. Categories have no amounts or balances. Category deletion/archive, hierarchy, and defaults remain planned. See [the endpoint contract](docs/API_CONVENTIONS.md#transaction-categories).

### Financial accounts

Apply [migration 00007](migrations/00007_create_financial_accounts.sql) before using `/api/v1/organizations/:organization_id/financial-accounts`. `POST` creates an account from `name`, `kind` (`cash` or `bank`), and required `currency: "IRR"`; `GET` lists accounts with `page`/`limit`; `GET /:account_id` retrieves one; `PATCH /:account_id` renames it using only `name`. Kind and currency stay fixed through the API. Owner/admin/accountant members can create/rename; every member can read. Every operation enforces the selected organization and current membership.

Names contain 1–120 Unicode code points after trimming and reject controls. Exact, case-sensitive names are unique across account kinds within an organization; conflicts return safe `409`. Currency is explicit: IRR uses whole Rial (scale zero), and Toman remains a presentation denomination. Account metadata includes no opening balance, current balance, amounts, bank credentials, or card data. Transactions record exact amounts separately; unreversed totals are available from the recorded activity route below. See [the endpoint contract](docs/API_CONVENTIONS.md#financial-accounts).

### Income/expense transactions

Apply [migration 00008](migrations/00008_create_transactions.sql) before using `/api/v1/organizations/:organization_id/transactions`. `POST` records a transaction; `GET` lists records with `page`/`limit`; `GET /:transaction_id` retrieves one. Owner/admin/accountant members can record; every member can read. Select an account and category within the organization: currency must be `IRR` and direction comes from the category's income/expense kind. Supply a positive whole-Rial `amount` as a decimal string, an explicit `occurred_at` RFC 3339 instant with at most six fractional digits, a canonical UUID `idempotency_key`, and optional `description` text.

Native tenant-scoped uniqueness makes identical retries return the same record (`200` instead of first-create `201`) across replicas and restarts; changed input with that key returns `409`. Generate a new key for each intended activity and reuse it after an uncertain response. Records retain their creator and creation instant, with no edit/delete route or mutable balance column. Linked full reversals and recorded account activity are implemented below; transfers, opening amounts, replacement workflows, and complete cash balances follow separately. See [the endpoint contract](docs/API_CONVENTIONS.md#incomeexpense-transactions) for exact bounds, validation, and retry semantics.

### Transaction history filters

Transaction collection GET accepts `page`/`limit`, optional `account_id`, and optional paired `from`/`to` instants. The account must belong to the selected organization; unknown/inaccessible filters return `404`. The interval selects original `occurred_at` values with inclusive `from` and exclusive `to`, before stable pagination. Supply both bounds with `from < to`, using the same strict RFC 3339/microsecond/UTC-year rules as recording. Unknown/duplicate/empty query values are rejected. Omitting filters preserves the original all-time history, including reversed originals and their unchanged DTOs. See [the shared filter contract](docs/API_CONVENTIONS.md#transaction-history-and-period-filters).

### Transaction reversals

Apply [migration 00009](migrations/00009_create_transaction_reversals.sql) before using `/api/v1/organizations/:organization_id/transactions/:transaction_id/reversal`. `POST` adds the transaction's single full reversal; `GET` retrieves it. Send only a required canonical UUID `idempotency_key` and `reason` string. Owner/admin/accountant members can reverse; every member can read. Reasons are trimmed, contain 1–2000 Unicode code points, and reject controls. Both the transaction and reversal remain immutable through the API, preserving their separate creator/time history.

Identical key/transaction/trimmed-reason retries return the original reversal with `200`; first creation returns `201`. A changed payload, a key reserved for another reversal in the organization, or a second reversal under a new key returns `409`. Native unique constraints protect races across replicas. Reversals fully void the original's recognition without copying/negating its amount or recording opposite-kind activity. Original transaction GET/list and creation-retry DTOs still include the unchanged originals; read the separate reversal endpoint to establish reversal status. This is a record correction, not a cash refund. Partial reversals, undoing a reversal, atomic linked replacements, and complete cash balances remain planned. See [the contract](docs/API_CONVENTIONS.md#transaction-reversals).

### Recorded account activity

`GET /api/v1/organizations/:organization_id/financial-accounts/:account_id/recorded-activity` returns the account's recorded income, expense, and net amounts in `IRR`. Every current member can read; unknown/inaccessible accounts return `404`. Send an empty body, optionally with the same paired `from`/`to` period filters. Account identity stays in the path. The operation uses the existing schema through migration 00009, with no new migration, dependency, or setting.

Amounts are exact decimal strings, including totals beyond signed 64-bit range; net can be negative. One database snapshot sums unreversed originals in the selected organization/account/currency and computes income minus expense. An empty or fully reversed account returns `"0"` totals. The response labels its `basis` as `"recorded_transactions"` and sets `opening_balance_included` to `false`. Without bounds it includes all stored occurred dates, including future dates. With bounds it includes only originals in `[from, to)` and echoes those normalized UTC instants. A later reversal still removes the original from its original period; history retains the audit record. Separate list/total requests use separate snapshots. Opening amounts, transfers, and complete imported history are not established, so recorded net does not establish the actual cash/bank balance. Originals and retry history remain unchanged. See [the contract](docs/API_CONVENTIONS.md#recorded-account-activity).

### Configuration

API configuration comes from the process environment. [`.env.example`](.env.example) lists API and local Compose settings; the Go application does not load env files automatically. Local `.env` and `.env.*` files are ignored by Git, with `.env.example` retained as the committed example.

| Variable | Default when absent | Validation |
| --- | --- | --- |
| `BIZNES_HTTP_PORT` | `8080` | Integer from `1` through `65535`. Empty, malformed, and out-of-range values fail startup with exit code `1`. |
| `BIZNES_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` (case-insensitive). Empty, whitespace-padded, and unsupported values fail startup with exit code `1`. |
| `BIZNES_HTTP_READ_HEADER_TIMEOUT` | `5s` | Positive Go duration. |
| `BIZNES_HTTP_READ_TIMEOUT` | `15s` | Positive Go duration. |
| `BIZNES_HTTP_WRITE_TIMEOUT` | `15s` | Positive Go duration. |
| `BIZNES_HTTP_IDLE_TIMEOUT` | `60s` | Positive Go duration. |
| `BIZNES_HTTP_SHUTDOWN_TIMEOUT` | `10s` | Positive Go duration. |
| `BIZNES_DATABASE_URL` | Required | Non-blank PostgreSQL URL or keyword/value connection string accepted by pgx. Invalid connection or pool settings fail startup safely. |
| `BIZNES_DATABASE_CONNECT_TIMEOUT` | `5s` | Positive Go duration bounding the initial connection check and capping new connection attempts. A shorter driver `connect_timeout` is retained. |
| `BIZNES_DATABASE_HEALTH_TIMEOUT` | `2s` | Positive Go duration bounding each readiness check, including waiting for a pooled connection; also bounds pgx checkout pings. |

Duration values use units such as `500ms`, `15s`, or `1m`. Empty, malformed, zero, negative, and overflowing durations fail startup with exit code `1`. Read/write settings are HTTP transport timeouts; they do not automatically cancel application work at that deadline.

Use native [pgx pool options](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool#ParseConfig) in the connection string, such as `pool_max_conns=10`, to set the connection budget. Without an override, pgx uses the larger of four or the CPU count. The API's health timeout takes precedence over `pool_ping_timeout`. Other pool lifecycle settings follow pgx connection-string options and defaults; minimum connection counts must be non-negative and no larger than the maximum, and the background health-check interval must be positive.

For example, in PowerShell:

```powershell
$env:BIZNES_HTTP_PORT = '9000'
$env:BIZNES_DATABASE_URL = 'postgres://biznes:YOUR_ENCODED_PASSWORD@127.0.0.1:5432/biznes?sslmode=disable&pool_max_conns=10'
go run ./cmd/api
Remove-Item Env:BIZNES_HTTP_PORT
Remove-Item Env:BIZNES_DATABASE_URL
```

Or in a POSIX shell:

```sh
BIZNES_DATABASE_URL='postgres://biznes:YOUR_ENCODED_PASSWORD@127.0.0.1:5432/biznes?sslmode=disable&pool_max_conns=10' BIZNES_HTTP_PORT=9000 go run ./cmd/api
```

Replace `YOUR_ENCODED_PASSWORD` with the URL-encoded form of the password chosen for Compose, and adjust the URL when overriding the database, user, or port. `sslmode=disable` is for this local container; use `sslmode=verify-full` with the appropriate trusted certificate configuration in deployments. pgx handles authentication and TLS. The API does not load `.env` automatically.

Configuration and database startup errors use fixed messages without exposing the connection string, password, or raw PostgreSQL errors. `internal/platform/database.Open` returns the native `pgxpool.Pool`; use operation contexts with `Exec`, `Query`, and `QueryRow`, close returned rows, and release explicitly acquired connections.

### Logging

The application uses standard-library `log/slog` to write one JSON object per line to stderr. Records include `time`, `level`, `msg`, and `service: "biznes"`. Lifecycle events identify the verified database pool (including its maximum connection count), HTTP listening, stopping, stopped, and pool closure. The listening event is emitted only after the port is bound successfully.

The configured log level is the minimum severity: `warn` and `error` suppress `INFO` lifecycle and ordinary request events. Requests record `request_id`, method, route template (empty for unmatched routes), status, and duration in milliseconds; responses with status `500` or higher are logged at `ERROR`. Query strings, raw URL paths, headers, bodies, and panic values are omitted from request and recovery logs.

Startup configuration failures always emit an `ERROR` record and exit with code `1`, even when log-level configuration is invalid. Errors use fixed messages without recording supplied values or the full configuration. Gin's default debug/request/recovery output is disabled in favor of structured logging.

### HTTP behavior

| Endpoint | Response | Meaning |
| --- | --- | --- |
| `GET /health` | `200` with `{"status":"ok"}` | Process liveness; does not check dependencies. |
| `GET /ready` | `200` with `{"status":"ready"}` | The lifecycle context is active and a bounded PostgreSQL ping succeeds. |
| `GET /ready` during an outage or shutdown | `503` with `{"status":"not_ready"}` | The database check fails or times out, the request is canceled, or shutdown begins while the handler is active. |
| `POST /api/v1/auth/register` | `201` with a public user `data` object | Validated identity creation with a securely hashed password, without granting a session. |
| `POST /api/v1/auth/login` | `200` with user, bearer token, and expiry in `data` | Credential verification and durable session creation. |
| `GET /api/v1/auth/me` | `200` with a public user `data` object | The supplied bearer session is active. |
| `POST /api/v1/auth/logout` | Empty `204` | Revoke the current active bearer session. |

Shutdown closes the listening socket, so new probe connections may fail instead of receiving a response. Liveness remains successful during database outages and for requests served during draining. The readiness ping observes both request cancellation and application shutdown without canceling other in-flight request contexts. Readiness can recover after a database outage without restarting the API; it verifies connectivity, not application schema. Unregistered paths, including `/` and `/api/v1`, return a JSON `404` error. Unsupported methods on registered paths return a JSON `405` error with `Allow`; only `GET` is currently registered for the probes.

Every handled request receives an `X-Request-ID`. A single supplied value is accepted if it contains 1–128 ASCII letters, digits, dots, underscores, or hyphens. Missing, duplicate, empty, or invalid values are replaced with a cryptographically random opaque ID. The ID is available as `request_id` in the Gin context and appears in the response header, request log, and recovered-panic error response.

Shared errors use `{"error":{"code":"...","message":"...","request_id":"..."}}`, with optional field/code `details`. They return `application/json` and `Cache-Control: no-store`. Recovery uses the same writer for a safe `500` with code `internal_error` before response commitment; after commitment it aborts without appending a second body or changing the status. Panic details and stack traces are omitted. Trusted proxy headers and automatic trailing-slash redirects are disabled.

The [API conventions](docs/API_CONVENTIONS.md) define `/api/v1`, `data`/optional `meta` success envelopes, status/error mappings, bounded JSON input, validation, pagination (default page `1`, limit `20`, caps `10000`/`100`), UTC timestamps, and opaque UUID IDs. Registration and authentication implement their input and success contracts; pagination will accompany the first list endpoint. There are no placeholder endpoints.

### Git workflow

From the repository root, review the current state:

```text
git status
git branch --show-current
```

Development takes place on `main`. This project-specific workflow supersedes the general `stage`/feature-branch and manual-commit rules. Preserve existing work, validate and review one cohesive increment, commit it with a Conventional Commit message, attempt to push to `origin/main` when configured, then stop until the human says `continue`. Never merge automatically.

Inspect recent history with:

```text
git log --oneline -n 10
```

### Receivables

Apply [migration 00010](migrations/00010_create_receivables.sql) before using `/api/v1/organizations/:organization_id/receivables`. POST records an amount owed by a current customer/both contact; GET lists records with page/limit and GET `/:receivable_id` retrieves one. Owner/admin/accountant members can record or retry; every member reads. All references and records stay within the selected organization.

Send required canonical UUID `idempotency_key` / `contact_id`, positive decimal-string `amount` through signed 64-bit maximum, explicit `currency: "IRR"`, and required Gregorian `due_date: "YYYY-MM-DD"` (years 0001–9999); an optional bounded Unicode `description` preserves whitespace. Due dates persist as native DATE, with no timestamp or guessed timezone. Matching retries return the first record across replicas/restarts, even after contact reclassification; changed payloads conflict. Contact classification is locked for new-record eligibility, and native composite FKs preserve tenant ownership.

Receivables retain creator/server creation history and expose no update/delete, payment, or status route. They record obligations without creating income or changing account activity. Settlements, outstanding totals, overdue/aging calculations, and corrections remain planned. See [the full contract](docs/API_CONVENTIONS.md#receivables).

### Payables

Apply [migration 00011](migrations/00011_create_payables.sql) before using `/api/v1/organizations/:organization_id/payables`. POST records an obligation owed to a current supplier/both contact; GET lists records with page/limit and GET `/:payable_id` retrieves one. Owner/admin/accountant members can record or retry; every member reads. Tenant scope, contact classification locks, exact IRR amounts, date-only due dates, description bounds, and safe input/errors follow the receivable conventions.

Send the same required UUID key/contact, positive decimal-string amount, explicit IRR currency, Gregorian due date, and optional description fields. Matching retries preserve the original creator/time across replicas/restarts, including after contact reclassification; changed payloads conflict. Payable keys are independent of receivable and transaction keys. Stored debt remains separate from expense/cash payment and account activity. Payments, allocations, outstanding/overdue calculations, and corrections follow separately. See [the full contract](docs/API_CONVENTIONS.md#payables).

## Useful commands

[dev.ps1](dev.ps1) provides a single entry point using [PowerShell 7 or newer](https://learn.microsoft.com/powershell/scripting/install/installing-powershell). Run it from the repository root, or use its absolute path from another directory; commands run in the repository and restore the caller's directory. Go commands require Go on `PATH`; database commands require Docker Compose and its running Linux engine. No Make, Task, standalone migration CLI, or additional linter is required.

```text
pwsh -File ./dev.ps1 check
```

Omitting the command also runs `check`. It stops at the first failure and exits nonzero; all commands propagate the native tool's exit code. The script does not load `.env` for Go, set credentials, start a database implicitly, or apply migrations during `run`.

| Command | Action |
| --- | --- |
| `check` | Check Go formatting, run `go test ./...`, run `go vet ./...`, then verify downloaded modules with `go mod verify`. Does not change files or require PostgreSQL. |
| `fmt-check` | List unformatted Go files under `cmd` and `internal`; fail if any exist, without changing them. |
| `fmt` | Format Go files under `cmd` and `internal` with `gofmt -w`. |
| `test` | Run `go test ./...`; currently checks package compilation because no project test files exist yet. |
| `lint` | Run the standard Go analyzer, `go vet ./...`. Add another linter only when a concrete gap requires it. |
| `run` | Run the API with the existing process environment; stop with Ctrl+C. |
| `db-up` | Start PostgreSQL and wait up to 60 seconds for Compose readiness. Uses the existing Compose configuration and `.env` handling. |
| `db-down` | Stop/remove the Compose containers and network, retaining the database volume. |
| `migrate-status` | Report database migration versions and states. |
| `migrate-up` | Apply pending SQL migrations explicitly. |
| `migrate-down` | Explicitly roll back one migration; review its `Down` SQL first because rollback may affect data. |

For example, after configuring the Compose password and the matching process `BIZNES_DATABASE_URL`:

```text
pwsh -File ./dev.ps1 db-up
pwsh -File ./dev.ps1 migrate-up
pwsh -File ./dev.ps1 run
```

Native Go, Compose, and migration commands documented above remain available, including migration `-dir` and `-timeout` flags. To validate without PowerShell:

```text
gofmt -l cmd internal
go test ./...
go vet ./...
go mod verify
git diff
git diff --check
git status --short
```

`gofmt -l cmd internal` must produce no output; unlike `fmt-check`, the native listing command does not fail solely because formatting differs. Gin, pgx, Goose, and their transitive dependencies are recorded in `go.mod` and `go.sum`.

Review new untracked files directly before staging; ordinary `git diff` does not include them. Inspect the staged increment with `git diff --cached` before committing.

## Continuous integration

The [Go validation workflow](.github/workflows/go.yml) runs on pushes to `main`, pull requests targeting `main`, and manual dispatch from [GitHub Actions](https://github.com/wikiccu/biznes/actions/workflows/go.yml). One Ubuntu 24.04 job uses the runner's PowerShell and the exact Go version from `go.mod`. Checkout and setup-go are pinned to release commit SHAs; setup-go caches Go modules/build output using `go.sum` as its dependency key.

It runs `dev.ps1 check` for formatting, package tests, vet, and module verification, then `go build ./...` and a check that `go.mod`/`go.sum` stayed unchanged. Go uses readonly module resolution and the installed toolchain. The workflow has read-only repository permissions, a 15-minute job limit, and cancels superseded runs for the same ref. It requires no database configuration or secrets; database runtime validation remains separate from these compilation/static checks. No project test files exist yet.

Reproduce the checks locally with:

```text
pwsh -File ./dev.ps1 check
go build -mod=readonly ./...
git diff --exit-code -- go.mod go.sum
```

Hosted execution results are reported in GitHub Actions; workflow configuration and local checks do not establish a successful hosted run.

## Next increment

Phase 2 next introduces receivable collection allocations against established income transactions, as described in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap). Define partial-payment limits, scoped retry identity, reversal handling, and exact outstanding totals without recording income/cash twice. Supplier payment allocations follow separately.
