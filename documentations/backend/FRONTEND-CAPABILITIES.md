# Frontend capabilities and backend integration

This document lists the frontend features that the current KailoPay backend can support. It describes the sandbox and Stellar testnet implementation in the local backend repository.

Use [`openapi/openapi.yaml`](../../openapi/openapi.yaml) as the request and response contract. This page explains how to use that contract from a browser application.

## Endpoint map

The API uses one base URL. The local default is `http://localhost:8080`. Use the deployed API URL in every environment instead of hard-coding the local value.

| Area | Endpoints | Frontend use |
|---|---|---|
| Runtime | `GET /livez`, `GET /healthz`, `GET /health`, `GET /readyz`, `GET /ready`, `GET /startupz` | Deployment checks or an internal status page. |
| API docs | `GET /docs/`, `GET /docs/openapi.yaml` | Developer reference. |
| Authentication | `POST /auth/register`, `POST /auth/login`, `GET /auth/google/login`, `GET /auth/google/callback`, `POST /auth/logout`, `POST /auth/email/verify`, `POST /auth/email/resend`, `POST /auth/password/forgot`, `POST /auth/password/reset`, `POST /auth/password/change` | Account creation, sign-in, verification, password recovery, and sign-out. |
| Profile | `GET /auth/me`, `PATCH /auth/me`, `GET /auth/me/avatar`, `PUT /auth/me/avatar`, `DELETE /auth/me/avatar` | Account settings and profile image. |
| KYC | `GET /v1/kyc`, `POST /v1/kyc/inquiry` | Verification status and Persona flow. |
| Developer keys | `POST /v1/api-keys`, `GET /v1/api-keys`, `DELETE /v1/api-keys/{id}` | Server-integration setup. Keep keys out of browser code. |
| Developer webhooks | `POST /v1/webhook-endpoints`, `GET /v1/webhook-endpoints`, `DELETE /v1/webhook-endpoints/{id}` | Configuration UI only. Delivery is not active yet. |
| Orders | `POST /v1/quotes`, `POST /v1/onramps`, `POST /v1/offramps`, `GET /v1/orders`, `GET /v1/orders/{id}` | Quote preview, buy, sell, order history, and order tracking. |
| Anchor discovery | `GET /.well-known/stellar.toml`, `GET /federation?q=...`, `GET /sep24/info` | Stellar integration and sandbox information. |
| SEP-24 | `POST /sep24/transactions/deposit/interactive`, `POST /sep24/transactions/withdraw/interactive`, `GET /sep24/transaction?id=...`, `GET /sep24/interactive/{id}`, `POST /sep24/deposit`, `POST /sep24/withdraw` | Authenticated JSON-based deposit and withdrawal screens. |
| Provider callbacks | `POST /callbacks/kyc/persona`, `POST /callbacks/payments/xendit` | Provider-to-backend traffic. Do not call these from the frontend. |

The `/sep24/deposit` and `/sep24/withdraw` routes are compatibility aliases. Use the nested `/sep24/transactions/.../interactive` routes for new code.

## Shared request rules

Apply these rules to every frontend API client:

- Send `Accept: application/json` for JSON endpoints.
- Send `Content-Type: application/json` for JSON request bodies.
- Use `credentials: "include"` for session-authenticated requests.
- Use `Authorization: Bearer pk_test_...` only from a trusted server.
- Send an `Idempotency-Key` for every new on-ramp, off-ramp, or SEP-24 start request.
- Keep IDR minor units, XLM amounts, quote rates, and cursors as strings unless the UI needs a separate display number.
- Treat `environment: "sandbox"` and `network: "stellar_testnet"` as response data that the UI must show.
- Parse both simple errors such as `{ "error": "..." }` and structured errors such as `{ "error": { "code": "...", "message": "..." } }`.

Create one idempotency key for one user action. Reuse that key with the same body when the network response is lost. Do not reuse it with a different body. The backend returns `409 IDEMPOTENCY_KEY_REUSED` for that conflict.

## Authentication and response matrix

Use the following matrix when deciding which client can call an endpoint:

| Client context | Endpoints | Required browser behavior |
|---|---|---|
| Public browser request | Health, API docs, `/.well-known/stellar.toml`, `/federation`, `/sep24/info` | No login is required. The TOML route returns text, not JSON. |
| Browser navigation | `/auth/google/login`, `/auth/google/callback` | Navigate to the URL with `location.assign`; do not call these routes through `fetch`. |
| Session cookie | `/auth/me`, password change, avatar, KYC, API keys, webhook management | Send `credentials: "include"`. The cookie is HttpOnly, so the frontend reads the returned user or status rather than the cookie value. |
| Session or server API key | On-ramp, off-ramp, order reads, and SEP-24 transaction routes | Use the session for the consumer web app. Use `Authorization: Bearer pk_test_...` only in a trusted server integration. |
| Provider callback | `/callbacks/kyc/persona`, `/callbacks/payments/xendit` | Never call these from the frontend. Persona and Xendit call them directly. |

Successful response conventions are also consistent across the API:

- `200` means a read, update, or safe idempotent replay returned a body.
- `201` means a new resource or order was created.
- `202` with `CHECKOUT_PENDING_RECONCILIATION` means the request may have reached the provider, but its result is unknown. Keep the returned `order_id` and poll it.
- `204` means success with no response body. Do not call `response.json()` for a `204` response.
- Collection responses use initialized arrays such as `orders`, `api_keys`, or `endpoints`. Order history also returns `next_cursor`; an empty cursor means there is no next page.

The shared failure shape can be typed as:

```ts
type ApiFailure = {
  error:
    | string
    | {
        code: string;
        message: string;
        retryable?: boolean;
        details?: string;
      };
  request_id?: string;
  order_id?: string;
};
```

Always branch on the HTTP status and stable `error.code`, not on the human-readable message. Keep `request_id` in client logs or support reports, but do not expose internal diagnostics to end users.

## Build these features now

### Account and profile

The frontend can provide these screens and actions:

- Create an account with email, password, and an optional display name.
- Verify an email address with the one-time token from the verification email.
- Sign in and sign out with the KailoPay session cookie.
- Start optional Google sign-in when the backend has Google credentials.
- Request a verification email again.
- Request a password reset, reset the password, or change the password.
- Read and edit the display name and Developer Mode setting.
- Upload, read, or delete a private profile avatar.

Relevant endpoints are `/auth/register`, `/auth/login`, `/auth/logout`, `/auth/google/login`, `/auth/google/callback`, `/auth/email/verify`, `/auth/email/resend`, `/auth/password/forgot`, `/auth/password/reset`, `/auth/password/change`, `/auth/me`, and `/auth/me/avatar`.

The login response sets the `kailopay_session` HTTP-only cookie. Browser requests that use the session must send `credentials: "include"`. The frontend origin must be configured in the backend. Browser requests that create orders must include that allowed `Origin`. If `Origin` is absent, the backend accepts a matching `Referer` origin.

#### Authentication request details

| Method and path | Body or query | Success response | UI behavior |
|---|---|---|---|
| `POST /auth/register` | JSON: `email`, `password`, optional `display_name` | `201` with `{ user }` | Show the email-verification step. This call does not sign the user in. |
| `POST /auth/email/verify` | JSON: `token` | `200` with `{ user }` | Mark the account as verified, then show sign-in. |
| `POST /auth/login` | JSON: `email`, `password` | `200` with `{ user }` and a session cookie | Load the authenticated app. |
| `GET /auth/google/login` | No body | `302` redirect | Navigate the browser to this URL. Do not use `fetch` for the redirect flow. |
| `GET /auth/google/callback` | Provider `code` and `state` query values | `302` to the configured frontend URL with a session cookie | Load the user session after the redirect. |
| `POST /auth/logout` | No body | `204` | Clear local user state. |
| `POST /auth/email/resend` | JSON: `email` | `202` | Show a generic message. The response does not reveal whether the account exists. |
| `POST /auth/password/forgot` | JSON: `email` | `202` | Show a generic password-reset message. |
| `POST /auth/password/reset` | JSON: `token`, `new_password` | `204` | Send the user to sign-in. All existing sessions are revoked. |
| `POST /auth/password/change` | JSON: `current_password`, `new_password` | `204` | Send the user to sign-in. All existing sessions are revoked. |

Passwords must contain 10 to 128 characters. Verification tokens expire after 24 hours. Password-reset tokens expire after one hour. Both token types are single-use.

Use the token from the email link as the `token` request field. The frontend owns the verification and reset routes. The backend owns token validation and account state changes.

