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

Rollback locks transactions and refuses any stored records. An empty rollback drops only transactions/indexes and the two new reference constraints with `RESTRICT`, retaining all prior domain data and migration history. These operations and version recording are transactional; dependencies or failures leave the schema/history intact. Never delete production financial records to force rollback. Use explicit corrective migrations and linked reversal records below for financial corrections.

## Transaction reversals

`00009_create_transaction_reversals.sql` creates `biznes.transaction_reversals` with native IDs, composite tenant/resource keys, a required original transaction reference, required UUID idempotency key, bounded non-empty/control-free reason, authenticated creator reference, and database-default creation time. A composite restricted FK to `transactions(organization_id, id)` prevents cross-tenant links and deletion/rekeying of referenced originals. The restricted creator FK and its index preserve provenance. The table stores no amount or duplicate financial classification and never rewrites the original record.

Unique `(organization_id, transaction_id)` permits only one full reversal; unique `(organization_id, idempotency_key)` protects retry identity across replicas. Both constraints are native and the application reads a committed winner in a fresh statement after INSERT/ON CONFLICT. Identical target/key/trimmed-reason input returns that reversal; changed input or a second key conflicts. Recording and reversal keys have independent operation namespaces. Existing transaction rows remain intact when applying this migration; current corrected recognition excludes originals with a linked full reversal.

The API needs transaction_reversals SELECT/INSERT, transactions SELECT, and existing identity/membership permissions for authentication/locking. Do not grant reversal UPDATE/DELETE or original transaction UPDATE/DELETE to the API role. Reversal creation does not need mutation permission on the original; it inserts a linked record and holds membership through commit. No triggers, new dependency/configuration, balance cache, or financial amount arithmetic are introduced.

Rollback locks reversals and refuses populated storage. Empty rollback drops only reversals/indexes with `RESTRICT`, preserving original transactions, every prior domain's data, and migration history. No prior migration is modified. Never delete production reversal/original records to make rollback succeed; use a deliberate corrective migration.

## Receivables

`00010_create_receivables.sql` introduces immutable API `biznes.receivables` with native UUID defaults, composite organization/resource keys, restricted organization/contact/creator FKs, and unique `(organization_id, idempotency_key)`. Amounts are positive BIGINT whole-Rial units with required IRR currency; `due_date` is a finite native DATE within Gregorian years 0001–9999. Description has the same 2000-character/LF/CR/tab policy as transactions. Database-default `created_at` and authenticated `created_by` preserve provenance. Indexes support stable tenant/creation-time/ID listing and contact/creator FK lookups. There is no amount arithmetic, account link, balance/status column, or updated timestamp.

The composite contact FK protects organization ownership, including direct SQL writes. The application locks the selected contact for customer/both eligibility while committing a new receivable. Classification is deliberately not part of the FK: later contact edits may reclassify it, while historical receivables and matching retries remain valid. The API needs receivables SELECT/INSERT, contacts SELECT and existing contact UPDATE permissions for `FOR SHARE`, plus current identity/session/membership permissions. A narrow finance role can grant contact UPDATE on kind for the row lock; no receivable UPDATE/DELETE is needed. Native uniqueness plus a fresh winner read protects retry identity across replicas/restarts; identical payloads preserve the first creator/time and changed payloads conflict.

Apply the migration explicitly before deploying receivable routes. Existing contacts/transactions/reversals remain unchanged; no prior migration, dependency, or environment setting is modified. Creating receivables never writes cash transactions or recorded account totals. Payment allocation, settlements, corrections, and overdue calculations follow separately.

Rollback takes an ACCESS EXCLUSIVE lock and refuses populated receivables. Empty rollback drops only this table/indexes with RESTRICT, retaining all earlier data and Goose history. Schema changes and version recording are transactional, including failures and dependency protection. Never delete production obligations to force rollback.

## Payables

