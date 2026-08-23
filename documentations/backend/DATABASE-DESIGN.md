# Database Design

## Week 1 migration status

`migrations/000001_week1_onramp.up.sql` is the authoritative clean-schema
migration. It includes users/sessions/credentials, one test API client per
developer/environment, hashed API keys, immutable quote fields, orders/events,
Xendit checkout and callback receipts, treasury accounts/reservations,
idempotency records, Stellar intents, and durable outbox leases.
`migrations/000002_payment_method_bri_va.sql` realigns the stored
`payment_method`/`method` enum with the public `qris`/`bri_va` contract.
`migrations/000003_self_hosted_auth.sql` adds Argon2id password credentials,
single-use verification/reset challenge tokens, and a partial unique index on
`users(email)` (ADR-002).
Runtime services use explicit transactions; `AutoMigrate` remains local/test
bootstrap only.

PostgreSQL integration tests in
`internal/repository/onramp_integration_test.go` apply these migrations and
verify constraints, reservations, settlement, and expiry behavior. They are
gated on `TEST_DATABASE_DSN` pointing at a disposable database and skip
otherwise.

## 1. Database principles

- PostgreSQL is the authoritative local state store.
- Use migrations; never rely on ORM auto-sync in shared environments.
- Use `timestamptz` in UTC and database-generated/default timestamps where appropriate.
- Monetary and asset values use exact types (`bigint` minor units or constrained `numeric`), never floating point.
- Public IDs use UUID v7/ULID-compatible values; internal sequence IDs are optional but never exposed.
- Foreign keys, unique constraints, and check constraints enforce invariants in addition to application validation.
- Store normalized provider data and minimal sanitized metadata, not unrestricted raw callback payloads.

## 2. Logical schema

```mermaid
erDiagram
    USERS ||--o{ USER_IDENTITIES : links
    USERS ||--o{ RETAIL_SESSIONS : authenticates
    USERS ||--o{ API_CLIENTS : owns
    API_CLIENTS ||--o{ API_KEYS : owns
    USERS ||--o{ ORDERS : creates_retail
    RETAIL_SESSIONS ||--o{ ORDERS : scopes
    API_CLIENTS ||--o{ ORDERS : creates_api
    ORDERS ||--o{ ORDER_EVENTS : records
    ORDERS ||--o{ PAYMENT_CHECKOUTS : uses
    PAYMENT_CHECKOUTS ||--o{ GATEWAY_EVENTS : receives
    ORDERS ||--o{ STELLAR_TRANSACTIONS : settles
    API_CLIENTS ||--o{ WEBHOOK_ENDPOINTS : configures
    ORDERS ||--o{ WEBHOOK_EVENTS : produces
    WEBHOOK_EVENTS ||--o{ WEBHOOK_ATTEMPTS : delivers
    API_CLIENTS ||--o{ IDEMPOTENCY_RECORDS : scopes
    ORDERS ||--o{ OUTBOX_MESSAGES : emits
    TREASURY_ACCOUNTS ||--o{ TREASURY_RESERVATIONS : reserves
    ORDERS ||--o| TREASURY_RESERVATIONS : holds
```

`auth_transactions` stores one-time OIDC login transactions and is not
referenced by other tables.

## 3. Core tables

### `users`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `status` | `text` | `active`, `disabled` |
| `display_name` | `text` | Required display value |
| `email` | `text` | Nullable profile/contact value; never the identity key |
| `avatar_object_key` | `text` | Nullable private MinIO object key; never a public URL or image bytes |
| `email_verified_at` | `timestamptz` | Nullable; copied from the identity provider after verified login |
| `developer_enabled_at` | `timestamptz` | Nullable opt-in timestamp for developer-management access |
| `created_at`, `updated_at` | `timestamptz` | UTC |

One local user can act through retail sessions, opt into Developer Mode, or receive separately controlled internal operator access. A provider proves the external identity (Google subject, or the email itself for credentials login); KailoPay stores the stable provider subject separately and owns local authorization. Since ADR-002 the email is also the unique match key for credentials login and verified-email linking.

### `user_identities`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `user_id` | `uuid` | FK to `users` |
| `provider` | `text` | `auth0` for `v0.1.0` |
| `subject` | `text` | Stable provider subject (Google `sub`, or the lowercased email for the `email` credentials provider) |
| `created_at` | `timestamptz` | UTC |
| `last_login_at` | `timestamptz` | Nullable UTC timestamp |

Unique: `(provider, subject)`. This permits future account linking without changing the local user ID and prevents duplicate local users for one provider identity.

