# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 0 — Product & Engineering Foundation.**

The product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). The Go module and minimal executable entry point are initialized. Configuration, logging, HTTP serving, business features, third-party dependencies, database schema, Docker setup, and CI remain planned.

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

It prints `biznes` and exits successfully. The entry point does not start an HTTP server yet. No database or Docker setup is required for this increment.

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
gofmt -l cmd/api
git diff
git diff --check
git status --short
```

`go test ./...` currently checks package compilation; no test files exist yet. `gofmt -l cmd/api` should produce no output. The module uses only the standard library, so there is no `go.sum` yet.

Review new untracked files directly before staging; ordinary `git diff` does not include them. Inspect the staged increment with `git diff --cached` before committing. Compose, migration, and other commands will be documented when their tools exist.

## Next increment

Add typed, environment-based configuration and `.env.example`. Structured logging and the Gin HTTP server follow as separate increments in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap).
