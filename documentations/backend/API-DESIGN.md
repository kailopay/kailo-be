# API Design

## 1. Contract principles

- Base path: `/v1`.
- JSON over HTTPS.
- All settlement amounts are strings or integer IDR minor units as defined by OpenAPI; never JSON floating-point numbers.
- Timestamps use RFC 3339 UTC.
- Public identifiers are opaque.
- Every response includes or echoes a request/correlation identifier.
- The checked-in OpenAPI specification is generated/validated with the implementation and becomes the executable contract.
- All pages and responses identify the environment as `sandbox` and network as `testnet` where relevant.

## 2. Authentication

KailoPay has one user identity model with separate authentication mechanisms by access mode.

### Retail web session

The web application redirects the user to Auth0 Universal Login and receives the authorization callback through the KailoPay backend. KailoPay maps the provider subject to a local user, then creates a short-lived retail session represented by an opaque, secure, HTTP-only cookie. Retail order reads and mutations require that session and are restricted to its own orders. Auth0 access and refresh tokens are not browser session credentials. Session creation, expiry, revocation, and CSRF protection for browser mutations must be documented in the deployed web contract.

The implemented backend endpoints are `GET /auth/login`, `GET /auth/callback`, `POST /auth/logout`, public `POST /auth/password/forgot`, protected `GET/PATCH /auth/me`, and protected `GET/PUT/DELETE /auth/me/avatar`. Auth0 hosts the page that accepts the new password; KailoPay never receives password or reset-token plaintext. A dedicated internal Auth0 Action callback revokes all local sessions after a successful reset. Profile image bytes remain in a private MinIO bucket while PostgreSQL stores only the object key. The callback consumes a ten-minute one-time transaction; the local session uses an eight-hour absolute lifetime and thirty-minute idle timeout. The executable OpenAPI contract is [`openapi/openapi.yaml`](../../openapi/openapi.yaml).

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

Off-ramp and retail-session order access are explicitly deferred beyond Week 1.

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
| 422 | `AMOUNT_OUT_OF_RANGE` | Valid shape but unsupported amount |
| 429 | `RATE_LIMITED` | Client exceeded sandbox limit |
| 502/503 | `EXTERNAL_SERVICE_UNAVAILABLE` | Provider/network unavailable; safe retry guidance required |
| 500 | `INTERNAL_ERROR` | Unexpected server error; reference request ID |

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
