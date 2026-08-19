# Auth0 Backend Authentication Design

## Status

Awaiting written review. The implementation direction was approved in chat:
Auth0 Universal Login with email OTP, a backend-for-frontend callback, and a
KailoPay-owned server-side session.

## Goal

Add human-user authentication to the Go backend without exposing Auth0 access
or refresh tokens to the browser. Auth0 proves the external identity; KailoPay
owns the local user, authorization, session expiry, revocation, and Developer
Mode boundaries.

## Non-goals

- Password storage, password reset, or custom login UI in the Go backend.
- Auth0 Management API integration.
- Browser-held access or refresh tokens.
- API-key authentication; that remains a separate developer-integration flow.
- Production KYC, identity-document collection, or account recovery workflows.

## Selected architecture

Use the backend-for-frontend pattern:

```text
Browser -> KailoPay /auth/login -> Auth0 Universal Login
Browser <- Auth0 authorization callback with one-time code
KailoPay -> Auth0 token endpoint (server-to-server)
KailoPay -> local user identity and session in PostgreSQL
Browser <- secure HTTP-only KailoPay session cookie
```

The browser never receives or stores Auth0 tokens. The authorization code is
single-use and exchanged only by the backend. The backend validates the ID
token issuer, audience, signature, expiry, nonce, and subject before creating
or loading the local user.

## HTTP contract

### `GET /auth/login`

Creates a short-lived login transaction containing a cryptographically random
state, nonce, and PKCE verifier. The state and nonce are stored as hashes; the
verifier is encrypted at rest. The transaction is valid for 10 minutes and
may be consumed only once. The handler redirects to the configured Auth0
authorization endpoint using the authorization-code flow and email OTP
connection.

### `GET /auth/callback`

Requires `code` and `state`. It atomically consumes the matching transaction,
exchanges the code with Auth0, validates the returned ID token, and upserts the
`auth0`/`sub` link in `user_identities`. It updates the local profile email and
verification timestamp when trusted claims are present, then creates a local
session and sets the session cookie.

The handler redirects only to the configured frontend success URL. It never
accepts an arbitrary redirect URL from the request. Invalid, expired, replayed,
or mismatched transactions return a sanitized `400` response and log only a
safe reason and request ID.

### `POST /auth/logout`

Requires the local session cookie. It revokes the current session, clears the
cookie, and returns `204`. Repeating logout is idempotent.

### `GET /auth/me`

Requires an active local session and returns safe local-user profile data,
Developer Mode status, and environment metadata. It never returns Auth0 token
claims or provider tokens.

## Local session policy

- Generate at least 256 bits using `crypto/rand`.
- Store only an HMAC-SHA-256 token hash in `retail_sessions`.
- Set a secure, HTTP-only, SameSite cookie; use the `__Host-` prefix when HTTPS
  deployment requirements permit it.
- Absolute lifetime: 8 hours.
- Idle lifetime: 30 minutes, enforced from `last_used_at`.
- Revoke on logout, disabled user, or explicit session revocation.
- Do not refresh Auth0 tokens to extend the local session.
- Update `last_used_at` at a bounded interval rather than on every request.

The session middleware loads the user through the session record and places a
small authenticated-user value in the request context. It rejects expired,
revoked, or disabled sessions before protected handlers execute.

## Persistence changes

The existing `users`, `user_identities`, and `retail_sessions` models remain
the source of local identity and session state. Add `auth_transactions`:

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `state_hash` | `bytea` | HMAC lookup value; unique |
| `nonce_hash` | `bytea` | HMAC comparison value |
| `code_verifier_ciphertext` | `bytea` | AES-GCM encrypted PKCE verifier |
| `expires_at` | `timestamptz` | 10-minute expiry |
| `consumed_at` | `timestamptz` | Nullable one-time-use marker |
| `created_at` | `timestamptz` | UTC |

The transaction is consumed under a row lock or equivalent conditional update
so concurrent callback delivery cannot exchange the same authorization code
twice. Expired transactions may be cleaned up asynchronously.

## Configuration

Add validated configuration for:

- Auth0 issuer URL.
- Auth0 client ID and client secret.
- Auth0 callback URL.
- Auth0 email connection name.
- Frontend success URL.
- Auth transaction encryption key.
- Session HMAC key.
- Session absolute and idle durations.
- Session cookie name and secure-mode policy.

Secrets are injected through the environment or managed secret store and are
never logged. Local development uses ignored `.env` values.

## Package boundaries

- `internal/adapter/auth0`: OIDC discovery, authorization URL construction,
  code exchange, and ID-token verification. Auth0 SDK/provider types do not
  escape this package.
- `internal/service/auth`: login transaction, identity upsert, session issue,
  logout, and authenticated-user workflows. It owns narrow persistence ports.
- `internal/repository`: GORM models and concrete persistence operations for
  users, identities, transactions, and sessions.
- `internal/handler/http`: route handlers and protocol/error mapping.
- `internal/handler/middleware`: local session authentication middleware.
- `internal/platform`: validated Auth0, cookie, crypto, clock, and database
  configuration.

## Error and security behavior

- Fail closed when Auth0 configuration is missing or malformed.
- Validate issuer and audience exactly; do not accept an arbitrary issuer from
  request data.
- Use request context and bounded HTTP client timeouts for Auth0 calls.
- Never log authorization codes, state, nonce, PKCE verifier, cookies, ID
  tokens, access tokens, refresh tokens, or full email payloads.
- Do not auto-link a new Auth0 subject to a local user by email alone.
- Map invalid callback state, invalid token, expired transaction, and disabled
  user to stable sanitized client errors.
- Preserve request/correlation IDs in logs and responses.

## Tests

Unit and handler tests must cover:

- Auth0 authorization URL contains the expected state, nonce, PKCE, issuer, and
  fixed redirect configuration.
- Callback rejects missing, mismatched, expired, and replayed state.
- Callback rejects invalid issuer, audience, signature, nonce, expiry, or
  missing subject.
- A valid callback creates one local user identity and one session; replay does
  not create another.
- Email/profile updates use trusted claims and do not merge users by email.
- Session middleware rejects missing, expired, idle, revoked, and disabled-user
  sessions and accepts a valid session.
- Logout revokes the session and clears the cookie; repeated logout is safe.
- Cookies have the required security attributes and no token is returned in a
  response body or log.

Auth0 network tests use a local test server or recorded contract fixture. A
separate integration test may run against a real Auth0 tenant and is not part
of the default unit-test command.

## Acceptance criteria

1. A user can complete Auth0 email OTP login and receive a local session cookie.
2. Auth0 tokens never reach browser responses or normal KailoPay API requests.
3. A local session survives Auth0 access-token expiry until its own expiry or
   revocation.
4. Callback replay and invalid token/state inputs cannot create a session.
5. Logout, session expiry, and disabled-user checks take effect immediately.
6. Configuration, API documentation, security documentation, models, tests,
   and local setup instructions describe the same behavior.
