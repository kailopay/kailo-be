# KailoPay frontend guide (Week 1 backend handoff)

This guide is for the frontend developer or agent building against the KailoPay
backend. It describes the product, exactly which backend capabilities exist
today, the request/response contracts you can rely on, and the rules the
frontend must follow. The checked-in contract authority is
[`openapi/openapi.yaml`](../../openapi/openapi.yaml); this guide explains it in
frontend terms and adds operational facts the spec cannot express.

## 1. What KailoPay is

KailoPay is an Indonesia-first fiat on-ramp/off-ramp for Stellar. The first
release (`v0.1.0`, 30-day sandbox sprint) proves one corridor in
**sandbox/testnet form only**:

1. A developer creates an IDR→XLM order and pays through a **Xendit sandbox**
   QRIS or BRI virtual-account checkout.
2. Xendit calls the backend; payment is verified and reconciled.
3. A worker transfers reserved **Stellar testnet XLM** from a treasury account
   to the customer's testnet address.

The money is never real. Two product rules follow from that:

- Every screen that can move value must display visible `Sandbox` and
  `Stellar Testnet` labels (product requirement FR-053). The API reinforces
  this: every order response carries `"environment": "sandbox"` and
  `"network": "stellar_testnet"`.
- The UI must never imply real-money settlement, KYC, or production readiness.

What remains out of scope for now: webhook delivery (endpoints manageable already),
SEP-24 anchor flows, federation, rate limits, mainnet.

## 2. Running the backend locally

```powershell
docker compose up -d postgres minio
go run ./cmd/migrate   # applies versioned migrations
go run ./cmd/api       # serves on HTTP_ADDRESS (default :8080)
```

Copy `.env.example` to `.env` first. The values a human must fill are listed
in `.env.example` and the root `README.md`; everything else has defaults. Required for API startup: database DSN, the two AUTH crypto keys,
`API_KEY_PEPPER`, `COINMARKETCAP_API_KEY`, `XENDIT_SECRET_KEY` +
`XENDIT_CALLBACK_TOKEN`, `STELLAR_TREASURY_ACCOUNT`, and MinIO credentials.
`STELLAR_TREASURY_SECRET` is only needed when also running `go run ./cmd/worker`
(the XLM transfer worker; without it, paid orders stay in
`stellar_processing`).

### CORS: there is none (by design)

The backend sends **no CORS headers**. The frontend must be either:

- served from the same origin as the API (reverse proxy), or
- developed with a dev-server proxy (Vite `server.proxy`, Next.js rewrites,
  etc.) forwarding `/auth`, `/v1`, `/livez`, etc. to the API origin.

Do not build the app expecting cross-origin fetches with credentials to work.

### Where the frontend lives and with which stack

**Open decision.** Nothing is recorded yet, and this repository is backend
only. The working assumption is a separate frontend app (suggested sibling
repository `kailopay-fe`), using a component framework with a dev-server proxy
(Vite + React recommended for the 30-day window). If you are the agent
starting the frontend, create the app in its own repository or folder, do not
add a frontend directory inside this backend repo, and record the chosen stack
in your repository README.

### Local end-to-end: how a payment actually completes

Creating an order locally works immediately, but Xendit's sandbox webhook
cannot reach `localhost`. An order will sit in `payment_pending` until the
backend receives the Xendit callback. To see paid → `stellar_processing` →
`completed` locally you need one of:

1. A public tunnel (ngrok / Cloudflare Tunnel) exposing the API, with the
   tunnel URL configured as the payment callback URL in the Xendit dashboard,
   then pay the QRIS/VA in the Xendit sandbox flow.
2. Xendit's sandbox payment simulator (dashboard) to fire the payment event
   for your payment request.

The backend has no manual "simulate payment" endpoint; do not look for one.
The XLM transfer itself needs `cmd/worker` running with
`STELLAR_TREASURY_SECRET` and a funded treasury account. Without that, paid
orders legitimately stay in `stellar_processing`, so check `docker compose`
and the worker logs before assuming the frontend is broken.

