# SEP wallet interoperability and sandbox payout design

Date: 2026-09-22

Status: Accepted and implemented on `main`

## Goal

Make KailoPay usable as a testnet anchor by an external Stellar wallet while
keeping the existing `/v1/onramps` and `/v1/offramps` contracts unchanged.

The protocol path will use canonical SEP-1 discovery, SEP-10 authentication,
SEP-24 interactive transfers and history, and SEP-38 indicative and firm
quotes. The off-ramp will use an explicit sandbox payout simulator. A
non-empty synthetic `destination_token` will always produce a successful
simulated payout after the Stellar deposit and retirement have completed.
The response will say that no IDR moved.

## Constraints and decisions

- All protocol and payout-simulator behavior is testnet-only.
- Classic Stellar accounts are supported through SEP-10. Contract-account
  authentication through SEP-45 is not advertised until a contract-account
  use case exists.
- Runtime wallet users must sign in or link to a KailoPay retail session from
  the interactive page and must pass the existing Persona approval gate.
- Automated tests may use `SEP24_TEST_AUTO_APPROVE_KYC`, but startup must reject
  that flag unless `APP_ENV=test` and the Stellar network is testnet. The flag
  is never accepted in local, staging, or production runtime configuration.
- The payout simulator accepts any non-empty destination reference within the
  existing length limit. It does not validate or contact a bank account.
- The simulator is selected by `OFFRAMP_PAYOUT_MODE=simulated`. Startup rejects
  this mode for any non-testnet network.
- Existing `/v1` routes keep their request and response shapes. SEP handlers
  remain protocol adapters and reuse the existing quote, KYC, order, payment,
  Stellar, idempotency, and ownership workflows.
- No production custody, real IDR payout, mainnet support, or production KYC
  claim is added by this change.

## User and wallet flow

### Discovery and authentication

1. The wallet reads `/.well-known/stellar.toml`.
2. The TOML advertises the transfer server, quote server, `WEB_AUTH_ENDPOINT`,
   and the anchor `SIGNING_KEY`.
3. The wallet requests a SEP-10 challenge for its classic Stellar account.
4. The wallet signs the challenge and exchanges it for a short-lived bearer
   JWT.
5. The wallet uses that JWT on SEP-24 transaction endpoints and SEP-38 firm
   quote endpoints.

The SEP-10 implementation uses a dedicated anchor signing secret. It never
reuses the treasury, deposit, or payout credentials. Challenge transactions
are bounded by timeout, network, account, home domain, and signature checks.
JWTs contain the authenticated Stellar account and expiry. The server rejects
expired, malformed, wrong-audience, or wrong-account tokens.

### Interactive linking and KYC

SEP-10 proves control of a Stellar account, but it does not identify a local
KailoPay user. A canonical SEP-24 initiation therefore creates a durable
interactive session before creating an order.

The interactive URL carries a short-lived, opaque transaction token. The
browser must establish a KailoPay retail session, link that session to the
authenticated Stellar account, and complete the Persona flow. Only then does
the server create the existing on-ramp or off-ramp order and persist the
SEP-24 transaction mapping.

The test-only KYC flag replaces the Persona approval check in automated tests
after the test has authenticated the SEP-10 account. It does not become a
public request header or a wallet-controlled parameter.

### Deposit

The wallet sends a standard SEP-24 deposit initiation request with
`asset_code`, optional amount or quote information, the Stellar account, and
an idempotency key. The server returns the standard interactive response with
`type`, `url`, and `id`.

After the user links the wallet and passes KYC, KailoPay uses the existing
on-ramp flow to create the IDR checkout. The interactive page displays the
hosted payment link. A confirmed sandbox payment starts the existing Stellar
settlement worker. Transaction polling exposes the current order state and
the Stellar transaction hash when settlement completes.

### Withdrawal

The wallet sends a standard SEP-24 withdrawal initiation request with
`asset_code`, the exact XLM `amount`, optional `quote_id`, and an idempotency
key. The interactive page collects the synthetic sandbox payout reference.
The page must label it as a testnet destination reference, not a bank account.

The wallet receives the anchor deposit account, exact amount, memo, and memo
type in the transaction projection. The deposit scanner verifies the exact
testnet payment and memo before queuing retirement.

After retirement confirmation, the payout simulator completes the payout
asynchronously. The SEP-24 transaction then becomes `completed` and includes
the sandbox disclosure and payout reference.

## Protocol contracts

### SEP-1 and CORS

`stellar.toml` advertises only implemented services. The transfer server and
quote server use the configured public API origin. The anchor adds the SEP-10
web-auth endpoint and signing key and does not advertise a contract-account
auth endpoint.

SEP-1 and SEP-24 public endpoints return the required CORS headers and handle
preflight requests. The existing private `/v1` origin policy remains in place.

