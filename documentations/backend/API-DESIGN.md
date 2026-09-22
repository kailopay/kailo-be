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

The implemented endpoints are `POST /auth/register`, `POST /auth/login`, `GET /auth/google/login`, `GET /auth/google/callback`, `POST /auth/logout`, `POST /auth/email/verify`, `POST /auth/email/resend`, `POST /auth/password/forgot`, `POST /auth/password/reset`, protected `POST /auth/password/change`, protected `GET/PATCH /auth/me`, and protected `GET/PUT/DELETE /auth/me/avatar`. Registration, verification resend, and password-reset requests persist encrypted email jobs in PostgreSQL and return without waiting for SMTP; the existing worker delivers them through the configured console or Gmail provider. Profile image bytes remain in a private MinIO bucket while PostgreSQL stores only the object key. The Google callback consumes a ten-minute one-time transaction; the local session uses an eight-hour absolute lifetime and thirty-minute idle timeout. The executable OpenAPI contract is [`openapi/openapi.yaml`](../../openapi/openapi.yaml).

### Developer API key

Sandbox integration endpoints use:

```http
Authorization: Bearer pk_test_<public_id>_<secret>
```

Invalid, revoked, or malformed keys return `401`. An authenticated client accessing another client's resource returns `404` or `403` according to the selected enumeration policy; use one policy consistently.

Developer management routes require an authenticated user session with Developer Mode enabled. This includes the dashboard, wallet profile, API-key, and webhook routes. Creating a new API key also requires an approved Persona sandbox KYC status. The session is not replaced by passing arbitrary user or client IDs.

Developer configuration is user-owned. A developer session may manage only API clients whose `owner_user_id` matches the authenticated user. API keys authenticate an API client; they do not authenticate the owning browser user. Disabling Developer Mode hides/blocks developer-management actions but does not silently revoke existing clients or keys.

### Order principal selection

The four order operations use one shared authentication boundary. If an
`Authorization` header is present, it must be a valid bearer API key and the
request uses the API-client principal. When the header is absent, the backend
authenticates the `kailopay_session` cookie and uses a retail-session principal.
The session must be active and belong to an email-verified user. A malformed or
invalid `Authorization` header does not fall back to the cookie.

Creating an on-ramp or off-ramp order additionally requires the owning user to
have `kyc.status=approved`. This gate is enforced in the usecase for both API-key
and retail-session principals; the frontend redirect is only a convenience.

| Credential | Principal | Order scope |
|---|---|---|
| `Authorization: Bearer pk_test_...` | API client plus owning user | Orders whose `client_id` matches the authenticated client |
| `kailopay_session` cookie | Verified retail session plus user | Orders with no client, a non-null creating session, and the authenticated user as creator |

The request body, path, and query string cannot select an owner. Session-authenticated
`POST /v1/onramps` and `POST /v1/offramps` require an exact configured `Origin`;
when `Origin` is absent, a matching `Referer` origin is accepted. API-key
requests do not require a browser origin.

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