### Getting a test destination account

The playground needs a funded Stellar testnet address to deliver XLM to.
Create one at <https://laboratory.stellar.org> (account creator, testnet),
then fund it once at `https://friendbot.stellar.org/?addr=<G...>`. Any funded
testnet account works; mainnet addresses are rejected by design.

### `AUTH_SUCCESS_REDIRECT_URL`

When you run the backend locally, set `AUTH_SUCCESS_REDIRECT_URL` in `.env`
to your frontend's post-login route (e.g. `http://localhost:3001/`). Google
sign-in redirects there after the backend sets the session cookie, so mount
your logged-in route at exactly that path. `AUTH_EMAIL_LINK_BASE_URL` (same
file) builds the verification and reset links; point it at the same frontend
origin.

## 3. Authentication model (self-hosted email + Google)

The frontend owns the auth forms. Email signup and login are plain JSON
calls; Google sign-in stays a redirect. The backend owns opaque sessions and
provider tokens never reach the browser.

- Session cookie: `kailopay_session` (name is configurable; default shown).
  It is `HttpOnly` and `SameSite=Lax`, so JavaScript cannot read it; the
  browser sends it automatically on same-origin requests. Nothing to store in
  the frontend.
- Email login: `POST /auth/register` then verify, then `POST /auth/login`
  sets the cookie and returns the user. No redirects involved.
- Google login: `window.location.href = GET /auth/google/login` (full
  navigation, not fetch). After Google, the backend sets the cookie and 302s
  to `AUTH_SUCCESS_REDIRECT_URL`; mount a route there. If the deployment has
  no Google credentials the endpoint returns 503, so offer the button only
  when it works (try it once, or feature-flag it).
- Verification and reset emails: in sandbox, links are logged to the backend
  console instead of being emailed. The links point at
  `{AUTH_EMAIL_LINK_BASE_URL}/auth/verify-email?token=...` and
  `/auth/reset-password?token=...`; build those routes, extract the token,
  and POST it to the API.

### Auth endpoints

| Method & path | Auth | Purpose |
|---|---|---|
| `POST /auth/register` | none | Create account; password 10-128 chars; 409 if email taken |
| `POST /auth/login` | none | Verify credentials; 401 invalid/locked, 403 unverified |
| `GET /auth/google/login` | none | 302 to Google (503 when not configured) |
| `GET /auth/google/callback` | none | Google redirect target; sets cookie, 302 to success URL |
| `POST /auth/logout` | session cookie | End the session |
| `POST /auth/email/verify` | none | Body `{"token"}`; 200 with user, 400 when used/expired |
| `POST /auth/email/resend` | none | Body `{"email"}`; always 202 |
| `POST /auth/password/forgot` | none | Body `{"email"}`; always 202 |
| `POST /auth/password/reset` | none | Body `{"token","new_password"}`; 204, revokes sessions |
| `POST /auth/password/change` | session cookie | Body `{"current_password","new_password"}`; 204 |
| `GET /auth/me` | session cookie | Current user profile |
| `PATCH /auth/me` | session cookie | Update `display_name` and/or `developer_enabled` |
| `PUT /auth/me/avatar` | session cookie | Multipart upload, field name `avatar` |
| `GET /auth/me/avatar` | session cookie | Streams the private image (respect `ETag`/`Cache-Control`) |
| `DELETE /auth/me/avatar` | session cookie | Remove avatar |

`GET /auth/me` returns `{"user": { "id", "display_name", "email",
"email_verified", "developer_enabled", "avatar_url?" }}`.

Auth errors use a simpler envelope than the order API:
`{"error": "<message>"}` with 401/400/403/409. Treat 401 as "not signed in"
and redirect to login; treat 403 "email is not verified" as a prompt to open
the verification link.

## 4. Developer Mode and API keys (build this now)

