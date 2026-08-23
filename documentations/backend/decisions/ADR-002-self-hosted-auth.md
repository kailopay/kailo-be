# ADR-002: Self-Hosted Email and Google Authentication

- Status: accepted
- Date: 2026-08-23
- Decision owners: KailoPay project owner and backend developer
- Supersedes: BE-004 ("Auth0 email login with KailoPay-owned PostgreSQL retail
  sessions") in `BACKEND-BACKLOG.md`

## Context

The Week 1 authentication slice used Auth0 Universal Login behind a BFF flow:
the backend redirected to Auth0, Auth0 proved the identity, and KailoPay minted
its own opaque PostgreSQL session. The session layer was already fully
self-owned.

Operating Auth0 in the sandbox exposed friction that will not shrink over the
30-day window:

- First logins fail until the user clicks Auth0's verification email, and
  verification email delivery from a dev tenant is unreliable or unconfigured,
  which hard-blocks every new reviewer account.
- Every flow customization (custom signup fields, tailored first-login
  handling, custom messaging) requires tenant-side Actions or paid features
  outside this repository.
- The tenant adds an external dependency and secret surface for zero product
  value in a sandbox release that never handles real credentials.

The product needs two login methods for `v0.1.0`: email and Google. Google is
already an OIDC provider we can consume directly with `go-oidc`.

## Decision

1. Replace the Auth0 identity layer with self-hosted authentication:
   - Email + password accounts with Argon2id hashed credentials
     (m=19 MiB, t=2, p=1) stored in PostgreSQL.
   - Google sign-in through direct OIDC (discovery, PKCE S256, nonce
     verification) with `coreos/go-oidc/v3` and `golang.org/x/oauth2`.
2. Keep the existing opaque session model unchanged: PostgreSQL
   `retail_sessions`, HMAC-hashed cookie tokens, idle and absolute lifetimes,
   `kailopay_session` cookie semantics, and all middleware.
3. Email and Google accounts auto-link: a Google identity whose email matches
   an existing account with `email_verified_at` set is attached to that
   account. Unverified emails never link.
4. Email verification and password reset use single-use, hashed, expiring
   challenge tokens (24h verification, 1h reset). In this iteration links are
   delivered by logging them to the server console (an `EmailSender` port with
   a console implementation); SMTP or a provider API can be wired later
   behind the same port without schema changes.
5. Brute-force mitigations for password login: Argon2id verification cost, a
   dummy-hash verification on unknown emails to equalize timing, and a
   10-failure / 15-minute lockout stored per credential.
6. JSON API endpoints with frontend-owned forms for email flows
   (`POST /auth/register`, `POST /auth/login`, token verification and password
   reset); Google remains a redirect flow. Google sign-in is optional: when
   `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` are empty the endpoints return
   503 so local development never requires Google credentials.

## Consequences

- KailoPay now owns credential storage security: hashing parameters, lockout
  policy, and token lifetimes are our responsibility and live in this
  repository under test.
- `users.email` gains a partial unique index; the `email` login identity is
  keyed by lowercased email.
- Password reset revokes all of the user's sessions; email verification is
  required before password login succeeds.
- Real email delivery and per-IP/per-account rate limiting (BE-072) remain
  deferred; Argon2id plus lockout are the interim mitigations.
- The Auth0 adapter, `AUTH0_*` configuration, the hosted password-reset
  request, and the `POST /internal/auth/password-reset-completed` webhook are
  removed. Existing `provider='auth0'` identity rows in development databases
  stop matching any login and can be ignored or cleaned manually.
- `golang.org/x/crypto` is promoted from an indirect to a direct dependency
  for Argon2id. No other dependency changes.

## Alternatives considered

### Keep Auth0 and configure the email provider

Rejected for this release. It keeps the external dependency and still requires
tenant-side configuration this repository cannot reproduce or test, and the
sandbox review flow stays blocked on email delivery from a dev tenant.

### Another hosted IdP (Clerk, Supabase Auth, etc.)

Rejected. Same external-dependency tradeoff as Auth0 with a new vendor.

### Google-only authentication

Rejected. The PRD requires an email login path for Indonesian reviewers; a
password path also keeps the sandbox demo usable without any Google account.
