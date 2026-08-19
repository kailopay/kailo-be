# Backend Implementation Phases

| Field | Value |
|---|---|
| Delivery window | 30 calendar days |
| Capacity baseline | One full-stack developer, 160 total SOW hours |
| Backend approach | Vertical slices in a modular monolith |
| Release | `v0.1.0` sandbox/testnet |

## 1. Sequencing strategy

Build the smallest end-to-end on-ramp first, then off-ramp, then anchor/developer integration, and finally hardening/evidence. Do not finish every database or framework layer before proving an external vertical slice.

Each slice includes:

- Public/application contract.
- Domain transition.
- Database transaction and outbox intent.
- External adapter behavior.
- Idempotency and failure tests.
- Structured logs and evidence capture.
- Relevant documentation update.

## 2. Phase 0: Decisions and risk probes

**Target:** Days 1-2, inside the 30-day/160-hour SOW window.

### Work

- Test Xendit sandbox Payment Requests v3 for QRIS, BRI virtual account, callback authentication, and status lookup.
- Select Xendit and record `ADR-001-xendit-native-xlm.md`.
- Use native XLM with a pre-funded testnet distribution wallet; no issued KIDR asset or trustline is required in Week 1.
- Decide developer-session mechanism, hosting/public URLs, repository license, secrets, and off-ramp evidence fallback.
- Confirm Go version, PostgreSQL version, Gin/Viper/GORM stack, migration/test strategy, and supported deployment model.

### Exit gate

- Provider checkout and callback mechanism are demonstrated.
- Testnet transaction is confirmed.
- All blocking decisions in backend `README.md` are resolved or have approved fallback.
- P0 backlog is re-estimated against remaining capacity.

## 3. Week 1: Platform, order API, gateway checkout

### Outcome

A developer can authenticate, create/query sandbox orders, and receive real provider sandbox QRIS/bank-transfer checkout instructions. Verified callbacks update local payment state once.

### Build order

1. Project/runtime/config structure and CI.
2. PostgreSQL migrations, repository transaction helper, IDs/clock, and structured logging.
3. API-key creation/verification and client ownership boundary.
4. Order aggregate, state transition policy, event history, idempotency records, and outbox.
5. Create/get/list on-ramp API contracts; defer off-ramp.
6. Selected provider adapter for QRIS and bank transfer.
7. Callback raw-body verification, deduplication, reconciliation, and durable settlement intent.
8. Initial OpenAPI and integration tests.
9. Native-XLM settlement worker with hash-first persistence and Horizon reconciliation.

### Week 1 gate

- Fresh checkout is created through the real provider sandbox.
- Callback with invalid authentication cannot mutate an order.
- Replayed paid callback creates one payment-confirmed transition and one settlement intent.
- Clean database can migrate and API tests pass.
- Public repository contains no secret.
- A verified payment creates one durable settlement job; the worker confirms one
  native-XLM testnet transfer before completing the order.

## 4. Week 2: Off-ramp and SEP-24

### Outcome

Build off-ramp and anchor capabilities on the Week 1 native-XLM settlement foundation.

### Build order

1. Off-ramp deposit instructions, transaction lookup/detection, and exact validation.
2. Sandbox payout intent and unknown-outcome reconciliation.
4. Gateway payout/simulation intent and reconciliation.
5. SEP-24 deposit/withdrawal interactive endpoints and synthetic KYC stub.
6. Public `stellar.toml` and federation endpoint.
7. Developer webhook event/outbox foundation.

### Week 2 gate

- Internal E2E on-ramp completes with transaction hash.
- Internal E2E off-ramp records deposit, retirement, and payout/simulation reference.
- Unknown Stellar result is reconciled before retry in automated tests.
- `stellar.toml` and federation configuration validate.
- SEP-24 state mapping does not contradict internal order state.

## 5. Week 3: Public integration, webhooks, deployment, formal tests

### Outcome

The backend is publicly reachable and independently integrable by the web app and external developers.

### Build order

