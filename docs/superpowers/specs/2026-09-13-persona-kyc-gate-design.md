# Persona KYC Gate Design

Date: 2026-09-13

## Decision

KailoPay will use Persona as the identity-verification provider. KYC is
user-scoped and is required before a verified retail user can create a
developer API key. The same approved KYC status is checked again when an
on-ramp or off-ramp order is created, so an existing API key cannot bypass a
later KYC decline, expiry, or revocation.

For the current sandbox anchor, an API-key order inherits the KYC status of
the API key's owning user. If the API becomes a third-party platform for
multiple end customers, customer-level KYC will be a separate design rather
than being inferred from the developer who owns the key.

Only Persona's `approved` decision unlocks API keys and value-movement
requests. `created`, `pending`, `completed` without approval, local
`pending_review`, `declined`, `failed`, and `expired` remain non-eligible
states. Persona's `marked-for-review` provider status maps to local
`pending_review`.

## Scope

This slice will add:

- a PostgreSQL-backed KYC inquiry and provider-event history without storing
  identity documents or raw Persona payloads;
- a consumer-owned KYC use case and Persona provider port;
- a Persona HTTP adapter for inquiry creation/resume and signed webhook
  verification;
- authenticated KYC status and inquiry-session endpoints;
- a public Persona webhook endpoint with timestamp tolerance, constant-time
  HMAC verification, event deduplication, and out-of-order event handling;
- a KYC check in API-key creation and in both order-creation use cases;
- a frontend embedded Persona verification page that uses the backend-created
  inquiry and treats the webhook-backed status endpoint as authoritative;
- OpenAPI, environment examples, backend/frontend documentation, and
  regression tests.

## Public API

The session-authenticated KYC surface will be:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/kyc` | Return the current user's safe KYC status and timestamps. |
| `POST` | `/v1/kyc/inquiry` | Create or resume the current user's Persona inquiry. |
| `POST` | `/callbacks/kyc/persona` | Receive and verify Persona inquiry events. |

The inquiry response contains only the Persona inquiry ID, configured
environment ID, optional session token, provider status, and local eligibility
status. The server never returns Persona API credentials, webhook secrets,
document fields, or verification results containing PII.

`POST /v1/api-keys`, `POST /v1/onramps`, and `POST /v1/offramps` return a stable
`KYC_REQUIRED` error when the owning user is not approved. The check belongs in
the use cases, not only in HTTP middleware, so API, browser, and future
internal callers share the same policy.

## Data model

Add a `kyc_inquiries` table representing each provider inquiry attempt:

- internal inquiry ID and user ID;
- provider name and unique Persona inquiry ID;
- provider status and local status;
- provider event timestamp, last event ID, created/completed/approved/expiry
  timestamps;
- created and updated timestamps.

Add a `kyc_provider_events` table with a unique `(provider, provider_event_id)`
constraint. Store event type, inquiry ID, event timestamp, payload hash, and
receipt timestamp only. Do not persist the raw webhook body or extracted
identity fields.

At most one inquiry in `creating`, `created`, `pending`, `completed`, or
`pending_review` may be active for a user. An approved inquiry remains the
user's current eligibility result; declined, failed, and expired inquiries are
terminal attempts that may be replaced by a later inquiry. Concurrent inquiry
creation is serialized by a user row lock plus a database uniqueness rule.
Retrying the inquiry endpoint returns the existing inquiry or a resumable
session rather than creating duplicates.

## Persona flow

1. The authenticated browser calls `POST /v1/kyc/inquiry`.
2. The backend finds the user's active inquiry or creates one using the
   configured Persona template and the opaque internal user ID as
   `reference-id`.
3. If Persona requires resumption, the backend creates a session token and
   returns it to the browser. The token is short-lived and is never logged or
   stored in PostgreSQL.
4. The frontend loads Persona's embedded flow with the inquiry ID and session
   token, if present.
5. Persona sends signed events to the callback endpoint. The callback updates
   local state only after signature, timestamp, event identity, inquiry
   ownership, and event ordering checks pass.
6. The frontend may refresh status after Persona's client callback, but only
   the persisted webhook-backed `approved` state unlocks the account.

Provider `completed` is not automatically treated as approved because Persona
workflows or manual review may still make the final decision. The configured
Persona template must therefore produce an `approved` event for a user to
become eligible.

## Security and failure handling

- Persona API calls use an HTTPS base URL, inherited request context, and a
  bounded HTTP timeout.
- The webhook reads the exact raw body, parses the `Persona-Signature` header,
  rejects timestamps outside a configured tolerance, computes HMAC-SHA256 over
  `<timestamp>.<raw body>`, and compares signatures in constant time.
- Duplicate provider events are successful no-ops after the first committed
  event. Events are applied by provider creation time, not arrival order.
- The callback returns quickly after durable status/event persistence; it never
  calls Persona synchronously to fetch PII.
- Provider failures and unknown outcomes leave KYC non-eligible and return a
  retryable service error to the browser. They never unlock an order or API
  key.
- A declined, failed, expired, or newer adverse decision removes eligibility.
  A later new inquiry may replace a terminal attempt while preserving history.
- Logs contain user/inquiry IDs, event type, status, and request correlation
  metadata, but never API keys, signatures, session tokens, raw bodies, or
  identity fields.

## Frontend experience

Add a session-protected `/verify-identity` route. Buy, sell, and developer API
key creation should direct non-approved users there with a safe return path.
The page explains that Persona collects the identity data, shows sandbox and
testnet context, embeds the Persona flow, and renders pending/review/declined
states without exposing provider diagnostics. The application does not mark a
user verified from the browser's `onComplete` callback.

## Verification

Tests will cover:

- status eligibility and all provider-to-local status mappings;
- API-key and order gates for pending, approved, declined, expired, and
  provider-error states;
- Persona request/response parsing, timeout and non-2xx handling;
- valid, stale, malformed, wrong-secret, duplicate, and out-of-order webhook
  events;
- concurrent inquiry creation producing one active inquiry;
- PostgreSQL constraints and event idempotency;
- frontend response parsing and redirects/return-path validation.

The baseline remains `gofmt`, `go vet ./...`, `go test ./...`,
`go test -race ./...`, and the frontend lint/build/test commands. A disposable
PostgreSQL instance is required for the repository integration tests.

## Documentation changes

Update the backend API design, implementation phases, backlog, frontend guide,
OpenAPI contract, environment example, and a Persona integration runbook in
the same change. The earlier synthetic KYC stub must be removed from public
claims. This change does not claim production regulatory compliance, KYB,
sanctions/PEP screening, AML monitoring, manual case management, or retention
policy approval.

## Out of scope

This slice does not repair the existing Week 2 settlement defects, implement
real fiat payout, store identity documents, or add an operator case-management
console. On/off-ramp execution remains sandbox/testnet functionality, but its
creation paths will be protected by the approved-KYC gate.
