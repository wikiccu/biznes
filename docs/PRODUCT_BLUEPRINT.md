# biznes — Product Blueprint

| Metadata | Value |
| --- | --- |
| Status | Living Document |
| Product | biznes |
| Repository | `biznes` |
| Architecture | Modular Monolith |
| Current Phase | Phase 0 — Product & Engineering Foundation |
| Current Increment | Step 1 — Product blueprint and README |
| Last Updated | 2026-10-04 |

This document describes the **intended final product**, its architecture direction, and an incremental path toward it. It is the source of truth for product vision, scope, feature planning, engineering decisions, and onboarding future developers and AI coding agents. Planned capabilities are not implemented capabilities.

Update this document when product direction, module boundaries, major technical choices, or roadmap priorities change. Record significant decisions in [Decisions & Changes](#decisions--changes). Features should advance this vision rather than accumulate as unrelated additions.

## Current State

The repository contains this blueprint and a concise `README.md`. The initial branch is `main`; the repository had no commits, application files, or configured remote at inspection. The first documentation increment is ready for human review and commit.

Implemented:

- The long-term product vision, capability map, architectural direction, and phased roadmap.
- Initial documented conventions for money, time, identifiers, APIs, tenant ownership, and development workflow.
- A README describing the current state and next development step.

Not implemented:

- Go module, application entry point, configuration, logging, or Gin HTTP server.
- Health endpoint, middleware, graceful shutdown, or shared API error handling.
- PostgreSQL environment, migrations, Docker setup, developer tooling, or CI.
- Authentication, business data, AI, integrations, or any other product capability.

Phase 0 remains in progress. Only its first documentation step is complete. Planned stack components and design conventions below describe implementation direction, not existing runtime behavior.

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

Candidate future modules are `identity`, `organization`, `contact`, `finance`, `receivables`, `payables`, `obligations`, `documents`, `assistant`, `actions`, `analytics`, `notifications`, `integrations`, `billing`, and `audit`. Names and boundaries may evolve with implemented use cases. Do not create empty modules, provider interfaces, or frameworks just to mirror this list.

Initial directory direction, introduced only as files become necessary:

```text
biznes/
├── cmd/api/
├── internal/
│   ├── platform/
│   │   ├── config/
│   │   ├── database/
│   │   ├── http/
│   │   └── logging/
│   └── <implemented domain modules>/
├── migrations/
├── docs/PRODUCT_BLUEPRINT.md
├── .env.example
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

This is a planned shape, not the current repository tree. Avoid a single global `services/`, `repositories/`, or `controllers/` hierarchy that mixes all domains.

## 7. Technical Direction

The initial backend stack is **Go, Gin, PostgreSQL, and Docker Compose**. Select supported stable versions at implementation time and record/pin them in the relevant files. Prefer the standard library where reasonable, including configuration, structured logging, HTTP server lifecycle, and signals.

Add a dependency only for a concrete need. Inspect current stable ecosystem conventions before choosing unspecified libraries, such as a database driver or migration tool. Use explicit migrations, never production ORM auto-sync.

Possible later components include Redis, S3-compatible storage, background workers, OpenTelemetry, LLM providers, OCR, and SMS providers. Introduce each with the phase that actually needs it. No current versions, providers, or pricing are being selected in this documentation increment.

## 8. API Direction

Use versioned REST APIs under `/api/v1/...`. Phase 0 provides `GET /health` with a minimal successful response:

```json
{"status":"ok"}
```

It is a process health check unless documented otherwise. Database readiness, if later introduced, should have explicit semantics rather than making this response imply unimplemented checks. Do not create fake business endpoints or introduce GraphQL prematurely.

The following are initial conventions to implement and refine with the first real API:

| Concern | Convention |
| --- | --- |
| Field names | English `snake_case` JSON and database names; idiomatic Go names internally. |
| Successful responses | Business APIs return `data` and optional `meta`; health retains its simple response. |
| Errors | An `error` object with stable `code`, safe `message`, optional field `details`, and `request_id`. Never expose secrets or internal stack traces. |
| Validation | Parse and validate transport input at the handler boundary; application services enforce domain invariants. |
| Status codes | Use meaningful HTTP codes: 400 malformed input, 401 unauthenticated, 403 forbidden, 404 unavailable resource, 409 conflict, 422 invalid domain input, 429 rate limit, and safe 5xx failures. Avoid existence leaks across tenants. |
| Pagination | Start with bounded `page`/`limit` pagination and count metadata where affordable; document defaults and caps. Use stable ordering with an ID tie-breaker. Change to cursors only for an evidenced need. |
| Filtering/sorting | Allowlist supported fields, validate ranges, and document order; never interpolate arbitrary client input into SQL. |
| Timestamps | RFC 3339 UTC instants; ISO `YYYY-MM-DD` for genuine date-only values, with business timezone semantics. |
| IDs | Opaque UUID identifiers; never treat knowing an ID as authorization. |
| Money | Integer minor units and currency; transmit potentially large amounts as decimal strings, not floating-point JSON values. |
| Authentication | Select the concrete session/token mechanism in Phase 1; never trust client-supplied user identity. |
| Authorization | Resolve organization access from authenticated membership and permissions on every operation, including tools, exports, and jobs. |
| Request ID | Attach a bounded, validated request ID to responses, errors, and logs; generate one when needed. |

Example business error shape, planned rather than implemented:

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

PostgreSQL is the initial authoritative store. Define constraints and indexes through reviewable migrations. Migration conventions and tooling are a separate Phase 0 increment; there is no schema yet.

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

Use opaque UUID primary identifiers consistently. Select a mature generation mechanism in the implementation step rather than adding a dependency now. References should be constrained and indexed appropriately. UUIDs do not replace access control.

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
| 1 | Product blueprint and concise README. | Complete; awaiting human review/commit. |
| 2 | Go module and minimal application initialization. | Not started. |
| 3 | Configuration and structured logging. | Not started. |
| 4 | Gin server, `/health`, request ID, recovery, basic errors, graceful shutdown. | Not started. |
| 5 | Docker/Compose and PostgreSQL local development environment. | Not started. |
| 6 | Migration foundation with documented commands. | Not started. |
| 7 | Formatting/linting and developer tooling. | Not started. |
| 8 | Initial validation and CI foundation. | Not started. |

Review and propose a Conventional Commit message after every increment; stop before the next meaningful increment. After Step 1 is reviewed and committed by the human, the next work is Step 2. Phase 0 does not include product features.

### Phase 1 — Business Core MVP

**Goal:** A user can create a business and record fundamental information manually.

Implement authentication, organizations, membership, contacts with customer/supplier roles, income, expenses, basic transactions, categories, and balances. Apply tenant isolation and baseline permission checks from the start. Keep the UX and domain simple; do not build complete accounting.

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

Validate each increment according to its behavior. Documentation changes need content, naming, link, and diff review. As code is added, formatting, `go vet`, existing `go test` checks, and an appropriate lint command should form the validation/CI foundation. These commands are planned; no Go toolchain configuration exists yet.

Meaningful future tests should protect business invariants, database behavior, and important HTTP contracts rather than chase arbitrary coverage or mock everything. The global rule remains in effect: **do not create new test files or modify existing tests without explicit user authorization for that task**. Existing tests may be inspected and run when useful. No tests are being added in this step.

## Development Workflow

This project's explicit workflow uses **`main` only** and supersedes the general stage/feature-branch rule for this project. Do not create feature branches, implement on `stage`, or switch to another base. There is currently no configured remote to fetch.

Before changes, inspect:

```text
git status
git branch --show-current
git log --oneline -n 10
```

The history command will report that no commits exist until the human makes the initial commit. Preserve unrelated and uncommitted work; never automatically reset, clean, restore, delete, or stash it.

For every increment:

1. Implement one cohesive change within the current phase.
2. Format the changed files as appropriate.
3. Run useful existing validation.
4. Inspect the full change, status, scope, and any temporary/debug content.
5. Explain the outcome and propose a precise Conventional Commit message.
6. Stop before the next meaningful increment for human review.

Do not automatically commit, push, or merge. The human controls commits and integration. Review newly created untracked files directly; ordinary `git diff` does not include them.

### Report after each step

Report the current branch and phase, step completed, files added/modified, implementation, validation, important decisions, risks/TODOs, suggested commit message, and next recommended step.

### Explicitly outside the initial foundation

Do not implement AI chat, OCR, billing, credits, SMS, tax integrations, complete accounting, Redis, Kafka, microservices, Kubernetes, event sourcing, CQRS, or a mobile application in Phase 0. They belong to later validated requirements. The current step creates only this blueprint and README; technical setup follows review.

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

For future major changes, add the date, decision, reason, affected capabilities/phases, and any migration implications. Update the relevant sections and Current State together so the blueprint continues to describe both the destination and the actual repository.
