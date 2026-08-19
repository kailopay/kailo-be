# KailoPay Product Backlog

| Field | Value |
|---|---|
| Version | 0.1 |
| Status | Initial backlog |
| Delivery target | Instaward Sandbox MVP `v0.1.0` |
| Capacity assumption | One full-stack developer, 160 hours over 30 calendar days |
| Estimation unit | Relative story points (SP), not hours |

## 1. How to use this backlog

- **P0 - Must:** required to satisfy the SOW or safely demonstrate its core value flow.
- **P1 - Should:** important quality, operability, or reviewer-experience work; deliver after the P0 path is secure.
- **P2 - Could:** useful enhancement that may be cut without invalidating the agreed MVP.
- **Future:** explicitly outside the Instaward.

Story points express relative uncertainty and complexity. They must be calibrated after Phase 0; they do not imply that all listed work fits automatically into 160 hours. The Phase 1 cut line in `PHASES.md` controls scope when capacity changes.

Initial status values are `Ready`, `Blocked`, or `Future`. During execution, use `In Progress`, `In Review`, `Done`, or `Removed` and record a reason for removed P0 work.

## 2. Definition of Ready

A story is ready when:

- Its user or system outcome is clear.
- Dependencies and external credentials are available or have a tested fallback.
- Acceptance criteria and evidence are understood.
- The story is small enough to complete and review within a few working days.
- It does not introduce production payment, mainnet, real KYC, licensing, mobile, merchant, or white-label scope.

## 3. Definition of Done

A story is done when:

- Acceptance criteria pass in the intended sandbox/testnet environment.
- Relevant automated tests and negative cases pass.
- Logs contain useful correlation data without secrets or sensitive data.
- Public API or user behavior changes are documented.
- Database migrations and configuration examples are reproducible.
- Evidence is linked when the story contributes to an SOW deliverable.
- Code is reviewed and integrated into the release branch.

## 4. Active Instaward backlog

### Epic E0: Mobilization and scope control

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-001 | As the delivery team, we need a public repository, license, base README, and project structure so the work is reviewable from day one. | P0 | 2 | Phase 0 | FR-070 | Ready |
| KLP-002 | As the team, we need to choose Xendit or Midtrans based on verified sandbox capabilities so integration risk is reduced early. | P0 | 2 | Phase 0 | DEC-01, FR-020 | Ready |
| KLP-003 | As the team, we need to define the test asset, issuer/distributor model, and burn/retirement flow so on-chain behavior is unambiguous. | P0 | 2 | Phase 0 | DEC-02, FR-030 | Ready |
| KLP-004 | As the team, we need hosting, domain, secret-management, and review-window decisions so public evidence can be delivered reliably. | P0 | 2 | Phase 0 | DEC-05 | Ready |
| KLP-005 | As the owner, I need a decision and risk log so unresolved assumptions do not silently change SOW scope. | P1 | 1 | Week 1 | Scope control | Ready |

#### Epic E0 acceptance

- Repository is public and contains no application secret.
- Gateway decision records QRIS, bank-transfer, callback-authentication, and payout-sandbox findings.
- Test asset configuration includes code, issuer, distribution/processing account, precision, and retirement behavior.
- Hosting decision identifies public app, API, documentation, `stellar.toml`, and federation URLs.

### Epic E1: Service and data foundation

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-010 | As a developer, I need a Go/Gin service with configuration validation and health endpoint so local and hosted environments start predictably. | P0 | 3 | Week 1 | NFR-012 | Ready |
| KLP-011 | As a developer, I need PostgreSQL migrations for clients, orders, events, payments, Stellar transactions, and webhooks so state is durable. | P0 | 5 | Week 1 | Data requirements | Ready |
| KLP-012 | As an API consumer, I need a consistent validation and error envelope so failures are actionable. | P0 | 3 | Week 1 | FR-015 | Ready |
| KLP-013 | As an operator, I need structured correlated logs so an order can be traced across payment, database, Stellar, and webhook processing. | P0 | 3 | Week 1 | NFR-008 | Ready |
| KLP-014 | As a maintainer, I need CI for build, lint, tests, and secret checks so the public release remains reproducible and safe. | P1 | 3 | Week 1 | NFR-001, NFR-012 | Ready |
| KLP-015 | As a maintainer, I need `.env.example`, migration, seed, and local-run instructions so reviewers can reproduce the service. | P1 | 2 | Week 1 | NFR-012 | Ready |
| KLP-016 | As a retail sandbox user, I can use a short-lived session and access only my own orders so web history is private. | P0 | 3 | Week 1 | FR-005-FR-007 | Ready |
| KLP-017 | As an authenticated user, I can enable Developer Mode and manage only my own API clients, keys, and webhooks through a protected session. | P0 | 3 | Week 1 | FR-005, FR-008-FR-009 | Ready |