This is the part of the product a frontend can fully build today: the
developer section.

1. User signs in (above).
2. `PATCH /auth/me` with `{"developer_enabled": true}` opts into Developer
   Mode (product decision: one opt-in flag per user).
3. With the session cookie, manage API keys:

| Method & path | Purpose |
|---|---|
| `POST /v1/api-keys` body `{"name": "..."}` | Creates the user's default test API client + one key |
| `GET /v1/api-keys` | Lists key metadata (never full keys) |
| `DELETE /v1/api-keys/{id}` | Revokes a key |

Key facts for the UI:

- `POST /v1/api-keys` returns the full plaintext key exactly once:
  `{"id", "client_id", "prefix", "key", "created_at"}`. Show it on a
  "copy now, we cannot show it again" screen. The server stores only a hash.
- Key format is `pk_test_<public_id>_<secret>`.
- Key management requires Developer Mode enabled (403 otherwise).
- Revoking keys does not disable Developer Mode, and vice versa.

## 5. The on-ramp order API

One scoping fact decides most of your architecture. Order endpoints
authenticate with a `pk_test_` API key (`Authorization: Bearer pk_test_...`),
and orders are owned by the API client that created them. Retail-session order
creation does not exist yet. So today the frontend can exercise the full order
flow in two honest ways:

- a **developer playground** where the user pastes their own `pk_test_` key at
  runtime (never bundle or embed a key in shipped code; FR-069), or
- your own small server-side BFF that holds a test key (server-side only).

A retail-session buy flow (no key handling in the browser at all) is planned
backend work; track `documentations/backend/BACKEND-BACKLOG.md` before
assuming it exists.

### `POST /v1/onramps` (requires `Idempotency-Key` header)

Request body:

```json
{
  "fiat": { "currency": "IDR", "amount_minor": "100000" },
  "payment_method": "qris",
  "stellar_destination": { "account": "G...", "memo": null }
}
```

Rules the frontend must enforce client-side (the server re-validates):

- `amount_minor` is a **decimal string** of IDR minor units (1 rupiah =
  1 unit). Never send a JS number; format from user input to a string.
- `payment_method` ∈ `qris` | `bri_va`.
- `stellar_destination.account` is a 56-char `G...` base32 testnet address;
  validate the shape before sending.
- `memo` optional, max 28 chars.
- `Idempotency-Key` header is required (any unique string, ≤255 chars).
  Generate one per user intent (e.g. UUID) and **reuse the same key on retry
  of the same request**; changing the body under the same key returns
  `409 IDEMPOTENCY_KEY_REUSED`.
- Amount bounds default to 10,000 to 10,000,000 IDR, but the server sets
  them in config. Rely on the error, not hardcoded limits.

Responses:

- `201`: order created. Body: `{"order": {...}}` (shape below).
- `200`: idempotent replay of the same key+body; treat like 201.
- `202 CHECKOUT_PENDING_RECONCILIATION`: the provider call outcome is unknown
  and the order is held. Show a neutral "processing, check back"
  state; do not let the user retry-create. The order exists, and re-POSTing
  the same key+body returns the current order.
- `400` `INVALID_REQUEST` / `INVALID_STELLAR_ACCOUNT`, `401`,
  `409` `IDEMPOTENCY_KEY_REUSED` / `INSUFFICIENT_LIQUIDITY`,
  `422` `AMOUNT_OUT_OF_RANGE`, `415` non-JSON content type,
  `503` `QUOTE_UNAVAILABLE` / `EXTERNAL_SERVICE_UNAVAILABLE`.

Error envelope for all `/v1` order endpoints:

```json
{ "error": { "code": "AMOUNT_OUT_OF_RANGE", "message": "..." }, "request_id": "..." }
```

Surface `request_id` when offering "contact/support" copy. The response also
echoes `X-Request-ID` as a header.

### Order response shape

