# Auth0 Backend Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Auth0 email-OTP login in the Go backend with a KailoPay-owned PostgreSQL session and a validated OpenAPI contract.

**Architecture:** Use a backend-for-frontend flow. Auth0 handles human identity verification through Authorization Code + PKCE; the Go callback validates the result, upserts the Auth0 subject into `user_identities`, and issues an opaque local session cookie backed by `retail_sessions`. Auth0 tokens remain server-side and are discarded after the callback because KailoPay does not need to call a user-authorized external API.

**Tech Stack:** Go 1.25, Gin, GORM PostgreSQL, Auth0 OIDC, `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3/oidc`, `github.com/getkin/kin-openapi`, `crypto/rand`, AES-GCM, HMAC-SHA-256, `httptest`.

**Spec:** `docs/superpowers/specs/2026-08-19-auth0-backend-design.md`

## Global Constraints

- Auth0 proves identity; KailoPay owns local users, authorization, sessions, expiry, and revocation.
- The browser never receives Auth0 access, refresh, or ID tokens.
- Generate session, state, nonce, and PKCE values with `crypto/rand`.
- Store only HMAC-SHA-256 session/state/nonce lookup values; encrypt PKCE verifiers with AES-GCM.
- Use a 10-minute one-time auth transaction, an 8-hour absolute session lifetime, and a 30-minute idle session timeout.
- Use `context.Context` from the HTTP request through Auth0 and PostgreSQL calls; every external call has a bounded timeout.
- Keep interfaces in `internal/service/auth`; keep GORM rows and provider types inside their adapters.
- Use stable sanitized HTTP errors and never log credentials, codes, state, nonce, PKCE verifiers, cookies, or tokens.
- Use synthetic identities only; no production KYC or identity documents.
- Keep the API contract in `openapi/openapi.yaml` and validate it in automated tests.
- The repository has no deployed production schema; register new models for local/test `AutoMigrate` and defer reviewed shared-environment migrations to the migration workflow.

## File Map

Create:

- `internal/adapter/auth0/client.go` — Auth0 discovery, authorization URL construction, code exchange, and ID-token verification.
- `internal/adapter/auth0/client_test.go` — Auth0 adapter unit tests with deterministic provider fixtures.
- `internal/service/auth/types.go` — Auth service commands, results, and authenticated-user context types.
- `internal/service/auth/ports.go` — Consumer-owned interfaces for auth transactions, identities, and sessions.
- `internal/service/auth/service.go` — Login, callback, logout, local-user lookup, and cryptographic token orchestration.
- `internal/service/auth/service_test.go` — Table-driven service tests using handwritten fakes.
- `internal/repository/auth_repository.go` — GORM implementations of the auth ports after the ports are defined.
- `internal/repository/auth_repository_test.go` — Repository contract tests that do not require Auth0; integration cases use the existing database test strategy.
- `internal/handler/middleware/auth.go` — Local-session middleware and request-context accessors.
- `internal/handler/middleware/auth_test.go` — Middleware tests for cookie/session/authorization behavior.
- `internal/handler/http/auth_handler.go` — Auth0 login/callback/logout/me HTTP handlers.
- `internal/handler/http/auth_handler_test.go` — `httptest` coverage for auth endpoint contracts.
- `openapi/openapi.yaml` — OpenAPI 3.0.3 authentication contract.
- `openapi/openapi_test.go` — OpenAPI parse/validation and contract assertions.

Modify:

- `go.mod`, `go.sum` — add OIDC/OAuth and OpenAPI validation dependencies.
- `.env.example` — document Auth0 and session configuration without secrets.
- `internal/platform/config.go`, `internal/platform/config_test.go` — add validated auth configuration.
- `internal/repository/models.go`, `internal/repository/models_test.go` — add `AuthTransaction` and register it for AutoMigrate.
- `internal/handler/http/router.go` — register auth routes and local-session middleware composition.
- `cmd/api/main.go` — construct Auth0 adapter, repository, service, handler, and middleware.
- `documentations/backend/API-DESIGN.md` — document the concrete endpoint contract and local cookie behavior.
- `documentations/backend/SECURITY.md` — document Auth0 callback validation, token handling, and transaction/session controls.
- `documentations/backend/README.md` — document required Auth0 configuration and local login setup.

