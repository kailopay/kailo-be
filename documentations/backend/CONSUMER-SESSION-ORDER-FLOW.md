# Consumer session order flow

Status: proposed backend implementation specification

This document defines the backend work needed for the authenticated consumer
buy and sell flows in the KailoPay web application. It complements the hosted
checkout flow in [`../frontend/ONRAMP-HOSTED-CHECKOUT-FLOW.md`](../frontend/ONRAMP-HOSTED-CHECKOUT-FLOW.md).

The consumer browser uses the existing `kailopay_session` cookie. The browser
never receives or sends a `pk_test_` key for a consumer order.

## 1. Implement session-owned orders on the existing paths

Keep the existing paths:

| Method | Path | Consumer use |
|---|---|---|
| `POST` | `/v1/onramps` | Create an IDR to XLM order |
| `POST` | `/v1/offramps` | Create an XLM to IDR order |
| `GET` | `/v1/orders` | List the signed-in user's Activity history |
| `GET` | `/v1/orders/{id}` | Read one order owned by the signed-in user |

Do not add `/v1/consumer/activity`. Activity is a view of the authenticated
user's orders, not a separate resource.

Each order request accepts either one API-key principal or one retail-session
principal:

```text
Developer request: Authorization: Bearer pk_test_<public_id>_<secret>
Consumer request:  Cookie: kailopay_session=<opaque_token>
```

If an `Authorization` header is present, authenticate the API key and use the
API-client principal. If the header is absent, authenticate the session cookie
and use the retail-user principal. This precedence keeps the existing
Developer Mode playground working because the browser sends its session cookie
alongside the developer's in-memory API key.

Do not accept a client ID, user ID, session ID, or owner type from a request
body, query string, or path. Resolve the owner only from authenticated
credentials.

## 2. Resolve and carry an order principal

Add one order authorization value that represents the authenticated owner:

```text
API client principal
  kind: api_client
  client_id: UUID
  owner_user_id: UUID

Retail session principal
  kind: retail_session
  user_id: UUID
  session_id: UUID
```

The exact Go type name is an implementation choice. The value must make the
principal kind explicit so a repository cannot accidentally query one owner
type with the other owner's identifier.

The order middleware must:

1. Authenticate a valid bearer key when `Authorization` is present.
2. Otherwise authenticate the active `kailopay_session` cookie.
3. Reject missing, malformed, expired, revoked, or disabled credentials with
   `401`.
4. Store the resolved principal in the request context.

The order handler must pass the resolved principal to the use case. The use
case must pass an owner-specific value to the repository. Do not keep using an
empty `ClientID` as a signal for a retail request.

A retail session does not require Developer Mode. A valid active session with a
verified user can create and read consumer orders.

## 3. Scope ownership in the database

The `orders` table already contains the columns needed for the ownership
split:

| Principal | `client_id` | `created_by_user_id` | `retail_session_id` |
|---|---:|---:|---:|
| API client | set | optional actor correlation | `NULL` |
| Retail session | `NULL` | set to authenticated user | set to creating session |

Keep the existing exactly-one-owner database check. Add an index for consumer
history:

```sql
CREATE INDEX idx_orders_retail_user_created
    ON orders(created_by_user_id, created_at DESC, id DESC)
    WHERE client_id IS NULL AND retail_session_id IS NOT NULL;
```

The session authenticates the user, and `created_by_user_id` scopes the user's
history. Use the user ID for consumer `GET /v1/orders` and
`GET /v1/orders/{id}` queries so a user can see orders created in an earlier
valid session. Require `retail_session_id IS NOT NULL` in those queries so an
API-client order cannot become visible through the consumer path.

The application must write the authenticated session's ID and user ID
together. It must reject a session record whose user ID does not match the
authenticated user value.

Do not return ownership columns in the public order response.

## 4. Scope idempotency by the same owner context

The consumer flow generates one `Idempotency-Key` for each purchase intent and
reuses it when a request outcome is unknown. The key must not be stored only
against `client_id`, because a consumer order has no API client.

Add a nullable retail owner column to `idempotency_records` and replace the
current single unique constraint with two partial unique indexes:

```sql
ALTER TABLE idempotency_records
    ALTER COLUMN client_id DROP NOT NULL;

ALTER TABLE idempotency_records
    ADD COLUMN retail_user_id uuid REFERENCES users(id);

ALTER TABLE idempotency_records
    ADD CONSTRAINT idempotency_records_one_owner_check CHECK (
        (client_id IS NOT NULL AND retail_user_id IS NULL)
        OR
        (client_id IS NULL AND retail_user_id IS NOT NULL)
    );

CREATE UNIQUE INDEX idx_idempotency_api_client
    ON idempotency_records(client_id, operation, key_hash)
    WHERE client_id IS NOT NULL;

CREATE UNIQUE INDEX idx_idempotency_retail_user
    ON idempotency_records(retail_user_id, operation, key_hash)
    WHERE retail_user_id IS NOT NULL;
```

Create a versioned migration with a matching down migration. Preserve existing
API-client records while changing the constraint.

For a retail request, use the authenticated user ID as the idempotency owner.
For an API request, use the authenticated client ID. A same-key request with a
different request hash returns `409 IDEMPOTENCY_KEY_REUSED`. A same-key request
with the same request hash returns the stored order result.

The request hash must include every business input that affects the order:

- operation
- IDR amount or XLM amount
- payment method when applicable
- Stellar destination or sandbox payout reference
- memo when applicable

Do not include the session ID in the request hash. A retry after session
renewal must still replay the same consumer intent for the same user.

## 5. Keep the create request contract unchanged

The consumer on-ramp sends the same JSON body as the developer API:

```json
{
  "fiat": {
    "currency": "IDR",
    "amount_minor": "100000"
  },
  "payment_method": "xendit",
  "stellar_destination": {
    "account": "G...",
    "memo": null
  }
}
```

The backend must accept this request with a retail session and create a retail
owned order. Keep all existing validation:

- `amount_minor` is a positive decimal string and remains an integer IDR minor
  unit internally.
- `currency` is `IDR`.
- `payment_method` is `xendit`, `qris`, or `bri_va`.
- `xendit` creates a hosted Xendit checkout with all configured payment
  channels.
- `qris` and `bri_va` restrict the hosted checkout to the selected channel.
- `stellar_destination.account` is a valid 56-character testnet account that
  starts with `G`.
- `stellar_destination.memo` is optional and has a maximum length of 28.
- The request includes a non-empty `Idempotency-Key` with a maximum length of
  255.

The consumer UI sends `xendit` by default. It does not display a payment
channel selector for the hosted checkout. Keep the restricted methods for
explicit developer or future consumer selection.

Apply the same principal and ownership rules to the existing off-ramp request:

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

## 6. Return the hosted checkout result

Successful on-ramp responses remain `201` for a new order and `200` for a
same-key replay. Return the existing order envelope with this checkout shape:

```json
{
  "checkout": {
    "id": "ps-...",
    "status": "ACTIVE",
    "presentation_type": "PAYMENT_LINK",
    "presentation_value": "https://checkout-staging.xendit.co/sessions/ps-...",
    "payment_link_url": "https://checkout-staging.xendit.co/sessions/ps-...",
    "expires_at": "2026-08-22T10:03:12Z"
  }
}
```

The provider adapter must return an HTTPS URL produced by Xendit. The client
must not be able to submit or replace the URL. Persist the URL with the
checkout record and expose it through both `POST /v1/onramps` and
`GET /v1/orders/{id}`.

The frontend stores the returned order ID before it navigates to
`payment_link_url`. Xendit owns the payment screen. The backend must not make
the frontend construct a QR code or virtual-account display for a
`PAYMENT_LINK` checkout.

Keep `presentation_value` for compatibility with older order records. New
Xendit hosted checkouts set both values to the same URL.

## 7. Make the 202 reconciliation response trackable

When checkout creation has an unknown provider outcome, keep the order in
reconciliation and return `202` with the stable error code. Include the order
ID so the consumer can link to its status screen:

```json
{
  "error": {
    "code": "CHECKOUT_PENDING_RECONCILIATION",
    "message": "The checkout outcome is being reconciled with the payment provider."
  },
  "request_id": "req_...",
  "order_id": "uuid"
}
```

If the current application service cannot return the order ID with the error,
change its result type or error type before wiring the consumer screen. Do not
make the frontend create another order to discover the first order.

The same idempotency key and request body may be submitted again to retrieve
the current order after reconciliation. A new key is a new purchase intent.

## 8. Scope the order list and detail responses

Keep the current response shapes:

```text
GET /v1/orders?limit=20&cursor=<opaque-cursor>
GET /v1/orders/<order-id>
```

For a retail-session principal:

- List only orders where `client_id IS NULL`, `retail_session_id IS NOT NULL`,
  and `created_by_user_id` equals the authenticated user ID.
- Read one order with the same ownership predicate.
- Return `404 ORDER_NOT_FOUND` for a missing order and an order owned by an API
  client or another user.
- Order by `created_at DESC, id DESC`.
- Keep `next_cursor` opaque. Do not expose SQL offsets or ownership data.

For an API-client principal, preserve the current client-owned behavior.

The list response remains:

```json
{
  "orders": [],
  "next_cursor": ""
}
```

This response feeds the consumer Activity page. The page does not need a
special activity query or a second order table.

## 9. Protect browser session mutations against CSRF

API-key requests do not use browser session authentication and do not need the
retail CSRF check.

For a session-authenticated `POST /v1/onramps` or `POST /v1/offramps`, validate
the browser origin before the use case runs:

- Add a configured allowlist of frontend origins.
- Accept an `Origin` that exactly matches one configured origin.
- If `Origin` is absent, accept a `Referer` whose origin exactly matches one
  configured origin.
- Reject a missing or mismatched origin with `403`.
- Keep `SameSite=Lax`, `HttpOnly`, and `Secure` outside local development on
  the session cookie.

Use the existing session cookie name, `kailopay_session`. Do not add a token to
the consumer request body or store a session token in browser storage.

If the deployment already has a stronger CSRF mechanism, use that mechanism
instead and document the header or token contract in the OpenAPI file.

## 10. Map status and error behavior for the consumer

The backend remains the source of truth. The frontend must not mark an order as
paid because the user returned from the Xendit page.

| Status | Backend meaning | Consumer behavior |
|---|---|---|
| `created` | Order exists while checkout is being established | Show a preparing state |
| `payment_pending` | Hosted checkout is ready or payment is incomplete | Show the hosted payment link and quote expiry |
| `payment_confirmed` | Xendit payment is confirmed | Show payment received |
| `stellar_processing` | The worker is sending XLM | Show transfer in progress |
| `completed` | Stellar transfer is confirmed | Show success and the testnet explorer link |
| `expired` | Payment window ended without confirmation | Show expiry and a new-purchase action |
| `payment_failed` | Provider rejected the checkout | Show failure and a new-purchase action |
| `stellar_failed` | Payment succeeded but XLM transfer failed | Tell the user not to pay again and show the order ID |
| `cancelled` | The order cannot receive payment | Show cancellation and a new-purchase action |

The consumer polls `GET /v1/orders/{id}` every 3 to 5 seconds while the order
is non-terminal. Stop at `completed`, `expired`, `payment_failed`,
`stellar_failed`, or `cancelled`.

Use the existing rich error envelope:

```json
{
  "error": {
    "code": "AMOUNT_OUT_OF_RANGE",
    "message": "The requested amount is outside the supported range."
  },
  "request_id": "req_..."
}
```

Return the same stable codes for both principal types:

| HTTP | Code | Consumer handling |
|---:|---|---|
| `400` | `INVALID_REQUEST` | Show field or request validation feedback |
| `400` | `INVALID_STELLAR_ACCOUNT` | Ask for a valid testnet destination |
| `401` | existing authentication error | Sign in again |
| `403` | CSRF or session policy failure | Stop the mutation and show a safe error |
| `404` | `ORDER_NOT_FOUND` | Treat the order as unavailable to this user |
| `409` | `IDEMPOTENCY_KEY_REUSED` | Stop. Do not create a replacement automatically |
| `409` | `INSUFFICIENT_LIQUIDITY` | Tell the user that sandbox liquidity is unavailable |
| `422` | `AMOUNT_OUT_OF_RANGE` | Ask for a supported amount |
| `202` | `CHECKOUT_PENDING_RECONCILIATION` | Show pending. Do not create again |
| `503` | `QUOTE_UNAVAILABLE` or `EXTERNAL_SERVICE_UNAVAILABLE` | Allow a new attempt with a new intent |

Never return raw provider bodies, API keys, session tokens, or stack traces.
Every response must retain `request_id` and echo `X-Request-ID`.

## 11. Keep the network boundary explicit

The consumer release supports sandbox and Stellar testnet only:

- Accept `stellar_testnet` as the only network.
- Reject mainnet destinations and mainnet order requests.
- Keep `environment: "sandbox"` and `network: "stellar_testnet"` in every
  order response.
- The frontend may show Mainnet as unavailable in a network selector, but the
  backend does not create or settle mainnet orders in this release.

