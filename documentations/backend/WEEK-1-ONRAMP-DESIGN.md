# Week 1 Production-Shaped On-Ramp Design

## 1. Goal

Deliver one end-to-end sandbox on-ramp:

```text
authenticate developer
  -> create idempotent IDR-to-XLM order and locked quote
  -> reserve testnet XLM
  -> create Xendit QRIS or BRI virtual-account checkout
  -> verify and reconcile payment callback
  -> atomically create one settlement intent
  -> transfer reserved XLM from the treasury distribution account
  -> reconcile through Horizon and complete the order
```

The implementation uses production-shaped boundaries and failure handling, but
never claims that Xendit sandbox IDR purchases testnet XLM or that the release
is suitable for real funds.

## 2. Scope

### Included

- Developer Mode opt-in through the authenticated Auth0 retail session.
- One user-owned sandbox API client per developer for Week 1.
- Create, list, and revoke `pk_test_` API keys; plaintext is shown once.
- API-key middleware with explicit API-client identity.
- Idempotent on-ramp creation and client-scoped get/list endpoints.
- CoinMarketCap XLM/IDR reference quote, configurable spread, five-minute
  validity, exact stroop conversion, and immutable quote snapshot.
- Native-XLM treasury inventory reservation.
- Xendit Payment Requests v3 for `QRIS` and `BRI_VIRTUAL_ACCOUNT`.
- Xendit callback-token authentication, event deduplication, and Payment
  Request status reconciliation.
- Atomic payment confirmation, event history, settlement intent, and outbox.
- Separate worker process for Stellar testnet transfer and reconciliation.
- Versioned PostgreSQL migration, OpenAPI, safe logs, local configuration, and
  focused automated tests.

### Excluded

- Off-ramp orders and IDR payouts.
- Buying XLM from an exchange or automatic treasury replenishment.
- Custom Stellar assets, trustlines, issuance, and retirement.
- SEP-24, federation, outgoing developer webhooks, production custody, KYC/AML,
  mainnet, and production payment credentials.

These exclusions remain visible in API responses and documentation.

## 3. Architecture

The modular monolith gains focused capabilities under the existing dependency
direction:

```text
cmd/api, cmd/worker
  -> handler/http and handler/middleware
  -> service/apikey, service/onramp, service/settlement
  -> entity

repository (PostgreSQL) implements service-owned persistence ports
adapter/coinmarketcap implements the on-ramp price port
adapter/xendit implements the on-ramp payment port
adapter/stellar implements the settlement network port
```

Provider request/response types stay inside adapters. Services use exact,
provider-neutral values. The API process does not receive the Stellar secret;
only `cmd/worker` constructs the signing adapter.

Manual constructor injection remains in each command composition root. No job
queue, dependency-injection framework, or second database is introduced.

## 4. Identity and API keys

`PATCH /auth/me` accepts an optional `developerEnabled` boolean in addition to
the display name. Disabling Developer Mode blocks management endpoints but does
not revoke existing keys.

Developer-management endpoints use the Auth0-backed local session:

- `POST /v1/api-keys` creates the user's default test API client if needed and
  returns one full key.
- `GET /v1/api-keys` returns safe key metadata only.
- `DELETE /v1/api-keys/{key_id}` revokes a user-owned key.

The key format is `pk_test_<public_id>_<secret>`. The public ID supports indexed
lookup. The secret contains 32 random bytes encoded without padding. PostgreSQL
stores only `HMAC-SHA-256(API_KEY_PEPPER, secret)` and a safe display prefix.
Verification uses constant-time comparison. The middleware places only the
authenticated API-client ID in the request context.

## 5. Quote and amount model

The create-on-ramp request supplies:

```json
{
  "fiat": { "currency": "IDR", "amount_minor": "100000" },
  "payment_method": "qris",
  "stellar_destination": { "account": "G...", "memo": null }
}
```

For IDR, one minor unit equals one rupiah. `fiat.amount_minor` is a decimal
string at the JSON boundary and an `int64` internally and in PostgreSQL.

The price adapter requests native XLM converted to IDR by CoinMarketCap asset
ID `512`. JSON numbers are decoded as decimal text and converted with exact
rational arithmetic. The quote policy:

1. Rejects market data older than two minutes.
2. Applies `QUOTE_SPREAD_BPS` to the IDR-per-XLM rate; the sandbox default is
   zero.
3. Computes XLM stroops by dividing IDR by the adjusted rate and rounding down
   once to seven decimal places.
4. Rejects zero, overflow, or configured out-of-range results.
5. Locks the quote for `QUOTE_TTL`, default five minutes.