#### Epic E1 acceptance

- A clean environment can start the API and database from documented commands.
- Schema migration up and down paths are tested in a disposable database.
- Error responses include a stable code, message, and request/correlation ID but no stack trace or secret.
- CI fails on build/test failure or detected committed credentials.

### Epic E2: Test API keys and order API

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-020 | As an integrating developer, I can create a `pk_test_` key so I can authenticate sandbox API requests. | P0 | 3 | Week 1 | FR-001-FR-003 | Ready |
| KLP-021 | As an integrating developer, I can revoke a test key so compromised test access can be disabled. | P1 | 2 | Week 3 | FR-004 | Ready |
| KLP-022 | As an integrating developer, I can create an on-ramp order with IDR amount, payment method, asset, and Stellar destination. | P0 | 5 | Week 1 | FR-010, FR-013-FR-016 | Ready |
| KLP-023 | As an integrating developer, I can create an off-ramp order with asset amount and sandbox IDR withdrawal details. | P0 | 5 | Week 1 | FR-011, FR-013-FR-016 | Ready |
| KLP-024 | As an integrating developer, I can retrieve an order and its relevant external references so I can display progress. | P0 | 3 | Week 1 | FR-012 | Ready |
| KLP-025 | As an integrating developer, I can list my orders with basic pagination so I can reconcile tests. | P1 | 2 | Week 3 | FR-012 | Ready |
| KLP-026 | As the platform, I process create-order idempotency keys so client retries do not create duplicate orders. | P0 | 3 | Week 1 | FR-016 | Ready |
| KLP-027 | As an operator, I can inspect an append-only order state history so unexpected transitions can be diagnosed. | P0 | 3 | Week 1 | FR-017 | Ready |

#### Epic E2 acceptance

- A key is shown once, stored hashed, and rejected after revocation.
- On-ramp input rejects invalid Stellar accounts, non-IDR currency, unsupported asset/payment method, and invalid amount.
- Off-ramp input rejects missing or invalid asset and withdrawal data.
- Repeating a create request with the same idempotency key and payload returns the original order; conflicting payload returns a documented error.
- Order responses expose sandbox/testnet context and never expose private keys or gateway secrets.

### Epic E3: Payment gateway sandbox

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-030 | As an on-ramp user, I receive a QRIS sandbox checkout for a valid order so I can complete the familiar payment flow. | P0 | 5 | Week 1 | FR-020, FR-021 | Blocked by KLP-002 |
| KLP-031 | As an on-ramp user, I receive a supported bank-transfer/virtual-account sandbox checkout so I can test a second local rail. | P0 | 3 | Week 1 | FR-021 | Blocked by KLP-002 |
| KLP-032 | As the platform, I authenticate and reconcile gateway callbacks before accepting payment state changes. | P0 | 5 | Week 1 | FR-022-FR-025 | Blocked by KLP-002 |
| KLP-033 | As the platform, I handle duplicate and out-of-order callbacks idempotently so token movement occurs exactly once. | P0 | 5 | Week 1-2 | FR-024, NFR-006 | Blocked by KLP-032 |
| KLP-034 | As an off-ramp user, I receive a sandbox withdrawal/payout reference or approved simulation evidence after asset retirement. | P0 | 5 | Week 2 | FR-026, DEC-03 | Blocked by KLP-002 |
| KLP-035 | As an operator, I can correlate and inspect sanitized gateway request/callback metadata so failed tests can be explained. | P1 | 3 | Week 2 | NFR-008 | Blocked by KLP-032 |

