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

`00001_create_biznes_schema.sql` creates the `biznes` namespace for application objects. Migrations and queries should qualify application objects as `biznes.<name>`. The migration fails if that schema already exists, so an existing schema is not silently adopted. Its rollback uses `DROP SCHEMA ... RESTRICT`, which refuses to remove a non-empty schema.

Goose owns `public.goose_db_version`, independently of the connection's `search_path`. Its PostgreSQL session advisory lock serializes migration execution by commands using this workflow. Leave history management to Goose. Each SQL migration runs in a transaction by default, with its version recorded atomically; an error rolls back that migration. Earlier successful migrations in the same `up` run remain applied.

Migrations run explicitly before application deployment; API startup never migrates or synchronizes schema. `/ready` continues to check connectivity, not schema version. Use migration credentials with DDL/history-table permissions; a deployed API role should receive only the access needed by implemented features. This increment does not provision deployment roles.

## User persistence

`00002_create_users.sql` creates global identities in `biznes.users`: UUIDv4 `id` defaults from PostgreSQL's built-in `gen_random_uuid()`, canonical unique `email`, encoded `password_hash`, and required `created_at`/`updated_at` instants with transaction timestamp defaults. No extension or Go UUID dependency is required. Business ownership will be represented through memberships, not a tenant column on users.

Email uses deterministic `C` collation and requires lowercase printable ASCII, 3–254 bytes, and exactly one `@` with non-empty parts. These are storage invariants, not a complete email parser. Registration must normalize and validate input before writing, preserve dots and plus tags, and handle uniqueness conflicts. The password verifier is required, printable ASCII, and bounded to 1024 bytes; trusted server code must generate a secure encoded hash. This migration does not implement hashing or authenticate users. Update statements must explicitly maintain `updated_at` when profile/credential mutation is introduced.

The users rollback takes an `ACCESS EXCLUSIVE` table lock inside Goose's transaction, checks for stored users, and refuses to proceed if any exist. This prevents a concurrent insert from slipping between the check and table removal. Empty-table rollback uses `DROP TABLE ... RESTRICT` to retain dependency protection. Failure leaves users and migration history intact. Never delete production users to make rollback succeed; use a deliberate corrective migration instead.

## Sessions

`00003_create_sessions.sql` creates `biznes.sessions` for durable bearer authentication. `token_hash` is a 32-byte SHA-256 digest primary key, `user_id` references global users with `ON DELETE RESTRICT`, and required creation/last-use/expiry instants have chronological checks. Only token digests are stored. The application supplies the eight-hour expiry and enforces fifteen minutes of idle time at lookup; neither lifetime is a database default or an automatic deletion rule.

The expiry index supports explicit maintenance, for example by an authorized deployment maintenance job:

```sql
DELETE FROM biznes.sessions WHERE expires_at <= CURRENT_TIMESTAMP;
```

Expiry is enforced even before maintenance. Idle-expired rows can remain until absolute expiry, and no cleanup worker is introduced. Schedule this maintenance in deployments to prevent expired rows accumulating. Logout deletes only the current session. The API requires users SELECT/INSERT and sessions SELECT/INSERT/UPDATE/DELETE permissions; membership/organization permissions remain separate.

Session rollback takes an `ACCESS EXCLUSIVE` table lock and refuses to drop any stored session rows, including expired ones. Failure retains both schema and history; empty rollback drops only the sessions table/index and preserves users. Never delete active production sessions automatically to force rollback. Apply migrations before starting the updated API, and use a deliberate corrective migration when a populated schema must change.

## Organizations and memberships

`00004_create_organizations.sql` creates `biznes.organizations` with native UUID defaults, required 1–120 character names without control characters, and creation/update timestamps. Application validation additionally trims Unicode whitespace and rejects blank names. Names are not unique. `biznes.memberships` links an organization and a global user through a composite primary key, restricted foreign keys, a required `owner`/`admin`/`accountant`/`staff` role, and a creation timestamp. The reverse `(user_id, organization_id)` index supports the authenticated user's organization list.

The concrete organization service creates an organization and its initial owner membership in one transaction. Reads join membership; rename locks and checks the caller's membership before updating. The API needs organizations SELECT/INSERT/UPDATE and memberships SELECT/INSERT plus UPDATE permission for PostgreSQL row locking. Application SELECT/UPDATE statements must keep their membership predicates; possessing an ID is not authorization. Later member-management operations must preserve at least one owner; no such endpoint exists now.

Rollback locks both tables and refuses removal if either contains rows. An empty rollback drops memberships before organizations with `RESTRICT`, preserving users and sessions. Failure leaves schema, data, and Goose history intact. Never delete production business data to force rollback. Apply the migration before deploying the organization routes.

## Contacts

`00005_create_contacts.sql` creates `biznes.contacts`, with a required organization reference (`ON DELETE RESTRICT`) and native UUID defaults under a composite `(organization_id, id)` primary key. This preserves tenant scope in the resource key and later references. Required names, customer/supplier/both classification, optional email/phone/notes (non-null strings defaulting to empty), and creation/update timestamps are explicit columns. Check constraints enforce field bounds and allowed control characters; application logic additionally trims/validates names and mailbox syntax. The `(organization_id, created_at, id)` index supports stable contact-list ordering.

The API needs contacts SELECT/INSERT/UPDATE in addition to the existing identity/organization permissions. Mutations and lists lock current membership in their transaction; single reads join it. Every data query/update includes the organization ID, and payloads cannot replace it. Rollback takes an `ACCESS EXCLUSIVE` table lock and refuses populated storage; empty rollback drops only contacts with `RESTRICT`, retaining users, sessions, organizations, memberships, and their migration history. Never remove production contacts to force rollback. Apply this migration before deploying contact routes.

