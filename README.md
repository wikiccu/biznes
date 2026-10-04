# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 0 — Product & Engineering Foundation.**

The product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). The Go module, typed environment configuration, structured JSON logging, Gin HTTP server lifecycle, health/readiness endpoints, local PostgreSQL Compose environment, PostgreSQL connection pool, and explicit SQL migration workflow are implemented. Business features and CI remain planned.

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

The initial migration creates the `biznes` schema for future application objects; Goose tracks versions in `public.goose_db_version`. There are no business tables yet. Rollbacks are explicit and may affect data; the initial rollback refuses to drop a non-empty schema. See the workflow for flags, naming, adding migrations, permissions, failure recovery, and rollback validation.

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

Shutdown closes the listening socket, so new probe connections may fail instead of receiving a response. Liveness remains successful during database outages and for requests served during draining. The readiness ping observes both request cancellation and application shutdown without canceling other in-flight request contexts. Readiness can recover after a database outage without restarting the API; it verifies connectivity, not application schema. Unregistered paths, including `/`, return `404`.

Every handled request receives an `X-Request-ID`. A single supplied value is accepted if it contains 1–128 ASCII letters, digits, dots, underscores, or hyphens. Missing, duplicate, empty, or invalid values are replaced with a cryptographically random opaque ID. The ID is available as `request_id` in the Gin context and appears in the response header, request log, and recovered-panic error response.

Recovery returns a safe `500` JSON error with code `internal_error` and a request ID, without exposing panic details or stack traces. Trusted proxy headers and automatic trailing-slash redirects are disabled. There are no placeholder business endpoints.

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

## Useful commands

```text
go test ./...
go vet ./...
gofmt -l cmd internal
git diff
git diff --check
git status --short
```

`go test ./...` currently checks package compilation; no project test files exist yet. `gofmt -l cmd internal` should produce no output. Gin, pgx, Goose, and their transitive dependencies are recorded in `go.mod` and `go.sum`.

Review new untracked files directly before staging; ordinary `git diff` does not include them. Inspect the staged increment with `git diff --cached` before committing. Compose and migration commands are documented above; broader developer commands will accompany their tools.

## Next increment

Establish versioned API response, error, validation, and pagination conventions as the next increment in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap).
