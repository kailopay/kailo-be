# KailoPay Product Requirements Document

| Field | Value |
|---|---|
| Product | KailoPay |
| Version | 0.1 |
| Status | Draft for review |
| Primary source | `KailoPay - SOW Proposal.docx`, submitted 31 July 2026 |
| Instaward target start | 15 August 2026 |
| Delivery window | 30 calendar days |
| Budget baseline | USD 4,800 for 160 hours at USD 30/hour |
| Target network | Stellar testnet |
| Payment environment | One Indonesian payment gateway in sandbox mode |

## 1. Document purpose

This PRD translates the approved Statement of Work (SOW) into testable product requirements for KailoPay. It is the baseline for product, engineering, QA, delivery, and evidence collection during the Instaward sprint.

Requirement labels are used throughout this documentation package:

- **Committed**: explicitly required by the SOW and expected within the 30-day sprint.
- **Derived**: necessary to make a committed capability safe, testable, or operable, but not separately stated in the SOW.
- **Future**: intentionally excluded from the Instaward and retained only as roadmap context.

If this PRD conflicts with the signed SOW, the SOW controls the Instaward commitment until the parties approve a written scope change.

## 2. Product summary

KailoPay is an Indonesia-first fiat on-ramp and off-ramp platform for Stellar. The initial release demonstrates an end-to-end sandbox corridor between Indonesian payment methods and a Stellar testnet asset:

1. A user creates an on-ramp order and pays through QRIS or bank transfer in a real payment gateway sandbox.
2. KailoPay receives and validates the payment callback.
3. KailoPay issues or transfers the configured test asset to the user's Stellar testnet account.
4. For off-ramp, the user sends the test asset to KailoPay and requests withdrawal to IDR.
5. KailoPay records the asset movement, burns or retires the test asset as configured, and initiates the sandbox withdrawal process.

The same capabilities are exposed through a user-facing web application, a public REST API protected by test API keys, and a SEP-24 anchor skeleton.

## 3. Problem statement

Indonesia has highly adopted local payment rails, including QRIS, bank transfers, and e-wallets, but Stellar currently lacks an Indonesia-native and SEP-aligned IDR corridor. Users and developers must rely on global providers that are not tailored to Indonesian payment behavior and are not necessarily Stellar-native anchors.

KailoPay addresses the first infrastructure gap by proving the technical and product flow in sandbox and testnet environments before regulated production deployment.

## 4. Goals and success criteria

### 4.1 Instaward goals

| ID | Goal | Success indicator |
|---|---|---|
| G-01 | Demonstrate a complete IDR-to-Stellar sandbox on-ramp | A reviewer can create an order, complete a sandbox QRIS or bank-transfer payment, and inspect the resulting Stellar testnet transaction hash. |
| G-02 | Demonstrate a complete Stellar-to-IDR sandbox off-ramp | A reviewer can initiate a sell order, submit the configured test asset, and observe the burn/retirement and withdrawal state progression. |
| G-03 | Provide reusable developer infrastructure | Public REST API, `pk_test_` API key authentication, OpenAPI specification, TypeScript SDK, webhook delivery, and integration documentation are publicly reviewable. |
| G-04 | Prove the anchor architecture | SEP-24 deposit and withdrawal skeleton, KYC stub, `stellar.toml`, and federation configuration are available on Stellar testnet. |
| G-05 | Make completion independently verifiable | Public repository, live sandbox URLs, transaction hashes, webhook logs, test results, demo recording, and Completion Report are supplied. |

### 4.2 Product success metrics

The following metrics are the release gate for version `v0.1.0`:

- At least one documented successful on-ramp flow using gateway sandbox checkout.
- At least one documented successful off-ramp flow.
- At least two documented end-to-end test runs in total, with reproducible inputs and evidence.
- Every completed on-chain order has a stored Stellar testnet transaction hash.
- Duplicate gateway callbacks do not create duplicate token transfers.
- API documentation covers every public endpoint and webhook event shipped in `v0.1.0`.
- A non-technical reviewer can follow the user guide and demo without repository access being required for the primary flow.

No production adoption, transaction-volume, revenue, or mainnet metrics are committed during the Instaward sprint.

## 5. Non-goals

The following are explicitly out of scope for the 30-day release:

- Live production payments or real-money settlement.
- Stellar mainnet deployment.
- Full KYC integration with providers such as VIDA or PrivyID.
- Bappebti licensing, legal opinions, or completion of regulatory compliance.
- Native iOS or Android applications.
- Merchant acquiring, merchant checkout, or a merchant payment gateway.
- White-label products.
- Multi-provider payment routing or automatic gateway failover.
- Production SLAs, 24/7 operations, or support commitments.

These items must not be presented as delivered Instaward functionality.

## 6. Users and personas

### 6.1 Retail sandbox user

An Indonesian user or reviewer who wants to test buying a Stellar test asset using a familiar checkout method or selling the test asset back to a sandbox IDR destination.

Needs: a clear quote, payment instructions, visible order status, transaction history, and understandable error/retry guidance.

### 6.2 Integrating developer

A wallet or dApp developer evaluating KailoPay as an embedded on/off-ramp.

Needs: test API keys, stable endpoints, OpenAPI documentation, request examples, webhook verification instructions, predictable error responses, and a sandbox test guide.

### 6.3 Ambassador reviewer

A reviewer who may have limited technical depth and must verify the SOW deliverables.

Needs: public URLs, a short user guide, a demo video, screenshots, testnet explorer links, webhook evidence, and a Completion Report mapped to the three SOW deliverables.

### 6.4 KailoPay operator/developer

The person running the demo environment and diagnosing failed orders.

Needs: structured logs, searchable order identifiers, safe replay procedures, environment setup documentation, and a release/evidence checklist.

## 7. Core user journeys

### 7.1 On-ramp journey

1. User selects the supported test asset and enters an IDR amount.
2. System displays a quote and identifies that the environment uses sandbox funds and Stellar testnet assets.
3. User supplies a valid Stellar testnet destination account.
4. System creates an on-ramp order and a gateway checkout for QRIS or bank transfer.
5. User completes the sandbox payment.
6. Gateway sends a callback; KailoPay validates authenticity and processes it idempotently.
7. KailoPay issues or distributes the configured test asset.
8. User sees the completed order, amount, destination account, and testnet transaction link.

### 7.2 Off-ramp journey

1. User selects the supported test asset and enters the amount to sell.
2. System creates an off-ramp order and displays the asset deposit instructions.
3. User transfers the test asset to the configured KailoPay testnet account or follows the SEP-24 withdrawal flow.
4. KailoPay detects or verifies the asset transfer.
5. KailoPay burns or retires the test asset according to the configured issuance model.
6. KailoPay initiates the supported sandbox withdrawal/payout step or records the sandbox withdrawal instruction when provider payout simulation is limited.
7. User sees the final status, transaction hash, and sandbox withdrawal reference.

### 7.3 Developer API journey

1. Developer obtains a key with the `pk_test_` prefix.
2. Developer creates a sandbox order through the REST API.
3. Developer redirects the user to, or displays, gateway checkout instructions.
4. Developer polls order status and/or receives signed KailoPay webhooks.
5. Developer reconciles the order using KailoPay order ID, gateway reference, and Stellar transaction hash.

### 7.4 SEP-24 journey

1. Wallet discovers the anchor through `stellar.toml`.
2. Wallet initiates an interactive deposit or withdrawal request.
3. User completes the stubbed KYC step.
4. The interactive flow creates the corresponding KailoPay order.
5. Order status reflects payment and on-chain state changes through the testnet lifecycle.

## 8. Functional requirements

### 8.1 Accounts and API access

| ID | Label | Requirement |
|---|---|---|
| FR-001 | Committed | The platform shall provide test API keys using the `pk_test_` prefix. |
| FR-002 | Derived | API keys shall be stored as non-reversible hashes and displayed in full only when created. |
| FR-003 | Derived | Every authenticated request shall be associated with an API client identifier for audit and rate-control purposes. |
| FR-004 | Committed | The web application shall include a developer section for test API key management and integration documentation. |
| FR-005 | Derived | The platform shall use one user identity model with explicit retail, developer, and restricted operator roles/capabilities. |
| FR-006 | Derived | Retail web flows shall use short-lived, revocable sessions and shall restrict order access to the authenticated retail user/session. |
| FR-007 | Derived | Every order shall have exactly one ownership context: retail user/session or developer API client, never both. |
| FR-008 | Derived | An authenticated user may opt into Developer Mode and shall access only API clients, API keys, webhook endpoints, and API-created orders that they own directly or through their API clients. |
| FR-009 | Derived | Operator/admin access shall be private and shall not be available through retail sessions or public test API keys. |