#### Epic E3 acceptance

- QRIS and bank-transfer checkout evidence comes from the selected provider's sandbox, not a hand-built fake checkout.
- A callback with invalid authentication is rejected without changing the order.
- Callback amount, currency, and external order reference must match before `payment_confirmed`.
- Replaying the same successful callback cannot create another settlement job.
- Payout limitations are disclosed; the UI and Completion Report must not claim a real IDR transfer when only a simulation occurred.

### Epic E4: Stellar testnet settlement

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-040 | As the platform, I can connect to funded Stellar testnet processing accounts so settlement can be demonstrated. | P0 | 3 | Week 1 | FR-030 | Blocked by KLP-003 |
| KLP-041 | As an on-ramp user, I receive the configured test asset after verified payment so the buy flow completes on-chain. | P0 | 5 | Week 2 | FR-031 | Blocked by KLP-032, KLP-040 |
| KLP-042 | As an off-ramp user, my received test asset is verified and burned or retired before sandbox withdrawal begins. | P0 | 8 | Week 2 | FR-032 | Blocked by KLP-023, KLP-040 |
| KLP-043 | As an operator, I can correlate each order with a Stellar memo/reference and transaction hash so evidence is auditable. | P0 | 3 | Week 2 | FR-033, FR-034 | Blocked by KLP-041 |
| KLP-044 | As the platform, I recover safely from uncertain or failed Stellar submissions so retries cannot duplicate asset movement. | P0 | 5 | Week 2 | FR-035, NFR-006 | Blocked by KLP-041 |
| KLP-045 | As a reviewer, I can open a Stellar testnet explorer link from a completed order so I can verify movement independently. | P1 | 2 | Week 3 | FR-054 | Blocked by KLP-043 |

#### Epic E4 acceptance

- A verified gateway payment produces exactly one successful testnet transaction.
- Asset code, issuer, amount, source, destination, network, memo/correlation reference, and transaction hash are persisted.
- Off-ramp rejects wrong asset, network, amount, or missing order correlation.
- A retry first checks the network/recorded submission state and cannot blindly resubmit an already successful transaction.

### Epic E5: SEP-24, KYC stub, and discovery

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-050 | As a wallet tester, I can initiate a SEP-24 interactive deposit flow that creates a KailoPay on-ramp order. | P0 | 5 | Week 2 | FR-040 | Blocked by KLP-022 |
| KLP-051 | As a wallet tester, I can initiate a SEP-24 interactive withdrawal flow that creates a KailoPay off-ramp order. | P0 | 5 | Week 2 | FR-041 | Blocked by KLP-023 |
| KLP-052 | As a reviewer, I see a clearly labelled synthetic KYC stub so the flow is demonstrated without claiming production verification. | P0 | 2 | Week 2 | FR-042, NFR-005 | Ready |
| KLP-053 | As a wallet tester, I can discover the testnet anchor from a valid public `stellar.toml`. | P0 | 2 | Week 2 | FR-043 | Blocked by KLP-004 |
| KLP-054 | As a wallet tester, I can resolve the documented federation configuration/service. | P0 | 3 | Week 2 | FR-044 | Blocked by KLP-004 |
| KLP-055 | As an operator, I can map SEP-24 transaction statuses to internal states consistently. | P1 | 3 | Week 2 | FR-045 | Blocked by KLP-050, KLP-051 |

#### Epic E5 acceptance

- Public `stellar.toml` is syntactically valid, uses testnet-safe endpoints, and advertises only implemented services.
- Deposit and withdrawal interactive pages identify both `Sandbox` and `Stellar Testnet`.
- KYC uses synthetic data and clearly states that no production identity decision is made.
- SEP-24 status never contradicts the internal order state.