The configured email links use `/auth/verify-email?token=...` and `/auth/reset-password?token=...`. Build those frontend routes to read the query token, call the matching backend endpoint, and remove the token from the address bar after the request completes.

#### Profile request details

`GET /auth/me` returns `{ "user": { ... } }`. The profile contains `id`, `display_name`, `email`, `email_verified`, `developer_enabled`, and an optional `avatar_url`.

When `avatar_url` is present, treat it as a backend-relative URL. Request it with the session cookie. Do not assume that the image is publicly accessible.

`PATCH /auth/me` accepts at least one of these fields:

```json
{
  "display_name": "Ada Lovelace",
  "developer_enabled": true
}
```

`PUT /auth/me/avatar` uses `multipart/form-data` with one file field named `avatar`. The backend accepts JPEG, PNG, and WebP images up to the configured size limit. `GET /auth/me/avatar` returns image bytes, so use the response `Content-Type` when displaying the image. `DELETE /auth/me/avatar` returns the updated `{ user }` object.

The avatar read is the one authenticated endpoint that is not JSON. Handle it separately from `apiFetch`:

```ts
const response = await fetch(`${API_BASE_URL}/auth/me/avatar`, {
  credentials: "include",
});
if (response.status === 404) {
  // The user has no avatar.
} else if (!response.ok) {
  throw new Error("Could not load avatar");
} else {
  const avatarURL = URL.createObjectURL(await response.blob());
  // Revoke the previous object URL when the image is replaced or unmounted.
}
```

### Identity verification

The frontend can provide a KYC status screen and an embedded Persona inquiry flow:

1. Call `GET /v1/kyc` to read the current status.
2. Call `POST /v1/kyc/inquiry` to create or resume an inquiry.
3. Use the short-lived inquiry session token returned by the backend in the Persona browser flow.
4. Refresh `GET /v1/kyc` until the status changes.

The backend response gives a direct Persona hosted-flow `url`, plus the Persona `environment_id` and `inquiry_id`. The frontend may open `url` directly, or pass those provider identifiers plus `session_token` when the response includes it to the Persona browser SDK configured for that environment. The frontend should not call Persona's REST API or persist the token. Completion in the browser is not proof of approval; wait for `GET /v1/kyc` to report `approved`, which is set from the signed Persona callback.

Only an approved KYC status can create an API key or an on-ramp or off-ramp order. The backend does not return provider secrets or identity-document fields to the browser.

`GET /v1/kyc` returns this shape:

```json
{
  "kyc": {
    "provider": "persona",
    "status": "pending",
    "provider_status": "pending",
    "inquiry_id": "inq_123",
    "created_at": "2026-09-15T08:00:00Z",
    "updated_at": "2026-09-15T08:01:00Z",
    "expires_at": null,
    "approved_at": null
  }
}
```

`POST /v1/kyc/inquiry` does not need a request body. It returns `{ "inquiry": { ... } }` including a direct hosted Persona `url`. The `inquiry` object may contain a short-lived `session_token` when the backend resumes an existing inquiry. Use that token only to start the Persona embedded flow. Do not store it as an application credential.

Use the public `status` field for UI decisions. Typical states are `not_started`, `creating`, `created`, `pending`, `completed`, `pending_review`, `approved`, `declined`, `failed`, and `expired`.

### Buy XLM with IDR

The frontend can build a buy screen for the IDR to XLM sandbox flow.

Before submission, the frontend can request an indicative price with
`POST /v1/quotes`. This endpoint accepts the same authenticated API-key or
retail-session principal as order creation and does not reserve liquidity or
create a checkout. The order create response is authoritative if the market
moves after the preview.

For a buy preview, send:

```json
{
  "direction": "buy",
  "fiat": { "currency": "IDR", "amount_minor": "100000" },
  "asset": { "network": "stellar_testnet", "code": "XLM" }
}
```

For a sell preview, send the XLM amount instead:

```json
{
  "direction": "sell",
  "fiat": { "currency": "IDR" },
  "asset": { "network": "stellar_testnet", "code": "XLM", "amount": "25.0000000" }
}
```

The response is `{ "quote": { ... } }` and includes the calculated `fiat`,
`asset`, `rate`, `adjusted_rate`, `spread_bps`, `source_at`, and `expires_at`.
Keep the amount and rate fields as strings. A preview can expire or become
stale; request a new preview when the user changes the amount, and always use
the quote returned by the subsequent order response for payment or deposit
instructions.

