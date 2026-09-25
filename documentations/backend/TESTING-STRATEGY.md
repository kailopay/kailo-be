# Backend Testing Strategy

## 1. Objectives

Testing must prove both business outcomes and failure safety:

- A verified payment causes one on-ramp settlement.
- An exactly verified asset deposit precedes simulated retirement and one off-ramp withdrawal result; simulation never fabricates on-chain evidence.
- Duplicates, concurrency, timeouts, and unknown external outcomes do not create duplicate value movement.
- Only users with approved Persona sandbox KYC status can create API keys or orders.
- Public API, SEP-24, and webhook behavior match their contracts.
- The TypeScript SDK remains compatible with the Go API and preserves exact amounts, idempotency, errors, and webhook signatures.
- Evidence is reproducible in payment-gateway sandbox and Stellar testnet.

## 2. Test layers

| Layer | Scope | External dependencies | Primary purpose |
|---|---|---|---|
| Unit | Domain value objects, transition policies, mapping, signature functions | None | Fast exhaustive invariant and edge-case checks |
| Repository integration | PostgreSQL migrations, constraints, repositories, transactions, leases | Ephemeral PostgreSQL | Persistence and concurrency correctness |
| Adapter contract | Payment/Stellar adapters against sanitized fixtures/fakes and optional sandbox | Controlled fake plus provider test environment | Provider mapping and error classification |
| API integration | HTTP middleware, auth, validation, idempotency, application usecases | App + ephemeral PostgreSQL, fake external ports | Public contract and security behavior |
| Worker integration | Outbox, leases, retries, reconciliation | App + PostgreSQL + fakes | Asynchronous correctness |
| Contract | OpenAPI and webhook schemas/examples | Running release candidate | Detect drift between docs and behavior |
| SDK contract | TypeScript client against OpenAPI and a running Go API | Node.js test matrix + release candidate | Detect SDK/API/type/serialization drift |
| End-to-end | Full sandbox payment and Stellar testnet flows | Real gateway sandbox + Stellar testnet | SOW acceptance and evidence |

## 3. Test environment strategy

- Unit/integration CI uses deterministic fake gateway and Stellar adapters.
- PostgreSQL tests run against the same supported major version as deployment. Set `TEST_DATABASE_DSN` to a disposable database to run the repository integration suite in `internal/repository`; the tests apply the versioned migrations and verify constraints and transactions. Local runs skip when the variable is unset; CI runs fail fast instead of silently omitting this coverage.
- The general CI race job excludes `internal/repository`, because those tests
  require PostgreSQL. The PostgreSQL CI job sets `TEST_DATABASE_DSN` and runs
  that package with the race detector enabled.
- Real provider sandbox tests are tagged and run manually or in protected CI with sandbox secrets.
- Stellar testnet tests use dedicated accounts/assets and handle network flakiness explicitly.
- API integration tests configure `HTTP_ALLOWED_ORIGINS` with exact local or
  HTTPS frontend origins and exercise the session-cookie CSRF boundary.
- No test requires production credentials, mainnet, real identity, or real bank data.

## 4. Domain test matrix

For each on-ramp and off-ramp state transition:

- Valid predecessor and guard produces expected state/event/outbox intent.
- Invalid predecessor is rejected without mutation.
- Duplicate command/event returns existing result.
- Concurrent aggregate version produces conflict and retry/reload behavior.
- Persistence failure rolls back state/event/outbox together.
- Terminal state cannot change.

Value-object tests cover zero/negative/overflow amounts, precision/rounding, currencies, asset issuer/code, Stellar accounts/memos, expiry boundaries, and canonical serialization.

## 5. API tests

Minimum cases:

- Missing, malformed, invalid, and revoked `pk_test_` keys.
- Missing, malformed, expired, revoked, and unverified retail sessions.
- When both credentials are present, the bearer API key takes precedence;
  malformed bearer credentials do not fall back to a session cookie.
- Cross-client, cross-user, and cross-session order access and replay denial.
- Retail history includes orders from earlier valid sessions for the same user
  but excludes API-client orders.
- Cookie-authenticated order POSTs accept an exact configured `Origin`, or a
  matching `Referer` origin only when `Origin` is absent; API-key POSTs do not
  require either header.
