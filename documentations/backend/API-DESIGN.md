# API Design

## 1. Contract principles

- Base path: `/v1`.
- JSON over HTTPS.
- JSON object keys use `snake_case` everywhere (`amount_minor`, `display_name`, `client_id`), matching the PostgreSQL column names and the Xendit/Stellar ecosystems the API translates between. Go struct field names remain PascalCase; the `json` struct tags own the wire naming. The pre-release auth slice briefly shipped camelCase keys; they were unified to snake_case before any consumer existed, and the OpenAPI file reflects the unified contract.
- All settlement amounts are strings or integer IDR minor units as defined by OpenAPI; never JSON floating-point numbers.
- Timestamps use RFC 3339 UTC.
- Public identifiers are opaque.
- Every response includes or echoes a request/correlation identifier.
- The checked-in OpenAPI specification is generated/validated with the implementation and becomes the executable contract.
- All pages and responses identify the environment as `sandbox` and network as `testnet` where relevant.

## 2. Authentication

KailoPay has one user identity model with separate authentication mechanisms by access mode.

### Retail web session

Authentication is self-hosted (ADR-002). The frontend owns the email forms and calls `POST /auth/register`, `POST /auth/login`, and the token endpoints directly; Google sign-in stays a redirect flow against Google's OIDC endpoints with PKCE. Successful logins create a local session represented by an opaque, secure, HTTP-only cookie; provider tokens never reach the browser. Passwords are stored as Argon2id hashes with per-credential lockout after ten failures. A Google identity whose verified email matches an existing account is linked to that account. Email verification tokens last 24 hours and password-reset tokens one hour; both are single-use and stored only as HMAC hashes. Email and password reset endpoints never disclose whether an account exists.

The implemented endpoints are `POST /auth/register`, `POST /auth/login`, `GET /auth/google/login`, `GET /auth/google/callback`, `POST /auth/logout`, `POST /auth/email/verify`, `POST /auth/email/resend`, `POST /auth/password/forgot`, `POST /auth/password/reset`, protected `POST /auth/password/change`, protected `GET/PATCH /auth/me`, and protected `GET/PUT/DELETE /auth/me/avatar`. In sandbox, verification and reset links are logged to the server console instead of being emailed. Profile image bytes remain in a private MinIO bucket while PostgreSQL stores only the object key. The Google callback consumes a ten-minute one-time transaction; the local session uses an eight-hour absolute lifetime and thirty-minute idle timeout. The executable OpenAPI contract is [`openapi/openapi.yaml`](../../openapi/openapi.yaml).

### Developer API key

Sandbox integration endpoints use:

```http
Authorization: Bearer pk_test_<public_id>_<secret>
```

Invalid, revoked, or malformed keys return `401`. An authenticated client accessing another client's resource returns `404` or `403` according to the selected enumeration policy; use one policy consistently.

API-key creation/revocation requires an authenticated user session with Developer Mode enabled. The session is not replaced by passing arbitrary user or client IDs.

Developer configuration is user-owned. A developer session may manage only API clients whose `owner_user_id` matches the authenticated user. API keys authenticate an API client; they do not authenticate the owning browser user. Disabling Developer Mode hides/blocks developer-management actions but does not silently revoke existing clients or keys.

### Operator access

Operator/admin access is private and role-restricted. It is not available through public API keys or retail sessions.

## 3. Common headers

| Header | Direction | Use |
|---|---|---|
| `Authorization` | Request | Test API key where required |
| `Cookie` | Request | Secure retail session where required by the web flow |
| `Idempotency-Key` | Request | Required for order creation; recommended/required for other mutations |
| `X-Request-Id` | Both | Client-provided if valid or server-generated |
| `KailoPay-Version` | Response | API/release version if useful |

Idempotency keys are scoped to API client and operation. Reuse with a different canonical request returns `409`.

## 4. Resource model

### Order response