`00011_create_payables.sql` introduces `biznes.payables` using the receivable storage invariants: native UUID defaults, composite tenant/resource keys, restricted organization/contact/creator FKs, positive BIGINT whole-Rial amounts with explicit IRR currency, finite Gregorian DATE within years 0001–9999, bounded descriptions, server creation time, and stable/contact/creator indexes. The organization/key unique constraint is independent of receivable/transaction/reversal namespaces. Existing migrations and stored records remain unchanged; applying 00011 supports populated contacts, transactions, reversals, and receivables.

The application locks the selected tenant contact and requires supplier/both classification for new records. Later reclassification preserves stored obligations and matching retries; classification remains outside the FK, while PostgreSQL enforces tenant ownership and restricted deletion/rekeying. The API needs payables SELECT/INSERT, contact SELECT and existing UPDATE permission for row locking, plus current identity/session/membership permissions. Do not grant payable UPDATE/DELETE to the API role; it records debt without writing expense or cash transactions. Shared fields/validation/scanning/DTO formatting preserve receivable behavior, and each resource retains concrete scoped SQL.

Apply 00011 explicitly before deploying payable routes. Rollback locks payables in ACCESS EXCLUSIVE mode and refuses populated storage; empty rollback drops only payables/indexes with RESTRICT, retaining every earlier domain and atomic Goose history. Never remove production debt to force rollback. No dependency or configuration setting is introduced. Payment allocations and exact outstanding totals are implemented in 00013 below; corrections and overdue rules remain separate increments.

## Receivable collections

`00012_create_receivable_allocations.sql` adds immutable receipt-to-obligation links with tenant/resource keys, an independent organization/idempotency key, positive BIGINT whole-Rial amounts, derived IRR currency, authenticated creator/server time, and indexes for stable debt history, receipt capacity, and creator references. New unique reference keys on receivables (organization/id/currency) and transactions (organization/id/kind/currency) support restricted composite foreign keys. A constrained internal `transaction_kind: income` column ensures allocation receipts are income transactions with matching tenant/currency; it is not user input or a public DTO field. Applying these uniqueness constraints builds indexes and locks populated tables; schedule the explicit migration appropriately.

Native constraints protect reference/row invariants. Aggregate receipt/debt capacities and current reversal eligibility are enforced by the application's fixed lock order: current membership, transaction-level receipt recognition advisory lock, then receivable advisory lock, followed by fresh READ COMMITTED statements. Receipt reversals acquire the same receipt lock. [PostgreSQL advisory locks](https://www.postgresql.org/docs/18/explicit-locking.html#ADVISORY-LOCKS) are cooperative and transaction-scoped; direct privileged SQL must follow the same rules. Lock identifiers use `hashtextextended('biznes:transaction:' || organization_id::uuid::text || ':' || transaction_id::uuid::text, 0)` and the analogous `biznes:receivable:` prefix. All allocation/reversal writers must use this protocol before enabling allocation traffic; older reversal writers do not participate. No UPDATE grant on immutable financial tables is required.

Apply 00012 explicitly, grant allocation SELECT/INSERT plus required existing debt/transaction/reversal reads, and upgrade the writers together. Financial allocations retain originals and never create duplicate income/cash. A full receipt void reopens every linked receivable through derived recognition, preserving allocation rows and retry identity. Summary rejects a privileged over-collected obligation rather than returning negative outstanding. The API exposes no allocation UPDATE/DELETE; keep those privileges away from its role. No dependency or configuration setting is added.

Rollback takes an ACCESS EXCLUSIVE allocation-table lock and refuses populated history, including allocations whose receipts were voided. Empty rollback drops only allocations/indexes and the two added reference constraints, preserving earlier schema/data and atomic Goose history. Do not delete production allocations to force rollback; use a corrective migration.

## Supplier payments

