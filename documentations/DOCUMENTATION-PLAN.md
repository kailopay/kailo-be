# KailoPay Documentation Plan

| Field | Value |
|---|---|
| Version | 0.1 |
| Status | Draft for review |
| Applies to | Instaward Sandbox MVP `v0.1.0` and future documentation roadmap |

## 1. Purpose

This plan defines the documents needed to build, test, operate, verify, and extend KailoPay. It separates documentation that must ship with the 30-day Instaward from material that is required only before production or mainnet operation.

The public documentation must consistently identify the product as a sandbox and Stellar testnet demonstration. It must not imply that KailoPay is licensed, production-ready, or transferring real IDR.

## 2. Documentation audiences

- **Ambassador reviewer:** verifies the three SOW deliverables with minimal technical expertise.
- **Sandbox user:** completes buy and sell demonstrations safely.
- **Integrating developer:** uses API keys, endpoints, webhooks, and SEP-24 flows.
- **Maintainer/operator:** builds, deploys, monitors, diagnoses, and restores the demo.
- **Product and delivery team:** controls scope, requirements, backlog, decisions, risks, and evidence.
- **Future legal/compliance/security stakeholders:** evaluate readiness for production phases.

## 3. Documentation principles

1. One source of truth per topic; link instead of copying content that can drift.
2. Every document has an owner, version/status, last-reviewed date, and intended audience.
3. Examples use sandbox credentials, synthetic identity data, and Stellar testnet addresses only.
4. API examples are validated against the released implementation.
5. Evidence links are durable enough for post-sprint review.
6. Known limitations are explicit and placed near the behavior they qualify.
7. Secrets, private keys, access tokens, real identity documents, and unredacted provider payloads are never published.

## 4. Required documentation catalog

### 4.1 Product and delivery baseline

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Product Requirements Document | P0 | Product/Project Owner | Before implementation baseline | All delivery roles | Goals, non-goals, personas, journeys, numbered requirements, NFRs, acceptance scenarios, risks, and scope-control rules are approved. |
| Delivery Phases | P0 | Project Owner | Before sprint start | Team, sponsor | Phase 1 maps to the four SOW weeks; future phases are clearly non-committed; gates and cut line are defined. |
| Product Backlog | P0 | Product + Engineering | Before sprint start; updated continuously | Delivery team | P0 stories map to requirements, dependencies and evidence; status and cut decisions are current. |
| Documentation Plan | P0 | Technical Writer/Developer | Week 1 | All document owners | Catalog, owners, due dates, templates, review rules, and evidence mapping are defined. |
| Decision Log / ADR Index | P0 | Tech Lead | Start in Phase 0 | Engineering, product | Gateway, asset model, payout evidence, auth, hosting, and license decisions have rationale, alternatives, consequences, and date. |
| Risk and Scope-change Log | P1 | Project Owner | Start in Phase 0 | Team, sponsor | Active risks, owners, mitigations, due dates, and approved scope changes are traceable. |

This repository package supplies the first four planning documents. The implementation repository should add the decision and risk logs when delivery begins.

### 4.2 Repository and contributor documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Root `README.md` | P0 | Lead Developer | Skeleton Week 1; final Week 4 | All | Product summary, sandbox warning, architecture overview, quickstart, environment setup, test commands, deployment URLs, docs links, evidence links, limitations, and license are current. |
| `LICENSE` | P0 | Project Owner | Phase 0 | Public users | Approved license text is present and referenced from README. |
| `CONTRIBUTING.md` | P1 | Lead Developer | Week 3 | Contributors | Branch/PR workflow, code style, tests, commit expectations, and security-reporting route are documented. |
| `CHANGELOG.md` / release notes | P1 | Lead Developer | Week 4 | Users, developers | `v0.1.0` features, fixes, breaking behavior, limitations, and upgrade/setup notes are recorded. |
| Environment/configuration reference | P0 | Lead Developer | Week 1 | Maintainers | Every variable includes purpose, required/optional status, example, environment, and secret classification without real values. |