### `retail_sessions`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `user_id` | `uuid` | FK to `users` |
| `token_hash` | `bytea` | High-entropy session token hash; plaintext never persisted |
| `expires_at` | `timestamptz` | Required expiry |
| `last_used_at`, `revoked_at` | `timestamptz` | Nullable activity/revocation timestamps |
| `created_at`, `updated_at` | `timestamptz` | UTC |

Unique: `token_hash`. Retail sessions are short-lived, revocable, and scoped to the user’s own orders.

### `auth_transactions`

One-time OIDC login transactions backing `GET /auth/login` and
`GET /auth/callback`.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `state_hash` | `bytea` | Unique; HMAC of the OIDC state value |
| `nonce_hash` | `bytea` | HMAC of the OIDC nonce |
| `code_verifier_ciphertext` | `bytea` | Encrypted PKCE verifier; never stored in plaintext |
| `expires_at` | `timestamptz` | Transaction lifetime; indexed for cleanup |
| `consumed_at` | `timestamptz` | Set exactly once; single-use enforcement |
| `created_at` | `updated_at` | `timestamptz` |

Unique: `state_hash`. A consumed or expired transaction can never complete a
second login.

### `auth_credentials`

Argon2id password credentials for email accounts (ADR-002).

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `user_id` | `uuid` | Unique FK to `users` |
| `password_hash` | `text` | Encoded Argon2id string; never a plaintext password |
| `failed_login_count` | `integer` | Non-negative; reset on success and password change |
| `locked_until` | `timestamptz` | Set when the failure count reaches the lockout threshold |
| `password_changed_at` | `timestamptz` | Rotated on change and reset |
| `created_at`, `updated_at` | `timestamptz` | UTC |

Google-only accounts have no row until they set a password; `UpdatePasswordHash`
upserts one.

### `auth_challenges`

Single-use hashed tokens for email verification (24h) and password reset (1h).

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `user_id` | `uuid` | FK to `users` |
| `token_hash` | `bytea` | Unique HMAC of the raw token; the raw token is never stored |
| `purpose` | `text` | `email_verification` or `password_reset` check |
| `expires_at` | `timestamptz` | Hard expiry |
| `consumed_at` | `timestamptz` | Set exactly once; single-use enforcement |
| `created_at` | `timestamptz` | UTC |

Consumption is guarded like login transactions: lock, conditional update, and
row-count verification.

### `api_clients`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `owner_user_id` | `uuid` | Non-null FK to the directly owning `users` record |
| `name` | `text` | Display name |
| `environment` | `text` | Check: `test` for `v0.1.0` |
| `status` | `text` | `active`, `revoked` |
| `created_at`, `updated_at` | `timestamptz` | UTC |

### `api_keys`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Internal key record ID |
| `client_id` | `uuid` | FK to `api_clients` |
| `public_id` | `text` | Unique lookup component embedded in key |
| `prefix` | `text` | Safe display prefix only |
| `secret_hash` | `bytea` or `text` | Hash/HMAC result; never plaintext |
| `created_at`, `last_used_at`, `revoked_at` | `timestamptz` | Nullable activity/revocation timestamps |

Recommended key format: `pk_test_<public_id>_<random_secret>`. Parse `public_id` for indexed lookup, then constant-time verify the secret against the stored hash.

### `orders`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Public order ID |
| `client_id` | `uuid` | Nullable FK for API-created orders |
| `created_by_user_id` | `uuid` | Nullable actor/correlation user FK |
| `retail_session_id` | `uuid` | Nullable FK for retail-created orders |
| `direction` | `text` | Week 1 CHECK pins `onramp`; `offramp` arrives with the off-ramp migration |
| `status` | `text` | State-machine value |
| `version` | `integer` | Optimistic concurrency, positive check |
| `currency` | `text` | `IDR` |
| `fiat_amount_minor` | `bigint` | Positive check |
| `asset_code`, `asset_issuer`, `network` | `text` | `XLM`, pinned testnet network check |
| `asset_amount` | `numeric(30,18)` | Canonical decimal rendering |
| `asset_amount_stroops` | `bigint` | Exact stroops; positive check; authoritative for settlement |
| `quote_provider`, `quote_source_at` | `text`, `timestamptz` | Price source and observation time |
| `quote_rate`, `quote_adjusted_rate` | `numeric(30,18)` | Raw and spread-adjusted IDR per XLM; positive checks |
| `quote_spread_bps` | `integer` | 0–10000 check |
| `quote_expires_at` | `timestamptz` | Quote lock expiry; indexed |
| `payment_method`, `gateway_provider` | `text` | `qris`/`bri_va` and `xendit` checks after migration 000002 |
| `stellar_source`, `stellar_destination`, `stellar_memo` | `text` | Nullable by direction/stage |
| `withdrawal_destination` | encrypted/minimized structured column | Synthetic sandbox data only |
| `expires_at` | `timestamptz` | Optional lifecycle expiry |
| `failure_code`, `failure_stage`, `failure_retryable` | typed columns | Safe public/operational data |
| `created_at`, `updated_at`, `completed_at` | `timestamptz` | UTC |

