# ADR-004: Persona sandbox KYC gate

- Status: accepted
- Date: 2026-09-13
- Decision owners: KailoPay project owner and backend developer
- Resolves: BE-052

## Context

KailoPay is an anchor-style sandbox application. The on/off-ramp execution is
not a production fiat service yet, but API-key creation and value-movement
requests still need a user-scoped identity-verification boundary that mirrors
the expected anchor flow. A frontend-only flag is insufficient because API
clients and future callers could bypass it.

## Decision

1. Use Persona as the identity-verification provider for the sandbox release.
2. Persist one local inquiry record per attempt and a normalized provider-event
   history. Store status, provider IDs, timestamps, and a SHA-256 body hash;
   never store raw identity documents or raw Persona webhook bodies.
3. Require local `approved` status before creating an API key, on-ramp order, or
   off-ramp order. Enforce the rule in the usecases for both API-key and retail
   session principals.
4. Create/resume inquiries on the backend. The browser receives only the
   inquiry ID, configured Persona environment ID, and an optional short-lived
   session token needed by the embedded flow.
5. Accept status changes only from a verified Persona callback. Verify the exact
   raw body with the timestamped `Persona-Signature` HMAC, reject stale headers,
   deduplicate provider event IDs, and apply events by provider event time so
   out-of-order delivery cannot downgrade an approved inquiry.

## Consequences

- Users complete Persona verification before they can create API keys or orders.
- The backend remains authoritative if the browser is modified or the embedded
  SDK callback is forged.
- Persona availability becomes an explicit dependency for starting/resuming an
  inquiry, while already persisted approval can still be checked locally.
- The sandbox gains realistic provider/callback behavior without making a
  production compliance claim.
- Production KYC/AML, sanctions screening, case management, data-retention
  policy, regulatory review, and provider contracting remain Future scope.

## Alternatives considered

### Frontend-only or synthetic KYC flag

Rejected: it does not protect API-key or API-created order paths and does not
exercise the signed provider callback boundary required by an anchor flow.

### Transak-hosted KYC flow

Deferred: Transak is the reference product flow for the eventual on/off-ramp,
but the current KailoPay slice has no active Transak order integration. Persona
provides a focused identity-verification boundary that can be integrated now
without coupling KYC state to a future ramp provider.

### Store provider payloads for later review

Rejected for the sandbox: retaining raw identity or callback payloads increases
privacy and breach impact without being needed for the approval gate. Keep
normalized facts and hashes only.