`00013_create_payable_allocations.sql` adds immutable expense-payment links to supplier obligations with tenant/resource keys, an independent organization/idempotency key, positive BIGINT whole-Rial amounts, derived IRR currency, creator/server time, and stable payable-history/payment/creator indexes. A new payable (organization/id/currency) unique reference key and migration 00012's existing transaction (organization/id/kind/currency) key support restricted composite FKs. Internal `transaction_kind` defaults to and is constrained to `expense`; it is not public input. The added unique constraint builds an index and locks populated payables, so schedule the explicit migration appropriately. No applied migration is edited.

Aggregate payment/debt limits and current void eligibility reuse the application protocol: membership lock, transaction recognition advisory lock, payable advisory lock, then fresh READ COMMITTED retry/eligibility/capacity statements. Expense reversals already take the shared transaction lock introduced with receivable allocations. Payable debt keys use `hashtextextended('biznes:payable:' || organization_id::uuid::text || ':' || payable_id::uuid::text, 0)`. All financial writers and privileged direct SQL must honor those cooperative locks and checks. The API needs allocation SELECT/INSERT plus existing financial reads/session/membership permissions, without UPDATE/DELETE on immutable financial records. No new reversal protocol, dependency, or configuration setting is introduced.

Apply 00013 before enabling payable allocation routes. Full expense voids reopen linked debt through derived recognition while preserving original obligations, transactions, allocation rows, and retry provenance. Over-paid obligations fail closed instead of returning negative outstanding. Allocation creation never inserts duplicate expense/cash. Concrete allocation fields, input validation, and scanning are shared with receivables; queries and public debt/collected/paid field names remain explicit.

Rollback takes an ACCESS EXCLUSIVE allocation-table lock and refuses any stored history, including voided-payment allocations. Empty rollback drops only payable allocations/indexes and the new payable reference constraint. It preserves migration 00012's transaction reference key, receivable allocations, and all earlier data/history. Failure rolls back schema/history atomically. Do not remove production allocations to force rollback; use a corrective migration.

## Outgoing payable instruments

`00014_create_payable_instruments.sql` adds immutable API `biznes.payable_instruments` records for outgoing checks linked to existing payables. Native UUID defaults, tenant/resource keys, an independent organization/idempotency key, fixed `check` kind, positive BIGINT whole-Rial IRR amounts, finite DATE years 0001–9999, bounded bank/reference/description text, creator/time and stable/FK indexes follow the existing patterns. A restricted composite FK reuses `payables(organization_id,id,currency)` from 00013. The payable supplies the contact association; no copied contact/account or duplicated underlying obligation is stored. Direction is outgoing; current recorded/canceled status is derived through the separate 00016 cancellation history below. Original metadata stays immutable.

Apply the migration before deploying the routes. Existing populated domains, migrations and financial reporting queries remain intact. API recording validates each amount against its original payable amount; that application cross-row rule is not a native aggregate capacity constraint. Instruments do not reserve current outstanding or alter settlement/cash. Multiple instruments and instruments on fully paid payables remain intent/history, with no claim of bank verification or clearance.

Grant instrument SELECT/INSERT, payable SELECT for new records, and existing identity/session/membership permissions including membership UPDATE for row locking on record/list. A column grant on membership role suffices for that lock. No instrument/payable UPDATE/DELETE, contact lock, allocation, reversal, transaction or financial account grant is needed. Current membership locks, native retry uniqueness and fresh winner reads preserve role/tenant/retry/provenance boundaries across replicas. Privileged direct SQL must retain application invariants.

Rollback locks the instrument table and refuses any stored records, retaining schema/history atomically. Empty rollback drops only the new table/indexes with RESTRICT, preserving all prior data and reference constraints. Do not remove production instruments to force rollback; use a corrective migration. No dependency or setting changes are required. Incoming instruments use the separate 00015 migration below; cancellation and outgoing settlement use the later migrations below.

## Incoming receivable instruments

`00015_create_receivable_instruments.sql` introduces immutable API `biznes.receivable_instruments` with the same native row/retry/index/rollback invariants as outgoing instruments. Its restricted composite FK uses `receivables(organization_id,id,currency)` from 00012; no prior migration, constraint or outgoing table is changed. Direction is incoming; current recorded/canceled status derives from separate 00017 cancellation history. Contact association follows the linked receivable. Apply 00015 explicitly before enabling incoming routes; upgrades retain populated outgoing and earlier financial records.