```json
{
  "order": {
    "id": "uuid",
    "status": "payment_pending",
    "environment": "sandbox",
    "network": "stellar_testnet",
    "fiat": { "currency": "IDR", "amount_minor": "100000" },
    "asset": { "code": "XLM", "amount": "40.0000000" },
    "quote": { "rate": "...", "adjusted_rate": "...", "spread_bps": 0,
               "source_at": "...", "expires_at": "..." },
    "payment_method": "qris",
    "stellar_destination": { "account": "G...", "memo": null },
    "checkout": {
      "id": "pr-...",
      "status": "REQUIRES_ACTION",
      "presentation_type": "QR_STRING",
      "presentation_value": "000201...",
      "expires_at": "..."
    },
    "stellar_transaction_hash": "only-when-confirmed",
    "failure_code": "only-when-failed",
    "created_at": "...", "updated_at": "..."
  }
}
```

- `asset.amount` is a decimal string with exactly 7 fraction digits (stroops).
  Parse with a decimal library or keep as string; never `parseFloat`.
- Checkout presentation:
  - `presentation_type: "QR_STRING"` (QRIS): render `presentation_value`
    as a QR code.
  - `presentation_type: "VIRTUAL_ACCOUNT_NUMBER"` (`bri_va`): display the
    VA number for manual bank transfer, with copy-to-clipboard.
- Quote is locked at creation (immutable); `quote.expires_at` bounds the
  payment window. Use it for a countdown, and stop accepting payment after
  expiry (the backend expires the order and releases the XLM reservation
  automatically).

### Reading orders

- `GET /v1/orders/{id}` returns 404 for missing and cross-client orders
  alike (same error; never assume existence).
- `GET /v1/orders?limit=1..100&cursor=` returns
  `{"orders": [...], "next_cursor": ""}`; `next_cursor` empty means no more
  pages. Cursor is opaque (an order id); don't parse it.

### Order status → UI mapping

| Status | Meaning | Suggested UI |
|---|---|---|
| `created` | Order persisted, checkout being established (or held in unknown-checkout reconciliation) | Brief "preparing" state |
| `payment_pending` | Checkout ready | Show QR/VA instructions + quote countdown |
| `payment_confirmed` → `stellar_processing` | Paid; XLM transfer in flight (usually brief) | "Sending your XLM…" spinner |
| `completed` | Done | Success + link `https://stellar.expert/lumen/testnet/tx/<stellar_transaction_hash>` |
| `expired` | Unpaid past expiry | Explain + offer new order |
| `payment_failed` | Checkout permanently rejected (see `failure_code`) | Error + retry as new order |
| `stellar_failed` | Transfer permanently failed after payment | "Paid. Support will resolve this" with order id and request ids; do not offer re-payment |

Transitions are one-way; there is no cancel endpoint yet. Polling `GET
/v1/orders/{id}` is the intended integration (webhooks come later); poll
gently, e.g. every 3-5 seconds while `payment_pending`, and back off after.

## 6. Health endpoints (useful for dev tooling)

`GET /livez`, `/readyz`, `/startupz` (plus legacy aliases `/health`,
`/healthz`, `/ready`). They are outside the OpenAPI file by design.

## 7. Not implemented yet

Do not build against any of these:

- Retail-session order creation/reading (orders are API-key-scoped only).
- ~~Off-ramp / sell flow~~ now implemented: `POST /v1/offramps` creates sell orders with deposit instructions and a simulated payout (`payout` object on the order, always `simulated:true`). A dedicated guide section will follow.
- Outgoing developer webhooks (event subscription UI has no backend yet).
- SEP-24 interactive flows, `stellar.toml`, federation.
- Quote preview endpoint (`GET` quote). Quotes are only produced inline by
  `POST /v1/onramps`; if you need "see rate before commit", that is a backend
  gap to request, not an existing endpoint.
- Rate limiting (429), cancel/refund, order filtering/search.
- CORS (see §2; use a proxy).

