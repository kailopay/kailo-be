# KailoPay Backend Documentation

| Field | Value |
|---|---|
| Baseline | KailoPay Instaward Sandbox MVP `v0.1.0` |
| Runtime assumption | Go 1.25+ with Gin |
| Primary database | PostgreSQL |
| Configuration | Viper with validated startup config |
| Persistence | GORM PostgreSQL adapter; consumer-owned interfaces remain outside GORM |
| Logging | Standard-library `log/slog` |
| External environments | Auth0 tenant, one payment gateway sandbox, and Stellar testnet |
| Architecture | Modular monolith with asynchronous workers and transactional outbox |
| Status | Implementation design for review |

## Purpose

This folder translates the product-level requirements in the parent `documentations/` folder into an actionable backend design. It covers only the 30-day sandbox/testnet commitment unless a section is explicitly labelled Future.

The documents describe intended behavior and boundaries. The checked-in OpenAPI file, reviewed migrations, configuration schema, and automated tests become the executable contracts once implementation begins. The repository currently includes a Go/Gin/GORM bootstrap under `cmd/` and `internal/`; business use cases and provider adapters are added incrementally.

## Go implementation notes

- `cmd/api` is the HTTP composition root; `cmd/worker` is the background-process composition root when workers are enabled.
- `internal/service/<capability>` owns narrow interfaces required by each use case.
- `internal/repository` contains concrete GORM persistence models and repositories; those models do not cross into entities or handler DTOs.
- `internal/platform` contains configuration, database lifecycle, and structured logging setup; it contains no business policy.
- `internal/adapter/<provider>` contains external payment and Stellar adapters; provider SDK types do not cross the adapter boundary.
- `cmd/automigrate` is a local/test convenience command only. It refuses other environments. Production schema changes must use reviewed, versioned migrations.

## Recommended reading order

1. [Backend Architecture](BACKEND-ARCHITECTURE.md)
2. [Domain Model](DOMAIN-MODEL.md)
3. [Order State Machines](ORDER-STATE-MACHINES.md)
4. [Database Design](DATABASE-DESIGN.md)
5. [API Design](API-DESIGN.md)
6. [Payment Gateway Integration](PAYMENT-GATEWAY-INTEGRATION.md)
7. [Stellar Anchor Integration](STELLAR-ANCHOR-INTEGRATION.md)
8. [Webhook Design](WEBHOOK-DESIGN.md)
9. [TypeScript SDK Design](TYPESCRIPT-SDK-DESIGN.md)
10. [Security](SECURITY.md)
11. [Observability and Runbook](OBSERVABILITY-AND-RUNBOOK.md)
12. [Testing Strategy](TESTING-STRATEGY.md)
13. [Implementation Phases](IMPLEMENTATION-PHASES.md)
14. [Backend Backlog](BACKEND-BACKLOG.md)

## Parent documents

- [Product Requirements Document](../PRD.md)
- [Product Delivery Phases](../PHASES.md)
- [Product Backlog](../BACKLOG.md)
- [Documentation Plan](../DOCUMENTATION-PLAN.md)

## Backend scope

### Included in `v0.1.0`

- Versioned REST API for sandbox on-ramp and off-ramp orders.
- Unified user identity with retail sessions, opt-in Developer Mode, and restricted operator access boundaries.
- User-owned developer API clients; API keys, webhook configuration, and API-created orders are scoped through their owning client.
- Hashed test API keys using the `pk_test_` prefix.
- PostgreSQL persistence, migrations, order events, and external-reference correlation.
- One payment gateway adapter for sandbox QRIS, bank transfer/virtual account, callbacks, and the available payout simulation.
- Stellar testnet asset issuance/transfer, deposit verification, burn/retirement, and transaction correlation.
- SEP-24 deposit and withdrawal skeleton, KYC stub, public `stellar.toml`, and federation configuration.
- Signed developer webhook delivery with bounded retries and attempt logs.
- Separate TypeScript SDK for server-side Node.js integrations; it consumes the Go backend's public API and OpenAPI contract.
- Structured logging, metrics, health/readiness checks, safe recovery procedures, and release evidence.

### Excluded from `v0.1.0`

- Production payment credentials or real IDR settlement.
- Stellar mainnet and production custody.
- Full KYC/AML, sanctions screening, or identity-document collection.
- Regulatory licensing implementation.
- Multi-gateway routing, merchant gateway, white-label, and mobile-specific backend features.
- Production SLA, 24/7 on-call, or high-availability commitments.

## Design invariants

1. Every environment and response makes sandbox/testnet status explicit.
2. Money and asset quantities never use binary floating-point arithmetic.
3. Untrusted callbacks do not mutate order state.
4. Duplicate requests, callbacks, jobs, Stellar submissions, and webhook deliveries are expected and handled safely.
5. A database transaction cannot make an external payment or Stellar transaction atomic; external effects use durable intent, idempotent processing, and reconciliation.
6. Order history is append-only and sufficient to reconstruct why a state changed.
7. Secrets, private keys, full API keys, and raw sensitive provider payloads never enter public logs.
8. Production-only requirements remain in roadmap documents and do not silently enter the Instaward release.

## Source-of-truth order

1. Finalized SOW for Instaward commitment.
2. Parent `PRD.md` for product behavior and acceptance.
3. `API-DESIGN.md`, then the checked-in OpenAPI specification for public API behavior.
4. `ORDER-STATE-MACHINES.md` for legal lifecycle transitions.
5. `DOMAIN-MODEL.md` and `DATABASE-DESIGN.md` for invariants and persistence.
6. Integration documents for provider-specific behavior.
7. `BACKEND-BACKLOG.md` for execution status.

Material design changes require an ADR and synchronized updates to affected documents, tests, and API contracts.

## Decisions required before implementation

- Select Xendit or Midtrans after confirming exact sandbox capabilities.
- Confirm test asset code, precision, issuer/distributor accounts, and retirement method.
- Agree acceptable off-ramp evidence when the selected gateway cannot execute a true sandbox payout.
- Select deployment topology, public hostnames, and managed-secret mechanism.
- Configure the selected Auth0 OIDC tenant and the minimum developer-session mechanism used to create and revoke test API keys.

Until those decisions are resolved, payment-provider classes, URLs, and secret names remain adapter/configuration concerns rather than embedded domain logic.
