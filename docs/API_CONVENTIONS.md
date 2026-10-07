# API conventions

This is the contract for business endpoints introduced under `/api/v1`. The executable implements identity/session routes, membership-scoped organization/contact/transaction-category/financial-account/transaction routes with pagination and account/occurred-period filters, linked full reversal routes, recorded account activity, health/readiness, and shared routing/recovery errors. `/api/v1` itself is not an endpoint. Handlers implement strict input validation and explicit success DTOs.

## Routes and responses

Register each implemented domain's routes in a Gin group with prefix `/api/v1`. Use English resource names and `snake_case` JSON fields. Breaking business contract changes require a new major API prefix. Define each endpoint's input, output, authorization, filters, and ordering alongside its implementation.

Business successes return `application/json` with a `data` member. For a single resource, `data` contains its DTO; for a list, it contains an array, including `[]` for an empty result. Use an explicit response DTO to choose public fields; do not serialize database models containing credentials or internal fields. Extra metadata belongs in optional top-level `meta`.

```json
{"data": [], "meta": {"page": 1, "limit": 20}}
```

The organization list implements this envelope. Use `200` for retrieval/update results and `201` for creation, with `Location` when the new resource has a retrieval URL. A `204` response has no JSON body. Successful responses must not contain an `error` member.

`GET /health` and `GET /ready` remain outside the business API prefix and keep their small `status` responses. Readiness checks database connectivity, not schema or authorization. Every request handled by Gin receives `X-Request-ID`; a supplied ID is accepted only under the bounded existing middleware rules. Do not duplicate it in successful business DTOs.

## Errors

Use `httpserver.WriteError` in `internal/platform/http` for public handler errors. It aborts the handler chain, writes the JSON envelope below, and sets `Cache-Control: no-store`. Return from the handler immediately after calling it; aborting the chain does not return from the current function. It obtains `request_id` from middleware; callers must not copy a raw header. If the response has already been committed, it aborts without appending an error body or attempting to replace the status. Recovery records the panic safely; it can produce a `500` envelope only before response commitment.

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

`details` is omitted when absent. Each `ErrorDetail` uses a public JSON/query/path field name and a stable code, such as `required`, `invalid_format`, `out_of_range`, or `invalid_date`. Report bounded field details in deterministic field order. Messages are safe developer-chosen English text; clients should use codes for behavior and localization. Never pass raw parser, validator, SQL, or dependency errors, panic values, rejected values, secrets, or arbitrary client strings into this writer. Application services retain domain errors; handlers deliberately map them to the public contract without a generic error-classification framework.

| Status | Default code | Meaning |
| --- | --- | --- |
| `400` | `invalid_request` | Malformed JSON, wrong JSON types, extra JSON values, unknown fields, malformed path/query input. |
| `401` | `unauthenticated` | Authentication is required or invalid; use the authentication scheme's required challenge header. |
| `403` | `forbidden` | An authenticated caller lacks permission for an action. |
| `404` | `not_found` | Unknown route or unavailable resource, including a resource outside the caller's tenant. |
| `405` | `method_not_allowed` | The path is registered for other methods; include `Allow`. |
| `409` | `conflict` | The requested change conflicts with current state. |
| `413` | `payload_too_large` | The endpoint's request body limit is exceeded. |
| `415` | `unsupported_media_type` | The endpoint does not accept the request's content type. |
| `422` | `validation_failed` | Parsed input violates field or domain constraints. |
| `429` | `rate_limited` | The caller exceeded an implemented rate limit; include `Retry-After` when known. |
| `500` | `internal_error` | Unexpected internal failure, with a fixed safe message. |
| `503` | `service_unavailable` | A dependency needed for the requested operation is unavailable. |

Identity, organization, contact, category, account, transaction, and reversal endpoints implement the applicable errors below, including membership/role enforcement. `/ready` retains its probe-specific `503` payload. Shared routing returns JSON `404` for unknown paths and JSON `405` with Gin's `Allow` header for unsupported methods on known paths. That includes `HEAD`/`OPTIONS` unless those methods are explicitly registered; HTTP `HEAD` responses carry only headers and status, without a body. Trailing-slash redirects remain disabled. A missing business resource must not disclose whether it exists in another tenant.

