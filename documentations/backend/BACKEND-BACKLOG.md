# Backend Backlog

| Field | Value |
|---|---|
| Release target | `v0.1.0` sandbox/testnet |
| Estimate | Relative story points (SP), recalibrated after Phase 0 |
| Capacity context | One full-stack developer, 160 total SOW hours |
| Status values | Ready, Blocked, In Progress, In Review, Done, Removed, Future |

## 1. Priority policy

- **P0:** required for SOW completion, settlement correctness, or critical security.
- **P1:** important for maintainability, operations, or developer experience; deliver after P0 is safe.
- **P2:** beneficial but first to cut.
- **Future:** production/mainnet or expansion work outside the Instaward.

Story points are not hours. Phase 0 must validate the P0 total against actual velocity and external-provider uncertainty.

## 2. Definition of Ready

- Requirement and SOW outcome are identified.
- Dependencies/credentials are present or a verified fake/fallback is defined.
- Acceptance and evidence are testable.
- External effect and idempotency behavior are explicit.
- No production/mainnet/KYC/regulatory scope is implied.

## 3. Definition of Done

- Code and migration/config changes are reviewed.
- Unit/integration/contract/negative tests pass as relevant.
- State/event/outbox behavior is atomic and idempotent.
- Logs/metrics are useful and sanitized.
- API and technical docs match behavior.
- SOW evidence link/reference is recorded where applicable.
- No secrets or real personal/payment data are committed or published.

## 4. Active backlog

### BE0: Decisions and foundation

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-001 | Probe Xendit/Midtrans sandbox and record provider ADR. | P0 | 3 | Phase 0 | Sandbox accounts | Ready |
| BE-002 | Define Stellar test asset/accounts/retirement and record ADR. | P0 | 2 | Phase 0 | Testnet access | Ready |
| BE-003 | Decide off-ramp sandbox evidence fallback and document wording. | P0 | 1 | Phase 0 | BE-001 | Blocked |
| BE-004 | Select Auth0 email login with KailoPay-owned PostgreSQL retail sessions for test-key management. | P0 | 2 | Phase 0 | Product decision | Done |
| BE-005 | Decide hosting, public URL topology, and managed secrets. | P0 | 2 | Phase 0 | Hosting access | Ready |
| BE-006 | Initialize Go module, dependency lock files, commands, license, and package layout. | P0 | 2 | Phase 0 | None | Ready |
| BE-007 | Configure CI build, lint, unit/integration test, dependency, and secret scans. | P1 | 3 | Week 1 | BE-006 | Blocked |

Acceptance: ADRs contain context/options/consequences; repository builds from clean checkout; no reusable credential exists in source/history.

### BE1: Platform and persistence

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-010 | Implement validated startup configuration and sandbox/testnet assertions. | P0 | 3 | Week 1 | BE-006 | Blocked |
| BE-011 | Add PostgreSQL connection, transaction helper, migration tooling, and clean-schema CI test. | P0 | 5 | Week 1 | BE-006 | Blocked |
| BE-012 | Implement IDs, injected clock, exact amount types, and canonical serialization. | P0 | 3 | Week 1 | BE-006 | Blocked |
| BE-013 | Implement request/correlation context and JSON logging with central redaction. | P0 | 3 | Week 1 | BE-006 | Blocked |
| BE-014 | Implement error taxonomy and stable HTTP error mapper. | P0 | 3 | Week 1 | BE-006 | Blocked |
| BE-015 | Implement transactional outbox schema, leasing, retry, and worker loop. | P0 | 5 | Week 1 | BE-011 | Blocked |
| BE-016 | Add liveness, readiness, release version, and migration checks. | P1 | 2 | Week 1 | BE-010, BE-011 | Blocked |
| BE-017 | Add sandbox seed/test-fixture tooling using synthetic data. | P1 | 2 | Week 1 | BE-011 | Blocked |

Acceptance: state/event/outbox commit atomically; multiple workers cannot hold one lease; logs contain correlation IDs and no secret fields.

