# Domain Model

## Week 1 implemented profile

The current order slice supports both an API-client principal and a verified
retail-session principal. Either principal requests an exact quote, reserves
pre-funded treasury stroops for an on-ramp, and receives XLM only after
authenticated payment reconciliation. Retail orders are owned by the user and
the creating session; API orders are owned by the API client. `TreasuryReservation`
prevents the same wallet inventory from backing multiple checkouts.
`StellarTransaction` and the transactional outbox preserve one settlement
intent across retries and unknown network outcomes. Off-ramp orders use the
same principal and idempotency rules and expose sandbox payout evidence.

## 1. Domain boundaries

The core domain is the lifecycle of a sandbox conversion order. Payment checkout, Stellar settlement, SEP-24, and outgoing webhooks are supporting capabilities around that lifecycle.

The order aggregate owns legal state transitions and invariants. External adapters report facts; they must not directly update arbitrary order columns.

## 2. Core value objects

| Value object | Fields | Invariants |
|---|---|---|
| `OrderId` | UUID v7 or ULID string | Globally unique, non-sequential public identifier |
| `Money` | integer minor units, ISO currency | Currency is `IDR` in `v0.1.0`; amount positive; no floating point |
| `AssetAmount` | decimal string/integer stroops, asset code, issuer, network | Positive, precision within configured asset limit, testnet only |
| `StellarAccount` | public key and optional memo/muxed details | Valid encoding and permitted testnet use |
| `PaymentMethod` | `xendit`, `qris`, or `bri_va` | Hosted checkout with all channels, or a restricted active gateway channel |
| `IdempotencyKey` | opaque client value | Scoped to authenticated API client or retail user and operation; length/charset bounded |
| `ExternalReference` | provider, type, value | Unique within provider and reference type |
| `FailureReason` | stable code, safe message, retryability | Does not contain secret or raw sensitive provider payload |

All amounts cross JSON API boundaries as strings or integer minor units. Floating-point values must not represent settlement quantities in Go, JSON contracts, or PostgreSQL.

## 3. Aggregates and entities

### 3.1 User and access identity

The `User` identity is shared by retail users, opt-in developers, and restricted
operators. Self-hosted email/password or optional Google OIDC authenticates the
person, while the local user, verified-email state, session, and authorization
state remain owned by KailoPay.

Fields:

- `id`, status, display name, optional email profile, nullable email-verification timestamp, nullable `developer_enabled_at`, and created/updated timestamps.
- One or more external identity links containing provider and stable subject; `email` is not used as the identity key.
- Retail sessions containing only a token hash, expiry, revocation, and last-used timestamps.

Invariants:

- A retail session can access only orders owned by that session/user.
- Session tokens are high entropy, displayed only to the browser, and never stored or logged in plaintext.
- An active authenticated user may enable Developer Mode for developer-management access.
- Disabling Developer Mode does not silently revoke API clients or keys; revocation is explicit.
- Production identity verification and real identity documents are not part of `v0.1.0`.

### 3.1.1 Order principal

`OrderPrincipal` is the application value passed through order handlers, use
cases, and repositories. It has an explicit kind and exactly one scope:

| Kind | Required fields | Ownership rule |
|---|---|---|
| `api_client` | `client_id`, owning `user_id` | Match `orders.client_id`; API-client idempotency scope |
| `retail_session` | `user_id`, `session_id` | Require `client_id IS NULL`, a non-null creating session, and matching `created_by_user_id`; retail-user idempotency scope |

The HTTP boundary chooses an API principal whenever `Authorization` is present;
otherwise it resolves the active `kailopay_session` cookie. Retail order
creation additionally requires email verification and an allowed browser origin
for `POST` mutations. No request field can choose or override the principal.

### 3.2 API client

Fields:

- Non-null owner user ID and creator/administrative ownership context.
- `id`, display name, environment (`test`), status, created/revoked timestamps.
- One or more API key records containing public lookup ID, secret hash, key prefix, last-used timestamp, and revocation timestamp.

Invariants:

- Only `pk_test_` keys are issued in `v0.1.0`.
- Full key plaintext is returned only at creation.
- Revoked clients/keys cannot authenticate new requests.
- Developer-management access requires the authenticated user to own the API client.
- Operator access is private and is not inferred from retail or Developer Mode access.

### 3.3 Order aggregate

Fields:

- Identity: `id`, ownership route, optional client/user/session IDs, direction, status, version, created/updated/expiry timestamps.
- Quote: IDR amount, asset amount, fees if any, rate source/description, quote expiry.
- Route: payment method, configured gateway, Stellar asset/network.
- Parties: Stellar source/destination and minimal synthetic sandbox withdrawal destination.
- Correlation: payment checkout/reference, payout reference, Stellar transaction references.
- Failure: safe code/message, retryability, failed stage.

Invariants:

- Direction is immutable.
- Currency, asset, network, and ownership route are immutable after checkout/asset instructions are issued.
- An order is owned either by an API client or by a retail user/session, never both.
- API authorization is resolved from the authenticated API client; retail authorization is resolved from the authenticated user plus active creating session. A user can read consumer orders created in an earlier valid session.
- State changes use optimistic versioning and a legal transition table.
- A completed on-ramp has a reconciled payment and successful Stellar transaction.
- A completed off-ramp has verified asset receipt, successful burn/retirement, and payout/simulation evidence.
- A failure never erases previously recorded external evidence.