---

### Task 1: Add Auth0 and session configuration

**Files:**
- Modify: `internal/platform/config.go`
- Test: `internal/platform/config_test.go`
- Modify: `.env.example`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces `platform.AuthConfig`, consumed by the Auth0 adapter, auth service, and API composition root.

```go
type AuthConfig struct {
	IssuerURL                   string
	ClientID                    string
	ClientSecret                string
	RedirectURL                 string
	EmailConnection             string
	SuccessRedirectURL          string
	TransactionEncryptionKey   string
	SessionHMACKey              string
	SessionAbsoluteLifetime    time.Duration
	SessionIdleLifetime        time.Duration
	TransactionLifetime        time.Duration
	CookieName                  string
	CookieSecure                bool
}
```

- [ ] Write named config tests for valid Auth0 settings, missing issuer/client credentials, invalid URLs, invalid base64 key lengths, non-positive durations, and insecure production cookie settings.
- [ ] Run `go test ./internal/platform -run Auth -count=1` and confirm the new tests fail because `AuthConfig` and bindings are absent.
- [ ] Add `Auth AuthConfig` to `platform.Config`, parse `AUTH0_ISSUER_URL`, `AUTH0_CLIENT_ID`, `AUTH0_CLIENT_SECRET`, `AUTH0_REDIRECT_URL`, `AUTH0_EMAIL_CONNECTION`, `AUTH_SUCCESS_REDIRECT_URL`, `AUTH_TRANSACTION_ENCRYPTION_KEY`, `AUTH_SESSION_HMAC_KEY`, `AUTH_SESSION_ABSOLUTE_LIFETIME`, `AUTH_SESSION_IDLE_LIFETIME`, `AUTH_TRANSACTION_LIFETIME`, `AUTH_COOKIE_NAME`, and `AUTH_COOKIE_SECURE`.
- [ ] Validate issuer, callback, and success URLs as absolute HTTPS URLs outside `local`; validate base64-decoded encryption/HMAC keys as exactly 32 bytes; validate `10m`, `8h`, and `30m` defaults; require `AUTH_COOKIE_SECURE=true` outside local.
- [ ] Add ignored `.env` examples with non-secret sample values and no reusable credentials.
- [ ] Run `go test ./internal/platform -run Auth -count=1` and `go test ./...`; confirm all config tests pass.
- [ ] Commit `feat(auth): add Auth0 and session configuration`.

### Task 2: Add auth transaction persistence

**Files:**
- Modify: `internal/repository/models.go`
- Test: `internal/repository/models_test.go`

**Interfaces:**
- `repository.AuthTransaction` maps to `auth_transactions` with `StateHash`, `NonceHash`, encrypted `CodeVerifierCiphertext`, `ExpiresAt`, `ConsumedAt`, and timestamps.

```go
type AuthTransaction struct {
	ID                     string     `gorm:"type:uuid;primaryKey"`
	StateHash              []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	NonceHash              []byte     `gorm:"type:bytea;not null"`
	CodeVerifierCiphertext []byte     `gorm:"type:bytea;not null"`
	ExpiresAt              time.Time  `gorm:"not null;index"`
	ConsumedAt             *time.Time `gorm:"index"`
	CreatedAt              time.Time  `gorm:"not null"`
}
```

- [ ] Extend model registration tests to require `auth_transactions` and continue rejecting organization tables.
- [ ] Run `go test ./internal/repository -run 'Auth|Migration' -count=1` and confirm the new registration test fails.
- [ ] Add the model and register it in `MigrationModels()`.
- [ ] Define repository methods for create, consume-once, and cleanup using `db.WithContext(ctx)`; consume must use a conditional update or row lock that rejects expired/consumed rows.
- [ ] Add repository tests for unique state lookup, expired transactions, and one-time consumption using fakes for the service contract; add a tagged PostgreSQL test for the actual locking/constraint behavior if the repository test harness is available.
- [ ] Run `go test ./internal/repository -run 'Auth|Migration' -count=1` and `go test ./...`.
- [ ] Commit `feat(auth): persist one-time login transactions`.