## 8. Non-negotiable frontend rules

1. Money and XLM amounts cross the wire as **strings**; keep them strings
   (or use a decimal library). No floating-point arithmetic on amounts, ever.
2. JSON bodies reject unknown fields with a `400`. Send only the documented
   fields and no client-only metadata.
3. Every value-movement screen shows `Sandbox` + `Stellar Testnet` badges.
4. Never embed, bundle, or log a `pk_test_` key. If the playground accepts a
  key, keep it in memory only and wipe it on logout.
5. Reuse the same `Idempotency-Key` for retries of the same intent; generate a
  new one for a new intent.
6. Treat the API as at-least-once: a 200 replay response is normal, not an
  error.
6. Do not expose raw error text from network failures; use the stable `code`
   and keep `request_id` available for support.

## 9. Screen map

The minimum app skeleton for Weeks 1-2 (routes are suggestions; the shapes are
the contract):

| Route (suggested) | Auth | Content |
|---|---|---|
| `/` | public | Landing: what KailoPay is, sandbox/testnet disclaimers, "Sign in" |
| `/login`, `/register` | public | Email login and signup forms (`POST /auth/login`, `POST /auth/register`); Google button links to `/auth/google/login` |
| `/auth/google/callback` | public | Google post-login landing target (set as `AUTH_SUCCESS_REDIRECT_URL`) |
| `/auth/verify-email` | public | Reads `?token=` from the console-emailed link, POSTs `/auth/email/verify` |
| `/auth/reset-password` | public | Reads `?token=`, collects a new password, POSTs `/auth/password/reset` |
| `/profile` | session | `GET/PATCH /auth/me` (display name, Developer Mode toggle), avatar upload/remove |
| `/developer` | session + Developer Mode | API key list/create/revoke, one-time key reveal, playground entry |
| `/developer/playground` | session + Developer Mode | Paste-your-own `pk_test_` key (memory only) → create order, show QRIS QR / BRI VA, poll status |
| `/developer/orders/:id` | session + Developer Mode (key) | Order detail: quote, checkout, status timeline, testnet explorer link on completion |
| `/auth/expired`, error states | public | Session-expired / generic error routes |

Status timeline per order: `created → payment_pending → stellar_processing →
completed` with terminal `expired` / `payment_failed` / `stellar_failed` states
mapped as in §5.

## 10. Suggested build order

1. App shell + auth screens: register/login forms, Google button,
   verify-email and reset-password token routes, `GET /auth/me`, logout,
   profile edit, and avatar upload.
2. Developer section: opt-in Developer Mode, API key create/list/revoke with
   one-time display.
3. Developer playground (paste-your-own-key, in-memory only): create on-ramp
   order → render QRIS QR / BRI VA → poll order status → success screen with
   the testnet explorer link.
4. Polish: error states per §5, sandbox labels, amount input that formats
   IDR with thousand separators and serializes as a minor-unit string.

The product-level acceptance criteria for the web app live in
`documentations/PRD.md` (§7 journeys, §8.6) and the phase plan in
`documentations/PHASES.md`.

## 11. Appendix: full request/response catalog

Everything below is verified against the current handlers. All responses echo
`X-Request-ID`. Global rules:

- JSON bodies reject unknown fields (`400`). Send exactly the documented
  fields and nothing else.
- All JSON object keys use snake_case on every endpoint (`amount_minor`,
  `display_name`, `api_key`, `stellar_transaction_hash`, `request_id`).
  Headers follow HTTP header style (`Idempotency-Key`, `X-Request-ID`).
  Copy keys exactly as shown in the examples.
- Body size limits: on-ramp create 16 KiB; auth/profile/API-key bodies 4 KiB;
  avatar upload 5 MiB default.
- There are currently two error envelope styles; the table at the end of this
  appendix shows both. Order endpoints use the rich envelope; auth and
  API-key endpoints use the simple one.

### Health