1. Complete developer API-key/webhook endpoint management.
2. Implement webhook signing, SSRF policy, bounded retry, attempt history, and documentation vectors.
3. Finalize public order representations, pagination, error catalog, and OpenAPI.
4. Implement the thin TypeScript SDK, webhook verifier, package docs, and SDK-to-Go-API contract tests.
5. Add health/readiness, metrics, reconciliation queries/jobs, and safe diagnostic views/queries.
6. Deploy migration, API, worker, PostgreSQL, discovery endpoints, and docs through HTTPS.
7. Run formal QRIS buy, bank-transfer buy, and sell evidence tests using the released API/SDK where appropriate.
8. Execute security/secret/contract checks.

### Week 3 gate

- Web frontend can use the deployed backend without private/manual database changes.
- OpenAPI examples pass against the release candidate.
- TypeScript SDK contract tests pass against the same Go release candidate, and no browser example embeds a `pk_test_` key.
- Outgoing webhook signature verifies and retry is observable.
- Public endpoints use HTTPS and leak no configuration/secrets.
- Required testnet hashes and provider sandbox references are recorded.

## 6. Week 4: Hardening, release, and handover

### Outcome

Backend `v0.1.0` is reproducible, documented, safely demonstrable, and mapped to SOW evidence.

### Work

- Resolve release-blocking correctness/security defects.
- Run full regression, callback replay/concurrency, unknown-outcome, and E2E tests.
- Verify database migration from clean state and deployment/rollback procedures.
- Finalize architecture, API, integration, security, testing, and runbook documentation.
- Produce sanitized evidence matrix and Completion Report inputs.
- Confirm stable public review URLs and create `v0.1.0` tag.

### Release gate

- All backend P0 stories meet acceptance criteria.
- No open defect can create unauthorized/duplicate settlement or false success.
- Build, lint, tests, contract validation, dependency scan, and secret scan pass.
- Gateway sandbox QRIS and bank-transfer evidence exists.
- On-ramp and off-ramp testnet evidence exists.
- SEP-24, `stellar.toml`, federation, and developer webhook evidence exists.
- OpenAPI, release code, deployment, and evidence use the same release version.

## 7. Backend cut line

Cut or simplify first:

1. Nonessential list filters and diagnostic UI.
2. Optional webhook event types and secret rotation UI.
3. Advanced metrics/dashboard packaging; keep structured logs and core counters.
4. Nonessential federation features; retain minimum valid configuration/service.
5. Additional adapter abstractions that are not needed by the selected provider.

Do not cut without written SOW change:

- Test API key authentication.
- On/off-ramp create and order-status API.
- QRIS and bank-transfer sandbox integration.
- Authenticated/idempotent gateway callback processing.
- Testnet issuance/transfer and retirement with transaction hashes.
- SEP-24 deposit/withdrawal skeleton, KYC stub, `stellar.toml`, and federation.
- Developer webhook delivery and logs.
- Public deployment, OpenAPI, formal E2E evidence, and Completion Report inputs.

## 8. Daily engineering rhythm

- Start: review P0 blocker, external-service status, and current vertical slice.
- Build: keep one primary story in progress and integrate continuously.
- Verify: run affected unit/integration tests before moving state.
- Evidence: capture stable references as flows complete.
- End: update backlog, risks/ADRs, docs, and remaining capacity.

Avoid postponing all integration and documentation to Week 4.

## 9. Change control

Every change request records:

- Product requirement/SOW deliverable supported.
- Backend components and contracts affected.
- New security/data/external dependency risk.
- Added estimate and displaced backlog item.
- Acceptance and evidence impact.
- Approver and decision date.

Unapproved work remains P2/Future.

## 10. Handover package

Backend handover must include:

- Release tag and deployment URLs/version.
- Validated OpenAPI and environment/config reference.
- Migration and local setup commands.
- Selected-provider ADR and integration guide.
- Stellar accounts/asset public configuration without secrets.
- `stellar.toml`, federation, SEP-24 verification steps.
- Test results and transaction/provider/webhook evidence.
- Deployment/recovery runbook and known limitations.
- Rotatable secret inventory by name/owner, never values.
