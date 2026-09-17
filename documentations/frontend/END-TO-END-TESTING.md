# Frontend end-to-end testing

Use this guide to test the current KailoPay sandbox from account creation
through KYC, API keys, orders, callbacks, and Stellar testnet settlement.

The guide covers the implemented v0.1.0 flow. It does not cover production
payments, production KYC, Stellar mainnet, or real IDR payouts.

The checked-in API contract is [openapi/openapi.yaml](../../openapi/openapi.yaml).
Use [FRONTEND-GUIDE.md](FRONTEND-GUIDE.md) for field details and
[PERSONA-KYC-RUNBOOK.md](../backend/PERSONA-KYC-RUNBOOK.md) for Persona
operations.

## Understand the KYC approval rule

Completing the Persona widget does not approve the user by itself.

The approval path is:

~~~text
Persona widget
  -> Persona sends a signed webhook
  -> KailoPay verifies the webhook signature
  -> KailoPay stores the provider event and updates the local status
  -> the frontend refreshes GET /v1/kyc
  -> status becomes approved
~~~

The browser onComplete callback only tells the frontend to refresh. The
backend trusts the verified Persona webhook and the persisted local status.

The usual status sequence is:

~~~text
created -> pending -> completed -> pending_review -> approved
~~~

Persona may skip a status or send a transition event. The exact status names
are created, pending, completed, pending_review, approved, declined, failed,
and expired.

Only approved unlocks API-key creation and order creation. A completed or
pending_review inquiry is not approved yet.

## 1. Prepare the local services

Use these local URLs unless you intentionally chose different ports:

| Service | URL |
|---|---|
| Backend API | http://localhost:8080 |
| Backend Swagger UI | http://localhost:8080/docs/ |
| Frontend | http://localhost:3001 |
| PostgreSQL | localhost:5432 |
| MinIO API | localhost:9000 |

The frontend must proxy API requests through its own origin because the
backend does not send CORS headers. Create kailopay-fe/.env.local with:

~~~env
API_ORIGIN=http://localhost:8080
~~~

Set these backend values for a local frontend on port 3001:

~~~env
HTTP_ADDRESS=:8080
HTTP_ALLOWED_ORIGINS=http://localhost:3001
AUTH_SUCCESS_REDIRECT_URL=http://localhost:3001/
AUTH_EMAIL_LINK_BASE_URL=http://localhost:3001/
~~~

Keep the Persona API key, Persona webhook secret, Xendit secret, callback
token, and Stellar private keys in the backend environment only. Do not put
them in frontend environment variables.

The Persona API key, inquiry template, and environment must belong to the same
Persona environment. A mismatch returns 403 Forbidden when the backend
creates an inquiry.

Start the required local services in separate terminals:

~~~powershell
docker compose up -d postgres minio
go run ./cmd/migrate
go run ./cmd/api
~~~

Start the worker when testing Stellar settlement or off-ramp processing:

~~~powershell
go run ./cmd/worker
~~~

Start the frontend from kailopay-fe:

~~~powershell
npm install
npm run dev
~~~

The worker needs STELLAR_TREASURY_SECRET and OFFRAMP_DEPOSIT_SECRET. Do not
start it with placeholder secrets.

## 2. Make provider callbacks reachable

Persona and Xendit cannot call localhost. Expose the backend through an HTTPS
development tunnel:

~~~text
https://<tunnel-host>/callbacks/kyc/persona
https://<tunnel-host>/callbacks/payments/xendit
~~~

Register the first URL in Persona and the second URL in the Xendit sandbox.
Use one tunnel for both paths when that is convenient.

Subscribe the Persona webhook to inquiry creation, start, completion, review,
approval, decline, failure, expiry, and transition events.

Use the real Persona sandbox flow or a provider-generated test event for the
frontend end-to-end test. The browser callback only refreshes KYC state. It does
not approve the user and it must not contain a Persona webhook secret.

### Sign a callback for a backend-only Postman test

Use this test to debug the callback handler. Do not use it as the frontend
approval flow.

Persona and the backend keep the same webhook signing secret. Persona does not
send the raw secret in the request. The request contains a derived HMAC value:

~~~text
Persona-Signature: t=<current Unix seconds>,v1=<64-character HMAC hex value>
~~~