## Transaction categories

`00006_create_transaction_categories.sql` introduces `biznes.transaction_categories` with a restricted organization FK, native UUID defaults, a composite `(organization_id, id)` primary key, name/kind, and creation/update timestamps. Names have 1–120 character/control constraints; kind is `income` or `expense`. A named unique constraint on `(organization_id, kind, name)` uses exact `C` collation and protects creation/rename races. A tenant/creation-time/ID index supports stable listing. Application validation trims Unicode names before writing; the API keeps kind fixed after creation.

The API needs transaction_categories SELECT/INSERT and UPDATE on name/updated_at, alongside current identity/membership permissions. Writes/lists hold the membership row lock; single reads join membership. Every data query/update includes organization scope. Owner/admin/accountant members can manage these categories; this does not broaden contact/organization write policies. Rollback locks the table and refuses stored rows; empty rollback drops only this table/indexes with `RESTRICT`, retaining prior domain data and migration history. Never remove production categories to force rollback. Apply the migration before deploying category routes. Categories do not store amounts or alter balances.

## Financial accounts

`00007_create_financial_accounts.sql` introduces `biznes.financial_accounts` with a restricted organization FK, native UUID defaults, a composite `(organization_id, id)` primary key, name/kind/currency, and creation/update timestamps. Names have 1–120 character/control constraints; kind is `cash` or `bank`, and explicit currency must be `IRR`. The named unique constraint on `(organization_id, name)` uses exact `C` collation across both kinds and protects creation/rename races. A tenant/creation-time/ID index supports stable listing. Application validation trims Unicode names; the API keeps kind/currency fixed after creation. These are financial metadata records without amounts or balances; transactions below reference the composite tenant/account key and account currency.

The API needs financial_accounts SELECT/INSERT and UPDATE on name/updated_at, alongside existing identity/membership permissions. Writes/lists lock current membership; single reads join it. Every query/update includes organization scope. Owner/admin/accountant members can manage accounts; all members can read. Rollback takes an `ACCESS EXCLUSIVE` table lock and refuses any stored rows; empty rollback drops only accounts/indexes with `RESTRICT`, preserving all prior domain data and history. Never remove production accounts to force rollback. Apply the migration explicitly before deploying account routes. Additional currencies require a later migration and explicit unit/scale semantics.

## Income/expense transactions

`00008_create_transactions.sql` creates `biznes.transactions` with composite tenant/resource keys and a unique `(organization_id, idempotency_key)`. It stores required account/category references, category-derived kind, positive `BIGINT` amount, explicit `IRR` currency, finite `occurred_at` within UTC years 1–9999, bounded description, authenticated `created_by`, and server-default creation time. Amounts are whole Rial units and descriptions permit LF/CR/tab but reject other controls. There is no balance column or updated timestamp.

The migration adds unique reference keys `(organization_id, id, currency)` on accounts and `(organization_id, id, kind)` on categories, then uses restricted foreign keys against those keys. This enforces tenant ownership and financial classification/currency consistency in PostgreSQL, including direct SQL writes. It also retains a restricted creator reference to global users. Indexes support stable tenant/creation-time/ID listing and foreign-key lookups. The migration supports existing populated accounts/categories and does not rewrite their data. Adding the reference constraints requires locks and builds indexes; schedule this explicit migration before deploying transaction routes.

The API needs transactions SELECT/INSERT, accounts/categories SELECT, and current identity/membership permissions (including membership locking). Do not grant transaction UPDATE/DELETE to the API role: current records are immutable through application routes. Native INSERT/ON CONFLICT and a fresh statement in the transaction enforce retry identity across replicas; an identical payload returns the stored record, while changed input with its key returns `409`. Direction/currency are selected from authorized organization-owned resources and provenance comes from authentication. There is no trigger, schema auto-sync, correction worker, or dependency/configuration setting.

Rollback locks transactions and refuses any stored records. An empty rollback drops only transactions/indexes and the two new reference constraints with `RESTRICT`, retaining all prior domain data and migration history. These operations and version recording are transactional; dependencies or failures leave the schema/history intact. Never delete production financial records to force rollback. Use explicit corrective migrations and future linked reversal records for financial corrections.

## Adding a migration

Create the next file directly in this directory using a unique, increasing five-digit version and descriptive English snake_case name, for example `00009_create_transaction_reversals.sql`. Keep the same numbering convention and resolve version collisions before applying migrations. Add meaningful SQL under the Goose annotations:

```sql
-- +goose Up
-- SQL that implements the actual schema change.

-- +goose Down
-- SQL that safely reverses that change, or an explicit failure if irreversible.
```

Replace the comments with the actual change before committing. Do not edit, renumber, or delete migrations after they have been applied to a shared database; add a corrective migration instead. Goose tracks versions without checksumming SQL contents. Do not insert missing earlier versions into applied history or enable out-of-order execution.

Follow [Goose's SQL annotations](https://pressly.github.io/goose/documentation/annotations/) for functions containing internal semicolons (`StatementBegin`/`StatementEnd`). Use `NO TRANSACTION` only when PostgreSQL requires it, such as a concurrent index operation, and document partial-failure recovery. Avoid environment substitution for schema SQL or secrets.

Validate a new migration against an isolated disposable PostgreSQL database: inspect pending status, apply it, inspect schema and data, apply again, review and exercise its rollback where safe, and reapply. Exercise a failure and verify that schema and history agree. Never use a production database for rollback validation or automatically drop data to repair migration history.
