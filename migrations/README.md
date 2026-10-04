# Database migrations

Use the repository's `cmd/migrate` executable, backed by [Goose v3.26.0](https://github.com/pressly/goose/releases/tag/v3.26.0). This stable version supplies context-aware migrations and PostgreSQL advisory locking without upgrading the API's existing dependencies. The library version and checksums are pinned in `go.mod` and `go.sum`; a separately installed Goose CLI is unnecessary.

## Running migrations

Start PostgreSQL and set `BIZNES_DATABASE_URL` in the process environment as described in the root README. The command reuses the application's configuration validation and pgx pool, including native connection-string options, TLS, and the startup connection deadline. All shared configuration settings must be valid. It does not load `.env` or start HTTP.

Run from the repository root:

```text
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
```

- `up` applies every pending migration in version order. Repeating it after success is a successful no-op.
- `down` rolls back exactly the most recently applied migration. At version zero it is a successful no-op. Run `go run ./cmd/migrate down` only when you intend that rollback and have reviewed its SQL and data effects.
- `status` prints each migration's version and `pending`/`applied` state. It can initialize Goose's history table on a fresh database, but does not apply application migrations.

Flags come before the command. `-dir` defaults to `migrations` relative to the working directory; `-timeout` defaults to `5m` and must be positive. For example:

```text
go run ./cmd/migrate -dir migrations -timeout 30s up
```

The deadline covers connection setup, lock waiting, and SQL work. Ctrl+C/SIGTERM also cancels this work. Goose releases locks with a separate cleanup context, and pgx closes canceled connections separately, so cleanup can extend beyond the work deadline. Use a direct PostgreSQL connection or session pooling for migrations; transaction-pooling proxies cannot preserve session advisory locks.

Database operations emit JSON records to stderr at INFO; failures emit ERROR and exit with code `1`. Invalid command usage exits with code `2` in the built executable (`go run` reports its own nonzero exit). Migration errors omit raw SQL, driver errors, and connection strings; a failed SQL migration identifies its version when available. Inspect that migration and the database state to diagnose a failure. Commands never accept credentials as command-line arguments.

## Schema and history

`00001_create_biznes_schema.sql` creates the `biznes` namespace for application objects. Future migrations and queries should qualify application objects as `biznes.<name>`. No business tables exist yet. The migration fails if that schema already exists, so an existing schema is not silently adopted. Its rollback uses `DROP SCHEMA ... RESTRICT`, which refuses to remove a non-empty schema.

Goose owns `public.goose_db_version`, independently of the connection's `search_path`. Its PostgreSQL session advisory lock serializes migration execution by commands using this workflow. Leave history management to Goose. Each SQL migration runs in a transaction by default, with its version recorded atomically; an error rolls back that migration. Earlier successful migrations in the same `up` run remain applied.

Migrations run explicitly before application deployment; API startup never migrates or synchronizes schema. `/ready` continues to check connectivity, not schema version. Use migration credentials with DDL/history-table permissions; a deployed API role should receive only the access needed by implemented features. This increment does not provision deployment roles.

## Adding a migration

Create the next file directly in this directory using a unique, increasing five-digit version and descriptive English snake_case name, for example `00002_create_users.sql`. Keep the same numbering convention and resolve version collisions before applying migrations. Add meaningful SQL under the Goose annotations:

```sql
-- +goose Up
-- SQL that implements the actual schema change.

-- +goose Down
-- SQL that safely reverses that change, or an explicit failure if irreversible.
```

Replace the comments with the actual change before committing. Do not edit, renumber, or delete migrations after they have been applied to a shared database; add a corrective migration instead. Goose tracks versions without checksumming SQL contents. Do not insert missing earlier versions into applied history or enable out-of-order execution.

Follow [Goose's SQL annotations](https://pressly.github.io/goose/documentation/annotations/) for functions containing internal semicolons (`StatementBegin`/`StatementEnd`). Use `NO TRANSACTION` only when PostgreSQL requires it, such as a concurrent index operation, and document partial-failure recovery. Avoid environment substitution for schema SQL or secrets.

Validate a new migration against an isolated disposable PostgreSQL database: inspect pending status, apply it, inspect schema and data, apply again, review and exercise its rollback where safe, and reapply. Exercise a failure and verify that schema and history agree. Never use a production database for rollback validation or automatically drop data to repair migration history.