- Valid/invalid on-ramp and off-ramp input.
- Required idempotency key, same-key same-body replay, and same-key conflicting-body rejection.
- Same retail-user idempotency key remains replayable after session renewal;
  the same key can be used independently by another retail user.
- Stable error envelope and request ID.
- Cursor pagination ordering and boundaries.
- Exact amount serialization without floating-point drift.
- Rate/body-size/content-type handling.
- Health/readiness responses leak no configuration.
- Profile updates cannot change Auth0-owned email fields; avatar upload enforces authentication, MIME allowlists, and size limits.
- Forgot-password responses do not disclose account existence; password-reset callbacks reject invalid secrets and revoke all local sessions for the provider subject.
- KYC status/inquiry endpoints require an authenticated session, return no raw
  provider payload, and map provider-unavailable errors safely.
- API-key and order creation return stable `KYC_REQUIRED` until the owning
  user's status is approved.

OpenAPI tests validate the document and compare representative request/response examples with the running API.

### TypeScript SDK tests

- Typed create/retrieve/list calls match the validated OpenAPI contract.
- IDR minor units and Stellar amounts never pass through floating-point conversion.
- Idempotency keys and request IDs are propagated correctly.
- API errors map to stable typed SDK errors without exposing credentials or unrestricted response bodies.
- Network retry tests preserve idempotency and surface unknown outcomes.
- Webhook signature vectors pass for valid raw bodies and fail for altered body, timestamp, signature, or secret.
- Authorization is never forwarded to another origin and is absent from thrown errors/log output.
- Package/type/module tests run on every supported Node.js version.

## 6. Payment callback tests

- Valid provider authentication and exact reconciliation.
- Missing/invalid signature/token.
- Wrong amount, currency, checkout ID, order reference, method, or status.
- Duplicate event replay sequentially and concurrently.
- Out-of-order pending/paid/failed events.
- Paid event after order expiry.
- Unknown checkout creation/payment status with reconciliation.
- Unknown checkout responses include the durable order ID and replay returns
  that same order instead of creating a second order.
- Adapter timeout, `429`, transient `5xx`, permanent rejection.

Use sanitized provider fixtures captured from official sandbox behavior. Fixtures contain no reusable credential or real user data.

## 7. Stellar tests

Unit/adapter tests:

- Correct network passphrase and asset construction.
- Destination/trustline validation.
- Exact asset amount and memo/correlation.
- Success, permanent failure, transient failure, and unknown submission mapping.
- Sequence-number conflict and reconciliation.
- Wrong asset/issuer/amount/destination/memo deposit rejection.
- Expired off-ramp deposits are excluded from scanning and transition once to
  `expired` with a matching order-event version.

Testnet evidence tests:

- Successful on-ramp issuance/transfer.
- Successful off-ramp deposit detection.
- Exact successful off-ramp deposit detection and explorer-resolvable deposit hash.
- Simulated retirement has no hash or ledger time; any retirement hash persisted
  before the simulator release is reconciled against Stellar rather than
  replaced with simulated evidence.

Tests must not assume instant ledger availability; use bounded polling with meaningful timeout diagnostics.

## 8. Outbox and worker tests

- State/event/outbox atomic commit and rollback.
- Mapped public order events create one durable webhook event and delivery
  outbox intent in the same transaction as the state transition.
- Multiple workers cannot process one lease concurrently.
- Expired lease can be recovered.
- Retry backoff and max-attempt behavior.
- Unknown external result invokes reconciliation before retry.
- Worker restart resumes durable unprocessed jobs.
- Poison job becomes exhausted/dead-letter-like evidence without blocking the queue.

## 9. Developer webhook tests

- Canonical body and HMAC signature positive/negative vectors.
- Timestamp tolerance and constant-time verification example.
- Stable event ID across retries/manual replay.
- `2xx`, timeout, retryable `429/5xx`, and permanent `4xx` handling.
- Retry schedule and exhaustion.
- Redirect disabled.
- SSRF rejection for loopback, private, link-local, IPv6 local, metadata-service, alternate encoding, and DNS-rebinding scenarios.
- Payload contains no secret or private provider data.

## 10. Developer experience API tests

- Dashboard overview and analytics use owner-scoped data, explicit UTC date
  ranges, stable cursors, initialized empty arrays, and exact integer/string
  amount serialization.
- Revenue summaries and entries read immutable order financial snapshots and
  do not recompute historical fees from mutable order state.