```text
GET /livez    -> 200 {"status":"ok"}
GET /readyz   -> 200 {"status":"ready"}          | 503 {"status":"not_ready"}
GET /startupz -> 200 {"status":"started"}        | 503 {"status":"starting"}
```

(Aliases `/health`, `/healthz`, `/ready` behave like liveness/readiness. All
send `Cache-Control: no-store`.)

### Auth

```text
POST /auth/register
  body  {"email":"user@example.com","password":"super-secret-1","display_name":"Febry"}
  -> 201 {"user":{"id","display_name","email","email_verified":false,...}}
     | 400 | 409 {"error":"email is already registered"}

POST /auth/login
  body  {"email":"user@example.com","password":"super-secret-1"}
  -> 200 {"user":{...}}
     Set-Cookie: kailopay_session=<opaque>; Path=/; HttpOnly; SameSite=Lax
     | 401 {"error":"invalid email or password"}   (also when locked)
     | 403 {"error":"email is not verified"}

GET /auth/google/login
  -> 302 Location: <Google>  | 503 {"error":"google sign-in is not configured"}

GET /auth/google/callback?code=...&state=...
  -> 302 Location: <AUTH_SUCCESS_REDIRECT_URL>
     Set-Cookie: kailopay_session=<opaque>; Path=/; HttpOnly; SameSite=Lax
     | 400 {"error":"authentication failed"}

POST /auth/logout            (session cookie)
  -> 204 (clears the cookie) | 500

POST /auth/email/verify      body  {"token":"<from console link>"}
  -> 200 {"user":{...}}  | 400 {"error":"invalid or expired token"}

POST /auth/email/resend      body  {"email":"user@example.com"}
  -> 202 (always) | 400

POST /auth/password/forgot   body  {"email":"user@example.com"}
  -> 202 (always; never reveals account existence) | 400

POST /auth/password/reset    body  {"token":"...","new_password":"fresh-password-1"}
  -> 204 (revokes all sessions) | 400

POST /auth/password/change   (session cookie)
  body  {"current_password":"...","new_password":"fresh-password-1"}
  -> 204 (clears cookie; sign in again) | 400 | 401
```

The sandbox backend logs verification/reset links to its console as
`email verification link ... link=http://localhost:3001/auth/verify-email?token=...`.
During local development, copy the token from that log line.

```text
GET /auth/me                 (session cookie)
  -> 200
{
  "user": {
    "id": "0a1f9c2e-...",
    "display_name": "Febry",
    "email": "user@example.com",
    "email_verified": true,
    "developer_enabled": false
  }
}
  | 401 {"error":"authentication failed"}
```

```text
PATCH /auth/me               (session cookie; both fields optional)
  body  {"display_name":"Febry","developer_enabled":true}
  -> 200 {"user":{ ...same shape as GET /auth/me... }}
  | 400 | 401 | 500
```

```text
PUT /auth/me/avatar          (session cookie; multipart/form-data, field "avatar")
  -> 200 {"user":{...}}      | 400 (not an image) | 401 | 413 (too large) | 500

GET /auth/me/avatar          (session cookie)
  -> 200 raw image bytes; Content-Type + ETag + Cache-Control: private, max-age=300
  | 401 | 404 (no avatar yet)

DELETE /auth/me/avatar       (session cookie)
  -> 200 {"user":{...}}      | 401 | 500
```

Avatar content type is detected server-side from the bytes. Send image/png,
image/jpeg, or image/gif; do not trust your own Content-Type header.

### API keys (session cookie + Developer Mode)

```text
POST /v1/api-keys
  body  {"name":"Playground key"}
  -> 201
{
  "api_key": {
    "id": "3f2a...",
    "client_id": "9c1d...",
    "prefix": "pk_test_ab12cd34ef56...wxyz",
    "key": "pk_test_ab12cd34ef567890<b64-secret>",   // shown EXACTLY ONCE
    "created_at": "2026-08-22T10:00:00Z"
  }
}
  | 400 (bad/missing name, >100 chars) | 401
  | 403 {"error":"Developer Mode is required"} | 500
```