Grant incoming instrument SELECT/INSERT, receivable SELECT for new records, and existing identity/session/membership permissions including membership row-lock permission for record/list. No outgoing-table/payable permission or financial mutation grant is needed. Shared concrete fields, validation/scanning and HTTP handling preserve family-specific JSON fields, while separate SQL and retry namespaces keep identical IDs/keys independent.

Application recording bounds each instrument to original receivable amount without aggregate reservation or settlement. Native constraints enforce positive whole-Rial IRR amounts, kind/date/text/reference invariants and restricted ownership/creator links; privileged direct SQL must preserve cross-row application rules. Incoming intent never creates duplicate receivable/income/cash or collection allocations.

Rollback takes an ACCESS EXCLUSIVE lock and refuses populated incoming instrument history, preserving schema/history atomically. Empty rollback drops only the new table/indexes with RESTRICT and retains outgoing instruments, receipt collections and all earlier data/reference keys. Do not delete production records to force rollback; use a corrective migration. No dependency or configuration setting is added.

## Outgoing instrument cancellations

`00016_create_payable_instrument_cancellations.sql` appends cancellation intent/history without altering instrument or financial rows. Composite organization/resource keys, restricted instrument/creator references, independent organization/key uniqueness, one cancellation per organization/instrument, required bounded nonblank/control-free reason, native creation time and a creator index follow the reversal pattern. No amount or duplicate debt/cash data is stored. The instrument unique key supports indexed status lookups.

Apply 00016 and grant cancellation SELECT to updated outgoing readers before deploying them. Cancellation writers additionally require cancellation INSERT and instrument SELECT, plus existing identity/session/membership locking permissions. Original instrument writers retain their existing insert/parent-read permissions. No cancellation/instrument UPDATE/DELETE or other financial mutation grant is needed. Membership locks and native uniqueness with fresh winner reads preserve authorized retry/provenance under races; cancellation is intent-only and does not participate in transaction recognition. Since 00018, outgoing cancellation writers additionally need settlement SELECT and share the instrument lock/history exclusion described below.

Outgoing get/list/record retries now derive recorded/canceled status with a same-tenant EXISTS in their statement snapshot. Incoming status uses separate 00017 history and requires no outgoing-cancellation access. Upgrade all readers before enabling cancellation traffic; older binaries always report recorded. Missing schema or SELECT fails closed instead of reporting fabricated status.

Rollback locks cancellation storage and refuses any stored history. Empty rollback drops only the cancellation table/indexes with RESTRICT, preserving original instruments and every prior financial domain/reference key. Roll back updated readers before dropping their required table. Failures retain schema/history atomically; do not remove production cancellations to force rollback. Bank cancellation and correction/undo remain separate workflows; incoming cancellation uses 00017 and outgoing settlement uses 00018 below. No dependency or setting is added.

## Incoming instrument cancellations

`00017_create_receivable_instrument_cancellations.sql` adds immutable incoming cancellation history with restricted incoming-instrument/creator links, composite tenant/resource keys, native one-per-target and organization/key uniqueness, bounded nonblank/control-free reason, creation time and creator index. It mirrors 00016's storage invariants without editing prior migrations or outgoing tables. Shared concrete input/fields/scanning and HTTP handling preserve the two families' contract; queries and key namespaces remain separate.

Apply 00017 and grant incoming cancellation SELECT before deploying updated incoming readers. Writers additionally require incoming cancellation INSERT and incoming instrument SELECT, plus existing identity/session/membership locking permissions. Original record writers retain their existing insert/parent-read grants. No outgoing-table or financial mutation permission is needed. Upgrade incoming readers before enabling cancellation traffic; earlier readers report recorded rather than canceled for incoming records. Missing/denied storage fails closed with safe 503 while outgoing history/status remains independent.