The quote is created when the order is created. This specification does not add
a pre-submit quote endpoint. The frontend shows the locked rate from the order
response and the quote expiry from `quote.expires_at`.

## 12. Implement the backend in these slices

### Authentication and middleware

- Add a middleware that resolves an API-client or retail-session principal.
- Preserve the existing API-key behavior and developer playground requests.
- Add the session-origin check for browser mutations.
- Add middleware tests for both credentials, missing credentials, expired
  sessions, invalid keys, and API key plus session cookie precedence.

### Application use cases

- Replace client-only owner inputs with an explicit owner context.
- Write `client_id` for API orders.
- Write `created_by_user_id` and `retail_session_id` for consumer orders.
- Apply the same owner context to create, replay, get, and list operations.
- Keep gateway calls and Stellar work outside the database transaction.
- Return a trackable order ID for `CHECKOUT_PENDING_RECONCILIATION`.

### Persistence

- Add the retail-history index.
- Update idempotency ownership and partial unique indexes.
- Update GORM models and repository queries.
- Add repository tests for cross-user, cross-session, and cross-client access.
- Keep order events, outbox messages, checkout records, and reservations
  attached to the same order regardless of owner type.

### HTTP contract

- Add `kailoSession` as an alternative security requirement to the four order
  operations in OpenAPI.
- Document session-owned order behavior and the `202` `order_id` field.
- Keep request and response JSON keys in `snake_case`.
- Validate the OpenAPI document against the running handler behavior.

### Frontend handoff

The frontend can then call the same-origin `/v1` paths with ordinary
`fetch` requests. It does not need a BFF or an API key for the consumer flow.

The consumer sequence is:

```text
POST /v1/onramps with the session cookie
  -> save order.id in sessionStorage
  -> navigate to checkout.payment_link_url
  -> read GET /v1/orders/{id} after return
  -> poll until a terminal status
```

## 13. Tests required before handoff

Add tests for all of the following:

1. A valid session creates an on-ramp order with retail ownership.
2. A valid session creates an off-ramp order with retail ownership.
3. The same session user can list orders created in an earlier session.
4. A user cannot read or list another user's consumer order.
5. A consumer request cannot read an API-client order.
6. An API-key request keeps the current client-owned behavior.
7. A browser request with both cookie and bearer key uses the bearer key path.
8. Same consumer idempotency key and same body return one order.
9. Same consumer idempotency key and different body return
   `IDEMPOTENCY_KEY_REUSED`.
10. The same key used by two different users does not cross replay.
11. A `202 CHECKOUT_PENDING_RECONCILIATION` response contains `order_id` and
    does not create a replacement order.
12. `PAYMENT_LINK` responses include `payment_link_url` and preserve
    `presentation_value` compatibility.
13. Consumer list pagination is stable and keeps the cursor opaque.
14. Missing or mismatched session origins cannot create an order.
15. Every ownership, validation, idempotency, and authentication failure
    returns a safe error and a request ID.

Run the normal backend verification baseline after implementation:

```text
gofmt ./...
go vet ./...
go test ./...
go test -race ./...
```

## 14. Documentation changes required with implementation

After the backend code is implemented, update these documents in the same
change:

- `openapi/openapi.yaml`
- `documentations/backend/API-DESIGN.md`
- `documentations/backend/DATABASE-DESIGN.md`
- `documentations/backend/DOMAIN-MODEL.md`
- `documentations/backend/SECURITY.md`
- `documentations/backend/TESTING-STRATEGY.md`
- `documentations/backend/BACKEND-BACKLOG.md`
- `documentations/backend/IMPLEMENTATION-PHASES.md`

Remove the statement that retail-session order access is deferred only after
the endpoint, migration, authorization, and test changes have landed.

## 15. Acceptance criteria

The backend implementation is ready for the consumer frontend when:

- A signed-in user can create a sandbox IDR to XLM order without a
  `pk_test_` key.
- The order is owned by that user and cannot be read by another user or API
  client.
- `GET /v1/orders` provides the user's Activity history across valid sessions.
- The order returns an Xendit hosted `payment_link_url`.
- The existing developer API-key flow remains compatible.
- Replays are idempotent for both owner types.
- Unknown checkout outcomes return a trackable pending response.
- Payment and settlement status remains backend-controlled.
- No endpoint accepts owner identifiers from the consumer request.
- OpenAPI, migrations, docs, and tests describe the same behavior.
