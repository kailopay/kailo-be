# Frontend capabilities and backend integration

This document lists the frontend features that the current KailoPay backend can support. It describes the sandbox and Stellar testnet implementation in the local backend repository.

Use [`openapi/openapi.yaml`](../../openapi/openapi.yaml) as the request and response contract. This page explains how to use that contract from a browser application.

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

### Identity verification

The frontend can provide a KYC status screen and an embedded Persona inquiry flow:

1. Call `GET /v1/kyc` to read the current status.
2. Call `POST /v1/kyc/inquiry` to create or resume an inquiry.
3. Use the short-lived inquiry session token returned by the backend in the Persona browser flow.
4. Refresh `GET /v1/kyc` until the status changes.

Only an approved KYC status can create an API key or an on-ramp or off-ramp order. The backend does not return provider secrets or identity-document fields to the browser.

### Buy XLM with IDR

The frontend can build a buy screen for the IDR to XLM sandbox flow.

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

The response contains the quoted IDR amount, the exact XLM amount, the quote expiry, and a checkout presentation. Use the returned `checkout.payment_link_url` when it exists. Do not construct a provider URL in the frontend.

IDR values use integer minor units encoded as strings. XLM values use decimal strings with up to seven fractional digits. Keep both values as strings in frontend state and request models.

### Track buy orders

The frontend can show an order detail page and order history with:

- `GET /v1/orders/{id}` for one owned order.
- `GET /v1/orders?limit=20&cursor=...` for cursor-paginated history.

For an on-ramp, the relevant states are `created`, `payment_pending`, `payment_confirmed`, `stellar_processing`, `completed`, `expired`, `payment_failed`, `stellar_failed`, and `cancelled`.

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

The create response returns `order.stellar_destination.account` and `order.stellar_destination.memo`. The user must send the exact XLM amount to that account with that memo before the deposit expires.

The `destination_token` is a synthetic sandbox reference. It is not a bank account, and the frontend must not present it as real bank details.

For an off-ramp, the relevant states are `asset_pending`, `asset_received`, `asset_invalid`, `retirement_processing`, `withdrawal_processing`, `completed`, `expired`, `retirement_failed`, `withdrawal_failed`, and `cancelled`.

When the order reaches `completed`, the response may include `payout`. The payout has `simulated: true` and the disclosure `sandbox simulation; no IDR was transferred`. Show that disclosure in the UI.

### Developer portal

After the user enables Developer Mode and passes KYC, the frontend can provide:

- Create a sandbox API key with `POST /v1/api-keys`.
- List key metadata with `GET /v1/api-keys`.
- Revoke a key with `DELETE /v1/api-keys/{id}`.
- Show the new key once, then instruct the user to store it securely.

The complete `pk_test_` API key is returned only when it is created. Never put it in browser JavaScript, local storage, a frontend bundle, or a public repository. Use it from a server-side integration.

Webhook endpoint management is described by `POST /v1/webhook-endpoints`, `GET /v1/webhook-endpoints`, and `DELETE /v1/webhook-endpoints/{id}`. The endpoint contract exists, but outbound webhook delivery is not active in the current worker. Treat webhook configuration as unavailable until the backend delivery worker is released.

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

### Public anchor metadata

The frontend or an integration screen can read:

- `GET /.well-known/stellar.toml` for the configured sandbox anchor descriptor.
- `GET /federation?q=name*kailopay` for a testnet account and memo lookup.

These routes describe the Stellar testnet and native XLM sandbox flow. They do not advertise a production asset or production payment rail.

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
| `401` | Missing or invalid credentials | Ask the user to sign in again. |
| `403` | `KYC_REQUIRED` or disallowed browser origin | Show the verification or configuration requirement. |
| `409` | `IDEMPOTENCY_KEY_REUSED` or insufficient liquidity | Keep the original request state and show the returned error. |
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

## Source files

- [OpenAPI contract](../../openapi/openapi.yaml)
- [Order state machines](ORDER-STATE-MACHINES.md)
- [Payment gateway integration](PAYMENT-GATEWAY-INTEGRATION.md)
- [Stellar anchor integration](STELLAR-ANCHOR-INTEGRATION.md)
- [Backend backlog and implementation status](BACKEND-BACKLOG.md)