### BE2: Identity and order domain/API

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-020 | Implement API client and `pk_test_` key generation, hashed storage, authentication, and revocation. | P0 | 5 | Week 1 | BE-011-BE-014 | Blocked |
| BE-021 | Implement order aggregate/value objects and on/off-ramp state-transition policy. | P0 | 5 | Week 1 | BE-012 | Blocked |
| BE-022 | Implement order/order-event repositories with optimistic concurrency. | P0 | 5 | Week 1 | BE-011, BE-021 | Blocked |
| BE-023 | Implement idempotency records and same-key conflicting-request detection. | P0 | 3 | Week 1 | BE-011 | Blocked |
| BE-024 | Implement `POST /v1/onramps` with validation and checkout intent. | P0 | 5 | Week 1 | BE-020-BE-023 | Blocked |
| BE-025 | Implement `POST /v1/offramps` with deposit instruction intent. | P0 | 5 | Week 1-2 | BE-020-BE-023 | Blocked |
| BE-026 | Implement client-scoped `GET /v1/orders/{id}`. | P0 | 2 | Week 1 | BE-020, BE-022 | Blocked |
| BE-027 | Implement cursor-paginated `GET /v1/orders`. | P1 | 3 | Week 3 | BE-026 | Blocked |
| BE-028 | Publish and validate initial OpenAPI with auth, amounts, errors, and examples. | P0 | 3 | Week 1 | BE-024-BE-026 | Blocked |
| BE-029 | Implement Auth0 profile editing, hosted password reset, local-session revocation, and private MinIO avatars. | P1 | 5 | Week 1 | BE-010, BE-011 | Implemented |

Acceptance: exact amount round-trips; invalid/cross-client requests fail; duplicate create returns one order; each legal transition creates one versioned event.

### BE3: Payment gateway

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-030 | Define provider-neutral payment port and selected-provider adapter mapping. | P0 | 3 | Week 1 | BE-001 | Blocked |
| BE-031 | Create real QRIS sandbox checkout with stable external/idempotency reference. | P0 | 5 | Week 1 | BE-024, BE-030 | Blocked |
| BE-032 | Create real bank-transfer/virtual-account sandbox checkout. | P0 | 3 | Week 1 | BE-024, BE-030 | Blocked |
| BE-033 | Implement raw-body callback authentication and sanitized receipt persistence. | P0 | 5 | Week 1 | BE-030, BE-011 | Blocked |
| BE-034 | Implement callback deduplication and amount/currency/reference/status reconciliation. | P0 | 5 | Week 1 | BE-033, BE-021 | Blocked |
| BE-035 | Persist one settlement intent after verified payment and process asynchronously. | P0 | 3 | Week 1-2 | BE-015, BE-034 | Blocked |
| BE-036 | Implement provider payment status reconciliation for timeout/late/unknown events. | P0 | 5 | Week 2 | BE-030, BE-034 | Blocked |
| BE-037 | Implement off-ramp payout/simulation intent, provider adapter, and reconciliation. | P0 | 5 | Week 2 | BE-003, BE-030 | Blocked |

Acceptance: invalid/mismatched/replayed callback cannot duplicate state/settlement; QRIS and bank transfer produce real sandbox checkout evidence; payout language matches evidence.

### BE4: Stellar testnet settlement

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-040 | Implement Stellar configuration, network assertion, account/asset validation, and public metadata. | P0 | 3 | Week 1-2 | BE-002, BE-010 | Blocked |
| BE-041 | Implement serialized/account-locked transaction submission and persistent intent/result model. | P0 | 5 | Week 2 | BE-011, BE-040 | Blocked |
| BE-042 | Implement on-ramp issuance/transfer worker with deterministic correlation. | P0 | 5 | Week 2 | BE-035, BE-041 | Blocked |
| BE-043 | Implement unknown submission reconciliation before retry. | P0 | 5 | Week 2 | BE-041 | Blocked |
| BE-044 | Generate off-ramp deposit instructions with exact asset/amount/memo and expiry. | P0 | 3 | Week 2 | BE-025, BE-040 | Blocked |
| BE-045 | Detect/look up and validate off-ramp deposit transaction. | P0 | 5 | Week 2 | BE-044 | Blocked |
| BE-046 | Implement burn/retirement worker and transaction correlation. | P0 | 5 | Week 2 | BE-045, BE-041 | Blocked |
| BE-047 | Link transaction hashes/explorer metadata to public order result. | P0 | 2 | Week 2-3 | BE-042, BE-046 | Blocked |

Acceptance: one paid order creates one testnet settlement; wrong deposits are rejected; unknown result reconciles before retry; off-ramp payout cannot precede confirmed retirement.