### 8.2 Quotes and orders

| ID | Label | Requirement |
|---|---|---|
| FR-010 | Committed | The REST API shall create on-ramp orders for buying the configured Stellar test asset with IDR. |
| FR-011 | Committed | The REST API shall create off-ramp orders for selling the configured Stellar test asset to IDR. |
| FR-012 | Committed | The REST API shall return the current state and relevant references for an order. |
| FR-013 | Derived | Every order shall have a unique, non-sequential public identifier and timestamps. |
| FR-014 | Derived | The order shall retain requested amount, quoted amount, fees if applicable, payment method, asset code, network, destination/source account, and external references. |
| FR-015 | Derived | The API shall reject unsupported currencies, assets, networks, payment methods, amounts, and malformed Stellar accounts with a documented error. |
| FR-016 | Derived | Create-order operations shall support an idempotency key to prevent duplicate orders after client retries. |
| FR-017 | Derived | Order status changes shall be append-only in an order-event audit trail. |

### 8.3 Payment gateway sandbox

| ID | Label | Requirement |
|---|---|---|
| FR-020 | Committed | KailoPay shall integrate one provider, Xendit or Midtrans, with sandbox credentials. |
| FR-021 | Committed | On-ramp checkout shall support QRIS and at least one bank-transfer/virtual-account method available in the selected provider sandbox. |
| FR-022 | Committed | The system shall receive payment confirmation callbacks from the selected gateway. |
| FR-023 | Derived | Gateway callbacks shall be authenticated using the provider-supported verification mechanism before state mutation. |
| FR-024 | Derived | Duplicate, delayed, and out-of-order callbacks shall be handled idempotently and logged. |
| FR-025 | Derived | Gateway amounts, currency, and external order references shall be reconciled against the KailoPay order before payment is accepted. |
| FR-026 | Committed | Off-ramp processing shall initiate or demonstrate the selected provider's available sandbox withdrawal/payout mechanism. |

### 8.4 Stellar testnet processing

| ID | Label | Requirement |
|---|---|---|
| FR-030 | Committed | Stellar accounts required for issuance/distribution and processing shall be configured and funded on testnet. |
| FR-031 | Committed | A confirmed and reconciled sandbox payment shall trigger exactly one testnet asset issuance or transfer. |
| FR-032 | Committed | A valid off-ramp request and received asset shall trigger the configured burn or retirement transaction before completion. |
| FR-033 | Derived | Stellar submissions shall use a unique order reference in the transaction memo or another documented correlation mechanism. |
| FR-034 | Derived | The order shall store transaction hash, ledger/network, source, destination, asset, amount, and submission result. |
| FR-035 | Derived | A failed or uncertain Stellar submission shall enter a recoverable state and shall not be blindly resubmitted in a way that can duplicate settlement. |

### 8.5 SEP-24 and anchor discovery

| ID | Label | Requirement |
|---|---|---|
| FR-040 | Committed | KailoPay shall expose a Stellar testnet SEP-24 deposit flow skeleton. |
| FR-041 | Committed | KailoPay shall expose a Stellar testnet SEP-24 withdrawal flow skeleton. |
| FR-042 | Committed | The interactive flow shall contain a clearly labelled KYC stub and shall not claim that production identity verification has occurred. |
| FR-043 | Committed | A valid `stellar.toml` shall be publicly accessible and advertise the implemented testnet services. |
| FR-044 | Committed | Federation configuration or a minimal federation service shall be publicly accessible and documented. |
| FR-045 | Derived | SEP-24 transaction status shall map consistently to the internal order state model. |

### 8.6 Web application

| ID | Label | Requirement |
|---|---|---|
| FR-050 | Committed | The web application shall provide a buy flow with QRIS and bank-transfer sandbox checkout. |
| FR-051 | Committed | The web application shall provide a sell flow from the supported test asset to sandbox IDR withdrawal. |
| FR-052 | Committed | The application shall show order status and transaction history. |
| FR-053 | Derived | Every screen that can initiate value movement shall display `Sandbox` and `Stellar Testnet` indicators. |
| FR-054 | Derived | Completed orders shall link to the corresponding Stellar testnet explorer transaction. |
| FR-055 | Derived | Failed, expired, or pending orders shall show a human-readable explanation and safe next action. |