The `PERSONA_WEBHOOK_SECRET` value must not appear in the request headers, the
frontend environment, or committed Postman collections. Add it to a local
Postman environment only when you need to generate a manual backend callback.
Use the signing secret for the exact Persona webhook endpoint. Do not use the
Persona API key.

Create this local Postman variable:

~~~text
persona_webhook_secret=<Persona webhook signing secret>
~~~

Set the request body to `raw` and `JSON`. Remove any manually added
`Persona-Signature` header. Add this pre-request script:

~~~javascript
const CryptoJS = require("crypto-js");

let secret = String(
  pm.environment.get("persona_webhook_secret") || ""
).trim();

if (!secret) {
  throw new Error("persona_webhook_secret is empty");
}

if (
  secret.length >= 2 &&
  ((secret.startsWith('"') && secret.endsWith('"')) ||
    (secret.startsWith("'") && secret.endsWith("'")))
) {
  secret = secret.slice(1, -1);
}

if (!pm.request.body || pm.request.body.mode !== "raw") {
  throw new Error("Request body must use raw JSON mode");
}

const body = pm.variables.replaceIn(pm.request.body.raw || "");

pm.request.body.update({
  mode: "raw",
  raw: body,
});

const timestamp = Math.floor(Date.now() / 1000).toString();
const digest = CryptoJS.HmacSHA256(
  `${timestamp}.${body}`,
  secret
).toString(CryptoJS.enc.Hex);

pm.request.headers.remove("Persona-Signature");
pm.request.headers.upsert({
  key: "Persona-Signature",
  value: `t=${timestamp},v1=${digest}`,
});

pm.request.headers.upsert({
  key: "Content-Type",
  value: "application/json",
});

console.log({
  secretLength: secret.length,
  bodyLength: body.length,
  timestamp,
  signatureLength: digest.length,
});
~~~

The `created-at` value inside the JSON body is a provider event timestamp,
such as `2026-09-15T00:00:00Z`. The `t` value in `Persona-Signature` is a
different value. It must contain the current Unix timestamp in seconds.

The backend verifies the HMAC against the exact raw body. Do not change
whitespace, add a trailing space, or edit a variable after the pre-request
script runs.

Expected callback results:

| Response | Meaning |
|---|---|
| `200` with `status=accepted` | The signature is valid and the inquiry is known. |
| `503` with `error.code=KYC_INQUIRY_NOT_FOUND` | The signature is valid, but the inquiry ID is unknown. This is a useful signature test. |
| `401` with `error.code=INVALID_CALLBACK_SIGNATURE` | The secret, header timestamp, signature, or exact body bytes do not match. |

Use a real inquiry ID for a successful processing test. Never put the secret in
frontend code or send it as `Persona-Signature`.

## 3. Create a Postman environment

Create these variables:

| Variable | Example |
|---|---|
| base_url | http://localhost:8080 |
| frontend_url | http://localhost:3001 |
| test_email | a unique test email |
| test_password | a password with at least 10 characters |
| api_key | set once after API-key creation |
| onramp_order_id | set from the on-ramp response |
| offramp_order_id | set from the off-ramp response |
| sep24_transaction_id | set from the SEP-24 response |

Let Postman manage the kailopay_session cookie for localhost. Do not copy the
session cookie into a committed collection.

Use a new test email for a new account. Reuse the same account for the
success-path sequence after it becomes KYC-approved.

## 4. Run health checks

Run these requests before testing the frontend:

~~~http
GET {{base_url}}/livez
GET {{base_url}}/readyz
GET {{base_url}}/startupz
GET {{base_url}}/docs/
~~~

Expect 200 from the health endpoints. The Swagger UI should load from
/docs/.

If /readyz fails, fix PostgreSQL or MinIO before testing the frontend.

## 5. Test registration and sessions

### Register an account

~~~http
POST {{base_url}}/auth/register
Content-Type: application/json
~~~

~~~json
{
  "email": "{{test_email}}",
  "password": "{{test_password}}",
  "display_name": "E2E Tester"
}
~~~

Expect 201.

The default local email sender prints the verification link and token in the
backend terminal. Copy the token into the frontend route:

~~~text
http://localhost:3001/auth/verify-email?token=<token>
~~~

You can also verify with Postman:

~~~http
POST {{base_url}}/auth/email/verify
Content-Type: application/json
~~~

~~~json
{
  "token": "<token>"
}
~~~

Expect 200.

### Log in

~~~http
POST {{base_url}}/auth/login
Content-Type: application/json
~~~