### 4.3 Architecture and engineering documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Architecture overview | P0 | Tech Lead | Week 1, update Week 4 | Developers, reviewers | Context and container diagrams, components, trust boundaries, payment/Stellar/webhook flows, and external dependencies match the release. |
| Data model and state-machine reference | P0 | Backend Developer | Week 1-2 | Developers, QA, operators | Entities, relationships, precision rules, on/off-ramp states, allowed transitions, and invariants are documented. |
| Payment gateway integration guide | P0 | Backend Developer | Week 2 | Developers, operators | Selected provider setup, sandbox methods, callback verification, reconciliation, idempotency, error mapping, and known sandbox limitations are covered. |
| Stellar asset and settlement guide | P0 | Stellar Developer | Week 2 | Developers, reviewers | Network, accounts/roles, asset code/issuer, issuance, burn/retirement, correlation, retry safety, and explorer verification are documented without secret keys. |
| SEP-24 and anchor discovery guide | P0 | Stellar Developer | Week 2-3 | Wallet developers, reviewers | `stellar.toml`, federation, deposit/withdraw endpoints, KYC stub, status mapping, and manual verification steps are documented. |
| ADRs | P0 | Decision owner | As decisions occur | Maintainers | Each material decision records context, chosen option, alternatives, rationale, consequences, and supersession status. |

Recommended ADRs for `v0.1.0`:

- ADR-001: Xendit versus Midtrans selection.
- ADR-002: Test asset issuance, distribution, and burn/retirement model.
- ADR-003: Order state machine and exactly-once settlement strategy.
- ADR-004: Off-ramp payout sandbox evidence model.
- ADR-005: Hosting and public URL topology.
- ADR-006: Demo user/developer authentication approach.

### 4.4 API and integration documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| OpenAPI specification | P0 | Backend Developer | Initial Week 1; final Week 3 | Integrating developers | Every shipped endpoint, schema, auth method, idempotency header, error, enum, and example validates and matches deployed behavior. |
| API quickstart | P0 | Backend Developer | Week 3 | Integrating developers | A developer can obtain a test key, create an order, inspect status, and identify evidence using copyable sandbox examples. |
| TypeScript SDK reference | P0 | SDK/Backend Developer | Week 3 | Node.js developers | Installation, initialization, supported methods, idempotency, exact amounts, error handling, webhook verification, compatibility, and browser-secret warning match the released package. |
| Authentication and API-key guide | P0 | Backend Developer | Week 3 | Integrating developers | `pk_test_` creation, one-time display, storage guidance, use, revocation, and error cases are documented. |
| Webhook reference | P0 | Backend Developer | Week 3 | Integrating developers | Event types, payloads, ordering, at-least-once delivery, retry policy, signature verification, deduplication, and test examples are documented. |
| Error catalog | P1 | Backend Developer | Week 3 | Developers, support | Stable error codes, HTTP statuses, causes, retryability, and corrective action match the API. |
| Sandbox testing guide | P0 | QA/Developer | Week 3 | Integrators, reviewers | Test inputs, provider sandbox actions, expected statuses, synthetic data, common failures, and reset/retry guidance are reproducible. |

### 4.5 User and reviewer documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Sandbox user guide | P0 | Product/Developer | Week 4 | Users, Ambassador reviewer | Buy, sell, status, history, and explorer-verification steps include screenshots and prominent sandbox/testnet warnings. |
| Reviewer verification guide | P0 | Project Owner | Week 4 | Ambassador reviewer | Each SOW deliverable has a short verification procedure, expected result, and evidence link requiring minimal technical knowledge. |
| Demo script and recording | P0 | Project Owner | Week 4 | Sponsor, public | Recording shows environment warning, QRIS/bank-transfer context, buy flow, sell flow, API/docs, testnet hashes, and known limitations. |
| FAQ and known limitations | P1 | Product Owner | Week 4 | All users | Clearly distinguishes testnet assets, sandbox payment behavior, KYC stub, payout simulation, and excluded production capabilities. |

