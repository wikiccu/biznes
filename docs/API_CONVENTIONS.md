# API conventions

This is the contract for business endpoints introduced under `/api/v1`. The current executable implements health/readiness and shared routing/recovery errors. It has no business endpoints yet, including at `/api/v1` itself. Input decoding, resource validation, success DTOs, and pagination will be implemented with the first endpoints that need them; this document specifies their contract.

## Routes and responses

Register each implemented domain's routes in a Gin group with prefix `/api/v1`. Use English resource names and `snake_case` JSON fields. Breaking business contract changes require a new major API prefix. Define each endpoint's input, output, authorization, filters, and ordering alongside its implementation.

Business successes return `application/json` with a `data` member. For a single resource, `data` contains its DTO; for a list, it contains an array, including `[]` for an empty result. Use an explicit response DTO to choose public fields; do not serialize database models containing credentials or internal fields. Extra metadata belongs in optional top-level `meta`.

```json
{"data": [], "meta": {"page": 1, "limit": 20}}
```

This illustrates a future list response, not a registered endpoint. Use `200` for retrieval/update results and `201` for creation, with `Location` when the new resource has a retrieval URL. A `204` response has no JSON body. Successful responses must not contain an `error` member.

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

These mappings are endpoint conventions, not implemented authentication, validation, or rate limiting. `/ready` retains its probe-specific `503` payload. Shared routing currently returns JSON `404` for unknown paths and JSON `405` with Gin's `Allow` header for unsupported methods on known paths. That includes `HEAD`/`OPTIONS` unless those methods are explicitly registered; HTTP `HEAD` responses carry only headers and status, without a body. Trailing-slash redirects remain disabled. A missing business resource must not disclose whether it exists in another tenant.

HTTP status and header semantics follow [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#section-15). Responses created by the HTTP server before Gin handles a request, such as malformed HTTP framing, do not necessarily have this envelope or a request ID.

## Input and validation

At each business handler boundary:

1. For JSON body endpoints, require the `application/json` media type, allow a UTF-8 charset parameter, and limit the body to `1 MiB` by default using `http.MaxBytesReader`. Upload endpoints must define their own limits and media types.
2. Decode exactly one JSON object into an explicit input DTO. Reject missing bodies, `null`, arrays, unknown fields, wrong types, and any trailing non-whitespace content. Use the standard-library decoder with `DisallowUnknownFields` and a second decode expecting `io.EOF`. Unknown-field checks alone do not enforce the object shape or a single JSON value.
3. Validate required fields, string lengths, formats, and ranges after decoding. Use the existing Gin validator where appropriate and select safe field/code details explicitly. Transport parsing failures are `400`; validly parsed but unacceptable values are `422`.
4. Call application logic with the request context. Application services enforce ownership and business invariants, including calls from future AI tools, imports, and jobs. Do not rely solely on transport validation or client-supplied tenant identity.

Require UTF-8 JSON as defined by [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259.html#section-8.1). Reject duplicate object keys, invalid UTF-8, and property names that do not match the published field names. The [standard decoder](https://pkg.go.dev/encoding/json#Decoder) alone accepts duplicate keys, matches struct names case-insensitively, and replaces invalid UTF-8; account for those behaviors when implementing the input boundary. Do not use maps of floating-point values to decode money. Use explicit string/integer types with checked conversions. Avoid Gin binding methods that write an automatic response; input errors must reach the shared writer with the chosen status and safe fields.

Query handlers must reject malformed percent encoding, unexpected parameters, and duplicate single-valued parameters. Allowlist filtering/sorting fields and values, validate their bounds, and pass only parameterized values to SQL. Never interpolate arbitrary client query text into SQL identifiers or order clauses.

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
