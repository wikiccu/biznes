# biznes — Product Blueprint

| Metadata | Value |
| --- | --- |
| Status | Living Document |
| Product | biznes |
| Repository | `biznes` |
| Architecture | Modular Monolith |
| Current Phase | Phase 1 — Business Core MVP |
| Current Increment | Step 6 — Tenant-scoped transaction categories |
| Last Updated | 2026-10-07 |

This document describes the **intended final product**, its architecture direction, and an incremental path toward it. It is the source of truth for product vision, scope, feature planning, engineering decisions, and onboarding future developers and AI coding agents. Planned capabilities are not implemented capabilities.

Update this document when product direction, module boundaries, major technical choices, or roadmap priorities change. Record significant decisions in [Decisions & Changes](#decisions--changes). Features should advance this vision rather than accumulate as unrelated additions.

## Current State

The repository contains this blueprint, a concise `README.md`, the Go module `github.com/wikiccu/biznes`, the `cmd/api` executable, a separate `cmd/migrate` schema command, `dev.ps1` for developer commands, a GitHub Actions Go validation workflow, and global identity, sessions, organizations, scoped contacts, and transaction categories. Development takes place on `main`, with `origin` configured as `https://github.com/wikiccu/biznes.git`. The API verifies its PostgreSQL pool before serving HTTP, with transport timeouts, request IDs, request logging, panic recovery, context-driven graceful shutdown, process liveness, and database readiness. Shared errors return JSON `404` for unknown routes and `405` with `Allow` for unsupported methods on registered paths. `POST /api/v1/auth/register` creates a user with a securely hashed password and explicit public response fields. Login creates durable bearer sessions; current-user/logout routes authenticate and revoke them. Organization routes create owner memberships and enforce membership/role access for lists, reads, and renaming. Contact routes provide scoped creation/listing/retrieval/replacement and customer/supplier/both classification. Category routes provide income/expense classification with scoped reads, authorized creation/renaming, and race-safe name uniqueness. Business API contracts are documented without placeholder endpoints.

Implemented:

- The long-term product vision, capability map, architectural direction, and phased roadmap.
- Initial documented conventions for money, time, identifiers, APIs, tenant ownership, and development workflow.
- A README describing the current state and next development step.
- A Go module requiring Go 1.27.1 or newer and the `cmd/api` executable entry point.
- Typed configuration in `internal/platform/config`, a committed `.env.example`, and Git ignore rules for local env files. `BIZNES_HTTP_PORT` defaults to `8080` when absent and accepts integers from `1` through `65535`; an explicitly empty value is invalid. Configuration comes from the process environment without automatic env-file loading.
- Standard-library JSON logging to stderr with timestamp, severity, message, and `service: "biznes"`. `BIZNES_LOG_LEVEL` defaults to `info` and accepts case-insensitive `debug`, `info`, `warn`, and `error`; empty, whitespace-padded, and unsupported values are invalid. Configuration failures are always logged at `ERROR` without echoing supplied values or dumping configuration.
- Gin v1.12.0 and the HTTP foundation in `internal/platform/http`. Read-header, read, write, idle, and shutdown timeouts default to `5s`, `15s`, `15s`, `60s`, and `10s`; their `BIZNES_HTTP_*_TIMEOUT` environment overrides must be positive Go durations.
- Ctrl+C/SIGTERM shutdown that stops accepting connections, waits for in-flight requests within the deadline, and closes remaining connections on timeout. Startup binding, serving, and shutdown failures are reported as errors.
- Bounded, validated `X-Request-ID` propagation with cryptographically random fallback IDs, structured completion logs using route templates instead of raw URLs, and safe recovered-panic `500` JSON responses before response commitment. Gin debug output, trusted proxy headers, and automatic trailing-slash redirects are disabled.
- `GET /health` returns `200` with `{"status":"ok"}` for process liveness. `GET /ready` returns `200` with `{"status":"ready"}` when the lifecycle is active and a bounded PostgreSQL ping succeeds, or `503` with `{"status":"not_ready"}` on dependency failure, timeout, or cancellation. Readiness observes request cancellation and shutdown without canceling other request contexts. Both use `Cache-Control: no-store`; readiness checks connectivity, not schema.
- A local PostgreSQL environment in `compose.yaml`, pinned to the official `postgres:18.6-trixie` image. It publishes port `5432` on `127.0.0.1` by default, requires a non-empty development password, uses a named volume mounted at `/var/lib/postgresql`, checks TCP readiness with `pg_isready`, and allows 30 seconds for clean shutdown. `.env.example` documents Compose settings; Compose reads `.env`, while the Go API still uses only process environment configuration.
- Native pgx v5.11.0 pooling in `internal/platform/database`. `BIZNES_DATABASE_URL` is required; `BIZNES_DATABASE_CONNECT_TIMEOUT` and `BIZNES_DATABASE_HEALTH_TIMEOUT` default to `5s` and `2s`, and must be positive durations. Native connection-string options configure the pool budget and lifecycle; invalid pool intervals/minimums fail startup safely. Startup verifies connectivity before HTTP binds. The pool remains available while HTTP drains and closes on success or HTTP failure. pgx may spend approximately 15 additional seconds cleaning up canceled connections to an unresponsive database. Connection strings, credentials, and raw driver errors are omitted from logs and probe responses.
- An explicit SQL migration command in `cmd/migrate`, using Goose v3.26.0 and the existing pgx pool/configuration. It supports `up`, one-step `down`, and `status`, with PostgreSQL session advisory locking, transactional SQL/version recording, a positive work deadline (default `5m`), and Ctrl+C/SIGTERM cancellation. Cleanup can extend beyond the work deadline. Errors omit raw SQL and credentials; failed SQL migrations identify their version. The initial migration creates the `biznes` application namespace and refuses to drop it when non-empty. Goose owns `public.goose_db_version`; the separate users migration supplies identity storage. `migrations/README.md` documents naming, commands, deployment permissions, failure handling, and rollback validation. The API never automatically migrates schema.
- A shared HTTP `WriteError` helper and `ErrorDetail` type in `internal/platform/http/response.go`. Routing and panic recovery use stable public codes, safe messages, optional field/code details, and the middleware's request ID, with JSON content type and `Cache-Control: no-store`. The writer aborts without appending an error or replacing a committed response. Gin supplies `Allow` for method errors. `docs/API_CONVENTIONS.md` defines `/api/v1`, success envelopes, status/error mappings, strict bounded JSON input, validation, bounded page/limit pagination, timestamps, IDs, and money representations. Identity, organization, contact, and category endpoints implement strict input validation, explicit success DTOs, reusable session authentication, and bounded pagination.
- A PowerShell 7 developer entry point, `dev.ps1`, for `check`, `run`, `test`, `lint`, `fmt`, `fmt-check`, `db-up`, `db-down`, and migration up/down/status. It uses existing Go and Compose commands, runs from the repository regardless of the caller's directory, restores that directory, and propagates native exit codes. The default `check` rejects unformatted Go without writing, then runs package tests, standard Go vet analysis, and module verification, stopping at the first failure. Go commands use process configuration; database startup, migration, and rollback remain explicit. `db-down` retains the volume. There is no additional task runner or Go dependency.
- A GitHub Actions workflow in `.github/workflows/go.yml` for pushes to `main`, pull requests targeting `main`, and manual dispatch. One Ubuntu 24.04 job reuses `dev.ps1 check`, builds all Go packages, and fails if module files changed. It selects Go from `go.mod`, caches using `go.sum`, pins checkout/setup-go to verified release commit SHAs, uses read-only repository permissions, limits the job to 15 minutes, and cancels superseded runs for the same ref. Go uses readonly module resolution and the installed toolchain. The workflow requires no database configuration or secrets and adds no project tests or Go dependencies.
- A global identity persistence model in `internal/identity/user.go` and `migrations/00002_create_users.sql`. `biznes.users` has native UUIDv4 IDs, canonical globally unique lowercase ASCII emails, an opaque required password verifier, and required creation/update instants with database defaults. Email storage is bounded to 3–254 bytes with one `@` and non-empty parts; encoded verifier storage is printable ASCII and bounded to 1024 bytes. The Go model excludes `PasswordHash` from JSON; registration uses an explicit response DTO. Update statements must maintain `updated_at`. Rollback takes a table lock and refuses to remove stored users, retaining transaction/history consistency and dependency protection.
- Global identity registration in `internal/identity/registration.go` and `internal/platform/http/registration.go`, using the existing native pgx pool. `POST /api/v1/auth/register` enforces an 8 KiB body cap, one JSON object, exact string fields, no unknown/duplicate fields, valid UTF-8/paired Unicode escapes, and no query parameters. Application logic validates plain ASCII addresses with DNS labels, trims email whitespace, lowercases after validation, and preserves dots/plus tags. Passwords contain 15–128 Unicode code points and are preserved without composition rules, trimming, normalization, or truncation. Argon2id v19 uses 19 MiB, two passes, one lane, a random 16-byte salt, and a 32-byte verifier in PHC format through the existing `golang.org/x/crypto v0.48.0` (now a direct dependency, no version upgrades). Registration and login share one password operation and one start/second per API process to bound anonymous work; excess returns `429` with `Retry-After: 1`. A parameterized insert has a five-second/request-cancellation deadline. Database uniqueness protects concurrent creation; safe `409` responses preserve existing users and safe `503` responses hide storage failures. Success is `201` with a credential-free `data` DTO and UTC instants, without a session or organization access. See [the endpoint contract](API_CONVENTIONS.md#registration) for validation and public-launch limitations. Registration itself continues to use the existing users schema.

- Authentication and durable sessions in `internal/identity/authentication.go`, shared password hashing/verification in `internal/identity/password.go`, HTTP login/session handlers in `internal/platform/http/authentication.go`, and `migrations/00003_create_sessions.sql`. The concrete identity service shares strict credential validation and a per-process password-work budget across signup/login. Login returns a fresh random 256-bit bearer token, stores only SHA-256 of decoded token bytes, and uses fixed-policy Argon2id with constant-time comparison; unknown accounts/unsupported verifiers still perform the fixed derivation and return the same safe `401`. Sessions have eight-hour absolute and fifteen-minute idle limits, enforced by PostgreSQL at every authenticated lookup. The Gin middleware refreshes idle activity, resolves the public user context, and fails closed on storage outages. `GET /api/v1/auth/me` returns public identity fields; `POST /api/v1/auth/logout` deletes the current session and returns empty `204`. Tokens require one canonical Bearer authorization header; cookie/query/body fallback is excluded. State/revocation work across API replicas and restarts; already authenticated requests may finish after revocation. Five-second database deadlines and safe errors/logs remain in force. The migration guards non-empty rollback and supplies an expiry index for explicit maintenance; expired rows are never usable before cleanup. There is no refresh, recovery, MFA, device management, new dependency, or environment setting. Organization authorization is handled separately through membership. See [the authentication contract](API_CONVENTIONS.md#authentication-and-sessions).

- Organizations in `internal/organization/service.go` and `internal/platform/http/organization.go`, with explicit `00004_create_organizations.sql`. Creation atomically inserts a native UUID organization and the authenticated creator's owner membership. Member-scoped list/get queries return only organization metadata and the caller's role; unknown and inaccessible IDs share `404`. Owners/admins can rename, while accountant/staff members receive `403`. Rename holds a membership row lock through commit to serialize demotion/removal. The first real list implements bounded page/limit parsing and stable creation-time/ID ordering without a count query. Shared strict string-object JSON parsing moved to `internal/platform/http/input.go` for actual identity/organization reuse, retaining duplicate/unknown/type/UTF-8/surrogate/body/media protections. Unicode names preserve Persian and zero-width non-joiners, trim surrounding whitespace, reject controls, and contain 1–120 code points. Storage has role/length/FK/composite-key constraints and a reverse membership index; guarded rollback preserves populated organizations and earlier identity/session data. Five-second operation deadlines and safe errors/logs remain in force. Invitation/member-management/ownership-transfer/organization-deletion APIs and financial capabilities remain planned. Contacts are implemented separately below; no dependency or environment setting is introduced. See [the organization contract](API_CONVENTIONS.md#organizations).

- Contacts in `internal/contact/service.go`, `internal/platform/http/contact.go`, and `00005_create_contacts.sql`. POST/list/get/PUT routes live under the selected organization and require a bearer session. All members can read; owners/admins can create/replace name, customer/supplier/both kind, email, phone, and notes. Every query/write constrains organization ownership; single reads join membership, while writes/lists reuse `organization.LockMembership` through transaction completion to serialize removal/demotion. Full replacement clears omitted optional fields and preserves identity/creation time. Strict string-object JSON, canonical path IDs, and bounded pagination are reused for actual contact endpoints. Unicode/Persian contact text is preserved, phone data remains free-form, mailbox syntax uses native `net/mail` without verification/case folding, and multiline notes have explicit bounds/control policy. Native composite tenant/resource keys, FK/value constraints, and stable list indexing protect persistence; guarded rollback preserves populated contacts and earlier domain data. Safe errors/logs, operation deadlines, and cancellation remain in force. No deletion/archive/search/tag/history/financial functionality, dependency, or setting is added. See [the contact contract](API_CONVENTIONS.md#contacts).

- Transaction categories in `internal/finance/category.go`, `internal/platform/http/category.go`, and `00006_create_transaction_categories.sql`. POST/list/get/PATCH routes are scoped under the selected organization and require bearer authentication. Owner/admin/accountant members can create or rename income/expense categories; every member can read. Category kind stays fixed through the API. Native exact `C`-collation uniqueness on organization/kind/name handles creation/rename races and returns safe `409` without modifying existing data. The same name is allowed in a different kind or organization. Composite tenant/resource keys, restricted organization FK, bounded Unicode names/kinds, stable list indexing, explicit timestamp maintenance, and guarded rollback protect persistence. Reads/writes reuse strict JSON, UUID/pagination, transaction-held membership locks, parameterized SQL, safe errors/logs, and five-second/request-cancellation deadlines. Accountants' category permissions do not broaden contact/organization write policies. No category deletion/archive/hierarchy/default/filter, monetary amount/account/transaction/balance capability, dependency, or setting is introduced. See [the category contract](API_CONVENTIONS.md#transaction-categories).

Not implemented:

- Financial accounts, recorded transactions, and balances; category deletion/archive/hierarchy/defaults/filtering and contact deletion/archive/search/tags/history.
- API containerization.
- Email ownership verification, common/breached password screening, client-aware ingress limits, MFA/recovery/device management, invitations/member management/ownership transfer, financial data, AI, integrations, and further business capabilities. Distinct registration creation/conflict statuses currently reveal email availability; an ownership-verification flow must address enumeration before public launch.

Phase 0's twelve foundation increments are complete; the [first hosted CI run for `c09a93b`](https://github.com/wikiccu/biznes/actions/runs/37362894429) passed. The pinned PostgreSQL image download succeeded on 2026-10-05, resolving the earlier regional `403` validation blocker. Runtime validation covers authenticated SQL, UTF8/Persian data, pool limits, startup/configuration failures, cancellation, dependency outages and recovery, shutdown cleanup, and named-volume persistence across container recreation. Migration validation covers fresh history/status, apply/reapply, rollback/reapply, protection of existing schema/data, concurrent migration serialization, lock and SQL deadlines, signal cancellation, safe errors, and connection cleanup. API foundation validation covers JSON routing errors, method/Allow behavior, HEAD responses, request-ID boundaries, log privacy, unchanged probe payloads, dependency outage/recovery, and shutdown cleanup. Developer commands were validated on Windows with PowerShell 7.6.5, including invocation outside the repository, caller-directory restoration, default checks, format rejection/repair, invalid commands, missing tools/configuration, native failure codes, real API serving, migrations, and volume retention. The CI workflow was checked with actionlint and local Go checks/builds. Actual hosted results remain authoritative in GitHub Actions. Phase 1's first increment adds user persistence, with PostgreSQL validation of schema/defaults, uniqueness, canonical email/hash bounds, null rejection, safe non-empty rollback, empty rollback/reapply, concurrent insert protection, and connection cleanup. Runtime validation uses isolated projects and removes their containers and volumes without changing existing databases or creating test/fixture files. Registration validation uses two real API processes and isolated PostgreSQL to verify strict body/media/query parsing, deterministic field errors, email normalization/bounds, preserved Unicode/whitespace passwords, independent Argon2id derivation through Python cryptography, random salts, public UUID/UTC DTOs, duplicate preservation and cross-process insertion races, admission throttling, locked-database deadlines, safe storage failure/recovery, credential log privacy, graceful shutdown, and pool cleanup. All validation resources were removed. Authentication validation uses real HTTP and isolated PostgreSQL for strict credential/token boundaries, malformed/unsupported verifier rejection, shared password admission, independent hash verification, digest-only session persistence, cross-process/restart behavior, absolute/idle expiry, idle refresh without lifetime extension, logout/replay denial, session isolation, fail-closed storage outages, operation deadlines, schema/FK constraints, safe rollback/history, expiry maintenance, log privacy, and pool cleanup. Validation resources were removed. [Hosted authentication CI for `5b42f81`](https://github.com/wikiccu/biznes/actions/runs/37371450050) passed. Hosted results remain authoritative in GitHub Actions. Planned stack components and design conventions below describe implementation direction, not existing runtime behavior.

Organization validation passed against two real API processes and isolated PostgreSQL: strict JSON/Unicode/media/body/query/path boundaries, authenticated owner creation, membership-scoped listing/retrieval, bounded pagination, owner/admin versus accountant/staff permissions, concurrent demotion serialization, immediate membership removal, SQL constraints, transactional creation failure, five-second lock deadlines, fail-closed storage outages/recovery, restart persistence, populated/empty migration rollback/history, identity/session preservation, safe logs, and released pool connections. Shared credential parsing was exercised for regression coverage. Disposable validation resources were removed; no test/fixture files were created.

Contact validation passed with two real API processes and isolated PostgreSQL: strict JSON/media/body/query/UUID/Unicode field boundaries, scoped create/list/get/replacement, customer/supplier/both kinds, optional-field clearing, stable bounded pagination, owner/admin versus read-only roles, access to two organizations without mixed data, identical IDs in separate tenants, demotion/removal races, shared membership/organization-rename regression, SQL constraints, populated/empty rollback and version history, five-second lock deadlines, client cancellation, fail-closed storage outages/recovery, restart persistence, safe logs, pool cleanup, and retention of existing identity/session/organization data. Validation resources were removed; no test/fixture files were added. [Hosted organization CI for `cfacee9`](https://github.com/wikiccu/biznes/actions/runs/37525434267) passed before this increment.

Category validation passed against two real API processes and isolated PostgreSQL: strict JSON/media/body/query/UUID/Unicode boundaries, scoped create/list/get/rename, fixed API kind, same-name/different-kind/tenant acceptance, case-sensitive duplicates, cross-replica duplicate creation/rename races, preserved conflict data, accountant versus staff policies, unchanged contact permissions, both-tenant access with identical resource IDs, demotion/removal, SQL/FK constraints, guarded populated/empty rollback/history, SQL lock deadlines, client cancellation, storage failure/recovery, restart durability, safe logs, pool cleanup, and preservation of prior domain data. Validation resources were removed and no test/fixture files were added. [Hosted contact CI for `eae2d02`](https://github.com/wikiccu/biznes/actions/runs/37542803485) passed before this increment.

## 1. Product Vision

biznes will become an **AI-powered operating intelligence system for small and medium businesses**: an intelligent business operator that understands financial and operational state, explains changes in plain language, identifies what needs attention, predicts upcoming problems, recommends responses, and eventually performs authorized actions.

Its evolution is:

```text
Record → Understand → Explain → Predict → Recommend → Act
```

The owner should be able to ask useful questions without accounting expertise. biznes starts as a financial intelligence layer, not a replacement for professional double-entry accounting software. A deeper accounting engine should be introduced only when validated customer needs justify it.

The following conversations are illustrative future experiences. Their numbers are fictional and are not product results.

```text
Owner: How is my business doing this month?

biznes: Revenue is 14% higher than the same period last month,
but collected cash increased by only 3%.
You have 184,000,000 Toman in overdue receivables.
Three customers account for 61% of that amount.
```

```text
Owner: Will I have enough money for my checks this month?

biznes: Based on current balances, expected collections, and obligations,
you may face a cash shortage of approximately 42,000,000 Toman
between October 18 and October 22.
This forecast depends on the expected collection dates shown below.
```

```text
Owner: Find customers more than 30 days overdue.
biznes: I found 17 customers with 126,000,000 Toman overdue.
Owner: Send reminders to all except Ahmadi.
biznes: Here are the recipients and message preview for approval.
Owner: Confirm.
biznes: Sent 16 reminders. The delivery results are available.
```

### Initial market and international reach

The initial audience is Iranian businesses. Product experiences should eventually support Persian and right-to-left presentation, Toman/Rial, Jalali date input and display, Iranian mobile numbers, customers and suppliers, checks, receivables, payables, local invoicing practices, and Iranian business/accounting realities.

The product name remains international: **biznes**. Code, database/entity/field names, API identifiers, and documentation use English. Language catalogs, calendars, currency presentation, phone normalization, and regional integrations belong at appropriate input/output or adapter boundaries. Country-specific behavior must not become a permanent assumption in core financial logic.

## 2. Target Users

| Persona | Main problems | Intended value |
| --- | --- | --- |
| Small shop owner | Sales and expenses are scattered; cash collection and check deadlines are easy to miss. | Fast recording, clear cash position, obligations, and daily attention items. |
| Service business owner | Work is delivered before payment; collections and recurring costs are hard to track. | Customer balances, overdue follow-ups, expense visibility, and cash forecasts. |
| Freelancer | Personal and business activity blur; invoices and irregular income complicate planning. | Simple business records, receivable tracking, and understandable cash planning. |
| Small company manager | Information arrives late or in disconnected tools. | Consolidated business snapshots, comparisons, and explanations grounded in records. |
| Financial manager | Obligations, partial payments, and expected collections require constant monitoring. | Aging, cashflow scenarios, reconciliation, and traceable reports. |
| Accountant | Multiple client businesses create repetitive collection, review, and reporting work. | Permissioned client access, exports, audit history, and an accountant workspace. |
| Multi-branch business owner | Branch performance and spending are difficult to compare or control. | Consolidated and branch views, scoped permissions, approvals, and collaboration. |

Early discovery should focus on a small set of these personas and real daily cash-management problems. Validate adoption and usefulness before broadening scope.

## 3. Core Product Principles

### Simplicity over traditional accounting complexity

Use the owner's language. Prefer “I paid 8,500,000 Toman for office rent today” to requiring debit accounts, credit accounts, journal entries, cost centers, or ledger codes. Structured forms and, later, natural-language entry should collect the information needed for accurate records without exposing unnecessary accounting machinery.

Natural-language interpretation produces a structured preview. Missing or ambiguous financial details must be resolved before saving. Expert workflows may exist later without burdening ordinary owners.

### Ground intelligence in authorized business data

Every business-specific answer must originate from data the current user is allowed to access. biznes must distinguish:

- **Database facts:** recorded transactions, due dates, balances, and their sources.
- **Calculated values:** deterministic aggregates, aging, and period comparisons.
- **Forecasts:** modeled future outcomes with assumptions and uncertainty.
- **Assumptions:** missing inputs or user-selected scenario conditions.
- **AI interpretations:** explanations or recommendations based on the evidence.

Never present estimates as confirmed facts. Explain relevant date ranges, currency, data freshness, missing information, and calculation basis. Revenue, invoiced amounts, and collected cash are distinct; profitability claims require sufficient cost and recognition data. The product must not become a generic chat wrapper.

### Progress toward authorized actions

The final assistant should create transactions, receivables, payables, reminders, invoices, and reports; classify expenses; schedule follow-ups; send payment reminders; and trigger workflows. Permissions, input validation, confirmations where sensitive, audit trails, and safe failure handling apply equally to manual and AI-assisted actions.

Read-only AI comes first. Action tools are a later phase with their own safeguards and review criteria.

### Earn trust through correctness and control

Authentication, authorization, tenant isolation, auditability, encryption where appropriate, secure secrets, backups and restore procedures, data export, account deletion and retention, AI access boundaries, rate limiting, and observability are eventual requirements.

Foundational protections must accompany the first features that require them. Tenant isolation and authorization begin with Phase 1; later advanced permissions do not postpone baseline security. Essential audit evidence, recoverability, and access to one's data must not depend on buying a higher plan.

## 4. Final Product Capability Map

This map defines the intended end-state. It does not authorize implementing every domain now.

### Identity & Organization

Registration and login, organizations/businesses, memberships, invitations, sessions, business roles, and permission enforcement. A user may belong to multiple businesses. Owner, administrator, accountant, and staff access should express permitted operations and scope. Later capabilities include multiple branches, approvals, advanced policies, and audit visibility.

### Contacts

A unified contact can be a customer, supplier, or both. Contacts include information, notes, tags, activity history, balances, transactions, invoices, receivables/payables, and payment history. Later insights highlight customer behavior, buying changes, concentration, and consistently late payment.

### Financial Activity

Income, expenses, transfers, accounts, cash, bank accounts, cards, categories, attachments, recurring transactions, imports, reconciliation, and a financial timeline. Transactions should have clear sources and corrections rather than unexplained balance edits. Transfers must not inflate revenue or expenses, and summaries must distinguish cash movement from business performance.

Start with simple financial records and intelligence. Introduce a deeper accounting engine only with a documented customer need and explicit migration plan.

### Receivables & Payables

Track money customers owe the business and money the business owes suppliers. Support due dates, partial payments, outstanding amounts, payment allocation/history, overdue status, aging, reminders, and follow-up workflows. Outstanding balances derive from obligations and valid payments; recording payment must not duplicate income or expense.

### Checks & Obligations

Incoming and outgoing checks are important for the Iranian launch. Capture amount, currency, due date, bank, owner/contact, direction, status, reminders, settlement, and cashflow impact. Future instruments can share an obligation concept while preserving their own lifecycle and attributes.

Avoid a core architecture in which every future financial commitment must be a check. Status transitions and settlement rules will be defined when this domain is implemented, including how an instrument relates to an underlying payable or receivable without double counting.

### Cashflow Intelligence

Show current cash position, historical cashflow, upcoming inflows/outflows, predicted balances, shortage warnings, scenario simulations, and forecast confidence. Separate actual balances from expected collections and uncertain inflows.

For example, an illustrative forecast might show 91M Toman expected cash on October 20 against 132M Toman of obligations, implying a potential 41M Toman deficit. Show which assumptions drive the outcome and allow the owner to adjust them.

### Dashboard / Business Daily Brief

The main experience should feel like a business briefing. Prioritize relevant attention items, explain their impact, and provide a clear next action alongside a financial snapshot.

```text
Good morning. Three things need your attention:
- A 63M Toman check is due tomorrow.
- Customer Ahmadi is 18 days overdue.
- Weekly revenue is up 11%.

Ask biznes anything about your business...
```

The eventual dashboard combines today's activity, revenue, collected cash, expenses, receivables, upcoming obligations, comparisons, and insight explanations. It must make the reporting period and business timezone clear.

### AI Business Assistant

Answer questions about sales today or this month, largest debtors, expense increases, affordability of next month's obligations, customers who stopped buying, major expenses, repeated late payment, cash position, and changes from the previous period.

Use deterministic, permissioned tools rather than sending the entire database to an LLM. Provide the result's evidence, relevant scope, and limitations in language the owner understands. Conversational context must remain scoped to the current user and business.

### AI Action Engine

Future tools may include:

```text
createTransaction()  createReceivable()  createPayable()
recordPayment()      createReminder()    sendReminder()
generateReport()     createInvoice()     scheduleTask()
```

Each action requires explicit authorization, server-side validation, the same application logic as manual entry, idempotency where retries could duplicate effects, and an audit trail. Sensitive actions need a preview and confirmation bound to the actual payload, recipients, organization, and cost where relevant.

Recheck permissions before execution. Report partial delivery or provider failure accurately; retries must not silently repeat financial writes or send duplicate messages. Automation runs only within a user's approved scope and can be revoked.

### Documents

Upload invoices, receipts, check images, and other financial documents. Extract structured fields such as merchant, date, amount, currency, and items; show a preview and uncertainty; obtain user confirmation; then attach the original document to the created record.

```text
Upload → OCR / Document AI → Structured preview → User confirmation
       → Validated financial record with original attachment
```

Never silently save uncertain extraction as financial truth. File access, retention, size/type validation, and safe processing belong in this capability when introduced.

### Imports & Integrations

CSV and Excel imports, accounting tools, e-commerce, POS, banking/open banking, payment providers, tax/invoicing systems, APIs, and webhooks are possible future inputs and outputs. Provide mapping, validation, previews, duplicate handling, and source traceability as appropriate.

External systems are adapters around core application services. Provider schemas must not dictate the core domain. Verify each integration's current technical and legal availability before implementing it; the roadmap makes no availability promise.

### Notifications & Reminders

In-app, email, SMS, and push channels should support overdue invoices, upcoming checks, low predicted cash, unusual expenses, daily briefings, and payment receipts. Respect recipient consent and user channel/time preferences, business timezone, delivery status, and duplicate prevention. Adding a channel depends on a demonstrated need and suitable provider availability.

### Analytics

Revenue, expenses, cashflow, profitability indicators where the available data permits, receivable aging, customer concentration, spending trends, customer behavior, period comparisons, anomalies, and forecasts. Deterministic metrics need documented definitions and must reconcile with underlying records; forecasts need assumptions and uncertainty.

### Accountant Portal

Accountants can work across multiple client businesses through explicit memberships and delegated permissions. Switching clients must never combine tenant data accidentally. A future accountant dashboard can prioritize client work and support collaboration, review, reports, and exports. This may become a distribution channel after validating accountant needs.

### Billing

Support subscription plans and credits for genuinely variable costs: document processing/OCR, expensive AI analysis, SMS, external APIs, and automation execution. Do not charge credits artificially for simple database operations. Subscription state, entitlements, usage metering, and an auditable credit ledger should form a cohesive capability when monetization is implemented.

## 5. Proposed Monetization Model

These are hypotheses for customer validation. **Do not choose final prices yet.** Packaging and limits may change with customer value, cost, and usage evidence.

| Plan | Intended value | Possible future scope |
| --- | --- | --- |
| Free | A genuinely useful entry tier for a small business. | One business, limited monthly transactions, basic contacts, receivables/payables, checks, dashboard, and limited AI questions. |
| Pro | More insight and less manual follow-up. | Higher or unlimited transaction limits, advanced analytics, forecasting, automated reminders, document allowances, more AI use, enhanced exports and backup options, multiple users, and automation. |
| Business | Collaboration and operational control. | Multiple branches, advanced permissions, extended audit views, workflows, APIs/webhooks, integrations, accountant collaboration, advanced automation, and larger limits. |

Basic operational backups, baseline audit collection, and users' ability to export their data remain trust requirements; paid plans may offer enhanced controls and reporting. Credits complement subscriptions only where variable costs justify them. Track usage transparently, disclose costs before billable actions, and keep entitlement decisions centralized rather than scattering plan-name checks through business services.

## 6. Architecture Vision

Start with a **modular monolith**, one deployable backend with clear internal ownership and interfaces. Do not start with microservices. Extract a component only when measured scaling, operational, or ownership requirements justify the cost.

```text
Web / Mobile (future clients)
          |
     Versioned REST API
          |
biznes Modular Monolith
          |
          +--- PostgreSQL (initial source of truth)
          +--- Job queue / workers (when needed)
          +--- Object storage (when documents need it)
                         |
                  External adapters
                  AI / OCR / messaging / integrations
```

Domain modules own their application behavior, persistence, and transport adapters. Other modules call deliberate application interfaces rather than reaching into private repositories. Shared platform code serves concrete technical needs; it must not become a generic dumping ground for domain logic.

The `identity` module implements user persistence, registration, authentication, and durable sessions. The `organization` module implements transactional creation/owner membership, scoped reads/listing, authorized renaming, and transaction-held membership access through native pgx. The `contact` module implements tenant-scoped contact creation/listing/retrieval/replacement and customer/supplier/both classification. The `finance` module begins with concrete transaction-category persistence, scoped listing/retrieval, creation, and renaming; monetary accounts/records follow separately. Candidate future modules are `receivables`, `payables`, `obligations`, `documents`, `assistant`, `actions`, `analytics`, `notifications`, `integrations`, `billing`, and `audit`. Names and boundaries may evolve with implemented use cases. Do not create empty modules, provider interfaces, or frameworks just to mirror this list.

Initial directory direction, introduced only as files become necessary:

```text
biznes/
├── .github/workflows/go.yml
├── cmd/
│   ├── api/
│   └── migrate/
├── internal/
│   ├── identity/
│   ├── platform/
│   │   ├── config/
│   │   ├── database/
│   │   ├── http/
│   │   └── logging/
│   └── <implemented domain modules>/
├── migrations/
├── docs/PRODUCT_BLUEPRINT.md
├── .env.example
├── compose.yaml
├── dev.ps1
├── go.mod
├── go.sum
└── README.md
```

This is a planned shape, not the current repository tree. Avoid a single global `services/`, `repositories/`, or `controllers/` hierarchy that mixes all domains.

## 7. Technical Direction

The initial backend stack is **Go, Gin, PostgreSQL, and Docker Compose**. Go 1.27.1, Gin v1.12.0, [pgx v5.11.0](https://github.com/jackc/pgx/releases/tag/v5.11.0), and [Goose v3.26.0](https://github.com/pressly/goose/releases/tag/v3.26.0) are recorded in `go.mod`, with dependency checksums in `go.sum`; `compose.yaml` pins the official PostgreSQL image to `18.6-trixie`. Goose uses a stable release compatible with the existing API dependency versions. PostgreSQL 18.6 is the current supported minor release verified against the [PostgreSQL versioning policy](https://www.postgresql.org/support/versioning/) and [official image tags](https://github.com/docker-library/docs/blob/master/postgres/README.md) at implementation time. Use native pgx pooling and explicit context-aware SQL operations; do not introduce an ORM or wrapper repository framework. Select supported stable versions for the remaining components and record/pin them in the relevant files. Prefer the standard library where reasonable, including configuration, structured logging, HTTP server lifecycle, and signals.

Add a dependency only for a concrete need. Inspect current stable ecosystem conventions before choosing unspecified libraries, such as a database driver or migration tool. Use explicit migrations, never production ORM auto-sync.

Possible later components include Redis, S3-compatible storage, background workers, OpenTelemetry, LLM providers, OCR, and SMS providers. Introduce each with the phase that actually needs it; provider and pricing decisions remain deferred until then.

## 8. API Direction

Use versioned REST APIs under `/api/v1/...`. Phase 0 provides `GET /health` with a minimal successful response:

```json
{"status":"ok"}
```

It is a process liveness check without dependency checks. `GET /ready` returns `200` with `{"status":"ready"}` while the lifecycle is active and a PostgreSQL ping succeeds within `BIZNES_DATABASE_HEALTH_TIMEOUT`, including pool acquisition. It returns `503` with `{"status":"not_ready"}` on dependency failure, timeout, request cancellation, or shutdown, and can recover after a database outage without an API restart. The ping uses request and lifecycle cancellation without canceling unrelated handlers. New connections may fail after the listener closes; requests still served during draining retain successful liveness responses. Both endpoints prevent caching and use the existing request ID and logging middleware. Readiness does not assert application schema exists. Do not create fake business endpoints or introduce GraphQL prematurely.

The [API conventions](API_CONVENTIONS.md) establish the business contract for `/api/v1`. Shared routing/recovery errors are implemented; business success DTOs, input decoding, resource validation, and list pagination will accompany real endpoints. The conventions include:

| Concern | Convention |
| --- | --- |
| Field names | English `snake_case` JSON and database names; idiomatic Go names internally. |
| Successful responses | Business APIs return `data` and optional `meta`; health retains its simple response. |
| Errors | The shared writer returns an `error` object with stable `code`, safe `message`, optional field/code `details`, and the middleware's `request_id`. Never expose secrets or internal stack traces. |
| Validation | Strict JSON object decoding with a default 1 MiB body cap, then bounded field validation at the handler boundary; application services enforce domain invariants. Implement this with the first JSON endpoint. |
| Status codes | Use meaningful HTTP codes: 400 malformed input, 401 unauthenticated, 403 forbidden, 404 unavailable resource, 405 unsupported method with `Allow`, 409 conflict, 413 oversized body, 415 unsupported media type, 422 invalid values, 429 rate limit, and safe 5xx failures. Avoid existence leaks across tenants. |
| Pagination | Default `page=1` and `limit=20`, capped at page 10000 and limit 100; reject invalid values and include count metadata only where affordable. Use stable ordering with an ID tie-breaker. Change to cursors only for an evidenced need. |
| Filtering/sorting | Allowlist supported fields, validate ranges, and document order; never interpolate arbitrary client input into SQL. |
| Timestamps | RFC 3339 UTC instants; ISO `YYYY-MM-DD` for genuine date-only values, with business timezone semantics. |
| IDs | Opaque UUID identifiers; never treat knowing an ID as authorization. |
| Money | Integer minor units and currency; transmit potentially large amounts as decimal strings, not floating-point JSON values. |
| Authentication | Select the concrete session/token mechanism in Phase 1; never trust client-supplied user identity. |
| Authorization | Resolve organization access from authenticated membership and permissions on every operation, including tools, exports, and jobs. |
| Request ID | Attach a bounded, validated request ID to responses, errors, and logs; generate one when needed. |

Example validation error for future business handlers, using the implemented shared envelope:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Please correct the highlighted fields.",
    "details": [{"field": "due_date", "code": "invalid_date"}],
    "request_id": "opaque-request-id"
  }
}
```

Specify endpoint contracts with their implementation. Keep handlers thin: parse, validate transport concerns, call application logic, and serialize results.

## 9. Data Architecture

### Source of truth and ownership

PostgreSQL is the initial authoritative store. Define constraints and indexes through reviewable migrations. The [migration workflow](../migrations/README.md) uses the pinned Goose library and explicit commands before deployment, with immutable sequential SQL files and transactional version recording. The first migration creates the `biznes` namespace and the second creates `biznes.users`; qualify application objects as `biznes.<name>`. Goose's history table lives in `public`. User rollback refuses to remove stored identities and holds a table lock to protect concurrent writers. The third migration creates durable sessions with digest keys, user references, chronological constraints, and an expiry-maintenance index. Session rollback locks the table and refuses to remove stored sessions. Organization/financial tables are not implemented, and database readiness still checks connectivity rather than schema version.

```text
User ── Membership ── Organization
                           |
                           +── Contact
                           +── Account / Transaction
                           +── Receivable / Payable
                           +── Obligation
                           +── Other business-owned records as introduced
```

A user is a global identity. A membership links a user to an organization with role/permissions. Contacts and financial records belong to an organization; references between them must stay within that organization. A user may have several memberships, including an accountant's client access. Branches later refine organization scope rather than replace it.

Initial identities use canonical lowercase ASCII email addresses, with deterministic `C` collation and database uniqueness. Registration normalizes casing and surrounding email whitespace, validates plain addresses/DNS labels, and preserves dots/plus tags. Internationalized email and phone login require a deliberate later normalization/authentication policy. Store only an encoded server-generated Argon2id password verifier; the schema's printable/length checks do not validate cryptographic strength. Registration preserves passwords exactly, and credential values must never be logged or exposed through responses. The persistence model's JSON exclusion is a safeguard, not a substitute for explicit response DTOs. Mailbox ownership verification and breached-password screening remain public-launch requirements.

Every business-owned row requires an `organization_id`. Resolve scope from authenticated, authorized context; apply it to reads, writes, aggregates, imports, attachments, jobs, AI tools, and exports. Validate cross-record ownership and use database constraints where possible to prevent cross-tenant references. IDs, hidden UI elements, and LLM instructions do not provide isolation.

### Money

Use a signed 64-bit integer amount in **currency minor units**, paired with an explicit currency code. Persist it as PostgreSQL `BIGINT` and exchange it in APIs as a decimal string such as `"85000000"` to avoid client numeric precision loss. Define the currency scale explicitly; never use binary floating-point arithmetic for financial values.

For the initial Iranian market, use **IRR** as the canonical currency with whole Rial as its unit. Toman is an input/display denomination: 1 Toman equals 10 Rial. For example, 8,500,000 Toman maps to 85,000,000 Rial. Do not label a Toman amount as an IRR amount or invent an implicit currency conversion. Display values that cannot be expressed as whole Toman without rounding must retain their exact precision.

Validate amount range, use checked arithmetic, and define rounding explicitly when fractional inputs or conversions are later supported. Any aggregate exceeding the supported range must fail safely or use an exact wider representation, never overflow silently. Do not add amounts across currencies without an explicit exchange-rate policy. Direction and sign rules belong to each implemented domain.

This is a chosen implementation direction, not a money package or schema created in Step 1.

### Time and localization

Store event timestamps as canonical UTC instants, using PostgreSQL `timestamptz` where applicable. Store genuine due dates as dates when they represent a local calendar day rather than an instant; interpret overdue cutoffs in the business's configured timezone. Preserve that distinction in APIs and reports.

Jalali conversion belongs to input/presentation. Do not use Jalali strings as fundamental database timestamps. Persian digits, Iranian phone formats, and currency display normalization should convert into validated canonical data at boundaries. Business timezone, locale, and denomination preferences must be configurable; an Iranian launch must not make every tenant permanently Iranian.

### Identifiers and integrity

Use opaque UUID primary identifiers consistently. Users use PostgreSQL's built-in `gen_random_uuid()` for UUIDv4 defaults without an extension or Go UUID dependency. References should be constrained and indexed appropriately. UUIDs do not replace access control.

Use explicit database transactions when operations must be atomic, such as recording a payment with its allocation or later updating a credit ledger. Propagate `context.Context` across request, application, database, and external-call boundaries. Define correction, deletion, retention, and audit semantics before implementing financial record mutations; never delete production data automatically.

## 10. AI Architecture

Introduce a provider boundary when Phase 4 needs it, keeping business services independent of any LLM vendor. Do not create an empty abstraction in Phase 0. Application tools, not the model, own calculations and data access.

```text
User question
    ↓
Intent / tool selection
    ↓
Authenticated, authorized application tool
    ↓
Deterministic scoped data + calculation context
    ↓
LLM explanation
    ↓
Answer with evidence, assumptions, and limitations
```

“Who owes me the most money?” should invoke logic such as `getTopDebtors()` with server-resolved organization scope, not ask an LLM to guess from arbitrary text. Tools return bounded, necessary data with clear definitions, date ranges, currency, and source freshness. Enforce permissions at execution even when a tool call appears reasonable.

Phase 4 is read-only. Phase 6 adds separately controlled mutation tools and confirmation flows. Treat imported documents and external text as untrusted data; they cannot grant permission or override tool policies. Avoid sending secrets or excessive business data to providers. Evaluate provider privacy, retention, deployment constraints, and cost when selecting them.

Track usage and failures, limit expensive operations, and verify output against deterministic results. When tools fail or evidence is incomplete, explain the limitation instead of inventing a financial answer.

## Development Roadmap

These phases express the initial direction and **may change as we learn from real users**. Phase advancement depends on useful, validated outcomes and the prerequisites below, not on creating folders or documenting an idea. No future phase is complete today.

### Phase 0 — Product & Engineering Foundation

**Goal:** Establish a professional repository, shared vision, and reliable application foundation without business features.

Deliverables are the blueprint and README, Go initialization, Gin server, configuration, graceful shutdown, structured logging, health endpoint, request ID, recovery middleware, basic error handling, `.env.example`, Docker setup, local PostgreSQL, migration tooling, an initial real project structure, formatting/linting, developer commands, and CI-ready validation.

Work in these reviewed increments:

| Step | Cohesive increment | Current status |
| --- | --- | --- |
| 1 | Product blueprint and concise README. | Complete and committed. |
| 2 | Go module and minimal application initialization. | Complete. |
| 3 | Typed, environment-based configuration and `.env.example`. | Complete. |
| 4 | Structured application logging. | Complete. |
| 5 | Gin HTTP server lifecycle, timeouts, graceful shutdown, request ID, recovery, and request logging. | Complete. |
| 6 | `/health` and `/ready` with distinct liveness/readiness semantics. | Complete. |
| 7 | PostgreSQL local development environment with Compose. | Complete; runtime validation now passed. |
| 8 | PostgreSQL connection lifecycle, pooling, and health checking. | Complete. |
| 9 | Migration foundation with documented commands. | Complete; isolated PostgreSQL runtime validation passed. |
| 10 | Versioned API response, error, validation, and pagination conventions. | Complete; contract documented and shared routing errors validated. |
| 11 | Formatting/linting and developer commands. | Complete; native command wrappers and Windows runtime validation passed. |
| 12 | Go validation and CI foundation. | Complete; workflow/local checks and the first hosted run for `c09a93b` passed. |

Validate and review each increment, create one meaningful Conventional Commit, attempt to push to `origin/main` when configured, then stop until the human says `continue`. All Phase 0 increments and their first hosted CI run passed. Resolve validation failures before adding business capabilities. Phase 0 does not include product features.

### Phase 1 — Business Core MVP

**Goal:** A user can create a business and record fundamental information manually.

Implement authentication, organizations, membership, contacts with customer/supplier roles, income, expenses, basic transactions, categories, and balances. Start with the user persistence foundation, then add registration and authentication as separate useful increments. Apply tenant isolation and baseline permission checks from the start. Keep the UX and domain simple; do not build complete accounting.

| Step | Cohesive increment | Current status |
| --- | --- | --- |
| 1 | Global user persistence model and schema constraints. | Complete; isolated PostgreSQL runtime validation passed. |
| 2 | Registration, bounded input validation, canonical email handling, and secure password hashing. | Complete; actual HTTP/PostgreSQL and independent Argon2id validation passed. |
| 3 | Authentication and session/token lifecycle. | Complete; actual HTTP/PostgreSQL lifecycle, failure, and rollback validation passed. |
| 4 | Organizations, memberships, and baseline tenant/permission enforcement. | Complete; creation/owner membership, scoped list/get, and owner/admin renaming. |
| 5 | Membership-scoped contact management with customer/supplier roles. | Complete; create/list/get/replace, owner/admin writes, all-member reads, and actual PostgreSQL tenant-isolation validation. |
| 6 | Tenant-scoped income/expense transaction categories. | Complete; create/list/get/rename, owner/admin/accountant writes, all-member reads, immutable API kind, and race-safe scoped name uniqueness. |
| 7 | Financial accounts, transaction records, and balances in useful increments. | Next; begin with account persistence and exact IRR/transaction semantics before recording financial activity. |

**Outcome:** An authorized user can maintain reliable records for their own business, with balances that reconcile to those records.

### Phase 2 — Money Owed & Obligations

**Goal:** Deliver practical daily cash-management value.

Implement receivables, payables, partial payments, due dates, overdue tracking, checks/general financial obligations, reminders, upcoming obligations, aging, and a basic cashflow dashboard. Define how payments and checks affect balances without duplicate recognition.

**Outcome:** Owners can see who owes them, what they owe, and which commitments need attention.

### Phase 3 — Business Dashboard

**Goal:** Give the owner a clear daily understanding of the business.

Implement today's activity, financial snapshot, overdue receivables, upcoming obligations, period comparisons, attention feed, and dashboard insights. Example fields include revenue, collected cash, expenses, open receivables, and upcoming obligations.

**Outcome:** A daily brief highlights actionable priorities and explains its figures without requiring accounting expertise.

### Phase 4 — AI Business Assistant

**Goal:** Natural-language questions over authorized business data.

Implement the provider abstraction, deterministic business tools, natural-language questions, conversational context, permission enforcement, and usage tracking. Initial questions cover sales today/month, expenses, largest debtors, upcoming payments, overdue receivables, cash position, and period comparisons.

**Outcome:** Answers agree with deterministic records and reveal missing evidence. **AI does not mutate data in this phase.**

### Phase 5 — Documents & Smart Data Entry

**Goal:** Reduce manual entry while preserving financial accuracy.

Implement receipt/invoice/check-image upload, OCR/document extraction, structured previews, confirmation, attachment-backed transaction creation, and CSV/Excel imports as validated needs justify them.

**Outcome:** Users can review and confirm extracted or imported data before it becomes a financial record. Uncertain OCR never silently creates records.

### Phase 6 — Automation & Actions

**Goal:** Turn the assistant into an authorized operator.

Implement an action framework, reminder and payment follow-up workflows, scheduled actions, permissioned AI tools, approvals/confirmations, audit trails, idempotency, and safe execution. Build on baseline security and audit capabilities rather than postponing them until now.

**Outcome:** A request such as “find customers more than 30 days overdue; send reminders to all except Ahmadi” produces a reviewable action and an accurate execution result. Users can revoke automation authority.

### Phase 7 — Forecasting & Intelligence

**Goal:** Help owners make decisions beyond reviewing history.

Implement cashflow forecasting, anomaly detection, expense trends, customer concentration, scenarios, warnings, and predictive insights. Establish forecast inputs and evaluate predictions against actual outcomes.

**Outcome:** Warnings such as a possible 38M Toman shortage in 12 days show their assumptions, uncertainty, and data freshness.

### Phase 8 — Integrations

**Goal:** Reduce duplicate work through useful, verified connections.

Potential adapters include accounting software, POS, e-commerce, payments, banking/open banking, SMS, tax/invoicing, external APIs, and webhooks. Earlier phases may add a specific adapter when their concrete capability requires it; this phase broadens the integration offering.

**Outcome:** Selected integrations have verified current technical/legal availability, safe permission handling, traceable synchronization, and defined duplicate/retry behavior.

### Phase 9 — SaaS Monetization

**Goal:** Fund useful capabilities with transparent pricing and usage.

Implement Free/Pro/Business plans, usage metering, credits and credit ledger, subscription lifecycle, billing invoices, payment provider abstraction, usage limits, and centralized feature entitlements.

**Outcome:** Customers understand limits and charges; costly actions cannot create duplicate charges on retry. Choose prices only after real customer validation.

### Phase 10 — Multi-User & Accountant Platform

**Goal:** Support deeper collaboration and accountant-led workflows.

Extend Phase 1 memberships with advanced roles/permissions, accountant access across clients, multiple branches, approvals, enhanced audit views, collaboration, and an accountant dashboard.

**Outcome:** Teams and accountants can work within explicit tenant and branch scopes while owners retain control.

### Phase 11 — Platform & Scale

**Goal:** Scale the validated product when actual usage requires it.

Potential work includes a public API, expanded webhooks, partner integrations, advanced observability, queue scaling, caching, read models, event/outbox patterns, and infrastructure scaling.

**Outcome:** Measured bottlenecks or delivery requirements justify each change. Do not introduce distributed systems, microservices, or infrastructure complexity without evidence.

## Engineering Principles

- **Boring and reliable:** Every technology and dependency must solve a current problem. Prefer standard-library and existing project capabilities.
- **Real module boundaries:** Organize around implemented domains. Keep platform code focused and avoid speculative empty modules.
- **Thin handlers:** Transport parsing and validation belong at the boundary; application services own behavior and invariants.
- **Explicit errors:** Use consistent stable codes and safe messages; distinguish domain failures from infrastructure failures.
- **Context and lifecycle:** Propagate cancellation and deadlines. Handle shutdown and resource cleanup deliberately.
- **Atomic changes:** Use explicit database transactions for operations that must succeed together; external side effects need defined retry and failure semantics.
- **Exact money and canonical time:** Follow the documented representation and avoid floating-point amounts or Jalali database timestamps.
- **Security at the boundary:** Authenticate, authorize, enforce tenant scope, validate input, and audit sensitive actions in server-side logic.
- **Secrets:** Never commit API keys, tokens, real database credentials, or other secrets. Supply `.env.example` with placeholders when configuration is implemented; keep actual local secrets out of version control.
- **Operational trust:** Introduce appropriate rate limiting, logs/metrics, backups with restore validation, and data lifecycle controls with the features and deployment they protect. Avoid logging sensitive financial content unnecessarily.
- **Focused changes:** Preserve existing work, reuse established patterns, follow repository formatting, and avoid unrelated cleanup or dependency upgrades.

### Validation and tests

Validate each increment according to its behavior. Documentation changes need content, naming, link, and diff review. `pwsh -File ./dev.ps1 check` checks Go formatting without writing, then runs `go test ./...`, `go vet ./...`, and `go mod verify`, failing at the first error. `fmt` applies `gofmt -w cmd internal`; `fmt-check` lists unformatted files and returns nonzero, whereas native `gofmt -l cmd internal` requires checking its output manually. Standard `go vet` is the current lint baseline; add another linter for a demonstrated gap. The script requires PowerShell 7; native commands remain documented for other shells. The GitHub Actions job runs these checks on Ubuntu 24.04, adds `go build ./...`, and verifies module files stayed unchanged. Validate workflow syntax with actionlint and report actual hosted execution separately from local checks. Windows commands and Windows/Linux builds were validated locally; native Linux command execution is checked by CI. `go run ./cmd/api` starts the server when PostgreSQL and the required connection string are configured. Check liveness/readiness responses during serving, dependency outages/recovery, and shutdown, request IDs, structured logs, safe configuration/connection/binding failures, timeouts, cancellation, and pool cleanup with the built executable. Validate Compose with `docker compose config --quiet`; use isolated projects for authenticated SQL, pool limits, and persistence across container recreation. Validate migration status, apply/reapply, safe rollback and failure behavior, concurrent execution, cancellation/deadlines, schema/history consistency, and connection cleanup against isolated PostgreSQL. Allow for separate driver/lock cleanup when testing cancellation. There are no project test files yet, so `go test` currently checks package compilation; the CI foundation does not replace database or business-behavior validation.

Meaningful future tests should protect business invariants, database behavior, and important HTTP contracts rather than chase arbitrary coverage or mock everything. The global rule remains in effect: **do not create new test files or modify existing tests without explicit user authorization for that task**. Existing tests may be inspected and run when useful. No tests are being added in this step.

## Development Workflow

This project's explicit workflow uses **`main` only** and supersedes the general stage/feature-branch and manual-commit rules for this project. Do not create feature branches, implement on `stage`, or switch to another base. Fetch the configured `origin` before changes and inspect whether local and remote history diverge; never merge automatically.

Before changes, inspect:

```text
git status
git branch --show-current
git log --oneline -n 20
git remote -v
```

Preserve unrelated and uncommitted work; never automatically reset, clean, restore, delete, or stash it. Stage only files belonging to the current increment.

For every increment:

1. Implement one cohesive change within the current phase.
2. Format the changed files as appropriate.
3. Run useful existing validation.
4. Inspect the full change, status, scope, and any temporary/debug content.
5. Create exactly one meaningful commit with a precise Conventional Commit message.
6. Attempt to push to `origin/main` when an `origin` remote exists.
7. Report the result and stop until the human says `continue`.

The implementation agent is explicitly authorized to commit and push each validated increment. Never merge automatically, amend existing commits routinely, commit credentials, or invent a remote URL. If no `origin` exists, keep the local commit and report the push as skipped. If pushing fails, retain the commit and report the error. Review newly created untracked files directly; ordinary `git diff` does not include them.

### Report after each step

Report the current branch and phase, step completed, files added/modified, implementation, validation, important decisions, risks/TODOs, commit hash/message, push status, and next recommended step.

### Explicitly outside the initial foundation

Do not implement AI chat, OCR, billing, credits, SMS, tax integrations, complete accounting, Redis, Kafka, microservices, Kubernetes, event sourcing, CQRS, or a mobile application in Phase 0. They belong to later validated requirements. Phase 1 currently implements global identity, sessions, organizations, scoped contacts, transaction categories, and per-operation membership/role authorization; monetary account/transaction/balance capabilities follow separately.

## Decisions & Changes

| Date | Decision | Reason / consequences |
| --- | --- | --- |
| 2026-10-04 | Name the product and repository `biznes`. | The owner's chosen international name; use it consistently in documentation and future code/configuration. |
| 2026-10-04 | Begin with a financial intelligence layer and evolve toward an authorized business operator. | Deliver daily business value without attempting complete accounting at launch. |
| 2026-10-04 | Target Iran first while keeping localization at boundaries. | Support local business realities without permanently coupling the core to one country. |
| 2026-10-04 | Use a modular monolith with Go, Gin, PostgreSQL, and Docker Compose. | A reliable initial stack; introduce additional infrastructure only for concrete needs. |
| 2026-10-04 | Use integer minor-unit money with explicit currency; IRR canonical storage and Toman input/display initially. | Exact arithmetic and a path to additional currencies without implicit denomination mixing. |
| 2026-10-04 | Use canonical UTC instants, explicit date-only semantics, and opaque UUID IDs. | Consistent storage and APIs; business timezone and calendar conversion stay explicit. |
| 2026-10-04 | Keep AI read-only in Phase 4; introduce controlled actions in Phase 6. | Establish trusted data access before allowing financial mutations or external effects. |
| 2026-10-04 | Use `main` only and stop after the first documentation increment. | Follow the project-specific prompt; leave all changes uncommitted for human review. |
| 2026-10-04 | Authorize one validated commit and push per turn on `main`, then wait for `continue`. | The autonomous development protocol supersedes the earlier manual-commit workflow; preserve unrelated work and never merge automatically. |
| 2026-10-04 | Initialize `github.com/wikiccu/biznes` with Go 1.27.1 and a minimal `cmd/api` executable. | Derive the module path from the configured GitHub remote; introduce dependencies and runtime capabilities only in their own increments. |
| 2026-10-05 | Use native pgx v5.11.0 pooling, a required API connection string, and a bounded database readiness ping. | Reuse driver connection/pool/TLS settings, verify the dependency before HTTP starts, keep process liveness independent during outages, and close the pool after HTTP drains. |
| 2026-10-05 | Use Goose v3.26.0 through an explicit `cmd/migrate` command; create a `biznes` application namespace and keep version history in `public`. | A mature context-aware migration provider with native PostgreSQL advisory locking; pin this stable release to preserve existing API dependency versions. Schema changes and rollback are explicit, versioned, and transactional; the initial rollback protects non-empty schemas. No schema auto-sync or business tables. |
| 2026-10-05 | Establish the `/api/v1` business contract and share routing/recovery error serialization. | Consistent safe JSON errors with request IDs, native Gin method handling, and explicit success/input/pagination/time/ID conventions. Keep probes simple and implement endpoint-specific decoders/DTOs/pagination with real features, without placeholder routes or additional dependencies. |
| 2026-10-05 | Use one PowerShell 7 script for developer commands, `gofmt` for formatting, and `go vet` as the lint baseline. | Works with the existing Windows tools without Make/Task or another Go dependency. Checks fail on formatting differences and native failures; native commands remain available in other shells. Database startup and migration/rollback are explicit, and stopping Compose retains the data volume. CI is a separate increment. |
| 2026-10-05 | Add one Ubuntu 24.04 GitHub Actions job, reusing developer checks and pinning checkout v7.0.1/setup-go v7.0.0 to release commits. | Select Go from `go.mod`, cache by `go.sum`, validate formatting/tests/vet/modules/builds without a database or secrets, and reject changed module files. Read-only permissions, a job deadline, and cancellation bound the workflow. Phase 0 implementation is complete; confirm hosted CI before beginning user persistence in Phase 1. |
| 2026-10-05 | Begin Phase 1 with a global user model and explicit users migration, after hosted Phase 0 CI passed. | Use native PostgreSQL UUIDv4 defaults, canonical unique ASCII email identities, bounded opaque password verifiers, and creation/update instants. Keep hashing, registration, authentication, and organization membership separate. Refuse non-empty rollback under a table lock to preserve identities against concurrent writes; use existing libraries and add no dependencies. |
| 2026-10-05 | Implement public registration with strict JSON, application-level email/password validation, and salted Argon2id through the existing crypto dependency. | Follow [OWASP's storage minimum](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) and [password length guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html). Preserve passwords exactly, return only public user fields, reuse the users schema/native pgx insert, and let database uniqueness protect races. Bound work per process and SQL deadlines without extra libraries/configuration. No sessions, email verification, or organizations in this increment. Before public launch, implement mailbox verification, password screening, trusted-ingress client limits, and indistinguishable registration responses to address the current availability disclosure. |

| 2026-10-06 | Add native PostgreSQL-backed opaque bearer sessions, login, current-user lookup, and logout revocation. | Reuse the strict credential boundary, fixed-cost Argon2id, safe errors, native pool, and shared password budget. Random 256-bit tokens retain only SHA-256 digests in storage; PostgreSQL enforces eight-hour absolute/fifteen-minute idle limits and shared revocation across replicas/restarts. Use explicit migration 00003 with FK/time/hash constraints, expiry maintenance index, and guarded non-empty rollback. No signing keys, dependencies, refresh infrastructure, or tenant access. Verify all lifecycle/error/rollback paths against isolated PostgreSQL; keep public-launch verification/screening/client limits and stronger financial-operation controls explicit. |
| 2026-10-06 | Add organizations, initial owner membership, and per-operation tenant/role authorization. | Use concrete native pgx transactions and qualified SQL; derive the caller from bearer authentication, join membership for every read/list, and lock membership through owner/admin rename. Follow [OWASP per-request authorization guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html#validate-the-permissions-on-every-request). Migration 00004 supplies UUID/timestamp defaults, bounded Unicode names, role/FK/composite-key constraints, reverse membership lookup, and guarded non-empty rollback. Implement the first actual pagination endpoint and reuse strict JSON input, without dependencies/configuration or speculative permission infrastructure. Member management and contacts/finance follow in separate increments. |
| 2026-10-07 | Add tenant-scoped contact records with customer/supplier/both classification and basic details. | Use concrete pgx SQL, composite organization/contact keys, restricted FKs, bounded native field constraints, stable indexing, and guarded migration 00005. Reuse strict JSON, UUID/pagination boundaries, and the transaction-held membership lock for reads/writes; owner/admin can write and every member can read. PUT replaces editable fields and clears omitted optional strings; email is unverified contact data, phone stays free-form, and multiline notes preserve text. Validate both-tenant access, duplicate IDs across tenants, role changes/removal, cancellation, outages, and rollback against real HTTP/PostgreSQL. No dependencies/settings, contact deletion/history, or financial behavior; financial persistence/categories follow next. |
| 2026-10-07 | Begin finance with tenant-scoped income/expense transaction categories. | Reuse concrete pgx SQL, strict JSON/UUID/pagination, and current membership locking, without dependencies/configuration or speculative money code. Explicit migration 00006 supplies composite keys, restricted FK, Unicode/kind bounds, deterministic organization/kind/name uniqueness, stable indexing, and guarded rollback. Keep API kind fixed, allow owner/admin/accountant creation/rename and all-member reads, and preserve prior contact/organization policies. Verify conflicts/races, tenant scope, role changes, cancellation, storage outages, and rollback using actual HTTP/PostgreSQL. Categories contain no amounts or balances; accounts and exact IRR transaction semantics follow before financial recording. |

For future major changes, add the date, decision, reason, affected capabilities/phases, and any migration implications. Update the relevant sections and Current State together so the blueprint continues to describe both the destination and the actual repository.
