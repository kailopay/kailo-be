# Backend Security

## 1. Security objective

Protect sandbox credentials, testnet signing keys, order integrity, and external-effect correctness while clearly avoiding any claim of production security or regulatory readiness.

Security controls in `v0.1.0` focus on the highest-risk boundaries: API keys, payment callbacks, Stellar signing, webhook SSRF/signing, secret handling, idempotency, and public evidence.

## 2. Security boundaries and assets

Assets:

- Payment gateway sandbox credentials and callback-authentication secret/token.
- Stellar testnet secret keys.
- API-key and webhook-signing secrets.
- Order/payment/transaction integrity and audit records.
- Deployment/database credentials.
- Public repository and release artifacts.

Trust boundaries:

- Public client to API.
- Payment provider to callback endpoint.
- API/worker to PostgreSQL.
- Worker to payment provider and Stellar testnet.
- Worker to arbitrary developer webhook endpoints.
- Deployment runtime to managed secret store.

## 3. Threat and control summary

| Threat | Impact | Required controls |
|---|---|---|
| Forged payment callback | Unauthorized asset issuance | Provider authentication/signature, exact reconciliation, event uniqueness, optional provider status lookup |
| Callback replay | Duplicate asset movement | Unique event ID/fingerprint, idempotent transition, single settlement intent |
| API-key theft | Unauthorized sandbox orders/data access | High-entropy secret, one-time display, hashed storage, revocation, TLS, log redaction, rate limits |
| Stellar secret exposure | Testnet asset/account compromise | Runtime secret injection, worker-only access, no DB/source/log storage, rotation procedure |
| Duplicate/unknown Stellar submission | Double settlement | Stable intent, account serialization, network reconciliation before retry, unique purpose constraint |
| Webhook SSRF | Access to internal/metadata services | HTTPS policy, IP/DNS validation, block private/link-local/metadata ranges, no redirects, revalidation |
| SQL/injection attacks | Data corruption/exposure | Schema validation, parameterized queries/ORM, no shell interpolation, least-privilege DB account |
| Cross-client access | Order data exposure | Authentication context in every repository query, ownership checks, enumeration policy tests |
| Secret leakage in public evidence | Credential compromise | Redaction, secret scanning, sanitized fixtures/logs/screenshots, release checklist |
| Misleading production claim | Legal/reputational risk | Persistent sandbox/testnet labels and explicit non-goals |

## 4. API-key design

Recommended format: `pk_test_<public_id>_<random_secret>`.

- Generate at least 256 bits of cryptographically secure secret entropy.
- Store the public lookup ID and a strong keyed hash/HMAC or password-hash-appropriate verification value; never plaintext.
- Return full key once and show only safe prefix/last characters later.
- Use constant-time comparison.
- Associate key with environment, client, status, and timestamps.
- Revoke immediately and make revocation effective across all instances.
- Do not place API keys in query strings.
- Redact `Authorization` and key-like patterns in logs/error reports.

## 5. Authentication and authorization

- Auth0 callback handling resolves a provider subject to one local user; authentication middleware then resolves either one active API client or one active retail session. A request cannot silently switch between contexts.
- Application/repository methods receive explicit API-client, user, and/or retail-session context as required by the operation.
- Provider subjects are stored in `user_identities` with a unique `(provider, subject)` constraint; email is not used as the authentication key.
- Every API order and webhook endpoint query is scoped by API-client ownership; every developer-management query is scoped through the client owner; every retail order query is scoped by user/session ownership.
- Retail session tokens are high entropy, stored only as hashes, sent only through secure HTTP-only cookies, expired and revocable, and protected against CSRF for browser mutations.
- Developer Mode and direct `owner_user_id` checks are enforced before developer-session configuration actions.
- Developer-session endpoints use CSRF/session protections appropriate to the selected frontend auth approach.
- There is no public endpoint that accepts an arbitrary `client_id` as authorization.
- Operator/admin functionality is not publicly exposed in `v0.1.0`; use controlled database/runbook procedures where necessary.

## 6. Input and protocol controls