### BE5: SEP-24 and discovery

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-050 | Implement SEP-24 deposit initiation mapped to on-ramp order. | P0 | 5 | Week 2 | BE-024 | Blocked |
| BE-051 | Implement SEP-24 withdrawal initiation mapped to off-ramp order. | P0 | 5 | Week 2 | BE-025 | Blocked |
| BE-052 | Implement interactive synthetic KYC stub and explicit warning. | P0 | 2 | Week 2 | BE-050, BE-051 | Blocked |
| BE-053 | Implement/document SEP-facing transaction status mapping. | P0 | 3 | Week 2 | BE-021, BE-050 | Blocked |
| BE-054 | Publish and validate testnet `stellar.toml`. | P0 | 2 | Week 2 | BE-005, BE-040 | Blocked |
| BE-055 | Implement minimal public federation lookup/configuration. | P0 | 3 | Week 2 | BE-005, BE-040 | Blocked |

Acceptance: discovery resolves to implemented HTTPS endpoints; deposit/withdraw flows create valid orders; KYC never claims verification or stores real identity documents.

### BE6: Developer webhooks

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-060 | Implement webhook endpoint registration/list/disable with one-time signing secret. | P0 | 5 | Week 2-3 | BE-004, BE-020 | Blocked |
| BE-061 | Implement SSRF URL policy, DNS/IP revalidation, timeout, and redirect controls. | P0 | 5 | Week 2-3 | BE-060 | Blocked |
| BE-062 | Define/version public event catalog and canonical payloads. | P0 | 3 | Week 2 | BE-021 | Blocked |
| BE-063 | Persist public events transactionally from order events/outbox. | P0 | 3 | Week 2 | BE-015, BE-062 | Blocked |
| BE-064 | Implement HMAC signing and delivery worker with attempt logs. | P0 | 5 | Week 2-3 | BE-061, BE-063 | Blocked |
| BE-065 | Implement bounded retry, exhaustion, and manual sandbox replay using the same event ID. | P1 | 3 | Week 3 | BE-064 | Blocked |
| BE-066 | Publish signature verification sample/vector and at-least-once guidance. | P0 | 2 | Week 3 | BE-064 | Blocked |

Acceptance: event is durable with state transition; SSRF cases fail; signature vector verifies; duplicate attempts preserve event ID; payload contains no secrets.

### BE7: Observability, security, and operations

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-070 | Add HTTP/order/gateway/Stellar/outbox/webhook metrics without high-cardinality labels. | P1 | 3 | Week 3 | Core flows | Blocked |
| BE-071 | Add reconciliation queries/jobs for aged non-terminal orders and unknown effects. | P0 | 5 | Week 2-3 | BE-036, BE-043 | Blocked |
| BE-072 | Add per-IP/per-key rate and payload/time limits. | P0 | 3 | Week 3 | BE-020 | Blocked |
| BE-073 | Harden CORS/headers/TLS assumptions and public error/config exposure. | P0 | 2 | Week 3 | HTTP deployment | Blocked |
| BE-074 | Verify secret injection/redaction and create rotation/exposure runbook. | P0 | 3 | Week 3 | BE-005, BE-013 | Blocked |
| BE-075 | Produce deployment, rollback, and external-unknown recovery procedures. | P0 | 3 | Week 3-4 | Core flows | Blocked |
| BE-076 | Create safe operator/reviewer evidence queries and sanitized export process. | P1 | 3 | Week 3 | Core schema | Blocked |

Acceptance: an order is traceable across all systems; secrets are absent from logs/artifacts; unknown effects have safe documented recovery; public endpoints pass baseline security checks.