Idempotency keys are scoped to the authenticated owner and operation: API
client for API-key requests, and retail user for session requests. Reuse with a
different canonical request returns `409`; a retry after a session renewal uses
the same retail-user scope and can replay the original order.

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
    "payment_method": "xendit",
    "checkout": {
      "id": "ps-demo",
      "status": "ACTIVE",
      "presentation_type": "PAYMENT_LINK",
      "presentation_value": "https://checkout-staging.xendit.co/sessions/ps-demo",
      "payment_link_url": "https://checkout-staging.xendit.co/sessions/ps-demo",
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
| `POST` | `/v1/quotes` | Public | N/A | Preview the current IDR/XLM buy or sell quote without creating an order |
| `POST` | `/v1/onramps` | Test key or verified retail session | Required | Create IDR-to-native-XLM order and Xendit checkout |
| `POST` | `/v1/offramps` | Test key or verified retail session | Required | Create XLM-to-IDR order with sandbox deposit instructions |
| `GET` | `/v1/orders/{order_id}` | Test key or verified retail session | N/A | Retrieve an order owned by the authenticated principal |
| `GET` | `/v1/orders` | Test key or verified retail session | N/A | List orders owned by the authenticated principal |

Retail history is user-scoped across valid sessions. API-key history remains
client-scoped. Public order responses omit `client_id`, `created_by_user_id`,
and `retail_session_id`.

### Developer configuration

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/v1/api-keys` | Developer session | Create and display one test key once |
| `GET` | `/v1/api-keys` | Developer session | List safe key metadata |
| `DELETE` | `/v1/api-keys/{key_id}` | Developer session | Revoke key |
| `POST` | `/v1/webhook-endpoints` | Developer session | Register endpoint and issue/show signing secret once |
| `GET` | `/v1/webhook-endpoints` | Developer session | List endpoint metadata |
| `GET` | `/v1/webhook-endpoints/{id}` | Developer session | Read one owned endpoint |
| `POST` | `/v1/webhook-endpoints/{id}/test` | Developer session | Queue a synthetic signed sandbox event |
| `DELETE` | `/v1/webhook-endpoints/{id}` | Developer session | Disable endpoint |
| `GET` | `/v1/webhook-deliveries` | Developer session | Inspect safe delivery attempts with filters/cursors |
| `POST` | `/v1/webhook-deliveries/{id}/replay` | Developer session | Replay one exhausted delivery with the same event ID |
| `GET` | `/v1/developer/overview` | Developer session | Account-scoped usage and exchange health |
| `GET` | `/v1/developer/analytics` | Developer session | Bounded hour/day/week usage analytics |
| `GET` | `/v1/developer/revenue/summary` | Developer session | Sandbox fee/revenue totals |
| `GET` | `/v1/developer/revenue/entries` | Developer session | Cursor-paginated immutable financial snapshots |
| `GET` | `/v1/developer/orders` | Developer session | Account-wide order and provider correlation |
| `GET`/`POST` | `/v1/developer/wallets` | Developer session | List/register SEP-10-verified wallet profile hints |
| `PATCH`/`DELETE` | `/v1/developer/wallets/{id}` | Developer session | Update or revoke a wallet profile hint |

Developer dashboard routes derive ownership from the session user and may only
narrow results with a client owned by that user. Exact IDR and XLM quantities
are serialized as strings. The default immutable financial policy is
`sandbox-zero-fee-v1`; its revenue figures are estimates and do not represent
real fiat earnings.

Webhook consumers receive the canonical raw JSON body with these headers:

```text
KailoPay-Event-Id: evt_...
KailoPay-Timestamp: 1787055000
KailoPay-Signature: v1=<hex HMAC-SHA256>
KailoPay-Event-Type: order.completed
KailoPay-Api-Version: 2026-08-01
```

The signature input is `<timestamp>.<exact_raw_request_body>` and is signed
with the endpoint secret. Delivery is at-least-once; consumers must verify the
timestamp, compare the HMAC in constant time, return `2xx` quickly, and
deduplicate by `KailoPay-Event-Id`. Network errors, `408`, `409`, `425`, `429`,
and `5xx` are retried within the configured attempt limit. Other `4xx` and
redirect responses are terminal. A manual replay reuses the immutable event
ID and creates a durable delivery attempt, not a new order event.

### Identity verification

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/v1/kyc` | Authenticated session | Return the current Persona status and safe inquiry metadata |
| `POST` | `/v1/kyc/inquiry` | Authenticated session | Create or resume the user's Persona inquiry and return the embedded-flow inputs |
| `POST` | `/callbacks/kyc/persona` | Persona signature | Verify, deduplicate, and apply a Persona inquiry event |

The browser receives only the Persona inquiry ID, sandbox environment ID, and a
short-lived session token when a resume is required. KailoPay does not store raw
identity documents or raw Persona webhook bodies. The callback is authoritative;
the browser must refresh `GET /v1/kyc` after the embedded flow completes.

### Anchor/public operations

SEP-24 and federation paths follow the applicable Stellar specifications and are detailed in `STELLAR-ANCHOR-INTEGRATION.md`. Classic Stellar wallets authenticate with SEP-10; SEP-45 contract-account authentication is not advertised. Health endpoints are intentionally outside `/v1`:

| Method | Path | Auth | Idempotency | Purpose |
|---|---|---|---|---|
| `GET` | `/auth?account=G...` | None | N/A | Create a SEP-10 challenge transaction |
| `POST` | `/auth` | None | N/A | Exchange the signed challenge for a short-lived bearer JWT |
| `POST` | `/sep24/transactions/deposit/interactive` | SEP-10 bearer JWT | `Idempotency-Key` required | Create a wallet-owned interactive deposit session |
| `POST` | `/sep24/transactions/withdraw/interactive` | SEP-10 bearer JWT | `Idempotency-Key` required | Create a wallet-owned interactive withdrawal session |
| `GET` | `/sep24/transactions?limit={n}` | SEP-10 bearer JWT | N/A | List recent wallet-owned SEP-24 transaction mappings |
| `GET` | `/sep24/transaction?id={transaction_id}` | SEP-10 bearer JWT | N/A | Return current status for an owned mapping |
| `GET`/`POST` | `/sep24/interactive/{transaction_id}` | Browser token + KailoPay session | N/A | Link the wallet to a retail user and complete the interactive session |

Initiation requests use `multipart/form-data` or the JSON equivalent. Deposit
requires `asset_code=XLM`, the authenticated wallet account, and positive
sandbox `amount_minor` IDR units unless a firm `quote_id` is supplied; `memo`
and `payment_method` are optional. Withdrawal requires `asset_code=XLM`, exact
decimal `amount` unless a firm `quote_id` is supplied. Its non-empty
`destination_token` is collected by the interactive page as a synthetic
sandbox reference. `/sep24/info`
labels deposit limits as `idr_minor` and withdrawal limits as `XLM`; this keeps
the sandbox's fiat-denominated quote input explicit. Initiation creates a
durable session; the existing order use cases run only after the browser links a
KailoPay retail session and approved Persona KYC is present. `sep24_transactions`
stores the stable wallet-to-transaction-to-order mapping; polling resolves the
order through the caller's SEP-10 account and never trusts a client-supplied
status. An unknown checkout outcome is reported as `pending_external` until
reconciliation.
Deposit responses also include the sandbox-specific `payment_link_url` when a
hosted checkout was created. Withdrawal responses do not include this field.
`/sep24/deposit` and `/sep24/withdraw` are retained as local compatibility
aliases. When `OFFRAMP_PAYOUT_MODE=simulated` is enabled on Stellar testnet,
retirement queues a deterministic sandbox payout and completes the order; the
optional payout evidence keeps its existing shape and explicitly discloses that
no real IDR moved. `disabled` leaves the order in `withdrawal_processing`.

### SEP-38 quote server

The anchor advertises `/sep38` through `ANCHOR_QUOTE_SERVER` in
`/.well-known/stellar.toml`. These endpoints use the same market-price and
spread policy as `/v1/quotes`, but expose Stellar's asset-identification format
(`iso4217:IDR` and `stellar:native`) for wallet and anchor integrations:

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/sep38/info` | Public | List supported quote assets and delivery methods |
| `GET` | `/sep38/prices?sell_asset=...&buy_asset=...` | Public | Return an indicative price for the requested pair |
| `GET` | `/sep38/price?...&sell_amount=...` or `buy_amount` | Public | Calculate the exact amount-specific result |
| `POST` | `/sep38/quote` | SEP-10 bearer JWT | Create a wallet-owned firm quote |
| `GET` | `/sep38/quote/{id}` | SEP-10 bearer JWT | Retrieve an unexpired, unconsumed firm quote |

`/sep38/price` requires exactly one amount. IDR values are integer minor units;
native XLM values use up to seven decimal places. `/v1/quotes` remains the
first-party convenience contract for the web app, while both surfaces call the
same quote policy and do not reserve liquidity or create an order. Firm quotes
are persisted for one wallet, expire, and can be consumed once by a matching
SEP-24 initiation. The `/v1/onramps` and `/v1/offramps` request/response
contracts are unchanged.

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
  "payment_method": "xendit",
  "stellar_destination": { "account": "G...", "memo": null }
}
```

Validation:

- Positive amount within configured sandbox bounds.
- `IDR`, native XLM on testnet, and `xendit`, `qris`, or `bri_va`. `xendit`
  opens a hosted checkout with all activated Xendit channels; the other two
  values restrict the hosted checkout to one channel.
- Valid Stellar account and memo/muxed-account policy.
- An approved Persona sandbox KYC inquiry is required before order creation. The
  demo does not accept production identity documents or production bank data.

Response: `201` for a new order or `200` for an idempotent replay, with the
immutable quote and Xendit hosted checkout URL. An unknown provider-create
outcome returns `202` with the durable `order_id` and is held for
reconciliation; KailoPay does not automatically create a replacement checkout.

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
- Response: `orders`, `next_cursor`, and the request ID header/body contract.
- Retail-session results include orders created in earlier valid sessions for
  the same user, but never API-client orders or another user's orders.

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
| 401 | `INVALID_API_KEY` | Missing, malformed, invalid, or revoked API key/session credentials |
| 403 | `ORIGIN_NOT_ALLOWED` | Session-authenticated mutation has no matching configured browser origin |
| 403 | `KYC_REQUIRED` | User has not completed approved Persona identity verification |
| 404 | `ORDER_NOT_FOUND` | Resource absent or not visible to the authenticated principal |
| 409 | `INVALID_ORDER_STATE` | Operation conflicts with current lifecycle |
| 409 | `IDEMPOTENCY_KEY_REUSED` | Same key used with a different request |
| 409 | `INSUFFICIENT_LIQUIDITY` | Treasury inventory cannot cover the order |
| 422 | `AMOUNT_OUT_OF_RANGE` | Valid shape but unsupported amount |
| 429 | `RATE_LIMITED` | Client exceeded sandbox limit |
| 202 | `CHECKOUT_PENDING_RECONCILIATION` | Checkout transport outcome unknown; the order is held for provider reconciliation and identified by `order_id` |
| 503 | `QUOTE_UNAVAILABLE` | Market data missing, stale, or produced an invalid quote |
| 503 | `KYC_PROVIDER_UNAVAILABLE` | Persona inquiry creation/resume is temporarily unavailable |
| 502/503 | `EXTERNAL_SERVICE_UNAVAILABLE` | Provider/network unavailable; safe retry guidance required |
| 500 | `INTERNAL_ERROR` | Unexpected server error; reference request ID |

Week 1 implements `INVALID_REQUEST`, `INVALID_STELLAR_ACCOUNT`,
`INSUFFICIENT_LIQUIDITY`, `IDEMPOTENCY_KEY_REUSED`, `AMOUNT_OUT_OF_RANGE`,
`ORDER_NOT_FOUND`, `CHECKOUT_PENDING_RECONCILIATION`, `QUOTE_UNAVAILABLE`, and
`EXTERNAL_SERVICE_UNAVAILABLE`, `KYC_REQUIRED`, and
`KYC_PROVIDER_UNAVAILABLE`. The remaining catalog entries
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
- Cross-client, cross-user, and cross-session resource access fails safely.
- Session-authenticated order mutations reject missing or mismatched origins.
- Same-key retries after session renewal return the original consumer order.
- Unknown checkout outcomes return a durable order ID for polling.
- Errors contain stable code and request ID without internal stack/provider secrets.
- Amounts round-trip exactly.
- Public schemas never claim production/mainnet behavior.