~~~json
{
  "email": "{{test_email}}",
  "password": "{{test_password}}"
}
~~~

Expect 200 and a kailopay_session cookie.

Check the session:

~~~http
GET {{base_url}}/auth/me
~~~

Expect 200 with email_verified: true.

The frontend flow is:

1. Open /register.
2. Register the account.
3. Open the verification link from the backend terminal.
4. Open /login.
5. Confirm that the dashboard loads after login.

### Test profile changes

Update the display name:

~~~http
PATCH {{base_url}}/auth/me
Content-Type: application/json
~~~

~~~json
{
  "display_name": "Updated E2E Tester"
}
~~~

Enable Developer Mode:

~~~http
PATCH {{base_url}}/auth/me
Content-Type: application/json
~~~

~~~json
{
  "developer_enabled": true
}
~~~

Test the frontend /profile page for:

- display-name update;
- Developer Mode enable and disable;
- avatar upload with a JPEG, PNG, or WebP image;
- avatar display through GET /auth/me/avatar;
- avatar removal.

### Test logout and invalid sessions

~~~http
POST {{base_url}}/auth/logout
~~~

Expect 204. A subsequent GET /auth/me must return 401.

Also check these cases:

| Case | Expected result |
|---|---|
| Login before email verification | 403 |
| Wrong password | 401 |
| Duplicate email registration | 409 |
| Password shorter than 10 characters | 400 |
| Missing session cookie on /auth/me | 401 |

Use a separate test account for password reset and password-change tests.
Those flows revoke existing sessions.

### Test password recovery

Request a verification email again:

~~~http
POST {{base_url}}/auth/email/resend
Content-Type: application/json
~~~

~~~json
{
  "email": "{{test_email}}"
}
~~~

Expect 202. The response must not reveal whether the account exists.

Request a password reset:

~~~http
POST {{base_url}}/auth/password/forgot
Content-Type: application/json
~~~

~~~json
{
  "email": "{{test_email}}"
}
~~~

Expect 202. Copy the reset token from the backend terminal and submit it:

~~~http
POST {{base_url}}/auth/password/reset
Content-Type: application/json
~~~

~~~json
{
  "token": "<reset-token>",
  "new_password": "new-e2e-password"
}
~~~

Expect 204. Log in with the new password. The previous session must no longer
work.

For an in-session password change, call POST /auth/password/change with the
current session:

~~~http
POST {{base_url}}/auth/password/change
Content-Type: application/json
~~~

~~~json
{
  "current_password": "new-e2e-password",
  "new_password": "another-e2e-password"
}
~~~

Expect 204 and log in again. If Google credentials are not configured, the
Google login endpoint may return 503. Treat that result as the expected
disabled-provider behavior.

## 6. Complete Persona KYC

Log in with the verified account before starting this section.

Read the initial status:

~~~http
GET {{base_url}}/v1/kyc
~~~

Expect 200 with status: not_started for a new account.

Create or resume the inquiry:

~~~http
POST {{base_url}}/v1/kyc/inquiry
~~~

Expect 201 with a direct Persona hosted-flow URL, inquiry ID, and environment ID:

~~~json
{
  "inquiry": {
    "status": "created",
    "provider_status": "created",
    "inquiry_id": "inq_...",
    "url": "https://inquiry.withpersona.com/verify?inquiry-id=inq_...",
    "environment_id": "env_...",
    "expires_at": null
  }
}
~~~

The initial response may not contain session_token. A later call resumes an
existing inquiry and may return a short-lived token.

Open the frontend route:

~~~text
http://localhost:3001/verify-identity
~~~

Click Start verification and complete the Persona sandbox flow with test
data. The frontend may open the returned `url` directly. If it uses the
embedded Persona client instead, pass the inquiry ID, environment ID, and
optional session token to the Persona client.

After Persona finishes, wait for the signed callback. Then call:

~~~http
GET {{base_url}}/v1/kyc
~~~

Expect the status to move through the provider result and eventually become
approved. The frontend must not display the account as approved based only
on the Persona widget callback.

Check the callback response in the tunnel or backend logs:

| Callback result | Meaning |
|---|---|
| 200 {"status":"accepted"} | Event was authenticated and stored |
| 401 with `error.code=INVALID_CALLBACK_SIGNATURE` | Signature, timestamp, or signed payload failed; in local mode, `error.details` identifies the safe validation reason |
| 400 with `error.code=CALLBACK_EVENT_CONFLICT` | Conflicting event; no second state change |
| 503 with `error.code=KYC_INQUIRY_NOT_FOUND` | Inquiry is not attached locally yet; Persona should retry |
| 503 with `error.code=CALLBACK_PROCESSING_FAILED` | Temporary processing failure; Persona should retry |

Before approval, enable Developer Mode first. Then verify that these requests
return 403 with error.code: KYC_REQUIRED:

~~~http
PATCH {{base_url}}/auth/me
Content-Type: application/json

POST {{base_url}}/v1/api-keys
POST {{base_url}}/v1/onramps
POST {{base_url}}/v1/offramps
~~~

`POST /v1/quotes` is public and should remain available before KYC approval;
it only previews a quote and does not create an order.

Use this body for the PATCH request:

~~~json
{
  "developer_enabled": true
}
~~~

After approval, repeat the API-key request. It must succeed.

## 7. Test Developer Mode and API keys

Developer Mode requires the authenticated session:

~~~http
PATCH {{base_url}}/auth/me
Content-Type: application/json
~~~

~~~json
{
  "developer_enabled": true
}
~~~

Create a key after KYC approval:

~~~http
POST {{base_url}}/v1/api-keys
Content-Type: application/json
~~~

~~~json
{
  "name": "postman-e2e"
}
~~~

Expect 201. The plaintext key appears once in the response. Store it only in
the local Postman environment as api_key.

List key metadata:

~~~http
GET {{base_url}}/v1/api-keys
~~~

The list must not contain the full plaintext key.

Revoke the key:

~~~http
DELETE {{base_url}}/v1/api-keys/<key-id>
~~~

Expect 204. A request using the revoked key must return 401.

Test the frontend /developer page for key creation, one-time display, list,
and revoke. The frontend must never bundle or log the key.

## 8. Test the consumer buy flow

The browser buy flow uses the verified session cookie. The API-key flow uses
the pk_test_ key from the previous section.

### Browser flow

1. Open /buy.
2. Enter an IDR amount within the configured sandbox limits.
3. Enter a funded Stellar testnet destination beginning with G.
4. Submit the order.
5. Confirm that the frontend redirects to the Xendit sandbox checkout URL.
6. Complete or simulate the Xendit sandbox payment.
7. Return to the frontend order page.
8. Poll the order until it reaches a terminal state.

The expected success path is:

~~~text
payment_pending -> payment_confirmed -> stellar_processing -> completed
~~~

The worker must be running for the final Stellar transfer. On completion,
check stellar_transaction_hash and open the Stellar testnet transaction:

~~~text
https://stellar.expert/explorer/testnet/tx/<stellar_transaction_hash>
~~~

The response must contain:

- environment: sandbox;
- network: stellar_testnet;
- IDR amounts as decimal strings;
- XLM amounts as strings with seven fractional digits;
- a checkout payment_link_url or compatible presentation value.

### Postman API-key flow

~~~http
POST {{base_url}}/v1/onramps
Authorization: Bearer {{api_key}}
Content-Type: application/json
Idempotency-Key: e2e-onramp-001
~~~

~~~json
{
  "fiat": {
    "currency": "IDR",
    "amount_minor": "100000"
  },
  "payment_method": "xendit",
  "stellar_destination": {
    "account": "GXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
    "memo": null
  }
}
~~~

Replace the example Stellar address with a real testnet address. Expect 201
for a new order or 200 for a replay.

Open the returned checkout URL in a browser. Do not mark the order paid when
the user returns from the checkout page. Wait for the verified Xendit callback.

### Test idempotency

Send the exact same request again with e2e-onramp-001. Expect 200 and the
same order ID.

Change the amount while keeping e2e-onramp-001. Expect 409 with
IDEMPOTENCY_KEY_REUSED. Use a new key for a new purchase intent.

### Test order reads

~~~http
GET {{base_url}}/v1/orders/{{onramp_order_id}}
Authorization: Bearer {{api_key}}

GET {{base_url}}/v1/orders?limit=20
Authorization: Bearer {{api_key}}
~~~

The list and detail responses must contain only orders owned by the API
client. The frontend /developer/orders pages must show the same order.

## 9. Test the consumer sell flow

The sell flow creates an off-ramp order and returns Stellar deposit
instructions. It does not move real IDR.