- API-key list responses expose safe metadata only; hashes, plaintext keys,
  signing secrets, and protected secret references never cross the handler
  boundary.
- Wallet profile create/update/delete requires a valid SEP-10 proof, preserves
  one primary wallet per developer, and rejects cross-owner access.
- Webhook endpoint detail, test delivery, delivery history, and replay are
  owner-scoped; replay is limited to exhausted attempts and is idempotent.
- Existing `/v1/onramps`, `/v1/offramps`, and `/v1/orders` route contracts and
  handler behavior remain unchanged while the developer routes are additive.

## 11. SEP-24 and federation tests

- `stellar.toml` syntax, HTTPS location, advertised URLs, asset, and testnet context.
- SEP-10 challenge construction, signature/account/network checks, one-time
  replay rejection, JWT expiry/audience/account checks, and CORS/preflight.
- Deposit/withdraw initiation creates no order before the interactive hand-off;
  browser-token hashing, wallet ownership, retail-session linking, KYC gate,
  idempotent completion, standard forms/JSON, status mapping, history filters,
  and lookup by protocol/Stellar/external ID.
- SEP-38 firm quote creation, ownership, expiry, exact amounts, one-time
  consumption, and SEP-24 quote correlation.
- Testnet payout simulator completion after exact deposit verification, unique
  payout/reference rows, outbox retry/recovery, and explicit no-on-chain-retirement
  and no-real-IDR disclosure.
- Persona sandbox inquiry/status behavior, approved-only gating, and explicit
  non-production KYC wording.
- Federation valid lookup, unknown name, malformed query, and safe output.
- Contract behavior checked against the selected Stellar specification/tooling.

## 12. Security and negative tests

- Secret scanner against repository and release artifacts.
- Dependency vulnerability scan with recorded disposition.
- Injection-shaped input against API/database boundaries.
- Oversized callback/API body.
- Header/log redaction.
- Open redirect and callback return-URL manipulation.
- Persona raw-body signature positive/negative vectors, timestamp tolerance,
  duplicate event replay, conflicting event identity, out-of-order events, and
  approved-status downgrade prevention.
- Webhook SSRF cases.
- Startup fails if environment is configured for mainnet/production in the Instaward deployment.

## 13. Required end-to-end evidence cases

### E2E-BUY-QRIS

Create on-ramp order, obtain real provider sandbox QRIS checkout, complete sandbox payment, process verified callback, issue/transfer test asset, complete order, and capture transaction hash.

### E2E-BUY-BANK

Create on-ramp order using supported sandbox bank transfer/virtual account and verify the same settlement guarantees.

### E2E-SELL

Authenticate an external classic wallet with SEP-10, start an SEP-24 off-ramp,
link a KailoPay retail session, complete the approved-KYC (or guarded test)
flow, send the exact test asset with correlation data, detect and capture the
deposit hash, record simulated retirement without a burn transaction, run the
sandbox payout simulator, and capture disclosed withdrawal evidence. Any
previously submitted retirement hash must be reconciled, not simulated over.

The SOW requires at least two documented flows. The release target should run QRIS buy and sell as the mandatory pair; bank-transfer buy is also required to demonstrate the promised method.

## 13. Test record format

Each formal test record contains:

- Test ID, date/time, tester, release/commit, API version, environment.
- Preconditions and synthetic inputs.
- Steps and expected results.
- Actual result and pass/fail.
- Order ID, provider checkout/event/payout references.
- Stellar public accounts, asset, transaction hashes, and explorer links.
- Sanitized screenshots/log references.
- Defect and retest result where applicable.

## 14. Release quality gate

Before `v0.1.0`:

- Build, lint, unit, repository, API, worker, adapter, contract, and security scans pass.
- No open defect can cause unauthorized/duplicate asset movement or misreport completed settlement.
- Existing `/v1/onramps` and `/v1/offramps` contract tests remain unchanged and pass.
- Required gateway sandbox and Stellar testnet E2E evidence passes.
- OpenAPI matches the deployed API.
- `stellar.toml`, federation, SEP-24, and webhook verification tests pass.
- Public artifacts contain no secrets/real PII.
- Known non-blocking limitations are documented in release notes and Completion Report.

Flaky external tests are not silently re-run until green; record the failed attempt, identify external versus product cause, and retain final evidence.