Send `POST /v1/onramps` with an `Idempotency-Key` header:

```json
{
  "fiat": {
    "currency": "IDR",
    "amount_minor": "100000"
  },
  "payment_method": "qris",
  "stellar_destination": {
    "account": "<56-character Stellar testnet account>",
    "memo": "optional-memo"
  }
}
```

Supported `payment_method` values are:

- `xendit`: use all channels enabled for the Xendit sandbox merchant.
- `qris`: restrict the hosted checkout to QRIS.
- `bri_va`: restrict the hosted checkout to BRI Virtual Account.

If `payment_method` is omitted, the backend uses `xendit`.

The response contains the quoted IDR amount, the exact XLM amount, the quote expiry, and a checkout presentation. Use the returned `checkout.payment_link_url` when it exists. Do not construct a provider URL in the frontend.

The normal success response is wrapped in an `order` property:

```json
{
  "order": {
    "id": "order-uuid",
    "status": "payment_pending",
    "environment": "sandbox",
    "network": "stellar_testnet",
    "fiat": {
      "currency": "IDR",
      "amount_minor": "100000"
    },
    "asset": {
      "code": "XLM",
      "amount": "40.0000000"
    },
    "quote": {
      "rate": "2500",
      "adjusted_rate": "2500",
      "spread_bps": 0,
      "source_at": "2026-09-15T08:00:00Z",
      "expires_at": "2026-09-15T08:05:00Z"
    },
    "payment_method": "qris",
    "stellar_destination": {
      "account": "<56-character Stellar testnet account>",
      "memo": "optional-memo"
    },
    "checkout": {
      "id": "provider-checkout-id",
      "status": "ACTIVE",
      "presentation_type": "PAYMENT_LINK",
      "presentation_value": "https://checkout.example.test/session",
      "payment_link_url": "https://checkout.example.test/session",
      "expires_at": "2026-09-15T08:05:00Z"
    },
    "created_at": "2026-09-15T08:00:00Z",
    "updated_at": "2026-09-15T08:00:00Z"
  }
}
```

The backend may omit `checkout`, `stellar_transaction_hash`, `deposit_transaction_hash`, `payout`, and `failure_code` when those values do not exist. Do not treat an omitted field as an empty successful value.

Render the checkout according to `presentation_type`:

| `presentation_type` | UI behavior |
|---|---|
| `PAYMENT_LINK` | Open `payment_link_url` or `presentation_value` in the hosted checkout. |
| `QR_STRING` | Render `presentation_value` as a QR payload. |
| `VIRTUAL_ACCOUNT_NUMBER` | Show `presentation_value` as the account number and show the expiry. |

The current Xendit adapter validates a hosted HTTPS payment link. The API schema still supports the other presentation types so the frontend can handle a different configured provider response.

IDR values use integer minor units encoded as strings. XLM values use decimal strings with up to seven fractional digits. Keep both values as strings in frontend state and request models.

The local example configuration accepts IDR amounts from `10000` through `10000000`. The deployment may use different limits, so let the API return `AMOUNT_OUT_OF_RANGE` instead of assuming these values are fixed.

### Track buy orders

The frontend can show an order detail page and order history with:

- `GET /v1/orders/{id}` for one owned order.
- `GET /v1/orders?limit=20&cursor=...` for cursor-paginated history.

The list can contain both on-ramp and off-ramp orders. The current public order response does not include a `direction` field. Use the order status and fields such as `checkout`, `deposit_transaction_hash`, and `payout` to render the current UI. Treat this as a backend contract limitation when designing a permanent order-type filter.

The list response is:

```json
{
  "orders": [
    { "id": "order-uuid", "status": "completed" }
  ],
  "next_cursor": "opaque-cursor-or-empty-string"
}
```

Treat `next_cursor` as opaque. To load the next page, send it unchanged as `cursor`; do not decode it, sort by it, or manufacture one. A `limit` between 1 and 100 is accepted; omitting it uses the backend default of 20.

For an on-ramp, the relevant states are `created`, `payment_pending`, `payment_confirmed`, `stellar_processing`, `completed`, `expired`, `payment_failed`, `stellar_failed`, and `cancelled`.