### 3.4 Order event

An immutable record of a domain-relevant change:

- Event ID/type/time, order ID, previous/new state, actor/source, correlation ID, safe metadata, and aggregate version.
- Sources include `api`, `gateway_callback`, `stellar_worker`, `reconciliation`, and `operator`.

Order events are audit history, not the sole persistence mechanism; `orders` retains current state for efficient access.

### 3.5 Payment checkout and event

Payment checkout stores provider, external checkout/session ID, method, IDR amount, status, expiry, hosted redirect URL, and sanitized provider response metadata.

Gateway event stores provider event ID/fingerprint, callback type, verified flag, received/processed time, matching result, payload hash, and processing error. Raw payload retention is minimized and redacted.

### 3.6 Stellar transaction

Stores purpose (`issue`, `transfer`, `deposit`, `burn`, `retire`), network, asset, amount, source/destination, memo/correlation reference, transaction hash, envelope/submission ID if safe, ledger time, status, attempt count, and last error.

The transaction hash is unique when known. An intent ID remains stable across reconciliation of an uncertain submission.

### 3.7 Developer webhook endpoint, event, and attempt

- Endpoint: owner client, URL, encrypted signing secret or derived key reference, subscribed types, status.
- Event: stable event ID/type/version, order reference, canonical payload, creation time.
- Attempt: endpoint, attempt number, scheduled/start/finish time, response code, duration, safe error, next retry.

## 4. Domain services

| Service | Responsibility |
|---|---|
| Quote policy | Validate supported route and derive configured sandbox quote/fees without claiming production market pricing |
| Order transition policy | Decide whether a requested transition is legal and which domain events result |
| Payment reconciliation policy | Compare callback provider, reference, currency, amount, and expected order state |
| Stellar settlement policy | Decide issuance/retirement intent and validate received asset evidence |
| Webhook event mapper | Convert internal domain events into stable public developer events |

Domain services remain deterministic; network calls occur behind application ports.

## 5. Application commands

Primary commands:

- `CreateOnrampOrder`
- `CreateOfframpOrder`
- `CreatePaymentCheckout`
- `ProcessGatewayCallback`
- `IssueOnrampAsset`
- `VerifyOfframpDeposit`
- `RetireOfframpAsset`
- `InitiateSandboxPayout`
- `ReconcileExternalOperation`
- `CreateApiKey` / `RevokeApiKey`
- `RegisterWebhookEndpoint`
- `DeliverDeveloperWebhook`

Every command has an explicit authentication/actor context, correlation ID,
input validation, transaction boundary, and idempotency rule. Order commands
carry `OrderPrincipal`; repositories never infer ownership from an empty client
ID or caller-supplied owner fields.

## 6. Domain events

Minimum internal event catalog:

- `order.created`
- `payment.checkout_created`
- `payment.confirmed`
- `payment.failed`
- `stellar.issuance_requested`
- `stellar.issuance_succeeded`
- `stellar.issuance_failed`
- `stellar.deposit_received`
- `stellar.retirement_requested`
- `stellar.retirement_succeeded`
- `withdrawal.requested`
- `withdrawal.completed`
- `withdrawal.failed`
- `order.completed`
- `order.failed`

Internal events may be richer than public developer webhooks. Public schemas require explicit versioning and data minimization.

## 7. Identity and correlation

Each end-to-end flow must be traceable using:

- `request_id` for one inbound HTTP request.
- `correlation_id` propagated across commands/jobs.
- `order_id` as the business root.
- Provider checkout/event/payout reference.
- Stellar intent ID, memo/correlation value, and transaction hash.
- Developer webhook event and attempt ID.

Logs use these identifiers, never private keys or full API-key secrets.

## 8. Precision and time

- IDR is stored as integer minor units according to the chosen API convention; the convention must be explicit in OpenAPI.
- Stellar amounts use exact decimal/string or stroop-based arithmetic according to asset precision.
- Rounding policy is deterministic, tested, and applied once at quote creation.
- Persist timestamps in UTC with timezone-aware database types.
- Use an injected clock in domain/application tests.
- Expiry decisions use server time and are recorded as events.

## 9. Data retention for sandbox

`v0.1.0` stores only data required to demonstrate and debug sandbox orders. Use synthetic identities and destinations. Retention length may be operationally configured, but deletion must not remove evidence needed for the agreed review period.

Production PII, identity documents, AML records, and legal retention schedules are Future scope.

## 10. Domain acceptance invariants

- No payment confirmation without a verified, matched callback.
- No on-ramp asset movement before payment confirmation.
- No off-ramp withdrawal before valid asset receipt and retirement.
- No terminal success without durable external evidence.
- No transition from a terminal state except an explicitly modelled administrative correction in a future design.
- No retry path can bypass reconciliation of an unknown external outcome.
- No retail session can read, replay, or mutate another user's order or
  idempotency record.
- A same-key retry after retail-session renewal replays the same user-scoped
  order and never creates a second intent.
