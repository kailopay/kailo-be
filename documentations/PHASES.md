# KailoPay Delivery Phases

| Field | Value |
|---|---|
| Version | 0.1 |
| Status | Draft for review |
| Planning anchor | 30-day Instaward SOW |
| Capacity assumption | One full-stack developer, 160 hours |
| Budget baseline | USD 4,800 at USD 30/hour |

## 1. Planning principles

1. The Instaward is a 30-calendar-day delivery commitment, not an open-ended discovery project.
2. Phase 1 must produce a demonstrable vertical slice before adding polish.
3. Evidence is produced with each capability, not postponed entirely to the final week.
4. Payment-gateway sandbox and Stellar testnet limitations must be discovered in the first days.
5. Production payments, mainnet, full KYC, licensing, mobile, merchant gateway, and white-label work remain outside Phase 1.
6. Future phases are directional and require separate discovery, legal review, funding, and approval.

## 2. Phase overview

| Phase | Name | Commitment | Primary outcome |
|---|---|---|---|
| 0 | Mobilization and risk burn-down | Included at the start of the 30-day sprint | Decisions, accounts, architecture, environments, and a proven gateway/testnet spike. |
| 1 | Instaward Sandbox MVP (`v0.1.0`) | **Committed by SOW** | Public sandbox on/off-ramp, REST API, SEP-24 skeleton, web app, docs, demo, and completion evidence. |
| 2 | Production-readiness foundation | Future proposal | Security, operations, reconciliation, provider contracting, and compliance design mature enough for controlled pilots. |
| 3 | Regulated mainnet pilot | Future proposal | Limited real-user corridor operated with approved legal/compliance controls. |
| 4 | Product expansion and scale | Future proposal | Additional rails, assets, merchants, mobile, white-label, and higher-scale operations. |

Phase 0 is the opening mobilization segment inside the Phase 1 30-day window and budget; it is shown separately only to make early risk controls visible. Phases 2-4 are not covered by the Instaward budget or acceptance commitment.

## 3. Phase 0: Mobilization and risk burn-down

**Target:** Days 1-2 of the 30-day window  
**Purpose:** Remove external blockers before the main build consumes significant capacity.

### Scope

- Confirm Xendit or Midtrans as the single sandbox provider.
- Verify that the selected sandbox exposes QRIS, bank transfer/virtual account, callbacks, and the available payout simulation.
- Choose the Stellar test asset code and issuer/distributor/burn model.
- Fund required Stellar testnet accounts and prove a small transfer.
- Decide the repository license, hosting approach, domain/subdomain layout, and secret-management approach.
- Establish the public repository, project structure, CI baseline, PostgreSQL, environment template, and decision log.
- Freeze the Must-have backlog and evidence format.

### Exit criteria

- A gateway sandbox checkout can be created manually or through a small integration probe.
- Gateway callback authentication documentation is available and understood.
- A Stellar testnet transaction is confirmed and linked in the decision/evidence log.
- All decisions in PRD section 15 have an owner and resolution or a documented fallback.
- No production credential or real customer data is required for Phase 1.

### Stop/go rule

If QRIS or bank-transfer sandbox capability is unavailable from the chosen provider, do not hide the limitation. Switch providers within the timebox or obtain written stakeholder acceptance of an evidence-equivalent sandbox method before proceeding.

## 4. Phase 1: Instaward Sandbox MVP

**Target:** 30 calendar days  
**Reference start in SOW:** 15 August 2026  
**Reference end if started on that date:** 13 September 2026  
**Release:** `v0.1.0`

If the actual approved start differs, retain the Day/Week sequence and recalculate calendar dates.

### 4.1 Week 1: API, payments, and Stellar foundation

**Goal:** Create an on-ramp order, receive a real sandbox checkout, and establish testnet connectivity.

Planned work:

- Create public repository, license, README skeleton, CI, and environment template.
- Set up the Go/Gin service, GORM PostgreSQL adapter, reviewed migrations, validation, and error model.
- Implement hashed `pk_test_` API keys.
- Implement create/get/list order operations and state-event history.
- Integrate the selected payment gateway for QRIS and bank-transfer sandbox checkout.
- Validate gateway callbacks and process them idempotently.
- Set up Stellar testnet accounts, asset configuration, and SDK adapter.
- Publish the initial OpenAPI specification.

Expected evidence:

- Public repository URL.
- Successful API order-creation response.
- Gateway sandbox checkout screenshot/link.
- Funded testnet account and initial transaction hash.
- Published OpenAPI preview.

Week 1 gate:

- A valid API request returns a usable sandbox checkout.
- A verified gateway callback changes order state once.
- Testnet access is automated from the service environment.

### 4.2 Week 2: End-to-end settlement and SEP-24 skeleton

**Goal:** Complete both value-flow vertical slices on testnet.

Planned work:

- Connect confirmed on-ramp payment to exactly-once asset issuance/transfer.
- Implement off-ramp asset deposit detection/verification, burn or retirement, and sandbox withdrawal processing.
- Persist payment, asset, and transaction correlation references.
- Implement outgoing signed developer webhooks and retry logs.
- Add SEP-24 interactive deposit/withdrawal skeleton and KYC stub.
- Publish `stellar.toml` and federation configuration.
- Build basic web buy and sell screens.

Expected evidence:

- One internal on-ramp testnet transaction hash.
- One internal off-ramp burn/retirement transaction hash.
- Webhook delivery log with signature metadata.
- Public `stellar.toml` and federation URLs.
- Basic web flow screenshots.

Week 2 gate:

- On-ramp and off-ramp both reach a terminal successful state in the controlled test environment.
- Duplicate payment callbacks cannot cause duplicate asset movement.
- SEP-24 discovery resolves to the correct interactive test flows.

