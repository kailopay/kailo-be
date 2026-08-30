# Backend Testing Strategy

## 1. Objectives

Testing must prove both business outcomes and failure safety:

- A verified payment causes one on-ramp settlement.
- A verified asset deposit and retirement precede one off-ramp withdrawal result.
- Duplicates, concurrency, timeouts, and unknown external outcomes do not create duplicate value movement.
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
- PostgreSQL tests run against the same supported major version as deployment. Set `TEST_DATABASE_DSN` to a disposable database to run the repository integration suite in `internal/repository`; the tests apply the versioned migrations, verify constraints and transactions, and skip when the variable is unset.
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

Testnet evidence tests:

- Successful on-ramp issuance/transfer.
- Successful off-ramp deposit detection.
- Successful burn/retirement.
- Stored hashes resolve on a public testnet explorer and match order evidence.

Tests must not assume instant ledger availability; use bounded polling with meaningful timeout diagnostics.

## 8. Outbox and worker tests

- State/event/outbox atomic commit and rollback.
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

## 10. SEP-24 and federation tests

- `stellar.toml` syntax, HTTPS location, advertised URLs, asset, and testnet context.
- Deposit/withdraw initiation and status mapping.
- KYC stub warning and synthetic-data behavior.
- Federation valid lookup, unknown name, malformed query, and safe output.
- Contract behavior checked against the selected Stellar specification/tooling.

## 11. Security and negative tests

- Secret scanner against repository and release artifacts.
- Dependency vulnerability scan with recorded disposition.
- Injection-shaped input against API/database boundaries.
- Oversized callback/API body.
- Header/log redaction.
- Open redirect and callback return-URL manipulation.
- Webhook SSRF cases.
- Startup fails if environment is configured for mainnet/production in the Instaward deployment.

## 12. Required end-to-end evidence cases

### E2E-BUY-QRIS

Create on-ramp order, obtain real provider sandbox QRIS checkout, complete sandbox payment, process verified callback, issue/transfer test asset, complete order, and capture transaction hash.

### E2E-BUY-BANK

Create on-ramp order using supported sandbox bank transfer/virtual account and verify the same settlement guarantees.

### E2E-SELL

Create off-ramp order, send correct test asset with correlation data, detect deposit, retire asset, initiate sandbox payout or approved simulation, and capture both Stellar and withdrawal evidence.

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
- Required gateway sandbox and Stellar testnet E2E evidence passes.
- OpenAPI matches the deployed API.
- `stellar.toml`, federation, SEP-24, and webhook verification tests pass.
- Public artifacts contain no secrets/real PII.
- Known non-blocking limitations are documented in release notes and Completion Report.

Flaky external tests are not silently re-run until green; record the failed attempt, identify external versus product cause, and retain final evidence.