```text
GET /v1/api-keys
  -> 200
{
  "api_keys": [
    {
      "id": "3f2a...",
      "client_id": "9c1d...",
      "name": "Playground key",
      "prefix": "pk_test_ab12cd34ef56...wxyz",
      "created_at": "2026-08-22T10:00:00Z",
      "last_used_at": "2026-08-22T11:30:00Z",     // omitted when never used
      "revoked_at": null                          // present after revoke
    }
  ]
}
```

```text
DELETE /v1/api-keys/{id}
  -> 204 | 401 | 404 | 500
```

### On-ramp and orders (`Authorization: Bearer pk_test_...`)

```text
POST /v1/onramps
  headers: Idempotency-Key: <uuid>, Content-Type: application/json
  body (QRIS):
{
  "fiat": { "currency": "IDR", "amount_minor": "100000" },
  "payment_method": "qris",
  "stellar_destination": {
    "account": "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
    "memo": null
  }
}
  body (BRI VA): identical but "payment_method": "bri_va"

  -> 201 (new) | 200 (idempotent replay)
{
  "order": {
    "id": "0f8e2c11-7b3a-4d1e-9f2b-3c4d5e6f7a80",
    "status": "payment_pending",
    "environment": "sandbox",
    "network": "stellar_testnet",
    "fiat": { "currency": "IDR", "amount_minor": "100000" },
    "asset": { "code": "XLM", "amount": "40.0000000" },
    "quote": {
      "rate": "2500",
      "adjusted_rate": "2500",
      "spread_bps": 0,
      "source_at": "2026-08-22T09:58:12Z",
      "expires_at": "2026-08-22T10:03:12Z"
    },
    "payment_method": "qris",
    "stellar_destination": { "account": "GAAAA...", "memo": null },
    "checkout": {
      "id": "pr_9f8e7d6c-...",
      "status": "REQUIRES_ACTION",
      "presentation_type": "QR_STRING",
      "presentation_value": "00020101021226600...5802ID59...",
      "expires_at": "2026-08-22T10:03:12Z"
    },
    "created_at": "2026-08-22T09:58:12Z",
    "updated_at": "2026-08-22T09:58:13Z"
  }
}

  error (rich envelope):
{
  "error": { "code": "AMOUNT_OUT_OF_RANGE", "message": "The requested amount is outside the supported range." },
  "request_id": "c2f0..."
}
```

For `bri_va`, `checkout.presentation_type` is `"VIRTUAL_ACCOUNT_NUMBER"` and
`presentation_value` is the VA number to display with copy-to-clipboard. On
completion the order also carries
`"stellar_transaction_hash": "<hex>"`; on failure `"failure_code": "..."`.

```text
GET /v1/orders/{id}
  -> 200 {"order":{ ...same shape... }} | 401 | 404 {"error":{"code":"ORDER_NOT_FOUND",...},"request_id":...}

GET /v1/orders?limit=20&cursor=<order-id>
  -> 200
{
  "orders": [ { ...order shapes as above... } ],
  "next_cursor": ""        // empty string = last page
}
  | 401
```

### Error envelopes at a glance

| Endpoints | Style | Example |
|---|---|---|
| `POST /v1/onramps`, `GET /v1/orders*` | `{"error":{"code","message"},"request_id"}` | `{"error":{"code":"QUOTE_UNAVAILABLE","message":"A fresh quote is currently unavailable; retry shortly."},"request_id":"c2f0..."}` |
| `/auth/*`, `/v1/api-keys*` | `{"error":"<message>"}` | `{"error":"authentication failed"}` (401/503), `{"error":"Developer Mode is required"}` (403), `{"error":"Bad Request"}` (400, plain HTTP status text) |

Handle both styles in your API client: branch on whether `error` is a string
or an object.