### Task 3: Define auth service ports and Auth0 adapter

**Files:**
- Create: `internal/service/auth/types.go`
- Create: `internal/service/auth/ports.go`
- Create: `internal/adapter/auth0/client.go`
- Test: `internal/adapter/auth0/client_test.go`

**Interfaces:**

```go
type Identity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
}

type LoginTransaction struct {
	ID                     string
	StateHash              []byte
	NonceHash              []byte
	CodeVerifierCiphertext []byte
	ExpiresAt              time.Time
	ConsumedAt             *time.Time
}

type SessionRecord struct {
	TokenHash  []byte
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

type UserProfile struct {
	ID                 string
	DisplayName        string
	Email              string
	EmailVerified      bool
	DeveloperEnabled   bool
}

type AuthenticatedUser struct {
	User         UserProfile
	SessionID    string
	LastUsedAt   time.Time
}

type SessionResult struct {
	RawToken string
	User     UserProfile
	ExpiresAt time.Time
}

type Clock interface {
	Now() time.Time
}

type Config struct {
	TransactionEncryptionKey []byte
	SessionHMACKey           []byte
	SessionAbsoluteLifetime time.Duration
	SessionIdleLifetime     time.Duration
	TransactionLifetime     time.Duration
}

type Provider interface {
	AuthorizationURL(ctx context.Context, state, nonce, codeChallenge string) (string, error)
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (Identity, error)
}

type TransactionStore interface {
	Create(ctx context.Context, tx LoginTransaction) error
	Consume(ctx context.Context, stateHash []byte, now time.Time) (LoginTransaction, error)
}
```

- [ ] Write adapter tests for authorization URL parameters, fixed issuer/callback configuration, email connection, and bounded HTTP client behavior; use a local test server rather than Auth0 network calls.
- [ ] Run `go test ./internal/adapter/auth0 -count=1` and confirm it fails because the adapter does not exist.
- [ ] Construct the adapter from `platform.AuthConfig` and an injected `*http.Client` with a finite timeout.
- [ ] Use OIDC discovery to configure `oauth2.Config`; use Authorization Code + PKCE and request only `openid profile email`.
- [ ] Add `golang.org/x/oauth2` and `github.com/coreos/go-oidc/v3/oidc` with the repository’s Go 1.25 dependency policy; defer `go mod tidy` until all imports are final.
- [ ] Verify issuer, audience, expiry, nonce, and subject through `go-oidc`; map provider failures to stable internal errors without returning token contents.
- [ ] Keep raw tokens and authorization codes inside the adapter call; never include them in `Identity`, logs, or errors.
- [ ] Run the adapter tests and `go vet ./internal/adapter/auth0`.
- [ ] Commit `feat(auth): add Auth0 OIDC adapter`.

### Task 4: Implement the auth service and local session issuance

**Files:**
- Create: `internal/service/auth/service.go`
- Test: `internal/service/auth/service_test.go`
- Create: `internal/repository/auth_repository.go`
- Test: `internal/repository/auth_repository_test.go`

**Interfaces:**

```go
type UserSessionStore interface {
	UpsertIdentityAndCreateSession(ctx context.Context, identity Identity, session SessionRecord) (UserProfile, error)
	RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error
	FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (AuthenticatedUser, error)
}

type Service struct {
	provider     Provider
	transactions TransactionStore
	users        UserSessionStore
	clock        Clock
	config       Config
}

func (s *Service) BeginLogin(ctx context.Context) (redirectURL string, err error)
func (s *Service) CompleteLogin(ctx context.Context, code, state string) (SessionResult, error)
func (s *Service) Logout(ctx context.Context, rawToken string) error
func (s *Service) Authenticate(ctx context.Context, rawToken string) (AuthenticatedUser, error)
```