Incoming get/list/record retries now derive recorded/canceled from same-tenant incoming cancellation EXISTS in their statement snapshot. Metadata, receipts, collection allocations, reversals and financial reports remain unchanged. Since 00019, incoming cancellation writers also require incoming settlement SELECT and share its instrument lock/history exclusion described below. Recording intent establishes no bank, custody, deposit or cash effect.

Rollback takes an ACCESS EXCLUSIVE lock and refuses populated cancellation history, preserving schema/history atomically. Empty rollback drops only incoming cancellation storage/indexes with RESTRICT and preserves outgoing cancellations, all original instruments and financial domains. Coordinate reader rollback before dropping their required table. Do not remove production history to force rollback; use a corrective migration. No dependency or setting is added.

## Outgoing instrument settlement links

`00018_create_payable_instrument_settlements.sql` adds immutable link history between outgoing checks and existing payable allocations. It adds unique `(organization_id,id,payable_id,currency)` reference keys to both parent tables, then restricted composite FKs from each link to enforce matching tenant/payable/currency. Positive BIGINT IRR amounts, native ID/time defaults, creator references, organization/key uniqueness, instrument/creation/ID pagination and allocation/creator indexes follow existing patterns. No prior migration or parent values are edited.

The API enforces aggregate historical capacities for each check and allocation using exact NUMERIC sums. It locks membership, then the payment-recognition key shared with allocations/full voids, then the instrument key shared with outgoing cancellation. Fresh statements after waits enforce capacity and cancellation exclusion. Links survive expense voids and keep their historical capacity; voids still reopen debt solely through existing allocation recognition. Outgoing cancellation rejects any stored link history. Native constraints do not enforce aggregate/lifecycle rules: privileged SQL must cooperate with the same locks and invariants.

Apply schema/grants before deploying writers. Settlement writers require link SELECT/INSERT, instrument/allocation/cancellation/reversal SELECT and existing authentication/membership locking permissions. Outgoing cancellation writers additionally require settlement SELECT. Link reads need SELECT and existing authentication; list also reads instruments and locks membership. No financial UPDATE/DELETE or dependency/configuration change is required. Upgrade or drain all older outgoing cancellation writers before settlement traffic; they lack the shared lock/history check. Incoming SQL/status remains independent.

Rollback locks link storage and refuses populated history atomically. Empty rollback drops only the new table/indexes and the two added parent reference constraints with RESTRICT, preserving populated original records and existing references. Roll back dependent writers before dropping their required storage; never delete production history to force rollback.

## Incoming instrument settlement links

`00019_create_receivable_instrument_settlements.sql` mirrors 00018 for incoming checks and receivable collection allocations. Added unique `(organization_id,id,receivable_id,currency)` keys on both incoming parents support restricted composite FKs enforcing shared tenant/receivable/currency. Native positive BIGINT IRR amounts, IDs/time/creator history, organization/key uniqueness and instrument/allocation/creator indexes follow existing storage patterns. Prior migration files, outgoing settlement history and financial values remain intact.

Exact NUMERIC sums and cooperating application locks bound historical amounts to both original check and allocation capacity. Lock order is membership, existing receipt-recognition key, incoming instrument key; incoming cancellation shares the last lock and rejects any settlement history. Full receipt voids preserve history/capacity while existing collection recognition reopens debt. Privileged SQL must follow the same aggregate/lifecycle rules and lock order.

Apply 00019 and grants before deploying. Incoming settlement writers require incoming link SELECT/INSERT, incoming instrument/allocation/cancellation/reversal SELECT and existing authentication/membership locks. Updated incoming cancellation writers also require incoming link SELECT. Link get needs only link/membership SELECT plus authentication; list also reads instruments and locks membership. Outgoing storage, financial UPDATE/DELETE and new dependencies/settings are unnecessary. Upgrade or drain all older incoming cancellation writers before enabling incoming settlement traffic.