```json
{
  "order": {
    "id": "01J...",
    "status": "payment_pending",
    "environment": "sandbox",
    "network": "stellar_testnet",
    "fiat": { "currency": "IDR", "amount_minor": "100000" },
    "asset": { "code": "XLM", "amount": "40.0000000" },
    "quote": {
      "rate": "2500",
      "adjusted_rate": "2500",
      "spread_bps": 0,
      "source_at": "2026-08-18T12:00:00Z",
      "expires_at": "2026-08-18T12:05:00Z"
    },
    "payment_method": "qris",
    "checkout": {
      "id": "pr-demo",
      "status": "REQUIRES_ACTION",
      "presentation_type": "QR_STRING",
      "presentation_value": "000201...",
      "expires_at": "2026-08-18T12:05:00Z"
    },
    "created_at": "2026-08-18T12:00:00Z",
    "updated_at": "2026-08-18T12:00:00Z"
  }
}
```

The reserved `.test` address and synthetic values are illustrative; released examples must be regenerated with safe values from the active provider sandbox.

## 5. Endpoint catalog

### Orders

| Method | Path | Auth | Idempotency | Purpose |
|---|---|---|---|---|
| `POST` | `/v1/onramps` | Test key | Required | Create IDR-to-native-XLM order and Xendit checkout |
| `GET` | `/v1/orders/{order_id}` | Test key | N/A | Retrieve an order owned by the authenticated API client |
| `GET` | `/v1/orders` | Test key | N/A | List only orders owned by the authenticated API client |

Retail-session order access is deferred. The off-ramp endpoint `POST /v1/offramps` is implemented as of Week 2 with simulated payout evidence.

### Developer configuration

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/v1/api-keys` | Developer session | Create and display one test key once |
| `GET` | `/v1/api-keys` | Developer session | List safe key metadata |
| `DELETE` | `/v1/api-keys/{key_id}` | Developer session | Revoke key |
| `POST` | `/v1/webhook-endpoints` | Developer session | Register endpoint and issue/show signing secret once |
| `GET` | `/v1/webhook-endpoints` | Developer session | List endpoint metadata |
| `DELETE` | `/v1/webhook-endpoints/{id}` | Developer session | Disable endpoint |

### Anchor/public operations

SEP-24 and federation paths follow the applicable Stellar specifications and are detailed in `STELLAR-ANCHOR-INTEGRATION.md`. Health endpoints are intentionally outside `/v1`:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/livez` | Process liveness; no dependency checks or secrets |
| `GET` | `/readyz` | Readiness with bounded dependency checks |
| `GET` | `/startupz` | One-time startup completion status |
| `GET` | `/health`, `/healthz`, `/ready` | Compatibility aliases |

## 6. Create on-ramp request

```json
{
  "fiat": { "currency": "IDR", "amount_minor": "100000" },
  "payment_method": "qris",
  "stellar_destination": { "account": "G...", "memo": null }
}
```

Validation:

- Positive amount within configured sandbox bounds.
- `IDR`, native XLM on testnet, and `qris` or `bri_va` only.
- Valid Stellar account and memo/muxed-account policy.
- No real identity or production-bank data required.

Response: `201` with the immutable quote and Xendit checkout representation.
An unknown provider-create outcome returns `202` and is held for reconciliation;
KailoPay does not automatically create a replacement checkout.

## 7. Create off-ramp request

```json
{
  "asset": { "network": "stellar_testnet", "code": "XLM", "amount": "6.2500000" },
  "withdrawal": {
    "currency": "IDR",
    "method": "sandbox_bank_transfer",
    "destination_token": "synthetic_demo_destination"
  }
}
```

Response includes asset deposit account, required memo/correlation data, expiry, and explicit sandbox warnings. Do not accept or publish real bank-account details for the demo when a provider token/synthetic reference is sufficient.

## 8. Pagination and filtering

`GET /v1/orders` uses opaque cursor pagination:

- Query: `limit` with maximum 100 and opaque `cursor`.
- Stable ordering: descending creation time plus ID tie-breaker.
- Response: `data`, `has_more`, `next_cursor`, `request_id`.

Avoid offset pagination for growing order history.

## 9. Error envelope

```json
{
  "error": {
    "code": "INVALID_STELLAR_ACCOUNT",
    "message": "The Stellar testnet destination is invalid.",
    "type": "validation_error",
    "field": "stellar_destination.account",
    "retryable": false
  },
  "request_id": "req_..."
}
```

Minimum stable codes:

| HTTP | Code | Meaning |
|---:|---|---|
| 400 | `INVALID_REQUEST` | Malformed JSON or invalid shape |
| 400 | `UNSUPPORTED_ROUTE` | Currency/asset/network/method not supported |
| 400 | `INVALID_STELLAR_ACCOUNT` | Invalid destination/source details |
| 401 | `INVALID_API_KEY` | Missing, malformed, invalid, or revoked key |
| 404 | `ORDER_NOT_FOUND` | Resource absent or not visible to client |
| 409 | `INVALID_ORDER_STATE` | Operation conflicts with current lifecycle |
| 409 | `IDEMPOTENCY_KEY_REUSED` | Same key used with a different request |
| 409 | `INSUFFICIENT_LIQUIDITY` | Treasury inventory cannot cover the order |
| 422 | `AMOUNT_OUT_OF_RANGE` | Valid shape but unsupported amount |
| 429 | `RATE_LIMITED` | Client exceeded sandbox limit |
| 202 | `CHECKOUT_PENDING_RECONCILIATION` | Checkout transport outcome unknown; the order is held for provider reconciliation |
| 503 | `QUOTE_UNAVAILABLE` | Market data missing, stale, or produced an invalid quote |
| 502/503 | `EXTERNAL_SERVICE_UNAVAILABLE` | Provider/network unavailable; safe retry guidance required |
| 500 | `INTERNAL_ERROR` | Unexpected server error; reference request ID |

Week 1 implements `INVALID_REQUEST`, `INVALID_STELLAR_ACCOUNT`,
`INSUFFICIENT_LIQUIDITY`, `IDEMPOTENCY_KEY_REUSED`, `AMOUNT_OUT_OF_RANGE`,
`ORDER_NOT_FOUND`, `CHECKOUT_PENDING_RECONCILIATION`, `QUOTE_UNAVAILABLE`, and
`EXTERNAL_SERVICE_UNAVAILABLE`. The remaining catalog entries
(`UNSUPPORTED_ROUTE`, `INVALID_API_KEY` body, `INVALID_ORDER_STATE`,
`RATE_LIMITED`, `INTERNAL_ERROR`) are Week 3 hardening work.

Provider-specific errors map to stable KailoPay codes; raw provider bodies are not returned.

## 10. Versioning and compatibility

- Breaking API changes require a new major path/version.
- Additive optional fields may be introduced in `/v1`; clients must ignore unknown fields.
- Enum additions can break strict clients and require release-note warnings.
- Public webhook payloads include their own `api_version`.
- OpenAPI and examples are tagged with `v0.1.0` release.

## 11. Rate and abuse controls

Initial sandbox controls:

- Per-IP unauthenticated request limit.
- Per-key request and order-creation limit.
- Callback endpoints protected primarily by provider authentication, plus size/time limits.
- Webhook endpoint registration subject to SSRF validation.

Limits are configurable and documented as sandbox controls, not production capacity guarantees.

## 12. API acceptance checks

- Every operation in the deployed API exists in validated OpenAPI.
- Examples pass contract tests against the release candidate.
- Duplicate create requests return one business order.
- Cross-client resource access fails.
- Errors contain stable code and request ID without internal stack/provider secrets.
- Amounts round-trip exactly.
- Public schemas never claim production/mainnet behavior.