### 4.3 Week 3: Usability, developer experience, deployment, and formal tests

**Goal:** Make the product publicly reviewable and independently testable.

Planned work:

- Finish QRIS/bank-transfer checkout UX, sell flow, order status, transaction history, and explorer links.
- Add obvious sandbox/testnet environment labels and user-safe error states.
- Finish the developer section for API key and webhook configuration.
- Complete OpenAPI schemas, examples, authentication, errors, webhook reference, and quickstart.
- Implement and test the thin TypeScript SDK against the deployed OpenAPI/API contract.
- Deploy API, web app, anchor discovery, and documentation to public URLs.
- Run and capture at least two formal end-to-end tests: one buy and one sell.
- Perform security, secret, migration, and recovery checks.

Expected evidence:

- Live public app, API, and documentation URLs.
- Two complete test records with inputs, screenshots/log extracts, expected results, actual results, and transaction hashes.
- Public integration quickstart and webhook verification example.

Week 3 gate:

- A new reviewer can execute the documented sandbox flow.
- All Must-have API and web acceptance scenarios pass.
- No secrets are present in the public repository or published logs.

### 4.4 Week 4: Release, evidence, and handover

**Goal:** Produce a complete, polished, verifiable delivery package.

Planned work:

- Resolve release-blocking defects and perform final regression tests.
- Finalize README, local setup, API guide, user guide, anchor guide, runbook, and known limitations.
- Record a concise demo showing both buy and sell flows.
- Complete the evidence matrix and Completion Report.
- Tag `v0.1.0` and retain stable review URLs.

Expected evidence:

- Public `v0.1.0` release tag.
- Demo recording.
- Completion Report with delivered scope, test results, evidence links, limitations, and production roadmap.
- Final evidence matrix for SOW Deliverables 1-3.

Release gate:

- All PRD success metrics pass.
- All SOW evidence types are present or explicitly marked partial with an accepted explanation.
- Known limitations do not misrepresent sandbox/testnet functionality as production-ready.

## 5. Phase 1 cut line

When capacity is constrained, work is reduced in this order:

1. Remove visual polish that does not block the reviewer journey.
2. Reduce optional dashboard/history filtering.
3. Defer nonessential webhook event types.
4. Defer proposed performance/accessibility enhancements that do not prevent safe use.
5. Simplify the federation implementation to the smallest valid, documented configuration.

The following may not be cut without a written SOW change:

- QRIS and bank-transfer sandbox checkout.
- On-ramp and off-ramp APIs and order status.
- Verified payment callbacks and end-to-end testnet movement.
- SEP-24 deposit/withdrawal skeleton, KYC stub, `stellar.toml`, and federation configuration.
- User-facing buy/sell flow.
- Test API keys, OpenAPI documentation, developer webhooks, and public review URLs.
- Two end-to-end test flows, demo, and Completion Report.

## 6. Phase 2: Production-readiness foundation

**Commitment:** Future; not part of the Instaward  
**Entry condition:** Phase 1 accepted and a funded production-readiness mandate exists.

Candidate workstreams:

- Legal entity, regulatory counsel, Bappebti and other applicable-regulator gap analysis.
- Production KYC/KYB, sanctions/PEP screening, AML monitoring, case management, and data-retention design.
- Payment provider commercial onboarding and production rail certification.
- Treasury, liquidity, pricing, fees, reconciliation, refund, dispute, and exception management.
- Hardened custody/key-management design, multisignature controls, rotation, and incident response.
- Threat model, penetration test, dependency/SAST scanning, backup/restore, disaster recovery, alerting, and SLOs.
- Ledger/accounting model and daily three-way reconciliation across KailoPay, gateway/bank, and Stellar.
- Customer support, complaint, manual review, and operational approval workflows.
- Production-grade SEP conformance review and mainnet launch checklist.

Exit criteria should be agreed with legal, compliance, security, finance, and operations stakeholders; Phase 1 evidence alone is not sufficient.

## 7. Phase 3: Regulated mainnet pilot

**Commitment:** Future; not part of the Instaward

Candidate scope:

- Limited mainnet asset/corridor with approved issuer, custody, reserves, and legal structure.
- Real payment acceptance and payouts for an approved user cohort and transaction limits.
- Full KYC/AML and risk-based review.
- Production support, reconciliation, monitoring, audit logs, incident response, and rollback controls.
- Pilot metrics covering completion rate, failure rate, settlement time, support cases, reconciliation breaks, and unit economics.

The pilot must use explicit go/no-go approval and must not start solely because the sandbox MVP works.

## 8. Phase 4: Product expansion and scale

**Commitment:** Future; not part of the Instaward

Possible initiatives:

- Additional Indonesian payment rails and a second gateway provider.
- Additional Stellar assets and liquidity partners.
- Merchant API, checkout, settlement, and dashboard.
- White-label wallet/dApp components.
- Native mobile applications.
- Higher-volume infrastructure, queueing, automated reconciliation, and data analytics.
- Geographic or currency expansion after separate regulatory assessment.

Each initiative requires its own PRD, business case, risk review, and measurable success criteria.

## 9. Milestone governance

### Weekly review inputs

- Demo of the newest vertical slice.
- Backlog progress against the Must-have cut line.
- Blockers and external dependency status.
- Evidence captured during the week.
- Risks, scope changes, and decisions.
- Remaining capacity against the 160-hour assumption.

### Scope-change rule

A scope change is accepted only when it records the request, rationale, affected SOW deliverable, effort impact, displaced work, evidence change, and approver. Unapproved additions remain in the Future backlog.

### Definition of phase completion

A phase is complete only when its functional outcome, acceptance evidence, documentation, known limitations, and operational handover are all present. Code completion alone is not phase completion.