The backend records `payment_confirmed` before it queues the Stellar transfer. The transfer transaction can move the stored order state to `stellar_processing` in the same database operation. The frontend may therefore see `payment_pending` followed by `stellar_processing` without observing `payment_confirmed`.

Poll the order endpoint while the order is non-terminal. Use `completed` only after the backend returns that state. A provider checkout redirect does not confirm payment by itself.

When the create request returns `202` with `CHECKOUT_PENDING_RECONCILIATION`, keep the returned `order_id` and do not create a duplicate order with a new idempotency key. The backend marks the order for provider reconciliation, but the background reconciliation path is not complete in this release.

### Sell XLM for IDR

The frontend can build a sell screen for the sandbox off-ramp flow.

Send `POST /v1/offramps` with an `Idempotency-Key` header:

```json
{
  "asset": {
    "network": "stellar_testnet",
    "code": "XLM",
    "amount": "25.0000000"
  },
  "withdrawal": {
    "currency": "IDR",
    "method": "sandbox_bank_transfer",
    "destination_token": "sandbox-bank-user-01"
  }
}
```

The intended create response contains `order.stellar_destination.account` and `order.stellar_destination.memo`. The user must send the exact XLM amount to that account with that memo before the deposit expires.

There is a backend response-mapping issue to account for during integration: the off-ramp repository currently stores the configured deposit account as `StellarSource`, while the public order serializer reads `StellarDestination`. Verify that `order.stellar_destination.account` is non-empty in the deployed response before enabling the send step. If it is empty, the backend must correct that mapping first; the frontend cannot safely infer the deposit account.

The `destination_token` is a synthetic sandbox reference. It is not a bank account, and the frontend must not present it as real bank details.

For an off-ramp, the relevant states are `asset_pending`, `asset_received`, `asset_invalid`, `retirement_processing`, `withdrawal_processing`, `completed`, `expired`, `retirement_failed`, `withdrawal_failed`, and `cancelled`.

When the order reaches `completed`, the response may include `payout`. The payout has `simulated: true` and the disclosure `sandbox simulation; no IDR was transferred`. Show that disclosure in the UI.

The backend records `asset_received` before it queues retirement. The stored order state can already be `retirement_processing` when the frontend polls it. Treat `asset_received` as a valid transitional state, not as a state that must appear.

For an off-ramp, the intended `order.stellar_destination` identifies the deposit account and memo. `order.deposit_transaction_hash` identifies the user's XLM deposit after the worker accepts it. `order.payout.reference` identifies the simulated IDR payout after the worker records it. Until the response-mapping issue above is fixed, treat an empty account as a backend error and keep the order in an instruction-unavailable state.

The shared order response includes `payment_method`, but that field is meaningful only for on-ramp orders. Use the off-ramp request's `withdrawal.method` and the returned `payout.method` for sell orders.

### Developer portal

After the user enables Developer Mode and passes KYC, the frontend can provide:

- Create a sandbox API key with `POST /v1/api-keys`.
- List key metadata with `GET /v1/api-keys`.
- Revoke a key with `DELETE /v1/api-keys/{id}`.
- Show the new key once, then instruct the user to store it securely.

The complete `pk_test_` API key is returned only when it is created. Never put it in browser JavaScript, local storage, a frontend bundle, or a public repository. Use it from a server-side integration.

Webhook endpoint management is described by `POST /v1/webhook-endpoints`, `GET /v1/webhook-endpoints`, and `DELETE /v1/webhook-endpoints/{id}`. The endpoint contract exists, but outbound webhook delivery is not active in the current worker. Treat webhook configuration as unavailable until the backend delivery worker is released.

#### API key request details

Create a key with a name between 1 and 100 characters:

```json
{
  "name": "Local checkout service"
}
```

The create response has this shape:

```json
{
  "api_key": {
    "id": "key-uuid",
    "client_id": "client-uuid",
    "prefix": "pk_test_",
    "key": "pk_test_<one-time-secret>",
    "created_at": "2026-09-15T08:00:00Z"
  }
}
```

`GET /v1/api-keys` returns `{ "api_keys": [] }`. The list contains metadata and never contains a secret. `DELETE /v1/api-keys/{id}` returns `204` when the key is revoked. A revoked key cannot authenticate later requests.

#### Webhook endpoint request details

The request accepts an HTTPS URL with a public host and no embedded credentials:

```json
{
  "url": "https://developer.example.test/kailopay/events",
  "event_types": [
    "order.payment_confirmed",
    "order.completed",
    "order.failed"
  ]
}
```

Omit `event_types`, or send an empty array, to subscribe to every supported order event. The supported event types are `order.created`, `order.payment_pending`, `order.payment_confirmed`, `order.asset_received`, `order.processing`, `order.completed`, `order.failed`, and `order.expired`.

The create response returns `{ "endpoint": { "id": "..." }, "secret": "whsec_..." }`. Store the secret when it is returned. The backend does not return it again. The current backend has a known ownership mismatch in this endpoint path, so the frontend should show webhook configuration as disabled until that backend defect is fixed.

### SEP-24 deposit and withdrawal screens

The frontend can build an authenticated JSON-based SEP-24 sandbox screen. The backend does not provide a wallet-facing HTML interactive page.

Use these endpoints:

- `GET /sep24/info` to read supported XLM limits and units.
- `POST /sep24/transactions/deposit/interactive` to start a deposit.
- `POST /sep24/transactions/withdraw/interactive` to start a withdrawal.
- `GET /sep24/transaction?id=...` to read transaction status.
- `GET /sep24/interactive/{id}` to read the authenticated transaction projection.

The deposit request is `multipart/form-data` with `asset_code=XLM`, a positive `amount_minor` IDR string, a Stellar testnet `account`, and an optional `memo` or `payment_method`.

The withdrawal request is `multipart/form-data` with `asset_code=XLM`, an exact XLM `amount`, and a synthetic `destination_token`.

The SEP-24 status values are `pending_user_transfer_start`, `pending_anchor`, `pending_external`, `completed`, `expired`, and `error`.

#### SEP-24 response details

`GET /sep24/info` returns configuration data. Do not hard-code the limits because the deposit range comes from backend configuration:

```json
{
  "deposit": {
    "XLM": {
      "enabled": true,
      "min_amount": 1,
      "max_amount": 100000,
      "amount_unit": "idr_minor",
      "fiat_currency": "IDR"
    }
  },
  "withdraw": {
    "XLM": {
      "enabled": true,
      "min_amount": 1,
      "max_amount": 100000,
      "amount_unit": "XLM",
      "fiat_currency": "IDR"
    }
  },
  "fee": {
    "XLM": {
      "type": "none"
    }
  }
}
```

Start responses have this shape:

```json
{
  "type": "interactive_customer_info_needed",
  "url": "http://localhost:8080/sep24/interactive/sep24-deposit-order-uuid",
  "id": "sep24-deposit-order-uuid",
  "kyc_required": false,
  "environment": "sandbox",
  "network": "stellar_testnet",
  "payment_link_url": "https://checkout.example.test/session"
}
```

`payment_link_url` is returned for a deposit when the mapped Xendit checkout has one. It is not returned for a withdrawal.

The response field `kyc_required: false` does not bypass the backend KYC gate. A start request can still return `403 KYC_REQUIRED` until the user's Persona status is `approved`.

The transaction response is wrapped in `transaction`:

```json
{
  "transaction": {
    "id": "sep24-withdraw-order-uuid",
    "kind": "withdraw",
    "status": "pending_user_transfer_start",
    "started_at": "2026-09-15T08:00:00Z",
    "updated_at": "2026-09-15T08:00:00Z",
    "withdraw_anchor_account": "<configured deposit account>",
    "withdraw_memo": "off-order-uuid",
    "withdraw_memo_type": "text",
    "amount_in": "25.0000000",
    "more_info_url": "http://localhost:8080/sep24/interactive/sep24-withdraw-order-uuid"
  }
}
```

The `id` is also the mapped order identifier in this sandbox implementation. Use the returned `id` when polling. Do not let the user submit a client-supplied status.

`GET /sep24/interactive/{id}` returns the same transaction under an additional wrapper:

```json
{
  "environment": "sandbox",
  "network": "stellar_testnet",
  "kyc_required": false,
  "transaction": { "id": "sep24-order-uuid", "status": "pending_anchor" }
}
```

Use `GET /sep24/transaction?id=...` for a standard transaction projection and the interactive route when the UI needs the sandbox/environment metadata. Both are owner-scoped and require authentication.

Use `FormData` for a SEP-24 start request:

```ts
const form = new FormData();
form.set("asset_code", "XLM");
form.set("amount_minor", "100000");
form.set("account", stellarTestnetAccount);
form.set("memo", "optional-memo");
form.set("memo_type", "text");
form.set("payment_method", "qris");

const result = await apiFetch<Sep24StartResponse>(
  "/sep24/transactions/deposit/interactive",
  {
    method: "POST",
    headers: { "Idempotency-Key": crypto.randomUUID() },
    body: form,
  },
);
```

For a withdrawal, replace `amount_minor`, `account`, `memo`, `memo_type`, and `payment_method` with `amount` and `destination_token`.

### Public anchor metadata

The frontend or an integration screen can read:

- `GET /.well-known/stellar.toml` for the configured sandbox anchor descriptor.
- `GET /federation?q=name*kailopay` for a testnet account and memo lookup.

These routes describe the Stellar testnet and native XLM sandbox flow. They do not advertise a production asset or production payment rail.

For federation lookup, send the synthetic name in the `q` query parameter:

```text
GET /federation?q=alice*kailopay
```

The response contains `stellar_address`, `memo`, and `memo_type: "text"`. The frontend can use these values when it builds a deposit instruction. A malformed or unknown name returns `404`.

The Stellar TOML response uses `Content-Type: application/x-stellar-toml`. The frontend can display the document or pass it to a Stellar integration. It is not a JSON response.

## Authentication choices

Use a local session for the consumer web application:

```ts
await fetch(`${API_BASE_URL}/auth/login`, {
  method: "POST",
  credentials: "include",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email, password }),
});
```

Use a bearer API key only from a trusted server:

```text
Authorization: Bearer pk_test_...
```

The order endpoints accept either authentication method. A browser application should use the session cookie. A backend integration should use the API key.

## Common frontend error states

The backend returns these states often enough to warrant dedicated UI:

| HTTP status | Code or condition | Frontend behavior |
|---|---|---|
| `400` | Invalid JSON, token, form value, or field | Keep the user on the form and show the field or request error. |
| `401` | Missing or invalid credentials | Ask the user to sign in again. |
| `403` | `KYC_REQUIRED` or disallowed browser origin | Show the verification or configuration requirement. |
| `404` | Resource is missing or outside the authenticated owner's scope | Show a not-found state. Do not reveal another user's resource. |
| `409` | `IDEMPOTENCY_KEY_REUSED` or insufficient liquidity | Keep the original request state and show the returned error. |
| `413` | Avatar is larger than the configured limit | Ask the user to choose a smaller image. |
| `415` | Order request is not JSON | Send `Content-Type: application/json`. |
| `422` | `AMOUNT_OUT_OF_RANGE` | Show the configured amount error. |
| `202` | `CHECKOUT_PENDING_RECONCILIATION` | Keep the returned order ID and wait for reconciliation. Do not retry with a new key. |
| `503` | Quote or external service unavailable | Keep the form data and allow a later retry with a new idempotency key. |

Order errors use this shape:

```json
{
  "error": {
    "code": "KYC_REQUIRED",
    "message": "approved identity verification is required"
  },
  "request_id": "request-id"
}
```

## Do not build against these assumptions

- Do not describe the on-ramp as issuing a custom token. The backend transfers pre-funded native XLM on Stellar testnet.
- Do not show the off-ramp as a real bank payout. The current payout is a recorded sandbox simulation.
- Do not assume a successful checkout redirect means the payment was confirmed.
- Do not call Xendit, Persona, Horizon, or federation signing flows directly from the browser. The backend owns those integrations.
- Do not depend on outbound developer webhooks until the backend adds delivery, signing, retries, and attempt records.

## Backend gaps the frontend should surface explicitly

These items are important when turning the endpoint map into production-ready screens:

- **Off-ramp deposit instructions:** the normal off-ramp response may have an empty `order.stellar_destination.account` because the repository and public serializer use different fields. Do not invent an account or derive one from `destination_token`. Block the transfer-instruction step and report a backend configuration/contract error until the mapping is fixed. The SEP-24 withdrawal projection currently has a separate `withdraw_anchor_account` field, but it should not be used to hide a broken normal off-ramp response.
- **Developer webhooks:** endpoint registration has a client-ownership mismatch in the current handler/repository path, and the worker does not deliver outbound events. Keep the settings page disabled or label it unavailable; do not promise that a registered URL will receive events.
- **Order direction:** the public order object does not include a stable `direction` field. For now, infer the view from the fields returned by the order and retain the original flow in local UI state. A permanent buy/sell filter needs a backend contract addition.
- **Unknown checkout outcomes:** a `202 CHECKOUT_PENDING_RECONCILIATION` response means the provider result is unknown. Keep the original order and idempotency key visible, and do not create a duplicate checkout. The reconciliation worker is not complete in this release.

