# Backend Week 3 Developer Experience Design

Status: Proposed for review

## Intent

Week 3 turns the KailoPay backend into a usable developer platform for
applications that let their users exchange IDR and Stellar testnet XLM. The
backend must expose the operational data and controls a developer needs to
integrate, monitor, and troubleshoot that exchange without changing the
existing `/v1/onramps` or `/v1/offramps` contracts.

This design is backend-only. It does not add or modify frontend code.

## Shared understanding and assumptions

- A developer is an authenticated KailoPay account that owns one or more API
  clients. API keys authenticate a client, while the developer dashboard uses
  the account's retail session.
- Dashboard aggregates are account-level by default and accept an optional
  `client_id` filter. This gives a developer one view across clients without
  losing client-level isolation.
- Orders remain the source of truth. Dashboard, analytics, revenue, and
  webhook views are read or audit projections over durable order and event
  records.
- API keys are server-side credentials. They must not be required in browser
  examples or exposed in dashboard responses after creation.
- Wallet addresses in a profile are verified external wallets. A submitted
  address is never trusted without a matching SEP-10 proof.
- The current environment is a sandbox on Stellar testnet. Revenue and payout
  values are explicitly marked simulated or estimated when no real fiat rail
  has moved value.
- Existing `/v1/onramps`, `/v1/offramps`, `/v1/orders`, and their error,
  idempotency, and response contracts remain compatible. New developer views
  are additive routes.

## Goals

1. Give developers a reliable account overview of API usage and exchange
   health.
2. Make order, Stellar, payment, payout, and webhook correlation inspectable.
3. Show fees and revenue without claiming that sandbox values are real money.
4. Make API-key and webhook lifecycle management safe and auditable.
5. Allow a developer to associate a verified wallet with their profile.
6. Keep read queries bounded, owner-scoped, and usable by a future dashboard.

## Non-goals

- Frontend screens or frontend state management.
- An internal operator/admin dashboard.
- Real bank payout or a production revenue-sharing program.
- Custodial wallet balances or private-key storage.
- Changing the legacy on-ramp/off-ramp request or response shape.
- A TypeScript SDK in this backend repository. SDK work can consume the final
  OpenAPI contract separately.

## Developer account and ownership model

The existing account-to-client-to-key relationship remains the ownership
boundary:

```text
retail user session
        |
        +-- developer account
                |
                +-- API client (test environment)
                        |
                        +-- API keys
                        +-- orders
                        +-- webhook endpoints
```

Session-authenticated developer routes resolve the user first and aggregate
only clients owned by that user. API-key-authenticated order routes continue to
see only the key's client-owned orders. No request may select an arbitrary
owner, client, or wallet account.

## Backend capabilities

### 1. Developer overview

Add a session-authenticated `GET /v1/developer/overview` route.

Query parameters:

- `client_id` optional owner-scoped filter.
- `from` and `to` optional UTC RFC3339 range.
- `currency` optional `IDR` or `XLM`; default returns both summaries.

The response contains:

- environment and network;
- period boundaries;
- total orders, active orders, completed orders, and failed orders;
- buy and sell counts;
- gross IDR volume, XLM volume, fee amount, and net amount as strings;
- success rate and average completion duration when enough data exists;
- pending webhook deliveries and exhausted deliveries;
- a bounded list of recent orders with stable correlation identifiers.

The overview is a read model. It must not recalculate or mutate an order
state, quote, payout, or webhook attempt.

### 2. Usage analytics

Add `GET /v1/developer/analytics` with a bounded time range and explicit
bucket size. The initial implementation supports `hour`, `day`, and `week`
within a maximum configurable range.

Each bucket can report:

- order count by direction and status;
- gross IDR and XLM volume;
- fee and net totals;
- completion and failure counts;
- average and percentile-free bounded latency summaries;
- payment-method counts for on-ramp orders;
- webhook delivery success, retry, and exhausted counts.

The query must be account-scoped and use indexed predicates on owner/client
and created timestamps. It must not emit high-cardinality labels such as raw
order IDs, API keys, wallet addresses, or webhook URLs in aggregate metrics.

### 3. Fees and revenue ledger

Add an immutable per-order financial snapshot owned by the order transaction.
The record stores:

- order ID and client ID;
- direction and environment;
- gross IDR amount;
- fee amount and fee currency;
- platform revenue amount;
- developer revenue-share amount, zero unless an explicit policy exists;
- net amount;
- fee policy version and source;
- `estimated` or `simulated` marker;
- created timestamp.

The default sandbox policy may set fees to zero, but the ledger still records
the policy result. A later fee-policy change must not rewrite historical rows.
Corrections use an adjustment record rather than mutating the original
snapshot.

Add:

- `GET /v1/developer/revenue/summary` for period totals;
- `GET /v1/developer/revenue/entries` for cursor-paginated order-level
  entries.

The response must say that the figures are sandbox estimates. It must not
imply that a developer has earned real funds or that a simulated payout is a
bank settlement.

### 4. Developer orders

Add `GET /v1/developer/orders` as an account-level, cursor-paginated order
read model. It supports:

- `client_id`;
- `direction`;
- `status`;
- `payment_method`;
- `from` and `to`;
- `limit` and `paging_id`.

Each row includes the existing public order view plus safe developer
correlation fields:

- API client ID and safe client name;
- gateway checkout/reference when present;
- Stellar intent and transaction hash when present;
- SEP-24 transaction ID, quote ID, and wallet account when present;
- payout reference and sandbox disclosure when present;
- latest state transition and timestamps.

The detail route remains owner-scoped. The existing public `/v1/orders` route
is not changed to expose account-wide data.

### 5. API-key management

Keep the existing API-key routes and semantics:

- `POST /v1/api-keys`;
- `GET /v1/api-keys`;
- `DELETE /v1/api-keys/{id}`.

Make them dashboard-ready by ensuring the safe metadata includes client name,
environment, prefix, created time, last-used time, and revoked time. The
plaintext secret is returned only once from creation. Usage updates must not
block the authenticated request if the usage timestamp is only observability
data, but failures must be visible in structured logs and tests.

Key ownership, Developer Mode, approved KYC, revocation, and constant-time
verification remain enforced in the usecase and repository layers.

### 6. Webhook delivery and observability

Retain the existing endpoint-management routes and add the missing delivery
surface:

- `GET /v1/webhook-endpoints/{id}`;
- `GET /v1/webhook-deliveries` with endpoint, event, status, and date filters;
- `POST /v1/webhook-endpoints/{id}/test` to enqueue a signed test event;
- `POST /v1/webhook-deliveries/{id}/replay` for a safe replay of an exhausted
  delivery using the same event ID.

Delivery behavior:

- persist a durable attempt before calling the endpoint;
- resolve and validate the destination at registration and immediately before
  delivery;
- reject loopback, private, link-local, metadata, and reserved destinations;
- use an explicit timeout and do not follow unsafe redirects;
- sign the raw body with a versioned HMAC signature containing event ID and
  timestamp;
- retry only bounded, retryable failures with persisted next-attempt time;
- mark an event exhausted after the configured attempt limit;
- use database leases so only one worker owns an attempt;
- never hold a database transaction open during the HTTP call.

Dashboard-safe delivery metadata includes event ID, endpoint ID, event type,
attempt number, status, HTTP status, duration, safe error, and next attempt.
It never includes the signing secret or raw sensitive provider payload.

### 7. Verified wallet profile settings

Add a developer wallet association under a new session-authenticated route:

- `GET /v1/developer/wallets`;
- `POST /v1/developer/wallets`;
- `PATCH /v1/developer/wallets/{id}`;
- `DELETE /v1/developer/wallets/{id}`.

The stored record includes wallet account, network, label, primary flag,
verification method, verified time, status, and timestamps. The database
enforces one active primary wallet per user and network.

`POST` requires a SEP-10 bearer proof whose wallet account and network match
the submitted record. The server stores the verified public account only. It
never stores a wallet secret seed or treats a user-provided account string as
proof.

Wallet settings are a developer convenience and a default integration hint.
They do not override the wallet identity supplied by a canonical SEP-24
request and do not alter legacy on-ramp/off-ramp request fields.

## Data and query design

Prefer additive tables and indexes over widening legacy public DTOs:

- `order_financials` or an equivalent immutable fee-ledger table;
- `developer_wallets`;
- indexes for owner/client plus created time and status;
- indexes for webhook attempt status plus next-attempt time;
- optional rollup tables only if query measurements show that direct indexed
  aggregation is insufficient.

All money values use integer IDR minor units or exact Stellar quantities.
Aggregate JSON values preserve exact strings. All timestamps are UTC.

## Error and authorization rules

- Account-wide developer routes return only resources owned by the session user.
- A `client_id` that belongs to another developer is indistinguishable from a
  missing resource at the public boundary.
- Invalid date ranges, bucket sizes, limits, and cursors return stable
  validation errors.
- Revoked API keys and disabled webhook endpoints cannot mutate or deliver.
- Replay is allowed only for an owned exhausted delivery and does not create a
  second public event ID.
- Wallet association fails unless SEP-10 ownership proof is valid and fresh.
- Revenue responses carry sandbox/simulation disclosure.

## Testing and acceptance

Unit and handler tests cover:

- account and client ownership filters;
- empty and bounded aggregate windows;
- exact amount aggregation without floating point arithmetic;
- fee-ledger immutability and zero-fee sandbox behavior;
- API-key metadata and secret non-disclosure;
- webhook signature vectors, SSRF cases, retry classification, leases, and
  replay idempotency;
- wallet proof mismatch, duplicate primary wallet, revoke, and cross-user
  access;
- unchanged legacy on-ramp/off-ramp contract tests.

Repository integration tests cover migrations, constraints, indexes,
concurrent delivery leasing, financial snapshot creation, and cross-client
queries. Full verification remains:

```text
gofmt -w internal cmd
go test ./...
go vet ./...
go test -race ./...
```

Week 3 backend acceptance is met when the new developer routes are documented
in OpenAPI, ownership and security tests pass, webhook delivery is observable,
and a developer can reconstruct an order, its money summary, its Stellar
references, and its webhook attempts without direct database access.

## Explicitly preserved behavior

- No changes to the request shape, required fields, route paths, idempotency
  semantics, or error envelope of `/v1/onramps` and `/v1/offramps`.
- No changes to the existing API-key authentication contract for order routes.
- No real fiat movement is claimed by the sandbox simulator.
- No frontend files are in scope for this backend Week 3 implementation.