### 4.6 Quality, security, and operations documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Test plan | P0 | QA/Developer | Week 2 | Team, reviewer | Unit, integration, callback replay, settlement, API contract, web E2E, negative, and release tests map to requirements. |
| Test results | P0 | QA/Developer | Week 3-4 | Team, Ambassador reviewer | Environment, build/version, cases, expected/actual result, evidence, defects, and final disposition are recorded. |
| Minimal threat model and security checklist | P0 | Tech Lead | Week 3 | Engineering, sponsor | Assets, trust boundaries, abuse cases, callback/webhook threats, secret/private-key risks, mitigations, and residual sandbox risks are reviewed. |
| Deployment runbook | P0 | DevOps/Developer | Week 3 | Maintainers | Prerequisites, configuration, migration, deploy, verification, rollback, log access, and public endpoint checks are executable. |
| Demo operations and recovery runbook | P1 | Developer | Week 4 | Maintainers | Restart, failed-order diagnosis, safe retry, test data cleanup, provider outage, and evidence fallback procedures are documented. |
| Backup/restore note | P1 | Developer | Week 4 | Maintainers | Sandbox database backup and restore are tested or limitations are explicitly recorded. |
| Security disclosure policy | P1 | Project Owner | Week 4 | Public researchers | A safe private contact route and response expectations are published without promising a production SLA. |

### 4.7 Completion and evidence documentation

| Document | Priority | Owner | Due | Audience | Definition of done |
|---|---:|---|---|---|---|
| Evidence matrix | P0 | Project Owner | Updated weekly; final Week 4 | Ambassador reviewer | All SOW deliverables map to stable repo, URL, screenshot, document, log, test, and transaction evidence; missing/partial items are explicit. |
| End-to-end test records | P0 | QA/Developer | Week 3 | Reviewer | At least one buy and one sell record include timestamp, inputs, expected/actual outcome, order/payment/withdrawal references, logs, and testnet hashes. |
| Completion Report | P0 | Project Owner | Week 4 | Sponsor, ecosystem | Executive summary, SOW mapping, delivered scope, architecture, tests, evidence, budget/time summary if required, limitations, risks, and production roadmap are complete. |
| `v0.1.0` release notes | P0 | Lead Developer | Week 4 | All | Release commit/tag, deployment URLs, documentation version, limitations, and evidence package are aligned. |

## 5. SOW evidence matrix template

The final project should instantiate the following matrix with working links:

| SOW deliverable | Required evidence | Verification action | Owner | Status |
|---|---|---|---|---|
| Deliverable 1: On/off-ramp API and payment integration | Public repo, live API URL, OpenAPI, test key guide, order screenshots/responses, QRIS and bank-transfer sandbox checkout evidence | Create or inspect buy/sell orders, verify status responses, and confirm provider sandbox checkout | Backend/Project Owner | Planned |
| Deliverable 2: Stellar testnet anchor skeleton | Testnet transaction hashes, `stellar.toml`, federation URL, SEP-24 deposit/withdrawal demo, KYC stub, webhook logs | Open explorer links, validate discovery files, follow interactive flows, and inspect webhook evidence | Stellar Developer/Project Owner | Planned |
| Deliverable 3: Web app and demo materials | Live app URL, user guide, developer docs, demo recording, Completion Report | Complete or watch buy/sell flows and review mapped final report | Frontend/Project Owner | Planned |

Allowed final statuses are `Present`, `Partial`, or `Missing`. `Partial` and `Missing` require a written explanation, impact, and corrective action.

## 6. Required document structures

### 6.1 OpenAPI specification

Must include:

- API title, semantic version, server environments, and sandbox warning.
- `pk_test_` authentication scheme.
- Idempotency header and behavior.
- On-ramp/off-ramp creation and order query operations.
- Request/response schemas, monetary precision, enums, examples, and error envelope.
- Webhook event schemas and signature headers.
- No production server URL or real credential.

### 6.2 Architecture overview

Must include:

- System context and major components.
- Trust boundaries and external systems.
- On-ramp sequence from API/UI through gateway callback to Stellar.
- Off-ramp sequence from asset receipt through retirement to withdrawal.
- SEP-24 discovery and interactive flow.
- Data stores, asynchronous/retry behavior, idempotency boundaries, and observability.
- Sandbox limitations and future production gaps.

### 6.3 Test record

Must include:

- Test ID, date/time, tester, version/commit, and environment.
- Preconditions and synthetic inputs.
- Steps, expected result, and actual result.
- KailoPay order ID and external provider reference.
- Stellar account/asset and transaction hash/explorer link.
- Relevant redacted log/screenshot references.
- Pass/fail status, defect link, and retest result.

### 6.4 Completion Report

Must include:

1. Executive summary.
2. Original objective and 30-day scope.
3. Delivered work mapped to Deliverables 1-3.
4. Architecture and user-flow summary.
5. Public URLs and repository/release details.
6. Test approach and results.
7. Evidence matrix with transaction hashes and webhook logs.
8. Deviations from the SOW and their approval status.
9. Known limitations and unresolved risks.
10. Path to production covering KYC, regulation, provider contracts, security, operations, liquidity, reconciliation, and mainnet.

## 7. Review and publication workflow

1. Author creates or updates the document in the same change as the related behavior.
2. Technical reviewer verifies correctness against code, configuration, API behavior, or test evidence.
3. Product/project owner verifies SOW scope and non-technical clarity.
4. Security review checks for credentials, private keys, PII, unsafe instructions, and misleading production claims.
5. Links and examples are tested against the release candidate.
6. Document status and last-reviewed date are updated.
7. Final documents are linked from the root README and included in the evidence matrix.

## 8. Documentation quality gate

Before `v0.1.0` is tagged:

- No unresolved implementation marker, placeholder URL, fake transaction hash, or unresolved template instruction remains in a P0 document.
- All public links return the expected resource through HTTPS.
- OpenAPI validates and request examples work against the sandbox deployment.
- User and reviewer instructions have been followed by someone other than the author or have a recorded dry-run result.
- Screenshots and logs redact credentials, tokens, private keys, and unnecessary personal data.
- Sandbox/testnet warnings are prominent and consistent.
- Documentation version and behavior match the tagged release.

## 9. Future production documentation

Before any production/mainnet pilot, create and approve at minimum:

- Regulatory applicability and licensing assessment.
- KYC/KYB, AML, sanctions, PEP, transaction-monitoring, and case-management policies.
- Privacy notice, data map, retention/deletion schedule, and data-subject process.
- Terms of service, user disclosures, complaints, refund, and dispute procedures.
- Custody/key-management policy and cryptographic ceremony/runbooks.
- Treasury, liquidity, fees, reserves, settlement, and three-way reconciliation procedures.
- Production threat model, penetration-test report, vulnerability management, access-control matrix, and audit-log policy.
- Incident response, business continuity, disaster recovery, backup/restore, and communication plans.
- SLOs/SLIs, alert catalog, on-call handbook, support handbook, and escalation matrix.
- Mainnet launch checklist, change management, rollback, and post-launch monitoring plan.

This list is a readiness baseline, not legal advice. Applicable obligations must be confirmed by qualified Indonesian legal and compliance professionals.

## 10. Ownership note

The SOW budgets one full-stack developer. Where the tables name separate roles, one person may perform multiple roles, but authorship and review should still be separated when practical. The Ambassador Chapter Lead or another stakeholder can provide independent scope/evidence review even when engineering capacity is singular.
