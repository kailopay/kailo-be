# KailoPay Documentation

This folder is the planning and delivery baseline derived from the KailoPay Instaward Statement of Work submitted on 31 July 2026.

## Documents

| Document | Purpose | Status |
|---|---|---|
| [Product Requirements Document](PRD.md) | Defines product goals, scope, personas, journeys, functional and non-functional requirements, acceptance scenarios, assumptions, and risks. | Draft for review |
| [Delivery Phases](PHASES.md) | Maps the committed 30-day Instaward delivery and separates future production/mainnet phases. | Draft for review |
| [Product Backlog](BACKLOG.md) | Provides prioritized epics and stories with dependencies, story points, requirement traceability, and acceptance criteria. | Initial backlog |
| [Documentation Plan](DOCUMENTATION-PLAN.md) | Defines all product, API, engineering, QA, operations, evidence, and future production documents required. | Draft for review |
| [Frontend Guide](frontend/FRONTEND-GUIDE.md) | Handoff for frontend developers/agents: implemented API surface, contracts, UI rules, and current limitations. | Living document |

## Source-of-truth hierarchy

1. The finalized/signed SOW controls the Instaward contractual commitment.
2. `PRD.md` controls detailed product requirements and acceptance behavior.
3. `PHASES.md` controls sequencing, gates, and scope cut line.
4. `BACKLOG.md` controls execution status and work decomposition.
5. `DOCUMENTATION-PLAN.md` controls documentation ownership and release evidence.

If two files conflict, resolve the conflict in the higher-ranked source and update all affected traceability references.

## Scope labels

- **Committed:** directly required by the 30-day SOW.
- **Derived:** required for a committed feature to be safe, testable, or operable.
- **Proposed:** recommended target that may be adjusted without violating the SOW.
- **Future:** outside the Instaward and subject to separate approval.

## Instaward scope summary

The `v0.1.0` target is a public, reviewable sandbox platform containing:

- Go/Gin on/off-ramp REST API with GORM-backed PostgreSQL order state.
- Test API keys using the `pk_test_` prefix.
- Official TypeScript SDK for server-side developer integrations; the backend runtime remains Go.
- One real Indonesian payment gateway integration in sandbox mode, supporting QRIS and bank transfer.
- Verified payment callbacks and developer webhook delivery.
- Stellar testnet issuance/transfer and burn/retirement evidence.
- SEP-24 deposit and withdrawal skeleton, KYC stub, `stellar.toml`, and federation configuration.
- User-facing buy/sell web app, order status, transaction history, and developer section.
- Public repository, live sandbox URLs, OpenAPI specification, test results, demo recording, and Completion Report.

## Explicitly excluded from the Instaward

- Production payments or real IDR settlement.
- Stellar mainnet.
- Full KYC-provider integration.
- Licensing and regulatory-compliance completion.
- Native mobile applications.
- Merchant payment gateway and white-label products.

## SOW traceability

| SOW element | PRD | Phase | Backlog | Required evidence/docs |
|---|---|---|---|---|
| Deliverable 1: API and payment integration | FR-001-FR-026, including unified identity, user-owned Developer Mode, and API-client access FR-005-FR-009 | Phase 1, Weeks 1-3 | E1-E3, E7 | Public repo/API, OpenAPI, screenshots/responses, sandbox URL, access-control tests |
| Deliverable 2: Stellar anchor skeleton | FR-030-FR-045 | Phase 1, Week 2 | E4-E5 | Testnet hashes, `stellar.toml`, federation, SEP-24 guide, webhook logs |
| Deliverable 3: Web app and demo | FR-050-FR-076 | Phase 1, Weeks 2-4 | E6-E8 | Live app, user/API docs, demo recording, Completion Report |
| Four-week execution plan | Release and scope control | Phase 1, Weeks 1-4 | Suggested sprint sequence | Weekly evidence and release gates |
| Out-of-scope constraints | PRD section 5 | Phases 2-4 | Future backlog | Completion Report limitations and roadmap |

## Immediate decisions before implementation

Resolve and record these items during Phase 0:

1. Select Xendit or Midtrans after testing exact sandbox capabilities.
2. Define the Stellar test asset and issuance/burn model.
3. Define acceptable off-ramp evidence if the gateway cannot simulate a full payout.
4. Select the minimum demo authentication approach.
5. Select public hosting and domain/subdomain layout.
6. Select the repository license.

## Maintenance rules

- Update requirements and acceptance criteria before implementing an approved behavior change.
- Update backlog status and evidence links at least twice per week.
- Record material technical decisions as ADRs.
- Keep all examples restricted to sandbox credentials, synthetic user data, and Stellar testnet.
- Do not tag `v0.1.0` while P0 documents contain placeholders, broken links, or unverified examples.
