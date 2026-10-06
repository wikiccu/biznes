# API conventions

This is the contract for business endpoints introduced under `/api/v1`. The executable implements identity/session routes, membership-scoped organization routes with pagination, health/readiness, and shared routing/recovery errors. `/api/v1` itself is not an endpoint. Handlers implement strict input validation and explicit success DTOs.

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

Identity and organization endpoints implement the applicable errors below, including membership/role enforcement. `/ready` retains its probe-specific `503` payload. Shared routing returns JSON `404` for unknown paths and JSON `405` with Gin's `Allow` header for unsupported methods on known paths. That includes `HEAD`/`OPTIONS` unless those methods are explicitly registered; HTTP `HEAD` responses carry only headers and status, without a body. Trailing-slash redirects remain disabled. A missing business resource must not disclose whether it exists in another tenant.

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

The explicit organization DTO contains only `id`, `name`, `role`, `created_at`, and `updated_at`. `role` is the caller's membership (`owner`, `admin`, `accountant`, or `staff`), not an organization-wide setting or another user's role. Timestamps are UTC instants; successful responses use `Cache-Control: no-store`. All roles can read organization metadata. Only owner/admin can rename; future contact/financial permissions need their own operation policies.

Create/rename share the identity endpoints' strict 8 KiB JSON boundary through `internal/platform/http/input.go`, accepting only a `name` string. Unknown/duplicate/case-variant fields, nulls, wrong types, malformed/trailing JSON, invalid UTF-8, and lone surrogate escapes return `400`. Missing/blank names return `422` with `name: required`; over 120 Unicode code points uses `out_of_range`; control characters use `invalid_format`. Control checks precede trimming, so tabs/newlines are rejected even at the edges. Unicode surrounding whitespace is trimmed; internal whitespace, Persian, combining characters, emoji, and zero-width non-joiners are preserved without normalization. Names need not be unique. Query strings are rejected on create/get/rename. GET requests require an empty body. Path IDs require canonical lowercase hyphenated UUID syntax (`400` otherwise).

Creation inserts the organization and initial owner membership in one transaction. The server sets the owner from authenticated identity; no owner/user/role input is accepted. A failed membership insert rolls back the organization. Creation is not idempotent: retrying an ambiguous successful request can create another organization. The list accepts only `page` and `limit` under the pagination contract below, ordered by `created_at ASC, id ASC`, and omits `total`. Empty/beyond-end pages return `[]`.

Single-resource reads join membership in the query. Unknown resources and resources outside the caller's memberships return the same safe `404 not_found`. A member with accountant/staff role receives `403 forbidden` for a valid rename. Rename checks and locks the membership row with `FOR SHARE` until its organization update commits, serializing concurrent removal/demotion; the update maintains `updated_at` and preserves `created_at`. Each service operation has a five-second/request-cancellation database deadline and returns a safe `503 service_unavailable` on storage failure. Membership is never cached in the bearer session. Requests already authorized may finish before a later revocation completes.

Apply `00004_create_organizations.sql` explicitly before deploying these routes. There are no invitations, member-management endpoints, ownership transfer, deletion, branch policies, or financial/contact routes yet. Creation grants only the creator's owner membership. No dependency or environment setting is added.

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

Money uses signed 64-bit integer minor units and an explicit currency. Exchange potentially large amounts as decimal strings; do not use floating-point JSON values or silently round. Initial canonical storage is IRR/Rial, with Toman converted at presentation boundaries as specified in the product blueprint. These are wire/domain conventions; no money, date, or UUID framework is added by this increment.