Use explicit check constraints for positive amounts, supported directions, network, terminal timestamps, and exactly one ownership route:

- API route: `client_id` is non-null; `retail_session_id` is null. Developer-dashboard access resolves the owner through `api_clients.owner_user_id`.
- Retail route: `created_by_user_id` and `retail_session_id` are non-null; `client_id` is null.

The application may retain `created_by_user_id` for an API order initiated from a developer session, but public API authorization is based on the API client.

### `order_events`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Event ID |
| `order_id` | `uuid` | FK |
| `aggregate_version` | `integer` | Unique with order ID |
| `event_type` | `text` | Stable internal name |
| `previous_status`, `new_status` | `text` | Nullable where not a transition |
| `source` | `text` | API/callback/worker/reconciliation/operator |
| `correlation_id` | `text` | Trace across processes |
| `metadata` | `jsonb` | Sanitized, versioned shape |
| `created_at` | `timestamptz` | Immutable |

Unique: `(order_id, aggregate_version)`.

### `payment_checkouts`

Columns include ID, order ID, provider, provider checkout ID, method, expected currency/amount, status, checkout/QR presentation reference, expiry, sanitized metadata, and timestamps.

Unique: `(provider, provider_checkout_id)`. Index: `(order_id, created_at)`.

### `treasury_accounts`

One row per treasury hot wallet. No secret is ever stored here.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `network` | `text` | Pinned to `stellar_testnet` |
| `public_account` | `text` | Treasury distribution address; unique with network |
| `observed_balance_stroops` | `bigint` | Last reconciled spendable native balance; non-negative |
| `reserved_stroops` | `bigint` | Sum of active reservations; `reserved <= observed` check |
| `operating_buffer_stroops` | `bigint` | Configured safety buffer; non-negative |
| `last_reconciled_at` | `timestamptz` | When the observed balance was refreshed from Horizon |
| `created_at`, `updated_at` | `timestamptz` | UTC |

The row is locked and reconciled against a fresh Horizon balance inside the
same transaction that creates a reservation. Available inventory is
`observed - reserved - operating_buffer`.

### `treasury_reservations`

Exactly one reservation per order.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `treasury_id` | `uuid` | FK to `treasury_accounts` |
| `order_id` | `uuid` | Unique; the order holding this inventory |
| `amount_stroops` | `bigint` | Positive check |
| `status` | `text` | `reserved`, `consumed`, `released`; paired timestamp checks |
| `reserved_at`, `consumed_at`, `released_at`, `expires_at` | `timestamptz` | Status transitions |
| `created_at`, `updated_at` | `timestamptz` | UTC |

Index: `(status, expires_at)` backs the worker sweep that expires unpaid
orders and releases their inventory. Orders held for operator resolution,
such as an unknown checkout outcome, keep their reservation.

### `gateway_events`

Columns include ID, provider, provider event ID or deterministic fingerprint, event type, checkout/order reference, payload hash, signature/authentication result, matching result, received/processed timestamps, processing status, and safe error.

Unique: `(provider, provider_event_id)` when provided; otherwise `(provider, payload_hash, event_type)` with a documented collision strategy. Week 1 requires `provider_event_id` to be present (Xendit always supplies `payment_id`) and enforces the first uniqueness rule only; the fallback rule remains future work for providers without event IDs.

### `stellar_transactions`

Columns include ID, order ID, stable intent ID, purpose, network, asset, amount, source, destination, memo, transaction hash, status (`pending`, `submitted`, `confirmed`, `failed`, `unknown`), attempt count, ledger/created-at time, last safe error, and timestamps.

Unique constraints:

- `(order_id, purpose)` for single-effect purposes in `v0.1.0`.
- `transaction_hash` when non-null.
- `intent_id`.

### `webhook_endpoints`

Columns include ID, non-null client ID, URL, status, encrypted secret/secret reference, subscribed event types, created/updated/disabled timestamps. Developer-session management verifies ownership through `api_clients.owner_user_id`.

URLs must pass SSRF policy validation before activation.

