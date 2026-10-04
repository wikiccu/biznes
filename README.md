# biznes

**biznes** is an AI-powered operating intelligence system for small and medium businesses. It will help owners record financial activity, understand their business, anticipate cash shortages, and eventually authorize actions through a trusted business assistant.

The initial market is Iran, with planned Persian, Toman/Rial, Jalali date, and local workflow support. The core architecture will remain suitable for other markets. Source code, entity names, and documentation use English.

## Project status

**Current phase: Phase 0 — Product & Engineering Foundation.**

The first documentation increment is complete: the product vision, architecture direction, and full roadmap are recorded in [docs/PRODUCT_BLUEPRINT.md](docs/PRODUCT_BLUEPRINT.md). No application code, business features, dependencies, database schema, Docker setup, or CI configuration have been implemented.

The blueprint is the living source of truth. Update it whenever a significant product or architecture decision changes.

## Planned stack

- Go and Gin for a versioned REST API.
- PostgreSQL as the primary source of truth, with explicit migrations.
- Docker and Docker Compose for the local environment.
- A modular monolith; add infrastructure and domain modules when an implemented requirement needs them.

Runtime versions and additional libraries will be selected during their respective implementation steps after checking current stable releases. Redis, AI providers, object storage, and background workers are future capabilities.

## Local development

This checkout currently contains documentation only. There is no server to run, database to migrate, or dependency installation step yet. Reading and editing the documentation requires Git and a text editor; Go and Docker will be needed as the foundation is implemented.

From the repository root, review the current state:

```text
git status
git branch --show-current
```

Development takes place on `main`. This project-specific workflow supersedes the general `stage`/feature-branch workflow. Preserve existing work, make one reviewable increment at a time, and leave committing and integration to the human developer.

Once the first commit exists, inspect recent history with:

```text
git log --oneline -n 10
```

## Useful commands

```text
git diff
git diff --check
git status --short
```

Git does not include untracked files in ordinary diffs. For this initial increment, review the new documents directly in an editor; after they are staged by the human developer, `git diff --cached` will show them.

Application, formatting, validation, Compose, and migration commands will be documented when those tools exist. The planned foundation includes `GET /health` and prepares for `/api/v1/...` without placeholder business endpoints.

## Next increment

After human review of these documents, initialize the Go module and minimal application entry point. Continue Phase 0 in the small steps listed in the [blueprint roadmap](docs/PRODUCT_BLUEPRINT.md#development-roadmap), completing and reviewing each step before starting the next.