Rollback locks the new table and refuses populated link history atomically. Empty rollback drops only that table/indexes and its added reference keys with RESTRICT, retaining populated original parents, outgoing links and all previous references. Coordinate dependent writer rollback before dropping the required table; never delete production history to force rollback.

## Account opening balances

`00020_create_account_opening_balances.sql` adds one immutable opening per organization/account, organization-scoped retry-key uniqueness, native identity, signed whole-Rial `BIGINT` amount (including zero and both signed bounds), explicit IRR, finite UTC-range `effective_at`, bounded/control-free source reason and creator/time provenance. A restricted composite account/currency FK reuses the existing reference key; the creator FK has an index. Populated accounts/history receive no defaults or financial rewrites. Opening means immediately before `effective_at`; subsequent activity begins inclusively by occurred time, independently of creation time. Existing recognition is unchanged.

Apply before deploying the routes. The API needs account_opening_balances SELECT/INSERT, financial_accounts SELECT, and existing identity/membership grants (including membership UPDATE for write row locks). Reads need no financial writes or membership locks. Withhold opening UPDATE/DELETE: API/privileges enforce immutability, without a trigger. Unique constraints plus a fresh read after INSERT contention preserve retries without account/advisory locks. No dependency, setting or transaction-writer upgrade is needed. Rollback locks the new table and refuses any stored opening, retaining schema/data/history; empty rollback drops only this table/indexes with RESTRICT. Never delete financial history to force rollback or repair an opening; corrections need a future explicit workflow.

## General financial obligations

`00021_create_financial_obligations.sql` creates standalone outgoing commitment records with native UUID identity, a composite tenant/resource key, organization-scoped retry-key uniqueness, bounded/control-free title, positive whole-Rial BIGINT amount, explicit IRR, finite date-only due date, bounded multiline description and creator/time provenance. Restricted organization/creator references and indexes preserve ownership/history; creation-time/ID indexing supports stable listing. Titles are not unique. The table has no account/contact/instrument dependencies or financial recognition effects. Applying it to populated prior domains adds no defaults or financial rewrites.

Deploy this explicit migration before the new routes. The API needs financial_obligations SELECT/INSERT plus existing identity/membership access, including membership UPDATE for write/list row locks. Single GET needs only financial SELECT and current membership. Withhold obligation UPDATE/DELETE; API and privileges enforce immutability without a trigger. Retry handling uses native INSERT/ON CONFLICT plus a fresh read, without extra advisory locks or other financial-domain permissions. Rollback locks the table and refuses any stored commitment, retaining schema/data/history; empty rollback drops only this table/indexes with RESTRICT. Never remove production records to force rollback. No dependency, setting or existing writer upgrade is introduced.

## Adding a migration

Create the next file directly in this directory using a unique, increasing five-digit version and descriptive English snake_case name, for example `00022_add_finance_indexes.sql`. Keep the same numbering convention and resolve version collisions before applying migrations. Add meaningful SQL under the Goose annotations:

```sql
-- +goose Up
-- SQL that implements the actual schema change.

-- +goose Down
-- SQL that safely reverses that change, or an explicit failure if irreversible.
```

Replace the comments with the actual change before committing. Do not edit, renumber, or delete migrations after they have been applied to a shared database; add a corrective migration instead. Goose tracks versions without checksumming SQL contents. Do not insert missing earlier versions into applied history or enable out-of-order execution.

Follow [Goose's SQL annotations](https://pressly.github.io/goose/documentation/annotations/) for functions containing internal semicolons (`StatementBegin`/`StatementEnd`). Use `NO TRANSACTION` only when PostgreSQL requires it, such as a concurrent index operation, and document partial-failure recovery. Avoid environment substitution for schema SQL or secrets.

Validate a new migration against an isolated disposable PostgreSQL database: inspect pending status, apply it, inspect schema and data, apply again, review and exercise its rollback where safe, and reapply. Exercise a failure and verify that schema and history agree. Never use a production database for rollback validation or automatically drop data to repair migration history.