### Epic E6: User-facing web application

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-060 | As a retail sandbox user, I can enter an IDR amount and Stellar destination and obtain a QRIS or bank-transfer checkout. | P0 | 5 | Week 2-3 | FR-050 | Blocked by KLP-030, KLP-031 |
| KLP-061 | As a retail sandbox user, I can create a sell order and follow test-asset deposit and sandbox withdrawal instructions. | P0 | 5 | Week 2-3 | FR-051 | Blocked by KLP-042 |
| KLP-062 | As a user, I can see current order status, important references, and transaction history. | P0 | 3 | Week 3 | FR-052 | Blocked by KLP-024 |
| KLP-063 | As a user, I always see clear sandbox and testnet indicators so I do not mistake the demo for real-money service. | P0 | 2 | Week 2 | FR-053 | Ready |
| KLP-064 | As a user, I receive safe explanations and next actions for pending, expired, or failed orders. | P1 | 3 | Week 3 | FR-055 | Blocked by order states |
| KLP-065 | As a developer, I can create/list/revoke test API keys and open integration docs from the developer section. | P0 | 3 | Week 3 | FR-004 | Blocked by KLP-020, KLP-021 |
| KLP-066 | As a mobile-web reviewer, I can execute the primary flow on a current Chromium browser without clipped or inaccessible controls. | P2 | 2 | Week 3 | NFR-011, NFR-013 | Ready |

#### Epic E6 acceptance

- Buy and sell flows work against deployed sandbox APIs, not hard-coded success data.
- Completed flows display order ID, payment/withdrawal reference, and relevant testnet explorer link.
- Every value-movement screen visibly states that funds/assets are sandbox/testnet only.
- UI errors do not expose stack traces, secrets, or raw private-provider responses.

### Epic E7: Developer webhooks and API experience

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-070 | As an integrating developer, I can register a sandbox webhook endpoint so I receive order lifecycle events. | P0 | 3 | Week 2 | FR-060 | Blocked by KLP-020 |
| KLP-071 | As an integrating developer, I can verify webhook signatures so I can reject forged events. | P0 | 3 | Week 2 | FR-061, FR-062 | Blocked by KLP-070 |
| KLP-072 | As the platform, I retry failed webhook deliveries with a bounded policy and retain attempt results. | P1 | 3 | Week 2-3 | FR-063, FR-064 | Blocked by KLP-070 |
| KLP-073 | As an integrating developer, I can use a complete OpenAPI specification with examples, errors, auth, and webhook schemas. | P0 | 5 | Week 1-3 | FR-072 | Ready |
| KLP-074 | As an integrating developer, I can follow a quickstart and complete a test order without reading source code. | P1 | 3 | Week 3 | Documentation | Blocked by KLP-073 |
| KLP-075 | As a Node.js developer, I can use a typed TypeScript client to create and query sandbox orders without hand-writing HTTP requests. | P0 | 5 | Week 3 | FR-065-FR-067 | Blocked by KLP-073 |
| KLP-076 | As a webhook consumer, I can use the TypeScript SDK to verify webhook signatures from the exact raw body. | P0 | 3 | Week 3 | FR-068 | Blocked by KLP-071, KLP-075 |
| KLP-077 | As a security reviewer, I can verify that SDK documentation prohibits exposing `pk_test_` keys in browser bundles. | P0 | 1 | Week 3 | FR-069 | Blocked by KLP-075 |

#### Epic E7 acceptance

- Events include stable event ID, type, timestamp, API version, order ID, and payload.
- Signature verification has at least one working example and negative test.
- Documentation warns that event delivery is at least once and consumers must deduplicate by event ID.
- OpenAPI validates successfully and matches deployed request/response behavior.

### Epic E8: Deployment, quality, and evidence