The order stores the provider name, source timestamp, raw rate, adjusted rate,
spread, fiat amount, XLM stroops, and quote expiry. Existing orders never
reprice.

## 6. Treasury inventory and reservation

The configured treasury distribution account is native-XLM-only. A
`treasury_accounts` row stores the public account, network, last reconciled
spendable stroops, reserved stroops, minimum reserve stroops, and reconciliation
time. No secret is stored in PostgreSQL.

Before checkout creation, the service obtains a fresh Horizon account balance
outside the database transaction. It then locks the treasury row and reconciles
the observed spendable balance. The same transaction creates one
`treasury_reservations` row and increments reserved stroops. Available inventory
is:

```text
observed native balance
  - Stellar base/minimum reserve
  - configured operating buffer
  - active reservation total
```

Reservations are unique by order and use `reserved`, `consumed`, or `released`.
An unpaid expired order releases its reservation. A confirmed Stellar transfer
consumes it. A paid order never releases inventory merely because settlement is
temporarily failing.

## 7. Order creation and checkout

`POST /v1/onramps` requires a `pk_test_` key and `Idempotency-Key`.

Processing order:

1. Validate JSON, IDR amount, method, and existing Stellar testnet destination.
2. Canonicalize the request and hash the idempotency key and request.
3. Return the stored result for a completed same-key/same-body request; return
   `409 IDEMPOTENCY_KEY_REUSED` for a conflicting body.
4. Obtain a fresh quote and reserve inventory.
5. Persist the `created` order, initial event, and checkout intent.
6. Commit before calling Xendit.
7. Create a Xendit Payment Request using the order ID as `reference_id`, exact
   IDR amount, country `ID`, currency `IDR`, configured channel code, and an
   expiry bounded by the quote/order expiry.
8. Persist the provider ID, presentation action (`QR_STRING` or
   `VIRTUAL_ACCOUNT_NUMBER`), checkout event, and `payment_pending` transition.
9. Complete the idempotency record with the `201` response.

The checkout call is synchronous for the first release. A transport timeout is
an unknown outcome. Because the Payment Request create operation does not
document a general idempotency key, KailoPay does not automatically create a
replacement. It holds the order in an internal `checkout_unknown` condition and
uses the stable order reference with Xendit transaction search/provider evidence
before an operator-authorized retry. If a permanent checkout failure is
established, fail the order and release the reservation.

## 8. Payment callback

`POST /callbacks/payments/xendit` accepts a bounded JSON body and requires the
configured `x-callback-token`. The token is compared in constant time before
trusted parsing or order lookup.

For an authenticated callback:

1. Hash the exact body and derive event identity from Xendit `payment_id`.
2. Insert the receipt under a unique `(provider, provider_event_id)` constraint.
3. Return the previous acknowledgement for a processed replay.
4. Resolve the stored payment request and order.
5. Retrieve the Payment Request from Xendit.
6. Require `SUCCEEDED`, matching payment-request ID, order reference, `IDR`,
   exact amount, and expected channel.
7. In one database transaction, record the reconciled event, move the order to
   `payment_confirmed` and then `stellar_processing`, create one Stellar
   transaction intent, and insert one settlement outbox message.
8. Return `200` without performing Stellar I/O.

Missing/invalid tokens return `401` and cannot mutate database state. A valid
but mismatched callback is retained as safe diagnostic evidence and cannot
confirm payment. A late paid event remains held for manual sandbox resolution.

## 9. Settlement worker

`cmd/worker` leases `stellar.settle_onramp` outbox messages with PostgreSQL
row locking and `SKIP LOCKED`. It serializes work for the one configured source
account.

For each intent:

1. Load the existing intent, order, and reserved stroops.
2. If a transaction hash already exists, reconcile it instead of building a
   new transaction.
3. Load the source account from Horizon to obtain the current sequence.
4. Build one native-XLM payment with a bounded time condition and memo derived
   from the public order ID.
5. Sign with the runtime-injected testnet treasury secret.
6. Persist the deterministic transaction hash before or together with marking
   the attempt submitted.
7. Submit through Horizon and classify the result as confirmed, permanent,
   retryable-before-submit, or unknown.
8. On confirmation, verify the successful transaction/payment facts, consume
   the reservation, record the hash, append settlement/completion events, mark
   the order `completed`, and finish the outbox row atomically.

An unknown result remains `unknown`; later processing queries Horizon by hash.
It does not rebuild or resubmit until the original transaction is resolved or
its time condition proves it can no longer succeed. A permanent failure before
value movement moves the order to `stellar_failed` and leaves the paid order's
reservation held for manual recovery. Only a confirmed transfer consumes the
reservation.

