# Weeks 1 and 2 backend checkpoint

| Field | Value |
|---|---|
| Checkpoint date | 2026-09-16 |
| Status | Implementation complete with documented limitations |
| Scope | Sandbox payments, Stellar testnet, authenticated API, and anchor integration |
| Release status | Not a formal release sign-off |

## Decision

Close Weeks 1 and 2 as implementation milestones. Keep formal acceptance and
release work open until the evidence, reconciliation, webhook, testing, and
deployment gates in this document are complete.

The current backend gives the frontend enough contract to build the account,
KYC, buy, order-history, developer-key, discovery, and SEP-24 screens. The
frontend must keep the sell transfer step disabled until the off-ramp deposit
account is present in the response.

## Week 1 result

The Week 1 implementation includes:

- A public repository with a Go module, package layout, `LICENSE`, `README.md`,
  migrations, API, worker, and configuration examples.
- PostgreSQL persistence with order state, order events, idempotency records,
  provider receipts, Stellar intents, and transactional outbox rows.
- Self-hosted email authentication, optional Google sign-in, session cookies,
  password recovery, profile editing, private avatars, API keys, and owner
  checks.
- `POST /v1/quotes`, `POST /v1/onramps`, `POST /v1/offramps`, `GET /v1/orders`,
  and `GET /v1/orders/{id}` with session or server API-key authentication.
- Xendit Payment Session creation with all-channel, QRIS-only, and BRI Virtual
  Account-only request modes.
- Signed Xendit callback validation, callback deduplication, amount and
  currency checks, and one durable settlement intent per paid order.
- Initial OpenAPI coverage for the public API and the frontend integration
  guide.

The Week 1 automated Go test suite passes with `go test -count=1 ./...`.

The Week 1 external evidence is not complete. The repository has a recorded
Xendit sandbox session, but QRIS and BRI Virtual Account screenshots, a
sanitized callback log, and a Stellar settlement hash are still missing.

## Week 2 result

The Week 2 implementation includes:

- Native XLM transfer on Stellar testnet from a pre-funded treasury account.
- Off-ramp deposit instructions, exact amount and memo matching, Horizon
  payment-history scanning, expiry handling, and burn-address retirement. The
  payout step is intentionally deferred; new orders remain in
  `withdrawal_processing` after retirement.
- Authenticated SEP-24 deposit and withdrawal initiation, transaction lookup,
  interactive transaction projection, and internal order-state mapping.
- Persona inquiry creation or resumption, signed callback processing, durable
  KYC status, and an approval gate for API keys and orders.
- Public `stellar.toml`, federation lookup, and SEP-24 asset information.
- Sanitized order-event persistence and delivery intents for developer
  webhooks.

The Week 2 implementation uses the accepted sandbox decisions in
`ADR-001-xendit-native-xlm.md`, `ADR-003-offramp-retirement-payout.md`,
`ADR-004-persona-kyc-gate.md`, and the current payout deferral in
`ADR-005-defer-offramp-payout.md`. It uses native XLM and does not claim
production IDR movement, Stellar mainnet custody, SEP-10 wallet
authentication, or a wallet-facing HTML SEP-24 page.

## Acceptance matrix

| Area | Checkpoint result | Remaining condition |
|---|---|---|
| Repository, license, and project structure | Implemented | Confirm the release branch and public repository evidence at release time. |
| Go, PostgreSQL, authentication, and owner checks | Implemented | Complete the remaining integration and security test suites. |
| On-ramp API and order state | Implemented | Capture real QRIS and BRI Virtual Account sandbox evidence. |
| Xendit callbacks and settlement intent | Implemented | Complete provider status reconciliation for unknown checkout outcomes. |
| Native XLM testnet settlement | Implemented | Capture a real settlement hash and expose complete public order evidence. |
| Off-ramp deposit and retirement | Implemented; payout deferred | Capture sell-flow evidence. A confirmed retirement leaves the order in `withdrawal_processing` until a payout rail is enabled. |
| SEP-24 and discovery | Implemented for the authenticated sandbox flow | Keep wallet interoperability and production anchor behavior outside this checkpoint. |
| Persona KYC gate | Implemented | Exercise signed, duplicate, invalid, and out-of-order callback cases in the release test suite. |
| Developer webhook events | Event records and outbox intents implemented | Add SSRF policy, signing, delivery, retries, and delivery attempt history. |
| Formal tests, public deployment, and E2E evidence | Incomplete | Finish the release gates below. |

## Open gates before formal acceptance

The following items remain open. They are not hidden by closing the
implementation checkpoint:

1. Capture QRIS and BRI Virtual Account sandbox screenshots and a sanitized
   callback request and response record.
2. Capture on-ramp and off-ramp Stellar testnet hashes and verify them against
   the public order records.
3. Implement provider reconciliation for a checkout request whose result is
   unknown after the provider call.
4. Complete webhook URL revalidation and SSRF controls, HMAC signing, the
   delivery worker, bounded retries, attempt records, and replay guidance.
5. Complete the missing domain, gateway, Stellar, API, and concurrency tests.
6. Enable and reconcile a real payout rail before allowing
   `withdrawal_processing` to become `completed`.
7. Deploy the API, worker, database, discovery endpoints, and documentation
   through public HTTPS URLs, then run the formal buy and sell flows.

## Frontend handoff

Use [Frontend capabilities and backend integration](FRONTEND-CAPABILITIES.md)
as the browser implementation reference. The frontend can implement the
following now:

- Registration, email verification, login, logout, Google redirect, password
  recovery, profile, and avatar screens.
- Persona inquiry launch and KYC status polling.
- IDR to XLM buy flow with hosted checkout presentation and order polling.
- Order history and order detail views with cursor pagination.
- Developer Mode and API-key management. Keep the full API key out of browser
  storage.
- Public anchor discovery and authenticated JSON SEP-24 screens.

The frontend must show sandbox and Stellar testnet labels, preserve
idempotency keys across uncertain retries, and keep a sell order in a
non-success state after retirement because the payout step is deferred.

The frontend must not call provider callbacks, use a `pk_test_` key in browser
code, treat a checkout redirect as payment confirmation, or present the
synthetic withdrawal destination as a real bank account.

## Source documents

- [Backend backlog](BACKEND-BACKLOG.md)
- [Implementation phases](IMPLEMENTATION-PHASES.md)
- [Week 1 evidence checklist](WEEK1-EVIDENCE.md)
- [Frontend capabilities and backend integration](FRONTEND-CAPABILITIES.md)
- [OpenAPI contract](../../openapi/openapi.yaml)