## Runtime and provider endpoints

### Health and API documentation

The API exposes these operational routes:

| Endpoint | Healthy response | Unhealthy or starting response |
|---|---|---|
| `GET /livez`, `GET /healthz`, `GET /health` | `200 { "status": "ok" }` | None. These routes only report that the process is running. |
| `GET /readyz`, `GET /ready` | `200 { "status": "ready" }` | `503 { "status": "not_ready" }` when the database check fails. |
| `GET /startupz` | `200 { "status": "started" }` | `503 { "status": "starting" }` before startup finishes. |

Use readiness and startup routes in deployment checks. Do not use them to decide whether a user's order succeeded.

The backend redirects `GET /docs` to `GET /docs/`. The Swagger UI is available at `/docs/`, and the checked-in OpenAPI document is available at `/docs/openapi.yaml`.

### Provider callbacks

The frontend does not call either callback endpoint:

| Endpoint | Caller | Authentication | Frontend responsibility |
|---|---|---|---|
| `POST /callbacks/kyc/persona` | Persona | `Persona-Signature` over the exact raw body | Refresh `GET /v1/kyc` after the user finishes the embedded flow. |
| `POST /callbacks/payments/xendit` | Xendit | `x-callback-token` | Poll the related order. Never mark an order paid from browser state. |

The backend stores and validates these callbacks. A callback request from a browser is not a supported substitute for a provider callback.

## Build the frontend API client

Use one request helper so every session request includes the cookie and every error follows the same parsing path:

```ts
const API_BASE_URL = "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: unknown,
  ) {
    super("KailoPay API request failed");
  }
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (response.status === 204) {
    return undefined as T;
  }

  const body = await response.json().catch(() => undefined);
  if (!response.ok) {
    throw new ApiError(response.status, body);
  }
  return body as T;
}
```

When you send `FormData`, do not set `Content-Type` yourself. The browser adds the multipart boundary.

For a new order, generate the idempotency key before the request and keep it with the form submission:

```ts
const idempotencyKey = crypto.randomUUID();
const result = await apiFetch<{ order: Order }>("/v1/onramps", {
  method: "POST",
  headers: { "Idempotency-Key": idempotencyKey },
  body: JSON.stringify(request),
});
```

If the request fails after the server may have received it, retry with the same idempotency key and the same body. Generate a new key only for a new user action.

Use the OpenAPI schemas to generate TypeScript types for `UserProfile`, `KYCStatus`, `Order`, `WebhookEndpoint`, `SEP24Transaction`, and the error responses. Keep provider callback payload types out of the browser application.

## Frontend implementation checklist

Build the frontend in this order:

1. Add the API base URL and one `apiFetch` helper.
2. Add registration, verification, login, logout, and `GET /auth/me`.
3. Add KYC status and Persona inquiry handling.
4. Add the buy form, checkout presentation, and order polling.
5. Add the sell form and order polling; enable deposit account/memo display only after the off-ramp response contains a non-empty deposit account.
6. Add order history and detail views for both directions.
7. Add profile, password, and avatar settings.
8. Add Developer Mode and API-key management. Keep the full key out of browser storage.
9. Add the JSON SEP-24 deposit and withdrawal screens if the product needs an anchor-facing flow.
10. Keep webhook configuration hidden or marked unavailable until outbound delivery is implemented.

For every order screen, show the environment and network labels. Show the quote expiry, payment or deposit instructions, the current status, and the failure code when the backend returns one.

## Source files

- [OpenAPI contract](../../openapi/openapi.yaml)
- [Order state machines](ORDER-STATE-MACHINES.md)
- [Payment gateway integration](PAYMENT-GATEWAY-INTEGRATION.md)
- [Stellar anchor integration](STELLAR-ANCHOR-INTEGRATION.md)
- [Backend backlog and implementation status](BACKEND-BACKLOG.md)