### `webhook_events`

Columns include ID, order ID, event type, API version, canonical payload JSON, creation timestamp, and optional source order-event ID.

### `webhook_attempts`

Columns include ID, event ID, endpoint ID, attempt number, status, scheduled/started/completed timestamps, HTTP status, duration, response-body hash/truncated safe diagnostic, safe error, and next-attempt time.

Unique: `(event_id, endpoint_id, attempt_number)`.

### `idempotency_records`

Columns include client ID, operation, idempotency-key hash, request hash, response status/body or created resource ID, state (`processing`, `completed`, `failed`), expiry, and timestamps.

Unique: `(client_id, operation, idempotency_key_hash)`.

Conflicting request hashes return `409 IDEMPOTENCY_KEY_REUSED`.

### `outbox_messages`

Columns include ID, topic/type, aggregate type/ID, payload JSON, created time, available time, lease owner/until, attempts, processed time, and last safe error.

Week 1 leases with `FOR UPDATE SKIP LOCKED` and indexes unprocessed messages with a partial index on `(available_at) WHERE processed_at IS NULL`, which is equivalent to the composite `(processed_at, available_at)` plan because the predicate fixes `processed_at IS NULL`.

## 4. Transaction boundaries

One transaction must cover:

- Current aggregate update with optimistic version check.
- Immutable order event insertion.
- New outbox message insertion.
- Idempotency-record completion when handling an idempotent API command.

External network calls must occur outside long-running database transactions. Persist intent first, commit, then process asynchronously.

## 5. Index plan

Minimum indexes:

- `user_identities(provider, subject)` unique for external identity lookup.
- `user_identities(user_id)` for local-user identity management.
- `retail_sessions(token_hash)` unique for session authentication.
- `api_clients(owner_user_id, created_at desc)` for developer-management listing.
- `orders(client_id, created_at desc, id desc)` for API-client cursor listing; implemented.
- `orders(retail_session_id, created_at desc)` for retail history; deferred until the retail web flow ships.
- `orders(status, updated_at)` for reconciliation/operations; implemented.
- `orders(gateway_provider, payment_method)` if operational lookup requires it.
- `order_events(order_id, aggregate_version)` unique; implemented.
- Provider-reference indexes on payment and gateway tables; implemented.
- `stellar_transactions(order_id, purpose)` and unique non-null hash; implemented.
- `treasury_reservations(status, expires_at)` for the reservation-expiry sweep; implemented.
- `webhook_attempts(status, scheduled_at)` for delivery worker; deferred with the webhook pipeline.
- `outbox_messages(available_at) where processed_at is null` partial; implemented.
- `idempotency_records(expires_at)` for retention cleanup; deferred until the cleanup job ships.

Review client-owner and session indexes with representative data before adding broader search indexes.

Confirm indexes using real query plans after representative sandbox data exists; do not add speculative indexes blindly.

## 6. Migration strategy

- Use immutable, ordered migration files.
- Backward-compatible schema changes precede code that depends on them.
- Destructive migration is prohibited in the 30-day review environment without a tested backup and explicit approval.
- CI applies all migrations to an empty database and runs integration tests.
- Deployment runs migrations once before new API/worker processes become ready.
- Seed scripts create only synthetic demo clients/assets and never production-like credentials.

## 7. Retention and cleanup

Sandbox cleanup jobs may expire:

- Idempotency records after the documented retry window.
- Old delivery diagnostics after evidence retention requirements are satisfied.
- Synthetic demo orders only after the Ambassador review window and evidence archive are complete.

Do not cascade-delete audit/evidence unexpectedly. Production retention policy is Future scope.

## 8. Backup and restore

Before final evidence collection:

- Produce a database backup using the selected hosting mechanism.
- Record release version and migration version.
- Perform at least one restore verification in a non-public environment when feasible.
- Document RPO/RTO as sandbox targets, not production guarantees.

## 9. Database acceptance checks

- Duplicate gateway event/reference insertion is rejected or returns the prior processed result.
- Duplicate Stellar purpose for the same order cannot create a second settlement intent.
- A retail session cannot read an order owned by another session or an API client.
- An authenticated developer cannot manage an API client, key, endpoint, or order owned by another user.
- An API client cannot read an order owned by another API client.
- An order cannot satisfy both the API and retail ownership routes.
- Illegal amounts, directions, networks, and aggregate versions fail safely.
- Order update, order event, and outbox insert commit or roll back together.
- API keys cannot be reconstructed from stored values.
- Database dumps and diagnostic queries do not contain private keys or full API-key plaintext.
