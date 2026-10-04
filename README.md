# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 0 — Product & Engineering Foundation.**

The product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). The Go module, typed environment configuration, structured JSON logging, Gin HTTP server lifecycle, and health/readiness endpoints are implemented. Business features, database schema, Docker setup, and CI remain planned.

The blueprint is the living source of truth. Update it whenever a significant product or architecture decision changes.

## Planned stack

- Go and Gin for a versioned REST API.
- PostgreSQL as the primary source of truth, with explicit migrations.
- Docker and Docker Compose for the local environment.
- A modular monolith; add infrastructure and domain modules when an implemented requirement needs them.

The Go module uses `github.com/wikiccu/biznes` and requires Go 1.27.1 or newer. Gin is pinned to [v1.12.0](https://github.com/gin-gonic/gin/releases/tag/v1.12.0). Additional libraries will be selected during their respective implementation steps after checking current stable releases. Redis, AI providers, object storage, and background workers are future capabilities.

## Local development

Install [Go 1.27.1 or newer](https://go.dev/dl/) and Git. From the repository root, run the minimal entry point:

```text
go run ./cmd/api
```

It loads and validates configuration and keeps serving HTTP on port `8080` by default. Stop it with Ctrl+C; deployments can send SIGTERM. Shutdown stops accepting connections and allows in-flight requests to finish within the configured deadline, then closes remaining connections if the deadline expires. A shutdown failure exits with code `1`.

Check the running server with:

```text
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
```

Both return JSON with an `X-Request-ID` response header and `Cache-Control: no-store`. No database or Docker setup is required yet.

### Configuration

Configuration comes from the process environment. [`.env.example`](.env.example) lists the supported variables; env files are not loaded automatically. Local `.env` and `.env.*` files are ignored by Git, with `.env.example` retained as the committed example.

| Variable | Default when absent | Validation |
| --- | --- | --- |
| `BIZNES_HTTP_PORT` | `8080` | Integer from `1` through `65535`. Empty, malformed, and out-of-range values fail startup with exit code `1`. |
| `BIZNES_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` (case-insensitive). Empty, whitespace-padded, and unsupported values fail startup with exit code `1`. |
| `BIZNES_HTTP_READ_HEADER_TIMEOUT` | `5s` | Positive Go duration. |
| `BIZNES_HTTP_READ_TIMEOUT` | `15s` | Positive Go duration. |
| `BIZNES_HTTP_WRITE_TIMEOUT` | `15s` | Positive Go duration. |
| `BIZNES_HTTP_IDLE_TIMEOUT` | `60s` | Positive Go duration. |
| `BIZNES_HTTP_SHUTDOWN_TIMEOUT` | `10s` | Positive Go duration. |

Duration values use units such as `500ms`, `15s`, or `1m`. Empty, malformed, zero, negative, and overflowing durations fail startup with exit code `1`. Read/write settings are HTTP transport timeouts; they do not automatically cancel application work at that deadline.

For example, in PowerShell:

```powershell
$env:BIZNES_HTTP_PORT = '9000'
go run ./cmd/api
Remove-Item Env:BIZNES_HTTP_PORT
```

Or in a POSIX shell:

```sh
BIZNES_HTTP_PORT=9000 go run ./cmd/api
```

Configuration errors identify the variable and constraint without echoing its value. No credentials are required at this stage; additional settings will accompany the capabilities that use them.

### Logging

The application uses standard-library `log/slog` to write one JSON object per line to stderr. Records include `time`, `level`, `msg`, and `service: "biznes"`. Server lifecycle events identify listening, stopping, and stopped states. The listening event is emitted only after the port is bound successfully.

The configured log level is the minimum severity: `warn` and `error` suppress `INFO` lifecycle and ordinary request events. Requests record `request_id`, method, route template (empty for unmatched routes), status, and duration in milliseconds; responses with status `500` or higher are logged at `ERROR`. Query strings, raw URL paths, headers, bodies, and panic values are omitted from request and recovery logs.

Startup configuration failures always emit an `ERROR` record and exit with code `1`, even when log-level configuration is invalid. Errors use fixed messages without recording supplied values or the full configuration. Gin's default debug/request/recovery output is disabled in favor of structured logging.

### HTTP behavior

| Endpoint | Response | Meaning |
| --- | --- | --- |
| `GET /health` | `200` with `{"status":"ok"}` | Process liveness; does not check dependencies. |
| `GET /ready` | `200` with `{"status":"ready"}` | The HTTP application is running and its lifecycle context is active. |
| `GET /ready` during shutdown | `503` with `{"status":"not_ready"}` | Shutdown has begun; readiness fails for requests that still reach the handler while connections drain. |

Shutdown closes the listening socket, so new probe connections may fail instead of receiving a response. Liveness remains successful for requests served during draining, and readiness does not cancel in-flight request contexts. Database checks will be added to readiness when the connection layer exists. Unregistered paths, including `/`, return `404`.

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
gofmt -l cmd/api internal
git diff
git diff --check
git status --short
```

`go test ./...` currently checks package compilation; no project test files exist yet. `gofmt -l cmd/api internal` should produce no output. Gin and its transitive dependencies are recorded in `go.mod` and `go.sum`.

Review new untracked files directly before staging; ordinary `git diff` does not include them. Inspect the staged increment with `git diff --cached` before committing. Compose, migration, and other commands will be documented when their tools exist.

## Next increment

Add a PostgreSQL local development environment with Compose as the next increment in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap).
