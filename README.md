# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 0 — Product & Engineering Foundation.**

The product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). The Go module, minimal executable entry point, typed environment configuration, and structured JSON logging are implemented. HTTP serving, business features, third-party dependencies, database schema, Docker setup, and CI remain planned.

The blueprint is the living source of truth. Update it whenever a significant product or architecture decision changes.

## Planned stack

- Go and Gin for a versioned REST API.
- PostgreSQL as the primary source of truth, with explicit migrations.
- Docker and Docker Compose for the local environment.
- A modular monolith; add infrastructure and domain modules when an implemented requirement needs them.

The Go module uses `github.com/wikiccu/biznes` and requires Go 1.27.1 or newer. Additional libraries will be selected during their respective implementation steps after checking current stable releases. Redis, AI providers, object storage, and background workers are future capabilities.

## Local development

Install [Go 1.27.1 or newer](https://go.dev/dl/) and Git. From the repository root, run the minimal entry point:

```text
go run ./cmd/api
```

It loads and validates configuration, emits an `INFO` initialization log with the selected HTTP port when the configured level permits it, and exits successfully. The entry point does not start an HTTP server yet. No database or Docker setup is required for this increment.

### Configuration

Configuration comes from the process environment. [`.env.example`](.env.example) lists the supported variables; env files are not loaded automatically. Local `.env` and `.env.*` files are ignored by Git, with `.env.example` retained as the committed example.

| Variable | Default when absent | Validation |
| --- | --- | --- |
| `BIZNES_HTTP_PORT` | `8080` | Integer from `1` through `65535`. Empty, malformed, and out-of-range values fail startup with exit code `1`. |
| `BIZNES_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` (case-insensitive). Empty, whitespace-padded, and unsupported values fail startup with exit code `1`. |

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

The application uses standard-library `log/slog` to write one JSON object per line to stderr. Records include `time`, `level`, `msg`, and `service: "biznes"`; initialization adds `http_port`. Stdout remains available for application output.

The configured log level is the minimum severity: `warn` and `error` suppress the `INFO` initialization event. Startup configuration failures always emit an `ERROR` record and exit with code `1`, even when log-level configuration is invalid. Errors use fixed messages without recording supplied values or the full configuration.

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

`go test ./...` currently checks package compilation; no test files exist yet. `gofmt -l cmd/api internal` should produce no output. The module uses only the standard library, so there is no `go.sum` yet.

Review new untracked files directly before staging; ordinary `git diff` does not include them. Inspect the staged increment with `git diff --cached` before committing. Compose, migration, and other commands will be documented when their tools exist.

## Next increment

Add the Gin HTTP server lifecycle, including timeouts, graceful shutdown, request IDs, recovery, and request logging, as the next increment in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap).