- Validate body, path, query, headers, content type, and maximum size at the boundary.
- Reject unknown/ambiguous enum and precision values according to OpenAPI policy.
- Validate Stellar public keys and memo/muxed rules using trusted libraries.
- Normalize URLs only through a dedicated webhook URL policy.
- Apply request timeouts and bounded parsing.
- Use HTTPS for all public endpoints and external calls.
- Configure secure response headers and restrictive CORS for browser-facing routes.

## 7. Payment callback security

- Preserve raw bytes required for signature verification.
- Verify configured provider mechanism before trusting payload fields.
- Reconcile provider ID, order reference, currency, exact amount, method, and final paid state.
- Persist event identity/fingerprint under a unique constraint.
- Never perform Stellar signing directly in the callback request.
- Rate/size-limit callbacks without blocking valid provider ranges unless the provider supplies authoritative IP guidance.

## 8. Stellar key and transaction security

- Signing secrets are injected into the worker only.
- Use dedicated testnet accounts and least-funded balances suitable for the demo.
- Serialize submissions per source account or enforce account-level locking.
- Configure network passphrase explicitly; assert testnet at startup.
- Include deterministic order correlation without leaking sensitive data.
- Reconcile unknown outcomes before retry.
- Store transaction hashes and public accounts, not secret seeds.

Production HSM, MPC, multisig governance, custody policy, and dual control are Future requirements.

## 9. Secret management

Secret categories:

- Database credential.
- Auth0 client secret and callback configuration.
- Gateway sandbox credential and callback secret/token.
- Stellar testnet secret keys.
- API-key hashing pepper/key.
- Webhook signing/encryption master key.
- Developer-session secret.

Rules:

- Inject from the selected managed environment; local development uses ignored `.env` files based on `.env.example`.
- Fail startup when required secrets are missing or obviously invalid.
- Never echo secrets during startup or health checks.
- Rotate exposed secrets and document testnet account replacement.
- CI scans repository history/current changes for credential patterns.

## 10. Database security

- Use TLS to managed PostgreSQL when supported/required.
- Runtime account receives only required schema permissions; migration credentials may be separate.
- Parameterize all queries.
- Encrypt/minimize synthetic withdrawal destinations and webhook signing secrets.
- Backups inherit secret/PII controls and are not published as evidence.
- Database errors map to safe public errors.

## 11. Logging and evidence redaction

Never log or publish:

- `Authorization` headers or complete API keys.
- Payment gateway credentials/signatures/tokens.
- Stellar secret seeds or signed envelopes that create avoidable risk.
- Webhook signing secrets.
- Database URLs with credentials.
- Real identity documents, bank details, or unnecessary raw callbacks.

Safe logs use IDs, provider name, safe status/code, payload hash, amount/currency where appropriate, and transaction hash/public account.

Screenshots and demo recordings require the same review as source code.

## 12. Dependency and supply-chain controls

- Pin dependencies with a committed lockfile.
- Use the supported Go toolchain declared by `go.mod` and keep dependencies current through reviewed updates.
- Run dependency vulnerability scanning and record unresolved findings.
- Minimize provider SDKs; prefer documented HTTPS clients when SDK quality/compatibility is poor.
- CI build/test/lint/secret scan must pass before release.
- Release tag identifies exact code and lockfile used for evidence.

## 13. Security verification checklist

- Invalid/revoked API keys fail and are not logged.
- Cross-client order access fails.
- Callback with missing/invalid authentication cannot mutate state.
- Callback replay cannot duplicate settlement.
- Amount/currency/reference mismatch cannot confirm payment.
- Unknown Stellar result reconciles before retry.
- Webhook registration blocks loopback/private/link-local/metadata destinations and redirects.
- Webhook signature sample has positive and negative tests.
- Public repository, logs, screenshots, video, and docs pass secret/PII review.
- Deployment confirms sandbox provider and Stellar testnet at startup.

## 14. Residual risk and future requirements

`v0.1.0` is a sandbox demonstration and is not suitable for real funds. Before production, complete independent threat modelling, penetration testing, secure custody/key management, KYC/AML/privacy controls, incident response, vulnerability management, access governance, regulatory review, reconciliation, and production operations design.