| ID | Story | Priority | SP | Target | Requirement | Status |
|---|---|---:|---:|---|---|---|
| KLP-080 | As a reviewer, I can access the live sandbox web app, API, docs, `stellar.toml`, and federation endpoints through public HTTPS URLs. | P0 | 5 | Week 3 | FR-071, NFR-002 | Blocked by KLP-004 |
| KLP-081 | As QA, I can run automated order, callback-idempotency, settlement, and webhook tests so regressions are detected. | P0 | 5 | Week 1-4 | Success metrics | Ready |
| KLP-082 | As a reviewer, I can inspect one documented successful buy and one documented successful sell test with on-chain evidence. | P0 | 5 | Week 3 | FR-073 | Blocked by core epics |
| KLP-083 | As a security reviewer, I can verify that public code/logs contain no secrets and that test flows use synthetic data. | P0 | 3 | Week 3-4 | NFR-001, NFR-005 | Ready |
| KLP-084 | As a reviewer, I can follow a user guide and watch a demo recording covering both flows. | P0 | 3 | Week 4 | FR-074 | Blocked by KLP-082 |
| KLP-085 | As the Ambassador reviewer, I can map each SOW deliverable to repository, URL, screenshot, document, log, and transaction evidence. | P0 | 3 | Week 4 | FR-070-FR-075 | Blocked by evidence |
| KLP-086 | As the project owner, I can publish a Completion Report describing delivered work, tests, limitations, and production roadmap. | P0 | 3 | Week 4 | FR-075 | Blocked by KLP-085 |
| KLP-087 | As the project owner, I can tag `v0.1.0` after all release gates pass so the reviewed version is immutable. | P0 | 1 | Week 4 | FR-076 | Blocked by release gate |
| KLP-088 | As an operator, I have a minimal deployment and incident/recovery runbook so the demo can be restored during review. | P1 | 3 | Week 4 | NFR-010, NFR-012 | Blocked by KLP-080 |

#### Epic E8 acceptance

- Public endpoints use HTTPS and do not expose admin interfaces or secrets.
- The two formal tests contain timestamp, environment, inputs, expected result, actual result, screenshots/log references, and Stellar explorer links.
- Evidence matrix has no unlabeled missing item; any partial item includes impact and corrective action.
- Release tag points to the code and documentation used for the final evidence run.

## 5. Suggested sprint sequence

| Order | Stories | Outcome |
|---|---|---|
| 1 | KLP-001-KLP-005 | External decisions and public project baseline. |
| 2 | KLP-010-KLP-013, KLP-020, KLP-022-KLP-024 | Durable order API vertical foundation. |
| 3 | KLP-030-KLP-033, KLP-040-KLP-041 | First successful sandbox payment-to-testnet on-ramp. |
| 4 | KLP-023, KLP-034, KLP-042-KLP-044 | Successful testnet-to-sandbox off-ramp. |
| 5 | KLP-050-KLP-054, KLP-070-KLP-073 | Anchor discovery, SEP-24, and developer integration. |
| 6 | KLP-060-KLP-065 | Reviewable user and developer web experience. |
| 7 | KLP-080-KLP-088 | Public deployment, tests, evidence, demo, report, and release. |

This sequence deliberately produces the first vertical slice before broad web polish.

## 6. Future backlog

These items are retained for roadmap visibility and must not enter the Instaward sprint without a formal scope change.

| ID | Initiative | Phase | Status |
|---|---|---|---|
| KLP-F001 | Production gateway commercial integration and real-money certification | 2 | Future |
| KLP-F002 | Full KYC/KYB, liveness, sanctions, PEP, and AML case workflows | 2 | Future |
| KLP-F003 | Legal and regulatory readiness, including Bappebti/applicable-regulator assessment | 2 | Future |
| KLP-F004 | Treasury, liquidity, pricing, fees, refunds, disputes, and three-way reconciliation | 2 | Future |
| KLP-F005 | Production custody/key management, multisignature, rotation, and emergency controls | 2 | Future |
| KLP-F006 | Threat model, penetration testing, security monitoring, backup, and disaster recovery | 2 | Future |
| KLP-F007 | Controlled Stellar mainnet and live-payment pilot | 3 | Future |
| KLP-F008 | Customer operations, support, complaints, and manual review tooling | 3 | Future |
| KLP-F009 | Second payment gateway and automated routing/failover | 4 | Future |
| KLP-F010 | Merchant gateway and merchant dashboard | 4 | Future |
| KLP-F011 | White-label wallet/dApp experience | 4 | Future |
| KLP-F012 | Native iOS and Android applications | 4 | Future |

## 7. Backlog review cadence

Review the backlog at least twice per week and record:

- Completed P0 outcomes and their evidence links.
- New blockers or changes to external-provider assumptions.
- Remaining P0 work versus capacity.
- P1/P2 items cut or promoted, with rationale.
- Scope-change requests and approver decisions.
- Known defects that affect the release gate.

The backlog is a living execution tool; the PRD remains the requirement baseline and the SOW remains the Instaward commitment authority.