### SEP-24

The canonical transfer-server routes are:

- `POST /sep24/transactions/deposit/interactive`
- `POST /sep24/transactions/withdraw/interactive`
- `GET /sep24/transactions`
- `GET /sep24/transaction`
- `GET /sep24/interactive/{id}`

The handler accepts standard SEP-24 names and preserves documented sandbox
extensions only where the protocol has no equivalent. Transaction objects use
`kind=deposit` or `kind=withdrawal`, standard status values, timestamps,
`more_info_url`, `amount_in`, `amount_out`, `quote_id`, and transaction IDs.
The lookup endpoint accepts `id`, `stellar_transaction_id`, or
`external_transaction_id`, with ownership checked against the SEP-10 account
or linked retail session.

The history endpoint supports `asset_code`, `kind`, `limit`, `no_older_than`,
`paging_id`, and `lang`. Paging is stable and owner-scoped.

### SEP-38

The quote server exposes:

- `GET /sep38/info`
- `GET /sep38/prices`
- `GET /sep38/price`
- `POST /sep38/quote`
- `GET /sep38/quote/{id}`

The supported pair is `iso4217:IDR` and `stellar:native`. Amounts remain exact
decimal strings. A firm quote is persisted with its rate, spread, amounts,
delivery method, owner account, and expiry. SEP-24 initiation accepts a valid
quote ID and rejects an expired, mismatched, or already-consumed quote.

The existing `/v1/quotes` preview remains unchanged and continues to use the
same market and spread policy.

## Payout simulator

The simulator is a worker-owned adapter, not HTTP-handler logic.

1. `ConfirmRetirement` moves the order to `withdrawal_processing` and writes a
   `payout.simulate_offramp` outbox message in the same transaction.
2. The worker leases that message and loads the order and quoted IDR amount.
3. The simulator creates one `offramp_payouts` row with method
   `sandbox_bank_transfer`, state `completed`, a deterministic reference such
   as `sandbox-payout-{order_id}`, and `completed_at`.
4. The repository moves the order to `completed`, records `payout.simulated`,
   and marks the outbox message processed in one database transaction.
5. A retry sees the unique order or reference row and returns success without
   creating a second payout.

The public `/v1` order response keeps the existing payout shape and adds the
explicit sandbox disclosure. The SEP-24 transaction projection exposes the
same reference as a sandbox external reference and states that no IDR was
transferred.

## Persistence and boundaries

- Add a durable interactive-session record for a SEP-10 account before an
  order exists. It stores direction, request parameters, wallet account,
  expiry, linked user, KYC state, and order ID after completion.
- Extend SEP-24 transaction records to retain the wallet account and support
  wallet-scoped authorization without changing existing API-client or retail
  ownership rules.
- Add a versioned migration for interactive sessions, quote records, wallet
  ownership fields, payout outbox support, and required unique indexes.
- Keep Stellar SDK types in the Stellar adapter. Keep JWT and challenge
  validation in the anchor authentication adapter/use case.
- Keep provider payout interfaces separate from the simulator so a future
  real payout adapter can replace it without changing order state policy.

## Error and safety rules

- Never mark a payout completed before the retirement transaction is confirmed.
- Never retry an unknown external Stellar result by creating a second intent.
- Reject a duplicate idempotency key with a different request body.
- Expire interactive sessions and firm quotes.
- Do not log JWTs, challenge transactions, wallet secrets, destination
  references, Persona payloads, or provider credentials.
- If the simulator is disabled or misconfigured, leave the order in
  `withdrawal_processing` and expose a safe pending state.

## Verification

The implementation must add:

- SEP-10 challenge, signature, JWT, expiry, account, and replay tests.
- Interactive linking and Persona approval tests.
- Test-only auto-approval tests proving the flag is rejected outside
  `APP_ENV=test` and testnet.
- SEP-24 contract tests for standard forms, transaction lookup identifiers,
  history filters, paging, status mapping, CORS, and interactive HTML.
- SEP-38 quote lifecycle tests for creation, retrieval, expiry, consumption,
  ownership, and exact amount arithmetic.
- Repository integration tests for migration constraints, payout idempotency,
  retry, and the full deposit, retirement, simulator, and completed-order
  transition.
- Existing `/v1` on-ramp and off-ramp contract tests unchanged and passing.

Run `gofmt`, `go vet ./...`, `go test ./...`, and `go test -race ./...` before
claiming completion.

## Documentation updates

Update the anchor integration guide, API design, order state machine, payment
gateway guide, OpenAPI contract, environment examples, and ADR-005. ADR-005
will be superseded for testnet by the explicit simulator decision, while its
warning about real IDR settlement remains in the public disclosure.
