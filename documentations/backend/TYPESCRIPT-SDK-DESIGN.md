# TypeScript SDK Design

## 1. Boundary

The KailoPay backend is implemented in Go. The official developer SDK remains a separate TypeScript package that consumes the public versioned REST API.

The SDK is a thin integration client. It must not implement order-state transitions, payment reconciliation, Stellar settlement, retry policy for value movement, or other backend business rules.

## 2. Runtime and distribution

- Source language: TypeScript.
- Primary runtime: supported Node.js LTS versions documented by the released package.
- Output: JavaScript plus TypeScript declarations.
- Package format: choose ESM-only or dual ESM/CommonJS once and document the decision before release.
- Version: semantic version independent from, but compatibility-mapped to, the KailoPay API version.
- Secret-key operations are server-side only. Never embed `pk_test_` keys in browser or mobile bundles.

Browser-safe interactive helpers may be designed later, but they must not contain a secret API key and are not required for `v0.1.0`.

## 3. Public client

Suggested surface:

```ts
const kailoPay = new KailoPay({
  apiKey: process.env.KAILOPAY_API_KEY,
  baseURL: "https://api.sandbox.kailopay.test",
});

const order = await kailoPay.onramps.create(
  {
    fiat: { currency: "IDR", amountMinor: "100000" },
    asset: { code: "KIDR", network: "stellar_testnet" },
    paymentMethod: "qris",
    stellarDestination: { account: "G..." },
  },
  { idempotencyKey: "checkout-session-123" },
);

const current = await kailoPay.orders.retrieve(order.id);
```

Minimum resources:

- `onramps.create(input, options)`
- `offramps.create(input, options)`
- `orders.retrieve(orderId)`
- `orders.list(params)`
- `webhooks.verifySignature(rawBody, headers, secret, options)`

The final method and field names must match the published package and examples.

## 4. Configuration

| Option | Requirement |
|---|---|
| `apiKey` | Required for API operations; accept value at construction and never log it |
| `baseURL` | Defaults only to the official sandbox URL; custom URL allowed for local/test use |
| `timeoutMs` | Finite default with per-request override |
| `maxNetworkRetries` | Conservative default for safe HTTP transport retries only |
| `fetch`/HTTP transport | Optional injection seam for tests and supported runtimes |
| `userAgent` | Includes SDK name/version without sensitive data |

The SDK must not silently switch to production or mainnet endpoints.

## 5. Amounts and serialization

- IDR uses integer minor units represented as strings where required by the API contract.
- Stellar quantities use canonical decimal strings.
- The SDK does not accept or convert JavaScript floating-point settlement values.
- Request serialization is deterministic where webhook/signature or idempotency hashing depends on exact bytes.
- Response types preserve unknown additive fields at runtime and avoid unsafe numeric coercion.

## 6. Authentication and secret safety

- Send `Authorization: Bearer pk_test_...` only to the configured KailoPay origin.
- Do not place the key in query strings, thrown errors, debug output, telemetry, or generated examples.
- Reject browser runtime for secret-key operations where reliable detection is possible; always document the prohibition because runtime detection is not a complete security boundary.
- Redirects must not forward authorization to another origin.
- Tests use synthetic non-working keys.

## 7. Idempotency and retries

- Mutation methods accept an explicit idempotency key.
- The SDK may generate a key only when the behavior is documented and the caller can recover/reuse it across their own retry boundary; explicit caller-supplied keys are preferred.
- Retry connection failures, selected transient statuses, and rate limits only when the method is safe and the idempotency key is stable.
- Never conceal an unknown outcome. Return a typed error that includes safe request/order references and instructs the caller to retrieve current state.
- Do not implement settlement retries; those belong to the Go backend.

## 8. Error model

Expose a stable `KailoPayError` containing:

- HTTP status.
- KailoPay error code and type.
- Safe message and optional field.
- `retryable` flag from the API when present.
- Request ID.
- Safe headers such as rate-limit/retry metadata.

Do not expose raw response bodies containing unexpected sensitive data. Preserve the original network cause through the runtime's supported error-cause mechanism without logging it automatically.

## 9. Webhook verification

`webhooks.verifySignature` must:

1. Accept the exact raw request body as bytes/string, not parsed/re-serialized JSON.
2. Read the documented timestamp and versioned signature headers.
3. Reject timestamps outside the configured/documented tolerance.
4. Compute HMAC-SHA256 over `<timestamp>.<raw_body>`.
5. Compare signatures in constant time.
6. Return the parsed typed event only after successful verification, or return verified bytes/event separately according to the final API.

Provide framework examples that preserve raw bodies, but keep the core verifier framework-neutral.

## 10. OpenAPI relationship

The SDK is contract-driven but does not require code generation. If handwritten:

- Types and methods are reviewed whenever OpenAPI changes.
- Contract tests run SDK requests against the Go release candidate.
- CI detects missing endpoint/error/schema coverage.

Introducing generated code requires a separate decision showing that it reduces drift without creating unreadable or unstable public types.

## 11. Testing

- Unit tests for serialization, exact amounts, headers, error mapping, retry classification, and webhook verification vectors.
- Transport tests proving API keys are not forwarded across origins or included in errors/logs.
- Contract tests against a controlled Go API instance.
- Sandbox smoke tests for create/retrieve/list order operations.
- Type-level tests for intended TypeScript usage.
- Package tests for each supported Node.js version and selected module format.

## 12. Release criteria

- Package documentation and examples use only sandbox/testnet data.
- Public methods map to validated OpenAPI operations.
- API key browser warning is prominent.
- Exact-amount and webhook-signature tests pass.
- Package contents contain no credential, private key, or environment file.
- Compatibility table identifies SDK, API, and minimum Node.js versions.
- The SDK version used in the demo/evidence is recorded with backend release `v0.1.0`.