### 8.7 Developer webhooks

| ID | Label | Requirement |
|---|---|---|
| FR-060 | Committed | KailoPay shall deliver payment and order lifecycle events to a developer-configured webhook endpoint. |
| FR-061 | Derived | Each webhook shall include event ID, type, creation time, API version, order ID, and event payload. |
| FR-062 | Derived | Webhook requests shall be signed and the verification method shall be documented. |
| FR-063 | Derived | Failed deliveries shall use a bounded retry policy and retain attempt logs. |
| FR-064 | Derived | The same event ID may be delivered more than once; documentation shall instruct consumers to process events idempotently. |

### 8.8 TypeScript SDK

| ID | Label | Requirement |
|---|---|---|
| FR-065 | Committed | KailoPay shall provide an official SDK implemented in TypeScript while the backend remains implemented in Go. |
| FR-066 | Derived | The SDK shall be a thin client over the versioned REST/OpenAPI contract and shall not duplicate order-state or settlement business rules. |
| FR-067 | Derived | The SDK shall support creating on-ramp and off-ramp orders, retrieving and listing orders, passing idempotency keys, and exposing stable KailoPay API errors. |
| FR-068 | Derived | The SDK shall provide webhook signature verification using the exact raw request body and documented timestamp tolerance. |
| FR-069 | Derived | Secret-key operations shall target server-side Node.js usage; documentation shall prohibit embedding `pk_test_` keys in browser bundles. Amounts shall remain exact strings or integer minor units. |

### 8.9 Evidence and release

| ID | Label | Requirement |
|---|---|---|
| FR-070 | Committed | Source code shall be available in a public GitHub repository with a license and setup README. |
| FR-071 | Committed | The sandbox API and web application shall have public review URLs. |
| FR-072 | Committed | An OpenAPI specification shall describe all public endpoints in the release. |
| FR-073 | Committed | At least two end-to-end test flows shall be documented with transaction hashes. |
| FR-074 | Committed | A demo video shall show both buy and sell flows. |
| FR-075 | Committed | A Completion Report shall describe delivered scope, test results, known limitations, and the path to production. |
| FR-076 | Derived | Release `v0.1.0` shall be tagged only after the evidence checklist is complete. |

## 9. Order state model

### 9.1 On-ramp states

`created -> payment_pending -> payment_confirmed -> stellar_processing -> completed`

Terminal or exceptional states: `expired`, `payment_failed`, `stellar_failed`, `cancelled`.

Rules:

- Only a verified and reconciled gateway callback can move an order to `payment_confirmed`.
- `completed` requires a recorded successful Stellar transaction hash.
- Duplicate callbacks may add an audit event but may not repeat asset issuance.
- A manual retry from `stellar_failed` must first determine whether the previous submission reached the network.

### 9.2 Off-ramp states

`created -> asset_pending -> asset_received -> burn_processing -> withdrawal_processing -> completed`

Terminal or exceptional states: `expired`, `asset_invalid`, `burn_failed`, `withdrawal_failed`, `cancelled`.

Rules:

- The received asset, amount, network, and order reference must match the order.
- Withdrawal processing cannot begin until the asset is accepted and the configured burn/retirement step succeeds.
- `completed` requires both a Stellar transaction hash and a sandbox withdrawal/payout reference or an explicitly documented sandbox simulation result.

## 10. Public API surface

The exact paths may follow the implementation's versioning convention, but `v0.1.0` must provide equivalent operations:

| Operation | Suggested method and path | Authentication |
|---|---|---|
| Create on-ramp order | `POST /v1/onramps` | Test API key or retail session |
| Create off-ramp order | `POST /v1/offramps` | Test API key or retail session |
| Get order | `GET /v1/orders/{order_id}` | Test API key or retail session |
| List orders | `GET /v1/orders` | Test API key or retail session |
| Create/list/revoke test keys | `/v1/api-keys` | Developer session |
| Register webhook endpoint | `/v1/webhook-endpoints` | Developer session or test API key |
| SEP-24 interactive deposit | SEP-24 endpoint | As required by the skeleton |
| SEP-24 interactive withdrawal | SEP-24 endpoint | As required by the skeleton |
| Federation lookup | Federation endpoint | Public |
| Health check | `GET /health` | Public, no secrets |