### Browser flow

1. Open /sell.
2. Enter an exact XLM amount such as 1.0000000.
3. Enter a synthetic sandbox payout reference.
4. Submit the order.
5. Copy the returned deposit account and memo.
6. Send the exact XLM amount to the deposit account on Stellar testnet.
7. Include the required memo.
8. Keep the worker running while it scans the deposit account.
9. Refresh the order until the simulated payout completes.

The expected path is:

~~~text
asset_pending -> asset_received -> retirement_processing
  -> withdrawal_processing -> completed
~~~

The completed order must include a payout object with simulated: true. No
real bank transfer occurs.

### Postman API-key flow

~~~http
POST {{base_url}}/v1/offramps
Authorization: Bearer {{api_key}}
Content-Type: application/json
Idempotency-Key: e2e-offramp-001
~~~

~~~json
{
  "asset": {
    "network": "stellar_testnet",
    "code": "XLM",
    "amount": "1.0000000"
  },
  "withdrawal": {
    "currency": "IDR",
    "method": "sandbox_bank_transfer",
    "destination_token": "sandbox-e2e-payout-001"
  }
}
~~~

Expect 201. The response contains the account and memo required for the
testnet deposit.

Test these failure cases:

- send the wrong asset amount;
- omit or change the memo;
- use a mainnet account;
- reuse the idempotency key with a changed body.

The backend must not mark an invalid deposit as completed.

## 10. Test order history and ownership

For the signed-in user, call:

~~~http
GET {{base_url}}/v1/orders?limit=20
~~~

Expect the user's session-owned orders across valid sessions.

Call:

~~~http
GET {{base_url}}/v1/orders/{{onramp_order_id}}
~~~

Expect 200 for an owned order and 404 for an order owned by another user or
another API client. Do not reveal whether an inaccessible order exists.

Test cursor pagination with limit=1. Treat next_cursor as opaque. Do not parse
it or construct a cursor from an order ID.

## 11. Test SEP-24 and anchor discovery

These endpoints are JSON or Stellar protocol endpoints. The backend does not
provide a wallet-facing SEP-24 HTML screen.

Run the public discovery requests:

~~~http
GET {{base_url}}/.well-known/stellar.toml
GET {{base_url}}/sep24/info
GET {{base_url}}/federation?q=demo*kailopay
GET {{base_url}}/sep38/info
GET {{base_url}}/sep38/prices?sell_asset=iso4217%3AIDR&buy_asset=stellar%3Anative
~~~

Expect 200 and confirm that the responses advertise sandbox and
stellar_testnet. Confirm that `stellar.toml` contains `TRANSFER_SERVER_SEP24`
and `ANCHOR_QUOTE_SERVER`.

For the amount-specific quote calculation, use exactly one amount parameter:

~~~http
GET {{base_url}}/sep38/price?sell_asset=iso4217%3AIDR&buy_asset=stellar%3Anative&sell_amount=100000
~~~

The response contains `total_price`, `price`, `sell_amount`, and `buy_amount`.
It is a calculation only and must not create an order.

Start a deposit with Postman using multipart/form-data:

~~~http
POST {{base_url}}/sep24/transactions/deposit/interactive
Authorization: Bearer {{api_key}}
Idempotency-Key: e2e-sep24-deposit-001
~~~

Form fields:

| Field | Value |
|---|---|
| asset_code | XLM |
| amount_minor | 100000 |
| account | funded Stellar testnet G... address |
| memo | optional |
| memo_type | text when a memo is present |
| payment_method | xendit |

The deposit response includes `payment_link_url`. Open that hosted Xendit
sandbox URL to start payment, then wait for the verified callback before
polling the SEP-24 transaction. The returned `url` remains the KailoPay
interactive projection URL.

Start a withdrawal with multipart/form-data:

~~~http
POST {{base_url}}/sep24/transactions/withdraw/interactive
Authorization: Bearer {{api_key}}
Idempotency-Key: e2e-sep24-withdraw-001
~~~

Form fields:

| Field | Value |
|---|---|
| asset_code | XLM |
| amount | 1.0000000 |
| destination_token | sandbox-e2e-sep24-payout-001 |

Save the returned transaction ID. Poll both projections:

~~~http
GET {{base_url}}/sep24/transaction?id={{sep24_transaction_id}}
Authorization: Bearer {{api_key}}