## 10. Read API

- `GET /v1/orders/{order_id}` returns only an order owned by the authenticated
  API client. Missing and cross-client orders both return `404`.
- `GET /v1/orders` returns client-owned orders ordered by `(created_at, id)`
  descending with a bounded limit and opaque cursor.

The public order includes sandbox/testnet labels, immutable quote details,
checkout presentation, status, safe failure, and Stellar transaction hash when
confirmed. It never exposes raw provider bodies, callback tokens, signing
material, full API keys, or internal leases.

## 11. Database and migration

The existing placeholder models become executable through a reviewed SQL
migration. The migration adds constraints and indexes for ownership, positive
amounts, legal enum values, aggregate versions, idempotency, callback identity,
one settlement intent per order, outbox leasing, treasury accounts, and one
reservation per order.

`cmd/automigrate` remains local/test convenience. A new versioned migration
runner applies ordered SQL files and records checksums in `schema_migrations`.
Shared environments do not use GORM AutoMigrate.

All repository calls use `db.WithContext(ctx)`. Transactions never include
CoinMarketCap, Xendit, or Horizon calls.

## 12. Configuration and process boundaries

API process secrets/configuration:

- `API_KEY_PEPPER`
- `COINMARKETCAP_BASE_URL`, `COINMARKETCAP_API_KEY`
- `QUOTE_TTL`, `QUOTE_MAX_AGE`, `QUOTE_SPREAD_BPS`
- `ORDER_MIN_IDR`, `ORDER_MAX_IDR`
- `XENDIT_BASE_URL`, `XENDIT_SECRET_KEY`, `XENDIT_CALLBACK_TOKEN`
- `XENDIT_API_VERSION`, `XENDIT_QRIS_CHANNEL`, `XENDIT_VA_CHANNEL`
- `STELLAR_HORIZON_URL`, `STELLAR_NETWORK_PASSPHRASE`
- `STELLAR_TREASURY_ACCOUNT`, `STELLAR_OPERATING_BUFFER_STROOPS`

Worker-only secret/configuration:

- `STELLAR_TREASURY_SECRET`
- lease, poll, submission timeout, and bounded retry settings

Local/test configuration may use fake adapters in tests. Runtime configuration
uses real sandbox/testnet adapters and fails startup on production/mainnet
values for `v0.1.0`.

## 13. Error handling and observability

Stable public errors cover invalid API key, invalid destination, unsupported
method, amount range, insufficient liquidity, quote unavailable, idempotency
conflict, order not found, and external service unavailable.

Logs are emitted once at the HTTP/process or worker boundary with request ID,
client ID, order ID, provider request/event ID, settlement intent ID, and
transaction hash when safe. Authorization, full keys, callback tokens, Stellar
secrets, signed envelopes, and unrestricted provider payloads are never logged.

## 14. Test strategy

Implementation follows test-first slices with small fakes owned by consuming
service packages.

Required focused tests:

- Exact XLM/IDR conversion, spread, stale quote, rounding, and overflow.
- API-key format, one-time plaintext, constant-time verification, revocation,
  Developer Mode, and cross-owner access.
- Order transitions, terminal immutability, inventory reservation/release, and
  insufficient inventory.
- Same-key replay and conflicting idempotency body.
- Xendit request/response fixture mapping, callback token, mismatches, replay,
  and status lookup.
- Atomic payment/event/settlement/outbox behavior.
- Worker lease ownership, one transfer per intent, confirmed/permanent/unknown
  mapping, and reconciliation before retry.
- HTTP validation, client ownership, exact amount serialization, stable errors,
  and OpenAPI validation.

Real Xendit and Stellar testnet evidence is a manual/protected check using local
secrets after implementation. The normal test suite never requires external
credentials.

## 15. Acceptance criteria

The branch is complete when:

1. A clean PostgreSQL database applies the versioned migration.
2. An Auth0 user can enable Developer Mode and create a test API key.
3. The test key creates and reads one client-owned on-ramp order.
4. QRIS and BRI VA requests map to real Xendit sandbox checkout instructions
   when credentials are supplied.
5. Invalid callback authentication cannot mutate an order.
6. Replaying a reconciled paid callback creates exactly one payment-confirmed
   transition and one settlement intent.
7. A pre-funded testnet treasury sends the reserved native XLM exactly once and
   the completed order stores a Horizon-resolvable transaction hash.
8. Unknown Stellar submission is reconciled before retry.
9. OpenAPI, focused tests, formatting, vet, and secret review pass.