The OpenAPI document is the contract authority for request and response schemas.

## 11. Non-functional requirements

| ID | Category | Label | Requirement |
|---|---|---|---|
| NFR-001 | Security | Derived | Secrets, private keys, API key plaintext, and gateway credentials shall never be committed to the public repository or returned in logs. |
| NFR-002 | Security | Derived | All public deployments shall use HTTPS. |
| NFR-003 | Security | Derived | Input shall be validated at the API boundary and database access shall use parameterized operations/ORM protections. |
| NFR-004 | Security | Derived | Gateway callbacks and outgoing developer webhooks shall use documented authenticity controls. |
| NFR-005 | Data protection | Derived | The KYC stub shall use synthetic data and avoid collecting real identity documents. |
| NFR-006 | Reliability | Derived | Payment callbacks, order creation, asset issuance, and webhook consumption shall be idempotent. |
| NFR-007 | Reliability | Derived | State transitions shall be transactional where order state and external references are persisted together. |
| NFR-008 | Observability | Derived | Logs shall be structured and correlate order ID, external payment ID, webhook event ID, and Stellar transaction hash without exposing secrets. |
| NFR-009 | Performance | Proposed | Under normal sandbox load, the API should return non-settlement requests within 1 second at p95, excluding third-party latency. |
| NFR-010 | Availability | Proposed | The public demo should remain available during the agreed review window; this is not a production SLA. |
| NFR-011 | Compatibility | Derived | The web experience shall support current desktop and mobile versions of major Chromium-based browsers. |
| NFR-012 | Maintainability | Derived | Environment configuration, migrations, seed data, tests, and local setup shall be reproducible from the public README. |
| NFR-013 | Accessibility | Proposed | Primary buy and sell flows should be keyboard operable and use labelled controls with readable contrast. |

## 12. Data requirements

At minimum, the domain model shall support:

- API clients and hashed test keys.
- Orders and direction (`onramp` or `offramp`).
- Quotes and monetary/asset amounts with explicit precision.
- Payment gateway checkouts, callbacks, and external references.
- Stellar transactions and network metadata.
- Order state-transition events.
- Developer webhook endpoints, events, delivery attempts, and results.
- Evidence references used in the final Completion Report.

Production PII storage design is out of scope. Any sandbox user data must be synthetic or minimized.
User identity, Developer Mode state, API-client ownership, and retail-session records must be sufficient to enforce the access boundaries in FR-005 through FR-009.

## 13. Analytics and observability

The sandbox release shall make the following operational questions answerable:

- How many orders exist by direction and status?
- Which order corresponds to a gateway payment reference?
- Which gateway callbacks were rejected, duplicated, or processed?
- Which Stellar transaction corresponds to an order?
- Which developer webhook events failed and how many retries occurred?
- Can the operator reconstruct the state-transition history of a test order?

A full analytics platform is not required; structured logs and database queries are sufficient for the Instaward.

## 14. Acceptance scenarios

### AC-01: Successful QRIS on-ramp

Given a valid Stellar testnet destination and supported IDR amount, when the user completes a QRIS sandbox payment and the gateway callback is verified, then exactly one asset issuance/transfer occurs, the order becomes `completed`, and the UI/API exposes the testnet transaction hash.

### AC-02: Successful bank-transfer on-ramp

Given a valid order, when the user completes the supported sandbox bank-transfer method, then the payment is reconciled and the test asset is delivered exactly once.

### AC-03: Duplicate payment callback

Given an already processed payment callback, when the same gateway event is delivered again, then no second asset transfer is created and the duplicate is visible in logs/audit history.

### AC-04: Successful off-ramp

Given a valid off-ramp order, when the expected test asset is received, then the asset is burned or retired, the sandbox withdrawal is initiated or demonstrably simulated, and the final order contains both on-chain and withdrawal evidence.

### AC-05: Invalid Stellar account

Given a malformed or network-inappropriate account, when an order is submitted, then the API rejects it with a documented client error and no checkout/order settlement is created.

### AC-06: Developer integration

Given a valid `pk_test_` key, when a developer creates an order and registers a webhook endpoint, then the documented lifecycle events are delivered with a verifiable signature and stable order reference.