HTTP status and header semantics follow [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#section-15). Responses created by the HTTP server before Gin handles a request, such as malformed HTTP framing, do not necessarily have this envelope or a request ID.

## Input and validation

At each business handler boundary:

1. For JSON body endpoints, require the `application/json` media type, allow a UTF-8 charset parameter, and limit the body to `1 MiB` by default using `http.MaxBytesReader`. Upload endpoints must define their own limits and media types.
2. Decode exactly one JSON object into an explicit input DTO. Reject missing bodies, `null`, arrays, unknown fields, wrong types, and any trailing non-whitespace content. Use the standard-library decoder with `DisallowUnknownFields` and a second decode expecting `io.EOF`. Unknown-field checks alone do not enforce the object shape or a single JSON value.
3. Validate required fields, string lengths, formats, and ranges after decoding. Use the existing Gin validator where appropriate and select safe field/code details explicitly. Transport parsing failures are `400`; validly parsed but unacceptable values are `422`.
4. Call application logic with the request context. Application services enforce ownership and business invariants, including calls from future AI tools, imports, and jobs. Do not rely solely on transport validation or client-supplied tenant identity.

Require UTF-8 JSON as defined by [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259.html#section-8.1). Reject duplicate object keys, invalid UTF-8, and property names that do not match the published field names. The [standard decoder](https://pkg.go.dev/encoding/json#Decoder) alone accepts duplicate keys, matches struct names case-insensitively, and replaces invalid UTF-8; account for those behaviors when implementing the input boundary. Do not use maps of floating-point values to decode money. Use explicit string/integer types with checked conversions. Avoid Gin binding methods that write an automatic response; input errors must reach the shared writer with the chosen status and safe fields.

Query handlers must reject malformed percent encoding, unexpected parameters, and duplicate single-valued parameters. Allowlist filtering/sorting fields and values, validate their bounds, and pass only parameterized values to SQL. Never interpolate arbitrary client query text into SQL identifiers or order clauses.

## Registration

`POST /api/v1/auth/register` is public and creates a global identity. It accepts no query string, including an empty `?`, and requires a single `Content-Type: application/json` header, optionally with `charset=utf-8` (case-insensitive). Other media parameters are rejected. Its endpoint-specific body cap is **8 KiB**, including whitespace, enforced for declared-length and chunked bodies.

```json
{"email":" Owner.Name+Tag@EXAMPLE.COM ","password":"a long private passphrase"}
```

Only exact `email` and `password` string properties are accepted. Duplicate keys (including equivalent escaped names), unknown keys, nulls/wrong types, invalid UTF-8, unpaired surrogate escapes, non-object input, and trailing non-whitespace produce `400 invalid_request`. Missing/empty fields produce `422 validation_failed` with `required`; email format failures use `invalid_format`, and password length failures use `out_of_range`. Details are ordered email then password and contain no supplied values.

Email normalization trims surrounding whitespace and lowercases ASCII after validation, preserving dots and plus tags. Accepted addresses use an unquoted local part of at most 64 bytes and a DNS domain with at least two labels. Labels contain ASCII letters/digits/hyphens, have 1–63 bytes, and cannot start/end with a hyphen; the full canonical email is at most 254 bytes. Display names, comments, domain literals, Unicode addresses, and provider-specific rewriting are excluded. Syntax validation does not prove mailbox ownership or deliverability.

Passwords have 15–128 Unicode code points, following the length direction in [OWASP's authentication guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html). Preserve every decoded character, including spaces and Unicode; never trim, normalize, truncate, or require particular character classes. Registration hashes with Argon2id v19 using 19 MiB, two passes, one lane, a random 16-byte salt, and a 32-byte key, encoded as `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>` with unpadded standard Base64. These costs meet the current [OWASP minimum](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html); benchmark and raise them as deployment resources permit.

Success is `201` with `Cache-Control: no-store` and an explicit `data` object containing `id`, `email`, `created_at`, and `updated_at`. IDs are canonical lowercase UUIDs and timestamps are UTC RFC 3339 instants. No password/verifier, session/token, organization access, or retrieval `Location` is returned. Registration continues to use the users schema; authentication/session routes require the separate migration described below.

Each API process shares one password operation at a time and at most one start per second across registration and login, after input validation and before hashing. Excess work returns `429 rate_limited` with `Retry-After: 1`. Hashing costs are fixed and cannot be interrupted; cancellation prevents subsequent persistence. Database operations have five-second deadlines, honor request cancellation, and use parameterized SQL. Storage failure returns a safe `503 service_unavailable`. PostgreSQL uniqueness handles registration races across API processes; an existing email returns `409 conflict` with a generic message and leaves all account fields unchanged. Success, failure, and application logs never expose passwords/verifiers or raw database/parser errors; the bearer token appears only in its deliberate login response field.

Public-launch requirements remain email ownership verification, common/breached password screening, and trusted-ingress limits per client. The current process-wide budget can be exhausted by one client and is independent in each replica. Distinct creation/conflict statuses reveal email availability even though duplicate messages omit the email; address this with indistinguishable responses as part of an ownership-verification flow. Use HTTPS for credential traffic.

## Authentication and sessions

`POST /api/v1/auth/login` accepts the registration transport contract: one strict email/password object, 8 KiB body cap, UTF-8 JSON media type, and no query string. Required/format/upper-length field failures use `422`; login accepts any non-empty password up to 128 Unicode code points without applying registration's minimum strength rule. Email normalization is shared with registration and passwords are preserved exactly.

Unknown accounts, wrong passwords, and unsupported stored password verifiers return the same `401 unauthenticated` message and `WWW-Authenticate: Bearer realm="biznes"`. All three run the fixed-cost Argon2id derivation, and comparison uses `crypto/subtle.ConstantTimeCompare`. Only the implemented v19/19 MiB/two-pass/one-lane policy, 16-byte salt, and 32-byte key are accepted; unsupported versions/costs, malformed Base64, and wrong component lengths cannot select work parameters or authenticate. This follows [OWASP's generic authentication-error guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#authentication-and-error-messages).

Success is `200` with `data.user` (the public user DTO), `data.access_token`, `data.token_type: "Bearer"`, and UTC `data.expires_at`. Each token contains 32 cryptographically random bytes, encoded as 43 unpadded Base64URL characters. Storage uses SHA-256 of those decoded bytes as the primary key in `biznes.sessions`, retaining no reusable token secret. Every successful login creates a fresh independent session. Responses use `Cache-Control: no-store`, set no cookie, and grant no business membership or role.

`GET /api/v1/auth/me` and `POST /api/v1/auth/logout` require an empty body, no query string, and a single `Authorization` header. Non-empty bodies/queries return `400 invalid_request` before session lookup. The HTTP-normalized header value is at most 128 bytes, uses case-insensitive `Bearer` followed by one or more ASCII spaces, and contains the exact canonical token. Padding, non-canonical Base64URL trailing bits, extra token content, duplicate headers, and alternate credential locations are rejected. Standard HTTP parsing normalizes outer header whitespace. Token transport and challenges follow [RFC 6750](https://www.rfc-editor.org/rfc/rfc6750.html#section-2.1).

Missing credentials return `401 unauthenticated` with `WWW-Authenticate: Bearer realm="biznes"`. Supplied invalid/unknown/expired/revoked credentials use the same safe JSON error and add `error="invalid_token"` to the challenge. No credential text or account detail appears in errors/logs. `me` returns `200` with `data` containing only the public user DTO. Logout removes the current session and returns empty `204`; subsequent checks using that token return `401`, including on other API processes. Concurrent requests already authenticated may finish after revocation. Other sessions are unaffected.

Sessions have an **eight-hour absolute lifetime** and a **fifteen-minute idle timeout**, enforced by PostgreSQL time on every lookup. A valid lookup atomically refreshes `last_seen_at` without extending `expires_at`. Expired sessions cannot be revived. These initial policies are informed by [OWASP session guidance](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html); revisit them with stronger authentication and risk controls before sensitive financial operations. Authentication queries and revocation have five-second/request-cancellation deadlines and fail closed with safe `503 service_unavailable` errors on storage failure. Sessions survive API restarts and are shared by replicas using the same database.

Apply `00003_create_sessions.sql` explicitly before deploying these routes. Expiry requires login again; no refresh, recovery, MFA, or device/session management is implemented. Organization authorization is checked separately through membership. Expired records remain unusable and require explicit [maintenance](../migrations/README.md#sessions) to bound growth; there is no background cleanup worker. No signing key, environment setting, or dependency is introduced.

## Organizations

All four routes require the existing bearer session before body/query/path validation. Identity comes exclusively from the authenticated context. The organization ID in a path selects a candidate resource; database membership authorizes it. User/role/tenant headers or body/query claims are never authorization inputs. This follows [OWASP's per-request authorization guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html#validate-the-permissions-on-every-request).

| Route | Access | Success |
| --- | --- | --- |
| `POST /api/v1/organizations` | Any authenticated user. | `201`, `Location: /api/v1/organizations/<id>`, `data` organization. |
| `GET /api/v1/organizations` | The caller's memberships only. | `200`, `data` array and `meta.page`/`meta.limit`. |
| `GET /api/v1/organizations/:organization_id` | Member of the selected organization. | `200`, `data` organization. |
| `PATCH /api/v1/organizations/:organization_id` | Owner or administrator of the selected organization. | `200`, renamed `data` organization. |

The explicit organization DTO contains only `id`, `name`, `role`, `created_at`, and `updated_at`. `role` is the caller's membership (`owner`, `admin`, `accountant`, or `staff`), not an organization-wide setting or another user's role. Timestamps are UTC instants; successful responses use `Cache-Control: no-store`. All roles can read organization metadata. Only owner/admin can rename; contact permissions are defined below; financial permissions need their own operation policies.

Create/rename share the identity endpoints' strict 8 KiB JSON boundary through `internal/platform/http/input.go`, accepting only a `name` string. Unknown/duplicate/case-variant fields, nulls, wrong types, malformed/trailing JSON, invalid UTF-8, and lone surrogate escapes return `400`. Missing/blank names return `422` with `name: required`; over 120 Unicode code points uses `out_of_range`; control characters use `invalid_format`. Control checks precede trimming, so tabs/newlines are rejected even at the edges. Unicode surrounding whitespace is trimmed; internal whitespace, Persian, combining characters, emoji, and zero-width non-joiners are preserved without normalization. Names need not be unique. Query strings are rejected on create/get/rename. GET requests require an empty body. Path IDs require canonical lowercase hyphenated UUID syntax (`400` otherwise).

Creation inserts the organization and initial owner membership in one transaction. The server sets the owner from authenticated identity; no owner/user/role input is accepted. A failed membership insert rolls back the organization. Creation is not idempotent: retrying an ambiguous successful request can create another organization. The list accepts only `page` and `limit` under the pagination contract below, ordered by `created_at ASC, id ASC`, and omits `total`. Empty/beyond-end pages return `[]`.

Single-resource reads join membership in the query. Unknown resources and resources outside the caller's memberships return the same safe `404 not_found`. A member with accountant/staff role receives `403 forbidden` for a valid rename. Rename checks and locks the membership row with `FOR SHARE` until its organization update commits, serializing concurrent removal/demotion; the update maintains `updated_at` and preserves `created_at`. Each service operation has a five-second/request-cancellation database deadline and returns a safe `503 service_unavailable` on storage failure. Membership is never cached in the bearer session. Requests already authorized may finish before a later revocation completes.

Apply `00004_create_organizations.sql` explicitly before deploying these routes. There are no invitations, member-management endpoints, ownership transfer, organization deletion, or branch policies yet. Creation grants only the creator's owner membership. Contacts, categories, and financial accounts use separate migrations and endpoints below. No dependency or environment setting is added.

## Contacts

All routes are beneath `/api/v1/organizations/:organization_id/contacts` and authenticate the bearer session before input validation. Both path IDs require canonical lowercase hyphenated UUID syntax. The organization is selected by the path and authorized from database membership using the authenticated user. Client body/query/header user, tenant, or role claims cannot grant access or move a contact.

| Route suffix | Access | Success |
| --- | --- | --- |
| `POST` collection | Owner/admin membership. | `201`, `data` contact, retrieval `Location`. |
| `GET` collection | Any membership. | `200`, `data` array and `meta.page`/`meta.limit`. |
| `GET /:contact_id` | Any membership. | `200`, `data` contact. |
| `PUT /:contact_id` | Owner/admin membership. | `200`, replaced `data` contact. |

The DTO contains only `id`, `organization_id`, `name`, `kind`, `email`, `phone`, `notes`, `created_at`, and `updated_at`. Optional text fields are strings, defaulting to `""`; timestamps are UTC instants, and all responses use `Cache-Control: no-store`. Names are not unique. A contact belongs to one organization and can be a customer, supplier, or both; these classifications are separate from a user's business role.

POST/PUT accept the shared strict UTF-8 JSON contract and 8 KiB body cap. Only the five editable string fields below are accepted. Missing `name`/`kind` yields `422 required`; nulls, wrong types, unknown/duplicate/case-variant keys, malformed/trailing JSON, invalid UTF-8, and lone surrogate escapes yield `400`. Query strings are rejected. Validation details appear in deterministic order: name, email, phone, notes, kind. Value-length failures use `out_of_range`; syntax/control failures use `invalid_format`.

| Field | Application validation |
| --- | --- |
| `name` | Required, trimmed Unicode whitespace, 1–120 code points, no control characters. |
| `kind` | Required exact `customer`, `supplier`, or `both`; no trimming/case conversion. |
| `email` | Optional; trim whitespace, at most 254 UTF-8 bytes, plain single mailbox parsed by Go `net/mail`, without a display name/comment/address rewriting. Preserve case and supported Unicode. This records contact data without verifying mailbox ownership or deliverability; identity login email policy remains separate. |
| `phone` | Optional free-form text; trim whitespace, at most 64 code points, no control characters. Preserve Persian digits and punctuation; no country-specific normalization or phone verification. |
| `notes` | Optional; preserve exact text, at most 2000 code points; allow LF/CR/tab, reject other control characters. |

Control checks precede trimming. Persian, combining marks, emoji, and zero-width non-joiners are preserved without normalization. PUT replaces all editable fields: name/kind remain required, and omitted email/phone/notes become empty strings. The tenant and resource ID cannot be changed. It preserves creation time and updates `updated_at`; concurrent authorized replacements use last committed values, without optimistic concurrency/version tokens. Creation is not idempotent; retries after ambiguous success can duplicate contacts.

Collection GET accepts only the shared bounded page/limit contract and requires an empty body. Results use `created_at ASC, id ASC` within the selected organization, with no count query; valid empty/beyond-end pages return `[]`. Listing first locks current membership in its transaction, so an inaccessible organization receives `404`, rather than an apparent empty list. Single GET requires no query/body and joins membership with both organization/contact predicates. Unknown or inaccessible resources share a safe `404 not_found`. Owner/admin writes also constrain both IDs; a contact from another organization cannot be accessed even if the caller belongs to both organizations. Read-only members receive `403 forbidden` for writes before contact lookup.

Contact writes and lists reuse `organization.LockMembership` (`FOR SHARE`) and hold it until commit, serializing concurrent membership removal/demotion. GET uses a membership join in its own statement. Every operation has a five-second/request-cancellation database deadline and fails closed on storage errors with safe `503` responses. Membership is never cached in a session; already authorized operations may finish before a later revocation completes. Contact content and raw storage/parser errors are omitted from application logs.

Apply `00005_create_contacts.sql` explicitly before deployment. The composite primary key is `(organization_id, id)`; organization IDs participate in every reference/lookup. There are no contact deletion/archive/search/filter/tag/history or balance endpoints yet, and no new dependency/configuration setting.

## Transaction categories

All routes are beneath `/api/v1/organizations/:organization_id/transaction-categories` and require the existing bearer session before input validation. The paths select candidates; current database membership authorizes each operation. Owner/admin/accountant members can create or rename categories; staff can read. This financial metadata policy does not grant accountants permission to manage contacts or organizations.

| Route suffix | Success |
| --- | --- |
| `POST` collection | `201`, `data` category, retrieval `Location`. |
| `GET` collection | `200`, `data` array and `meta.page`/`meta.limit`. |
| `GET /:category_id` | `200`, `data` category. |
| `PATCH /:category_id` | `200`, renamed `data` category. |

The explicit DTO contains `id`, `organization_id`, `name`, `kind`, `created_at`, and `updated_at`, with canonical UUIDs and UTC timestamps. Responses use `Cache-Control: no-store`. POST accepts only `name` and `kind` strings. PATCH accepts only `name`; kind and tenant/resource IDs cannot be changed. Both use the shared strict UTF-8 JSON/media contract and 8 KiB body cap, reject query strings, and return `400` for unknown/duplicate/case-variant keys, wrong types/nulls, malformed/trailing JSON, invalid UTF-8, or lone surrogate escapes.

Names are trimmed Unicode text, 1–120 code points, with control checks before trimming. Persian, combining characters, emoji, and zero-width non-joiners are preserved without normalization or case folding. Kind is exactly `income` or `expense`, without trimming/case conversion; transfers are outside those classifications. Missing/blank fields use `422 required`, excessive name length uses `out_of_range`, and controls/invalid kinds use `invalid_format`, with deterministic name-then-kind details. PATCH requires a name and preserves kind, ID, tenant, and creation time while maintaining `updated_at`.

Exact, case-sensitive names are unique per organization and kind under PostgreSQL `C` collation. The same name in another organization or the other kind is allowed. Duplicate creation/rename returns safe `409 conflict` without names, SQL, or existing record details in the error. Native uniqueness protects races across API processes. Failed renames leave the existing category unchanged. Retrying an identical trimmed name/kind returns a conflict while that name remains stored; a later rename can free it for creation again. There is no idempotency-key or optimistic-concurrency implementation, and successful concurrent renames use last committed values.

Collection GET accepts only page/limit, requires an empty body, orders by `created_at ASC, id ASC` within the organization, and omits `total`. Valid empty/beyond-end pages return `[]`; inaccessible organizations receive `404`. Single GET rejects body/query strings and joins membership with both organization/category predicates. All path IDs require canonical lowercase hyphenated UUID syntax. Unknown/inaccessible resources share a safe `404`, even when the caller belongs to multiple organizations. Staff receive `403` for valid writes before category lookup. Client user/role/tenant claims cannot grant access or move a category.

Writes/lists reuse `organization.LockMembership` through transaction completion, serializing concurrent removal/demotion. Single reads use a membership join. Each operation has a five-second/request-cancellation database deadline and returns a safe `503` for storage failure. Already authorized operations may finish before later revocation completes. Category names and raw dependency/parser errors are omitted from application logs. Apply `00006_create_transaction_categories.sql` explicitly before deploying the routes. The composite `(organization_id, id)` resource key preserves tenant scope. There are no category deletion/archive/hierarchy/default/filter or balance endpoints yet; no dependency/configuration setting is added.

## Financial accounts

All routes are beneath `/api/v1/organizations/:organization_id/financial-accounts` and require the existing bearer session before input validation. Owner/admin/accountant members can create or rename accounts; every member can read. Accountants' financial metadata permissions do not broaden contact or organization permissions.

| Route suffix | Success |
| --- | --- |
| `POST` collection | `201`, `data` account, retrieval `Location`. |
| `GET` collection | `200`, `data` array and `meta.page`/`meta.limit`. |
| `GET /:account_id` | `200`, `data` account. |
| `PATCH /:account_id` | `200`, renamed `data` account. |
| `GET /:account_id/recorded-activity` | `200`, derived activity `data`; see [recorded account activity](#recorded-account-activity). |

The account metadata DTO contains `id`, `organization_id`, `name`, `kind`, `currency`, `created_at`, and `updated_at`, with canonical UUIDs and UTC timestamps. Responses use `Cache-Control: no-store`. POST accepts exactly the required `name`, `kind`, and `currency` strings, for example:

```json
{"name":"Main cash","kind":"cash","currency":"IRR"}
```

PATCH accepts only `name` and preserves kind, currency, ID, tenant, and creation time while maintaining `updated_at`. Both use the shared strict UTF-8 JSON/media contract and 8 KiB cap, reject query strings, and return `400` for unknown/duplicate/case-variant keys, wrong types/nulls, malformed/trailing JSON, invalid UTF-8, or lone surrogate escapes. Opening/current balances and monetary amount fields are rejected as unknown inputs.

Names are trimmed Unicode text, 1–120 code points, with control checks before trimming. Persian, combining characters, emoji, and zero-width non-joiners are preserved without normalization or case folding. Kind is exactly `cash` or `bank`; currency is exactly `IRR`, with no trimming, casing conversion, default currency, or implicit foreign exchange. IRR uses whole Rial units (scale zero); Toman is a presentation denomination (1 Toman = 10 Rial). Other currencies require a deliberate later implementation and migration. Missing fields, trimmed blank names, and empty kind/currency return `422 required`, excessive name length uses `out_of_range`, and controls/invalid kind/currency use `invalid_format`, in deterministic name-then-kind-then-currency order.

Exact, case-sensitive names are unique per organization across both account kinds under PostgreSQL `C` collation. The same name in another organization is allowed. Duplicate creation/rename returns safe `409 conflict` without changing existing data or exposing names, SQL, or existing records. Native uniqueness protects races across API processes. Identical creation retries conflict while the name remains stored; renaming can free it. There is no idempotency-key or optimistic-concurrency implementation for this metadata, and successful concurrent renames use last committed values.

Collection GET accepts only page/limit, requires an empty body, orders by `created_at ASC, id ASC` within the organization, and omits `total`. Valid empty/beyond-end pages return `[]`; inaccessible organizations receive `404`. Single GET rejects body/query strings and joins current membership with both organization/account predicates. All path IDs require canonical lowercase hyphenated UUID syntax. Unknown/inaccessible resources share a safe `404`, even when the caller belongs to multiple organizations. Staff receive `403` for valid writes before account lookup. Client identity/role/tenant claims cannot grant access or move an account.

Writes/lists hold `organization.LockMembership` through transaction completion, serializing concurrent removal/demotion. Single reads use a membership join. Operations have five-second/request-cancellation database deadlines and safe `503` storage errors; already authorized operations may finish before later revocation completes. Account names and raw dependency/parser errors are omitted from application logs. Apply `00007_create_financial_accounts.sql` explicitly before deployment; the composite `(organization_id, id)` key preserves tenant scope. Account metadata and write routes contain no opening/current balances, transaction amounts, deletion/archive, bank/card identifiers or credentials, reconciliation, or integrations. Unreversed recorded totals are served separately below. No dependency/configuration setting is added.

## Income/expense transactions

All routes are beneath `/api/v1/organizations/:organization_id/transactions` and require the existing bearer session before input validation. Owner/admin/accountant members can record transactions; every member can read. Current membership is checked on every operation, including retries; staff cannot replay a write, and removed members receive `404`.

| Route suffix | Success |
| --- | --- |
| `POST` collection | `201` for new activity or `200` for an identical retry, `data` transaction, retrieval `Location`. |
| `GET` collection | `200`, `data` array and `meta.page`/`meta.limit`. |
| `GET /:transaction_id` | `200`, `data` transaction. |

POST accepts the required string fields `idempotency_key`, `account_id`, `category_id`, `amount`, `currency`, `occurred_at`, and optional `description` (default `""`). It uses the shared strict UTF-8 JSON/media contract, 8 KiB cap, and no query string. Unknown/duplicate/case-variant keys, numbers/nulls/wrong types, malformed/trailing JSON, invalid UTF-8, and lone surrogate escapes return `400`. Client `kind`, `created_by`, `organization_id`, `id`, and balances are not accepted.

```json
{
  "idempotency_key":"61f57640-2a52-4b4c-9baf-52e3ad93de75",
  "account_id":"3e44f242-4fb1-47ba-b92f-4bbcfeaf2a5a",
  "category_id":"9cfe2188-0b2b-49a2-b758-63227c647146",
  "amount":"85000000",
  "currency":"IRR",
  "occurred_at":"2026-10-07T12:34:56.123456+03:30",
  "description":"Cash receipt"
}
```

The three input IDs must be canonical lowercase hyphenated UUIDs; missing IDs use `422 required` and malformed values use `invalid_format`. Accounts/categories must exist in the selected organization, even when the caller belongs to both organizations. A new key with unavailable references returns a generic `404` without revealing which resource is missing. Direction is derived from the category and stored as `income` or `expense`; amount is always positive, with income contributing positively and expense negatively to future recorded net activity.

Amount uses canonical ASCII decimal digits without signs, leading zeroes, whitespace, fractions, separators, or exponents. Supported values are `"1"` through `"9223372036854775807"`, stored as PostgreSQL `BIGINT` and returned as decimal strings. Missing amount uses `required`; malformed strings use `invalid_format`; zero/overflow use `out_of_range`. Currency is required and exactly `IRR`, using whole Rial units (scale zero), and must match the account. No rounding, floating-point amount, Toman input, or implicit currency conversion is performed.

`occurred_at` is required: strict RFC 3339 with uppercase `T`/`Z` or a numeric offset, at most six decimal fractional digits, and a normalized UTC year from 1 through 9999. Invalid calendar/time/offset values, leap seconds, unsupported precision, and instants outside that UTC range use `422 invalid_format`; omission uses `required`. Sub-microsecond input is rejected instead of silently rounded by PostgreSQL. Offsets normalize to UTC; past and future instants are accepted without business-date restrictions. Description preserves whitespace/Persian/Unicode and supports up to 2000 code points, with LF/CR/tab allowed but other controls rejected. Excessive length uses `out_of_range` and invalid controls use `invalid_format`. Field details are ordered idempotency key, account, category, amount, currency, occurred time, then description.

The explicit response DTO contains `id`, `organization_id`, `idempotency_key`, `account_id`, `category_id`, `kind`, `amount`, `currency`, `occurred_at`, `description`, `created_by`, and `created_at`. IDs are canonical UUIDs; timestamps use UTC. `created_by` comes from the authenticated creator, and `created_at` comes from PostgreSQL. Account/category renaming does not rewrite financial values or provenance. Every response uses `Cache-Control: no-store`.

Generate one new UUID idempotency key per intended activity and reuse it after a timeout, disconnection, or uncertain `503` outcome. A unique `(organization_id, idempotency_key)` constraint protects simultaneous retries across processes. Identical validated account/category IDs, amount, currency, normalized occurred instant, and exact description return the original DTO with `200` and the same `Location`. Omitted versus empty description and equivalent UTC offsets are identical; different descriptions/instants/amounts/references return safe `409 conflict` with no original payload in the error. Retries recheck current write permission; any authorized recording member can replay identical input while the original creator stays unchanged. Keys remain reserved for the record's lifetime; the same key may be used independently in another organization. `X-Request-ID` is diagnostic and cannot substitute for this key. A new key creates a distinct record, even for an otherwise identical payload.

Transaction collection GET accepts page/limit and the [account/occurred-period filters](#transaction-history-and-period-filters) below, requires an empty body, orders by `created_at ASC, id ASC` within the organization, and omits `total`. Valid empty/beyond-end filtered pages return `[]`. Single GET rejects body/query strings and joins current membership with both organization/transaction predicates. Path IDs use the shared `400` canonical-UUID guard; unknown/inaccessible resources share `404`. Writes/lists hold the membership lock through commit; operations have five-second/request-cancellation database deadlines and safe `503` storage errors. Already authorized operations may finish before later revocation completes. Monetary values, descriptions, input keys, credential data, and raw storage/parser errors are omitted from application logs.

Apply `00008_create_transactions.sql` explicitly before deployment. Composite foreign keys enforce both tenant scope and account currency/category kind consistency; referenced accounts/categories cannot be deleted, and their currency/category kind cannot be changed while records depend on them. Records have no edit/delete endpoint (`PATCH`/`PUT`/`DELETE` return `405`) and no mutable balance cache. Retain the originals; full linked reversals are implemented below so corrections do not create unrelated opposite-kind revenue/expense. Partial reversals, atomic linked replacements, transfers, opening amounts, contact allocation, broader kind/category filters, and complete cash-balance workflows are not implemented. Recorded account activity below uses the exact signed sum of unreversed originals and is distinguished from actual cash position when opening activity/history is incomplete.

## Transaction history and period filters

Transaction collection GET accepts only `page`, `limit`, optional `account_id`, and optional paired `from`/`to`. Recorded account activity GET accepts only paired `from`/`to`; its account comes from the path, and an `account_id` query override is rejected. Other collections continue to accept only their existing page/limit parameters; single-record reads and writes continue to reject query strings. These reads require an empty body, an active bearer session, and current organization membership.

Keys are case-sensitive and each supplied parameter must have one non-empty value. Unsupported keys, duplicates (including aliases that decode to the same key), empty values, a bare `?`, malformed percent escapes/semicolon separators, and non-empty bodies return safe `400 invalid_request`. Standard URL decoding applies: encode a positive offset's `+` as `%2B`, or use UTC `Z`. Existing pagination format/range validation runs before finance-filter validation. A non-empty malformed account selector uses `422 invalid_format` on `account_id`; a canonical selector not found in the selected organization uses generic `404`, even when the caller is a member of its actual owner organization. An accessible account with no records in the interval returns `[]`.

Bounds use the exact recording timestamp parser: strict RFC 3339 with uppercase `T`/`Z` or numeric offsets, valid calendar/time/offset values, at most six fractional digits, and a normalized UTC year from 1 through 9999. Date-only/Jalali text, whitespace, leap seconds, excessive precision, and instants outside that UTC range are rejected rather than rounded. Omit both for all recorded dates; when one is supplied the other is required (`422 required`). Malformed bounds use `422 invalid_format`. The normalized `from` must precede `to`; equal or reversed instants use `422 out_of_range` on `to`. Filter details are ordered account, from, then to. Both endpoints select original `occurred_at >= from AND occurred_at < to`; creation/reversal timestamps and the current clock do not define the period. Backdated and future records are included when their original occurred instant matches. No business timezone is inferred; clients select explicit instants for their local calendar boundaries.

For example, a transaction collection query selecting one account's October 2026 records is:

```text
?account_id=c5847f36-b8ed-4b6a-85ea-24436b7d8c65&from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z
```

History applies predicates before `ORDER BY created_at, id` and page/limit; it keeps original transaction DTOs, including fully reversed originals, and existing `meta.page`/`meta.limit` without a count. It is audit history rather than a sum of recognized activity. Read the original's reversal resource when deciding whether its amount contributes to a corrected total. A full reversal created after the selected period still excludes that original from the period's activity; it never adds opposite-kind or dated refund activity. Omitting filters preserves all-time response shapes and values. Amounts remain exact decimal strings and no financial record, creator, key, or timestamp is rewritten.

Filtered lists hold current membership through commit and perform a scoped account lookup when requested; period activity keeps its one-statement scoped membership/account/transaction/reversal snapshot. Separate calls and pages do not share a snapshot and can change after concurrent commits. Five-second/request-cancellation deadlines, fail-closed dependency errors, and log privacy remain in force. Existing account/transaction indexes and schema through migration 00009 are reused; no migration, dependency, configuration, financial cache, or count query is added. Collection kind/category/contact filters, cursors, collection summaries, and historical “as known then” reports remain planned.

## Transaction reversals

The singular resource `/api/v1/organizations/:organization_id/transactions/:transaction_id/reversal` requires the existing bearer session. `POST` creates a full reversal (`201`) or returns an identical retry (`200`), with `data` and the resource's `Location`. `GET` returns `200` with `data`. Owner/admin/accountant members may create or replay; every current member may read. Staff writes return `403`, while missing/removed organization membership returns `404`. Tenant scope includes both organization and original transaction IDs, even for callers belonging to several organizations or identical IDs in two tenants.

POST accepts exactly two required strings, `idempotency_key` and `reason`, using the shared strict UTF-8 JSON/media contract, 8 KiB cap, and no query string. IDs/amount/currency/kind/creator/time fields cannot override the path or original values. Unknown/duplicate/case-variant keys, wrong types/nulls, invalid UTF-8/surrogates, and malformed/trailing JSON return `400`. For example:

```json
{"idempotency_key":"88978711-cd44-4e68-b654-032962d12c7e","reason":"Duplicate entry"}
```

The key is a canonical lowercase hyphenated UUID; omission/empty uses `422 required` and malformed values use `invalid_format`. Reason is trimmed Unicode text, 1–2000 code points, without normalization/case folding. Controls are rejected before trimming, including LF/CR/tab; Persian, combining text, emoji, and zero-width non-joiners are preserved. Missing/trimmed blank reason uses `required`; excessive length uses `out_of_range`; controls use `invalid_format`. Field details are ordered key then reason.

A reversal has a server-generated `id`, `organization_id`, original `transaction_id`, `idempotency_key`, trimmed `reason`, authenticated `created_by`, and server-default UTC `created_at`. It stores no copied amount, currency, category, or mutable status. First creation adds one linked record, retaining the original's amount/kind/account/category/currency/occurred instant/description/creator/creation time and original recording key. Current corrected recognition fully excludes that original from its original income/expense period; reversal `created_at` is audit history, not a new dated cash movement. This is a full void of an erroneous record, not a refund or payment.

Native uniqueness on `(organization_id, transaction_id)` permits at most one full reversal; `(organization_id, idempotency_key)` reserves one reversal key for its lifetime. Same target/key/trimmed reason returns the original reversal with `200` and the same `Location` across replicas/restarts, preserving the first reversal creator even if another authorized member retries. Different target/reason under that key or a new key for an already reversed target returns safe `409 conflict`. A new key with an unknown/out-of-scope original returns generic `404`; a key already used in the selected organization's reversals can conflict before target availability is reported. Concurrent matching requests produce one `201` and a `200` replay; competing keys/reasons/targets yield one creation and a conflict. Reversal and transaction-recording keys have separate operation namespaces, each scoped by organization; `X-Request-ID` remains diagnostic. Reuse the same key and reason after uncertain timeouts/disconnections/`503` outcomes.

GET requires an empty body and no query string. It joins current membership with organization/original predicates and returns generic `404` for an original with no reversal, unknown originals, or inaccessible resources. Path IDs reuse the canonical UUID guard (`400`). Original transaction GET/list and creation-retry DTOs remain unchanged and retain reversed originals; clients must retrieve this resource when establishing reversal status. Raw transaction lists are record history, not corrected balance reports.

Creation holds current membership through commit; GET uses a membership join. Both have five-second/request-cancellation database deadlines and safe `503` storage errors, including a missing reversal migration. Authorization is rechecked on retries and storage failures fail closed; already authorized operations may finish before later revocation completes. Every handled response uses `Cache-Control: no-store`. Reasons, input keys, original financial values, credentials, and raw dependency/parser errors are omitted from application logs. Apply `00009_create_transaction_reversals.sql` before deployment.

The composite FK links the original within its organization and prevents deleting/rekeying a referenced original; a restricted global-user FK retains reversal provenance. Reversals have no edit/delete/undo route (`PATCH`/`PUT`/`DELETE` return `405`), cannot themselves be targeted as original transactions, and expose no collection/balance endpoint. Partial reversals, actual refunds, atomic linked replacements, and historical “as known then” reporting remain future capabilities. A later replacement needs a new transaction/key; no atomic replacement or replacement link is implemented now. Recorded account activity below excludes fully reversed originals using exact wider arithmetic and explicit opening/history limitations. Future period reports must retain those correction semantics and audit originals.

## Recorded account activity

`GET /api/v1/organizations/:organization_id/financial-accounts/:account_id/recorded-activity` requires the existing bearer session and returns `200` with a single `data` object. Owner/admin/accountant/staff members can read. Current membership, selected organization, and account are checked together; unknown accounts and missing/removed membership share generic `404`, including callers belonging to multiple tenants and identical account/original IDs across tenants. An accessible account with no unreversed transactions returns zero totals instead of `404`.

The explicit DTO contains `organization_id`, `account_id`, account `currency`, `income_amount`, `expense_amount`, `net_amount`, `basis`, and `opening_balance_included`. With paired period filters it also contains the normalized UTC `from` and `to`; both are omitted for all-time activity. Amounts use canonical decimal strings in whole Rial, without fractions/exponents/leading zeroes; income/expense are nonnegative, and net may be negative. Empty or fully reversed activity in the selected scope returns `"0"` for all three. With October 2026 period bounds, for example:

```json
{
  "data": {
    "organization_id": "49a21d43-fd32-4798-88a9-0ec245f0355b",
    "account_id": "c5847f36-b8ed-4b6a-85ea-24436b7d8c65",
    "currency": "IRR",
    "income_amount": "12000000",
    "expense_amount": "4500000",
    "net_amount": "7500000",
    "basis": "recorded_transactions",
    "opening_balance_included": false,
    "from": "2026-10-01T00:00:00Z",
    "to": "2026-11-01T00:00:00Z"
  }
}
```

Income and expense sum their matching original kinds; net is income minus expense. The query constrains transactions by organization/account/currency and excludes a transaction only when a reversal matches both its organization and original ID. Full reversals remove the original's recognition rather than adding opposite-kind activity; both audit records and their recording/reversal retry identities remain untouched. PostgreSQL [`sum(bigint)` returns exact `numeric`](https://www.postgresql.org/docs/18/functions-aggregate.html), so totals and subtraction can exceed signed 64-bit limits without a BIGINT cast or binary floating-point arithmetic. The query converts those results directly to decimal text and uses zero for empty sums. Current IRR account currency and whole-Rial units stay explicit; there is no currency conversion.

`basis: "recorded_transactions"` and `opening_balance_included: false` state the result's limits. Without bounds it covers all committed unreversed originals visible to the query, including backdated and future-dated records. Paired bounds select original occurred instants using the shared inclusive/exclusive filter contract; a later full reversal corrects the original period irrespective of its own creation time. No opening amount, imported-history completeness, transfer, refund movement, or reconciliation basis is established. A zero recorded net does not establish an empty bank/cash account. The result is recorded activity for all time or an explicitly selected period. It does not assert actual cash position, historical “as known then” recognition, or an authoritative as-of timestamp. Collection summaries and complete balance workflows remain planned.

Amounts, membership, account, originals, and reversal exclusions use one [PostgreSQL statement snapshot](https://www.postgresql.org/docs/18/transaction-iso.html#XACT-READ-COMMITTED). Uncommitted changes are excluded; a later request after commit reflects the committed record/reversal, while separate requests may see different snapshots. GET uses a membership join like other single reads; already authorized operations may finish before later revocation completes. There is no separately cached balance, read-triggered financial mutation, or update/delete route. Computation reads the account's indexed records on demand with a five-second/request-cancellation deadline; a rollup is deferred until measured latency justifies it.

GET accepts an empty body and optional paired `from`/`to` as specified above; page/limit, currency, account, and identity query overrides are rejected. Structural violations use `400 invalid_request`; semantic filter errors use `422`, and malformed canonical path UUIDs use the shared `400` guard. Missing/invalid bearer sessions use `401`. Unsupported methods, including `HEAD`/`OPTIONS`, return shared `405` with `Allow: GET`. Responses use `Cache-Control: no-store` and safe errors; amounts, credentials, raw SQL/dependency errors, and raw paths are omitted from application logs. Dependency failures, denied SELECT permissions, and missing transaction/reversal schema return safe `503` rather than fabricated zero totals. Apply existing migrations through `00009` before deployment. No new migration, dependency, or configuration is added; financial SELECT permissions plus existing identity/session/membership permissions suffice, with no transaction/reversal INSERT/UPDATE/DELETE needed for this read.

## Pagination

List endpoints use `page` and `limit` initially:

| Parameter | Absent default | Accepted range |
| --- | --- | --- |
| `page` | `1` | `1` through `10000` |
| `limit` | `20` | `1` through `100` |

Accept ASCII decimal digits only. Explicitly empty, signed, whitespace-padded, duplicate, non-integer, or overflowing values are `400 invalid_request`. Parsed values outside the accepted range are `422 validation_failed` with `out_of_range` details. Reject invalid values instead of silently clamping them. Compute `(page - 1) * limit` with checked integer arithmetic; the current caps keep this offset bounded.

Return `meta.page` and `meta.limit` as integers. `meta.total` is optional and is included only when an exact count is affordable; an omitted count does not mean zero. Apply the same tenant scope and filters to the count and data queries. A requested page beyond the available results returns `200` with `data: []` and the requested page/limit.

Every list endpoint must specify stable ordering with a unique ID tie-breaker. Page results can shift when data changes between requests. Do not promise a snapshot across requests; add cursors when measured data size or consistency requirements justify them. Do not introduce a pagination repository or count queries before a real list endpoint needs them.

## Timestamps, identifiers, and money

Represent instants as RFC 3339 strings normalized to UTC with `Z`, for example `2026-10-05T12:34:56Z`. Preserve supported fractional seconds when needed; use Go's `time` package and normalize values with `UTC()` before serialization. Accept supported RFC 3339 offsets on input and normalize them. Genuine date-only values use Gregorian `YYYY-MM-DD`, with explicit business-timezone semantics; Jalali conversion belongs at presentation/input boundaries.

Use opaque UUID resource IDs, serialized in canonical hyphenated lowercase form. Validate path ID syntax before lookup and enforce tenant authorization on every lookup. UUID generation will be selected with the first persistence model; a UUID is not an authorization token.

Money uses signed 64-bit integer minor units and an explicit currency. Exchange potentially large amounts as decimal strings; do not use floating-point JSON values or silently round. Initial canonical storage is IRR/Rial, with Toman converted at presentation boundaries as specified in the product blueprint. Transaction recording implements exact IRR amounts with native integer/string conversion and PostgreSQL constraints, without a money, date, or UUID framework.