GET {{base_url}}/sep24/interactive/{{sep24_transaction_id}}
Authorization: Bearer {{api_key}}

GET {{base_url}}/sep24/transactions?limit=20
Authorization: Bearer {{api_key}}
~~~

The transaction must remain owner-scoped. A different API key must receive
404 for the single-transaction routes and must not see the first key's history
in the list response.

## 12. Test developer webhook configuration

Outbound developer webhook delivery is staged in the current release. Test
endpoint registration and metadata, but do not expect an order event to reach
the endpoint yet.

Create an endpoint with the authenticated session:

~~~http
POST {{base_url}}/v1/webhook-endpoints
Content-Type: application/json
~~~

~~~json
{
  "url": "https://webhook.site/<your-token>",
  "event_types": [
    "order.created",
    "order.completed",
    "order.failed"
  ]
}
~~~

Expect 201. Store the returned secret only in a secure local variable. The
secret is not returned again.

Test:

~~~http
GET {{base_url}}/v1/webhook-endpoints
DELETE {{base_url}}/v1/webhook-endpoints/<endpoint-id>
~~~

Expect 200 from the list request and 204 from the delete request.

## 13. Test authentication and ownership failures

Run these checks with a second account and a second API key:

| Test | Expected result |
|---|---|
| Missing session on /v1/kyc | 401 |
| Persona inquiry creation with a mismatched provider environment | 503 with local diagnostics |
| API-key creation before KYC approval | 403 KYC_REQUIRED |
| Order creation before KYC approval | 403 KYC_REQUIRED |
| Missing API key and session on an order route | 401 |
| Revoked API key on an order route | 401 |
| Invalid Stellar destination | 400 |
| Amount outside configured limits | 422 |
| Missing Idempotency-Key on order creation | 400 |
| Session order mutation with an unapproved Origin | 403 |
| Cross-user order detail request | 404 ORDER_NOT_FOUND |
| Same idempotency key and same body | 200 replay |
| Same idempotency key and changed body | 409 IDEMPOTENCY_KEY_REUSED |

Display the request_id from each API error when the frontend shows a support
message. Do not display provider secrets, raw provider responses, or stack
traces.

## 14. Frontend acceptance checklist

Mark the frontend test complete only when these checks pass:

- The app starts against the backend through the same-origin proxy.
- Registration, email verification, login, logout, and session expiry work.
- Profile, Developer Mode, avatar upload, and avatar removal work.
- The KYC page opens Persona with the returned inquiry data.
- The KYC page waits for the signed webhook before showing approved.
- API-key creation shows the plaintext key once and never stores it in source
  code or browser storage.
- The developer playground creates an on-ramp and opens the Xendit checkout.
- The consumer buy page creates a session-owned on-ramp.
- The consumer sell page creates a session-owned off-ramp and shows deposit
  instructions.
- Order detail and activity pages poll backend status instead of guessing state
  from browser navigation.
- Completed on-ramp orders show a Stellar testnet transaction link.
- Completed off-ramp orders show that the payout is simulated.
- SEP-24 discovery, deposit, withdrawal, and status polling work.
- API errors show stable messages and preserve the request_id.
- Sandbox and Stellar testnet labels appear on every value-movement screen.

## Troubleshooting

| Symptom | Check |
|---|---|
| Frontend requests fail before reaching the API | Set API_ORIGIN=http://localhost:8080 and restart Next.js |
| Login succeeds but protected frontend calls return 401 | Check the kailopay_session cookie and same-origin proxy |
| Persona returns 403 Forbidden | Match the Persona API key, template, and environment |
| Persona returns 400 Idempotency error | Retry through POST /v1/kyc/inquiry; do not create a manual local record |
| Persona widget completes but status stays completed | Check the public HTTPS callback URL, Persona event subscriptions, and webhook secret |
| Persona callback returns 401 | Check the exact raw body, Persona-Signature, webhook secret, and clock tolerance |
| Xendit order stays payment_pending | Check the public Xendit callback URL or use the Xendit sandbox simulator |
| Order stays stellar_processing | Start the worker and check the treasury testnet account and worker secret |
| Off-ramp stays asset_pending | Send the exact XLM amount with the exact deposit memo |
| API-key creation returns KYC_REQUIRED | Poll GET /v1/kyc until the local status is approved |

Never solve a failed callback by changing the frontend status locally. Fix the
provider callback, then read the backend status again.