- [ ] Write table-driven service tests for valid login, random-value uniqueness, state mismatch, expired transaction, callback replay, invalid provider identity, disabled user, logout idempotency, session expiry, idle timeout, and session token hashing.
- [ ] Run `go test ./internal/service/auth -count=1` and confirm the tests fail before implementation.
- [ ] Implement crypto helpers with `crypto/rand`, HMAC-SHA-256, AES-GCM, and constant-time comparisons; keep raw values out of persistence and logs.
- [ ] Implement `BeginLogin` to create state, nonce, PKCE verifier/challenge, encrypt the verifier, persist the transaction, and return the provider URL.
- [ ] Implement `CompleteLogin` to consume state once, decrypt the verifier, exchange/verify the code, upsert the `auth0` identity, and issue a random local session with absolute expiry.
- [ ] Implement `Logout` and `Authenticate`, enforcing revoked/expired/idle sessions and disabled users.
- [ ] Use a bounded `last_used_at` update interval so authentication does not write on every request.
- [ ] Implement the GORM repository methods behind `TransactionStore` and `UserSessionStore` using `db.WithContext(ctx)` and explicit transactions for identity/session creation; do not expose `*gorm.DB` to the service.
- [ ] Add repository contract tests for transaction consumption, user-identity upsert, session creation, active-session lookup, and revocation; add a tagged PostgreSQL test for row-lock behavior.
- [ ] Add `github.com/google/uuid` only if the existing ID-generation boundary does not provide UUIDs, then run `go mod tidy` after the final imports are present and review the dependency diff.
- [ ] Run the service and repository tests, `go test ./internal/service/auth -race`, and `go test ./...`.
- [ ] Commit `feat(auth): implement Auth0 login and local sessions`.

### Task 5: Implement local-session middleware

**Files:**
- Create: `internal/handler/middleware/auth.go`
- Test: `internal/handler/middleware/auth_test.go`

**Interfaces:**

```go
type SessionAuthenticator interface {
	Authenticate(ctx context.Context, rawToken string) (auth.AuthenticatedUser, error)
}

func RequireSession(authenticator SessionAuthenticator) gin.HandlerFunc
func AuthenticatedUser(ctx context.Context) (auth.AuthenticatedUser, bool)
```

- [ ] Write `httptest` cases for missing cookie, malformed cookie, invalid session, expired session, revoked session, disabled user, and valid session; assert no user context is present on failures.
- [ ] Run `go test ./internal/handler/middleware -run Session -count=1` and confirm the tests fail.
- [ ] Read the configured cookie, call the auth service with `r.Context()`, store only the authenticated local-user context value, and map failures to `401`.
- [ ] Apply cookie security attributes consistently: `HttpOnly`, configured `Secure`, `SameSite=Lax`, `Path=/`, and no untrusted `Domain`.
- [ ] Add a separate helper for optional session extraction used by logout and health-safe routes.
- [ ] Run middleware tests and `go test ./internal/handler/middleware -race`.
- [ ] Commit `feat(auth): add local session middleware`.

### Task 6: Add HTTP handlers, routes, and composition

**Files:**
- Create: `internal/handler/http/auth_handler.go`
- Test: `internal/handler/http/auth_handler_test.go`
- Modify: `internal/handler/http/router.go`
- Modify: `cmd/api/main.go`

**Interfaces:**

```go
type AuthHandler struct {
	service *auth.Service
	config platform.AuthConfig
}

func (h *AuthHandler) Login(c *gin.Context)
func (h *AuthHandler) Callback(c *gin.Context)
func (h *AuthHandler) Logout(c *gin.Context)
func (h *AuthHandler) Me(c *gin.Context)
```