### BE8: Testing, deployment, documentation, and release

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-080 | Build domain transition/invariant unit test suite. | P0 | 5 | Week 1-2 | BE-021 | Blocked |
| BE-081 | Build PostgreSQL repository, constraint, transaction, lease, and concurrency tests. | P0 | 5 | Week 1-2 | BE-011, BE-015, BE-022 | Blocked |
| BE-082 | Build API auth, ownership, validation, idempotency, error, and OpenAPI contract tests. | P0 | 5 | Week 1-3 | BE-020, BE-024-BE-028 | Blocked |
| BE-083 | Build gateway callback replay/mismatch/timeout/reconciliation tests. | P0 | 5 | Week 1-2 | BE-033-BE-036 | Blocked |
| BE-084 | Build Stellar success/failure/unknown/deposit-validation tests and testnet harness. | P0 | 5 | Week 2 | BE-041-BE-046 | Blocked |
| BE-085 | Build webhook signing/retry/SSRF test suite. | P0 | 5 | Week 3 | BE-061-BE-065 | Blocked |
| BE-086 | Deploy migration, API, worker, database, discovery, and docs through public HTTPS URLs. | P0 | 5 | Week 3 | BE-005, core flows | Blocked |
| BE-087 | Run/capture formal QRIS buy, bank-transfer buy, and sell E2E evidence. | P0 | 5 | Week 3 | BE-086 | Blocked |
| BE-088 | Finalize OpenAPI, backend docs, test results, limitations, and Completion Report inputs. | P0 | 3 | Week 4 | BE-087 | Blocked |
| BE-089 | Execute final security/secret/dependency/regression gate and tag `v0.1.0`. | P0 | 3 | Week 4 | All P0 | Blocked |

Acceptance: tests and scans pass on the release commit; public URLs/evidence match the tag; E2E hashes/references are valid; no P0 document contains unresolved implementation placeholders.

### BE9: TypeScript SDK

| ID | Story | Pri | SP | Target | Dependency | Status |
|---|---|---:|---:|---|---|---|
| BE-090 | Define the TypeScript SDK public client, runtime/module support, versioning, and compatibility policy. | P0 | 2 | Week 3 | BE-028 | Blocked |
| BE-091 | Implement typed on-ramp/off-ramp create and order retrieve/list methods against the OpenAPI contract. | P0 | 5 | Week 3 | BE-028, BE-090 | Blocked |
| BE-092 | Implement exact amount serialization, idempotency propagation, timeouts, safe retries, and typed error mapping. | P0 | 5 | Week 3 | BE-091 | Blocked |
| BE-093 | Implement framework-neutral webhook signature verification using raw body bytes and constant-time comparison. | P0 | 3 | Week 3 | BE-066, BE-090 | Blocked |
| BE-094 | Add Node.js/type/package/API contract tests, documentation, examples, and browser API-key warning. | P0 | 5 | Week 3 | BE-091-BE-093 | Blocked |

Acceptance: the SDK remains a thin TypeScript client; contract tests pass against the Go API; exact amounts are preserved; webhook vectors verify; API keys are never embedded in browser examples or emitted in errors/logs.

## 5. Recommended critical path

```text
BE-001/002/005/006
  -> BE-010/011/012/013/014/015
  -> BE-020/021/022/023/024
  -> BE-030/031/033/034/035
  -> BE-040/041/042/043
  -> first on-ramp E2E
  -> BE-025/037/044/045/046
  -> first off-ramp E2E
  -> BE-050..055 and BE-060..066
  -> BE-090..094
  -> BE-086/087/088/089
```

Testing stories run alongside their corresponding implementation; they are shown separately for tracking, not deferred to the end.

## 6. Future backlog

| ID | Item | Status |
|---|---|---|
| BE-F001 | Production gateway contracts, credentials, certification, refunds, disputes, and multi-provider routing | Future |
| BE-F002 | Production KYC/KYB, sanctions/PEP/AML monitoring, case management, and identity retention | Future |
| BE-F003 | Mainnet custody/HSM/MPC/multisig, key ceremony, rotation, and emergency controls | Future |
| BE-F004 | Treasury, liquidity, pricing, fees, reserves, and three-way reconciliation ledger | Future |
| BE-F005 | Production SLOs, HA, autoscaling, disaster recovery, on-call, and incident communications | Future |
| BE-F006 | Independent penetration test, continuous vulnerability management, and access governance | Future |
| BE-F007 | Customer support/refund/complaint/manual-review operator tooling | Future |
| BE-F008 | Merchant API, settlement, reporting, and dashboard backend | Future |
| BE-F009 | White-label multi-user configuration and isolation backend | Future |
| BE-F010 | Additional assets, networks, currencies, and geographic corridors | Future |

## 7. Backlog governance

Review twice weekly:

- P0 complete/in-progress/blocked and evidence generated.
- External provider/testnet risks and ADR changes.
- Remaining capacity and P1/P2 cuts.
- Defects affecting unauthorized/duplicate settlement or false completion.
- Scope changes and displaced items.

Removing or changing a P0 item requires a documented mapping to the SOW impact and approval.