### AC-07: SEP-24 discovery

Given a compatible test wallet or manual reviewer, when it reads `stellar.toml` and starts deposit/withdrawal, then the advertised endpoints resolve to the interactive sandbox flows and expose a KYC stub.

### AC-08: Evidence review

Given only the published evidence package, when the Ambassador reviewer follows the verification checklist, then each of the three SOW deliverables can be classified as present, partial, or missing without private system access.

## 15. Assumptions and decisions required

| ID | Topic | Current assumption | Required decision |
|---|---|---|---|
| DEC-01 | Payment gateway | Exactly one of Xendit or Midtrans is integrated for `v0.1.0`. | Select provider before gateway implementation begins. |
| DEC-02 | Stellar asset | A non-production test asset will demonstrate issuance and burn/retirement. | Confirm asset code, issuer model, decimals, and explorer representation. |
| DEC-03 | Off-ramp payout | Provider sandbox capability may differ from production payout behavior. | Define the acceptable evidence when a true sandbox payout is unavailable. |
| DEC-04 | Application authentication | Use one unified user identity model. Retail uses a lightweight guest session; an authenticated user may opt into Developer Mode and directly own API clients; API calls use client-owned `pk_test_` keys. | Approved for `v0.1.0`: team workspaces and member-role administration are deferred. Developer Mode controls dashboard access, while explicit client/key status controls machine access. |
| DEC-05 | Hosting | Public API, web, and anchor discovery require stable review URLs. | Select hosts and domain/subdomain layout. |
| DEC-06 | Repository license | SOW requires a public repository with a license. | Select the open-source license before publication. |
| DEC-07 | Backend runtime | The repository implements the backend in Go with Gin, GORM PostgreSQL, Viper, and `log/slog`. | Project decision for `v0.1.0`: document this implementation-stack variance because the original SOW described Node.js/TypeScript; functional deliverables and acceptance evidence remain unchanged. |
| DEC-08 | Developer SDK runtime | The official integration SDK remains implemented in TypeScript and is versioned against the public API/OpenAPI contract. | Approved for `v0.1.0`: keep the SDK separate from the Go backend and target server-side Node.js for secret-key operations. |

Unresolved decisions shall be recorded in an ADR or project decision log and must not silently expand scope.

## 16. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Gateway approval or sandbox feature limitations | Blocks QRIS, bank transfer, or payout evidence | Validate sandbox accounts and the exact supported methods in Phase 0; document any provider limitation immediately. |
| Duplicate callbacks or uncertain network submission | Duplicate asset movement | Enforce idempotency, persist external IDs, and verify network state before retry. |
| SEP-24 scope becomes larger than a skeleton | Consumes the 30-day delivery window | Limit to deposit/withdraw interactive flows, KYC stub, discovery, and testnet lifecycle required by the SOW. |
| One developer across API, anchor, web, docs, and deployment | Schedule slippage | Use a vertical-slice sequence, enforce a Must-have cut line, and create evidence continuously. |
| Public repository exposes secrets or private keys | Security incident and invalid delivery | Use secret scanning, `.env.example`, test-only funded accounts, and a release security checklist. |
| Regulatory expectations are mistaken for production readiness | Reputational/legal risk | Display sandbox/testnet labels and explicitly document that licensing, production KYC, and real-money operation are excluded. |
| External services are unavailable during review | Evidence cannot be verified | Retain screenshots, logs, transaction hashes, and recorded demo in addition to live URLs. |

## 17. Release and scope control

Version `v0.1.0` is eligible for release when all Must-have backlog items, the two end-to-end evidence flows, documentation, public deployments, demo recording, and Completion Report have passed review.

Any proposed addition must identify:

1. The SOW deliverable it supports.
2. The backlog items displaced or the added capacity.
3. Its acceptance evidence.
4. Whether it changes the 30-day commitment.

Items without a direct Instaward outcome shall move to a future phase rather than enter the active sprint.

## 18. Future product direction

Future phases may cover production payment contracts, full KYC and AML workflows, legal and regulatory readiness, Stellar mainnet, security hardening, production operations, mobile applications, merchant tools, white-label capabilities, additional assets, and additional payment providers. These are roadmap candidates, not `v0.1.0` commitments.