- [ ] Write handler tests for `302` login redirect, callback success redirect and cookie, missing callback parameters, sanitized callback errors, `204` logout, and authenticated/unauthenticated `/auth/me`.
- [ ] Run the handler tests and confirm they fail before route/handler implementation.
- [ ] Implement handlers with Gin protocol mapping; never place Auth0 tokens or raw identity claims in response bodies.
- [ ] Register public `/auth/login` and `/auth/callback` routes, local-session `/auth/logout`, and protected `/auth/me`.
- [ ] Construct the Auth0 adapter, repository, auth service, middleware, and handler in `cmd/api/main.go` using manual dependency injection.
- [ ] Keep Auth0 discovery and database dependencies out of health-handler logic; use request contexts and startup validation.
- [ ] Run handler tests, `go test ./internal/handler/http -race`, and `go test ./...`.
- [ ] Commit `feat(auth): expose Auth0 authentication routes`.

### Task 7: Publish and validate the OpenAPI contract

**Files:**
- Create: `openapi/openapi.yaml`
- Create: `openapi/openapi_test.go`
- Modify: `documentations/backend/API-DESIGN.md`
- Modify: `documentations/backend/README.md`
- Modify: `documentations/backend/SECURITY.md`

**Interfaces:**
- OpenAPI paths must match the handlers from Task 6 exactly: `/auth/login`, `/auth/callback`, `/auth/logout`, and `/auth/me`.
- Public schemas are `UserProfile`, `EnvironmentMetadata`, and `ErrorResponse`; Auth0 tokens, authorization codes, state, nonce, and PKCE values are never schemas.

```yaml
openapi: 3.0.3
paths:
  /auth/me:
    get:
      security:
        - kailoSession: []
```

- [ ] Write the failing OpenAPI test that loads `openapi/openapi.yaml`, validates it with `kin-openapi`, asserts all four paths, and asserts `/auth/me` uses the `kailoSession` cookie security scheme.
- [ ] Run `go test ./openapi -count=1` and confirm it fails because the document is absent.
- [ ] Add the complete OpenAPI 3.0.3 document with redirect/error responses, cookie security, examples, and sandbox labels.
- [ ] Add assertions that no public schema or operation parameter contains `access_token`, `refresh_token`, `id_token`, `authorization_code`, `state`, `nonce`, or `code_verifier`.
- [ ] Update backend API/security documentation with links to the OpenAPI file and the email-OTP/local-session distinction.
- [ ] Run `go test ./openapi -count=1` and `go test ./...`.
- [ ] Commit `docs(auth): publish Auth0 OpenAPI contract`.

### Task 8: Complete verification and integration documentation

**Files:**
- Modify: `.env.example`
- Modify: `documentations/backend/IMPLEMENTATION-PHASES.md`
- Modify: `documentations/backend/BACKEND-BACKLOG.md`
- Test: all affected `*_test.go` files

- [ ] Add local setup instructions for creating an Auth0 Regular Web Application, enabling the email passwordless connection, setting the callback URL, and generating base64 32-byte local keys without committing values.
- [ ] Document exact callback/logout URLs and the difference between Auth0 login and `pk_test_` API keys.
- [ ] Mark the relevant identity/auth backlog item complete only after default tests pass and a real Auth0 tenant test is recorded separately.
- [ ] Run `go fmt ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `go test -race ./...`.
- [ ] Run `go mod tidy` only after all imports are final and review the dependency diff.
- [ ] Review logs and test fixtures for authorization codes, cookies, tokens, email addresses, and secrets.
- [ ] Commit `test(auth): verify Auth0 authentication flow`.

## Self-Review Checklist

- [ ] Every spec section maps to one or more tasks: flow, HTTP contract, OpenAPI, session policy, persistence, configuration, package boundaries, security behavior, tests, and acceptance criteria.
- [ ] No task depends on an undefined type, method, route, or file.
- [ ] No task stores or exposes Auth0 tokens in the browser.
- [ ] OpenAPI paths and security scheme match the planned Gin routes.
- [ ] Tests are written before implementation in every code task and use fakes only at the provider/database boundary.
- [ ] No production migration is implied for the scaffold; new models are registered for local/test AutoMigrate.
- [ ] The plan does not introduce organizations, team roles, password storage, or API-key behavior into the Auth0 slice.
