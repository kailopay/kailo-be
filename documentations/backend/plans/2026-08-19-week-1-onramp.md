# Week 1 On-Ramp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a production-shaped sandbox IDR-to-native-XLM on-ramp using Xendit, CoinMarketCap, PostgreSQL reservations, and a pre-funded Stellar testnet treasury.

**Architecture:** Keep entities deterministic, declare narrow ports in the consuming service packages, and implement PostgreSQL/Xendit/CoinMarketCap/Stellar details in outward adapters. The API creates and reconciles payment state; a separate worker owns Stellar signing and settlement.

**Tech Stack:** Go 1.25, Gin 1.12, GORM 1.31/PostgreSQL 18, Xendit Payment Sessions, CoinMarketCap quotes v3, Stellar Go SDK/Horizon testnet, kin-openapi.

**Spec:** `documentations/backend/WEEK-1-ONRAMP-DESIGN.md`

## Global Constraints

- Use Xendit test mode only. Payment Sessions use the hosted payment link; optional restrictions use channel codes `QRIS` and `BRI_VIRTUAL_ACCOUNT`.
- Use native XLM on Stellar testnet from a pre-funded treasury distribution account.
- IDR uses integer rupiah; XLM uses integer stroops; never use floating point.
- CoinMarketCap is an indicative price provider, not executable liquidity.
- No external network call occurs inside a PostgreSQL transaction.
- Payment callbacks never submit Stellar transactions.
- Unknown external outcomes are reconciled before retry.
- API process configuration excludes the Stellar treasury secret; only `cmd/worker` receives it.
- Off-ramp, SEP-24, custom assets, treasury replenishment, and production/mainnet behavior are excluded.
- Use test-first red/green cycles and run only affected packages during implementation.

---

### Task 1: Exact value objects and runtime configuration

**Files:**
- Create: `internal/entity/amount.go`
- Create: `internal/entity/amount_test.go`
- Create: `internal/entity/order.go`
- Create: `internal/entity/order_test.go`
- Create: `internal/platform/id.go`
- Create: `internal/platform/id_test.go`
- Modify: `internal/platform/config.go`
- Modify: `internal/platform/config_test.go`
- Modify: `.env.example`

**Interfaces:**
- Produces: `entity.IDR`, `entity.Stroops`, `entity.Order`, `entity.OrderStatus`, `entity.PaymentMethod`, and `platform.NewID() (string, error)`.
- Produces: `platform.OnrampConfig`, `platform.XenditConfig`, `platform.CoinMarketCapConfig`, `platform.StellarConfig`, and `platform.WorkerConfig` on `platform.Config`.

- [ ] **Step 1: Write failing exact-amount and transition tests**

```go
func TestStroopsString(t *testing.T) {
    tests := []struct {
        value entity.Stroops
        want  string
    }{
        {value: 1, want: "0.0000001"},
        {value: 10_000_000, want: "1.0000000"},
        {value: 123_456_789, want: "12.3456789"},
    }
    for _, tt := range tests {
        if got := tt.value.String(); got != tt.want {
            t.Fatalf("String() = %q, want %q", got, tt.want)
        }
    }
}

func TestOrderRejectsIllegalTransition(t *testing.T) {
    order := entity.Order{Status: entity.OrderStatusCreated, Version: 1}
    if err := order.Transition(entity.OrderStatusCompleted); !errors.Is(err, entity.ErrInvalidOrderState) {
        t.Fatalf("Transition() error = %v", err)
    }
}
```

- [ ] **Step 2: Run tests and confirm they fail because the types do not exist**

Run: `go test ./internal/entity ./internal/platform`

Expected: compile failure for missing amount/order/config declarations.

- [ ] **Step 3: Implement exact integer formatting, the documented on-ramp state machine, cryptographic UUID generation, and config validation**

```go
type IDR int64
type Stroops int64

const StroopsPerXLM Stroops = 10_000_000

func (s Stroops) String() string {
    whole := int64(s) / int64(StroopsPerXLM)
    fraction := int64(s) % int64(StroopsPerXLM)
    return fmt.Sprintf("%d.%07d", whole, fraction)
}
```

Configuration validation must reject non-testnet passphrases, production Xendit URLs, blank secrets, non-positive timeouts/ranges, invalid basis points, and a treasury secret present in API-only configuration.

- [ ] **Step 4: Run focused tests and format**

Run: `go fmt ./internal/entity ./internal/platform && go test ./internal/entity ./internal/platform`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/entity internal/platform .env.example
git commit -m "feat(onramp): add exact domain values and configuration"
```

### Task 2: Versioned schema and transaction-ready repositories

**Files:**
- Create: `migrations/000001_week1_onramp.up.sql`
- Create: `migrations/000001_week1_onramp.down.sql`
- Create: `internal/platform/migrate.go`
- Create: `internal/platform/migrate_test.go`
- Create: `cmd/migrate/main.go`
- Modify: `internal/repository/models.go`
- Modify: `internal/repository/models_test.go`
- Modify: `cmd/automigrate/main.go`
- Modify: `migrations/README.md`

**Interfaces:**
- Produces: `platform.ApplyMigrations(ctx context.Context, db *sql.DB, fs fs.FS) error`.
- Produces models `TreasuryAccount`, `TreasuryReservation`; adds exact quote fields and unique settlement-purpose constraints.

- [ ] **Step 1: Write failing migration discovery/checksum tests and model registration tests**

```go
func TestMigrationFilesRequireOrderedPairs(t *testing.T) {
    migrationFS := fstest.MapFS{
        "000001_example.up.sql":   {Data: []byte("select 1")},
        "000001_example.down.sql": {Data: []byte("select 1")},
    }
    migrations, err := platform.DiscoverMigrations(migrationFS)
    if err != nil {
        t.Fatal(err)
    }
    if len(migrations) != 1 || migrations[0].Version != 1 {
        t.Fatalf("migrations = %#v", migrations)
    }
}
```

Extend `TestMigrationModelsAreExplicitlyRegistered` to require `treasury_accounts` and `treasury_reservations`.

- [ ] **Step 2: Run tests and verify the missing runner/models fail**

Run: `go test ./internal/platform ./internal/repository`

Expected: compile/test failure for missing migration API and tables.

- [ ] **Step 3: Add migration runner and reviewed SQL**

The SQL migration must create/alter:

- `api_clients`, `api_keys`
- `orders`, `order_events`
- `payment_checkouts`, `gateway_events`
- `stellar_transactions`, `idempotency_records`, `outbox_messages`
- `treasury_accounts`, `treasury_reservations`

It must include positive amount checks, legal status/method/network checks, ownership checks, unique callback IDs, unique `(order_id, purpose)`, unique reservation order, and unprocessed-outbox lease indexes. Store migration version and SHA-256 checksum in `schema_migrations`.

- [ ] **Step 4: Run focused tests and SQL static review**

Run: `go fmt ./internal/platform ./internal/repository ./cmd/migrate ./cmd/automigrate && go test ./internal/platform ./internal/repository ./cmd/...`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add migrations internal/platform internal/repository cmd/migrate cmd/automigrate
git commit -m "feat(database): add versioned week 1 schema"
```

### Task 3: Developer Mode and test API keys

**Files:**
- Create: `internal/service/apikey/service.go`
- Create: `internal/service/apikey/service_test.go`
- Create: `internal/repository/apikey_repository.go`
- Create: `internal/handler/middleware/apikey.go`
- Create: `internal/handler/middleware/apikey_test.go`
- Create: `internal/handler/http/apikey_handler.go`
- Create: `internal/handler/http/apikey_handler_test.go`
- Modify: `internal/usecase/auth_usecase.go`
- Modify: `internal/usecase/auth_usecase_test.go`
- Modify: `internal/repository/auth_repository.go`
- Modify: `internal/handler/http/auth_handler.go`

**Interfaces:**
- Consumes: `platform.NewID`, API key HMAC pepper, authenticated `auth.UserProfile`.
- Produces: `apikey.Store` with `Create`, `List`, `Revoke`, and `FindActiveByPublicID` methods.
- Produces: `apikey.Service.Create(ctx, userID, name) (CreatedKey, error)`, `List`, `Revoke`, and `Authenticate`.
- Produces middleware context accessor `middleware.APIClientFromContext(*gin.Context) (string, bool)`.

- [ ] **Step 1: Write failing service tests for one-time keys, revocation, ownership, and Developer Mode**

```go
func TestCreateReturnsPlaintextOnceAndStoresDigest(t *testing.T) {
    store := &fakeStore{developerEnabled: true}
    service := newTestService(t, store)
    created, err := service.Create(context.Background(), "user-1", "Default")
    if err != nil {
        t.Fatal(err)
    }
    if !strings.HasPrefix(created.Plaintext, "pk_test_") {
        t.Fatalf("plaintext = %q", created.Plaintext)
    }
    if bytes.Contains(store.created.SecretHash, []byte(created.Plaintext)) {
        t.Fatal("stored hash contains plaintext key")
    }
}
```

- [ ] **Step 2: Run the new package test and verify red**

Run: `go test ./internal/service/apikey`

Expected: compile failure because the service does not exist.

- [ ] **Step 3: Implement service, PostgreSQL adapter, middleware, and handlers**

Parse keys with exactly four underscore-delimited components, decode the secret with `base64.RawURLEncoding`, find by public ID, compute HMAC-SHA-256, and compare with `subtle.ConstantTimeCompare`.

Extend profile update with optional `DeveloperEnabled *bool`. Creating a key ensures one default `test` API client for the owner only after Developer Mode is enabled.

- [ ] **Step 4: Run service/repository/HTTP tests**

Run: `go fmt ./internal/service/apikey ./internal/repository ./internal/handler/... ./internal/usecase && go test ./internal/service/apikey ./internal/repository ./internal/handler/... ./internal/usecase`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/service/apikey internal/repository internal/handler internal/usecase
git commit -m "feat(auth): add developer test API keys"
```

### Task 4: Quote policy and CoinMarketCap adapter

**Files:**
- Create: `internal/service/onramp/quote.go`
- Create: `internal/service/onramp/quote_test.go`
- Create: `internal/adapter/coinmarketcap/client.go`
- Create: `internal/adapter/coinmarketcap/client_test.go`
- Create: `internal/adapter/coinmarketcap/testdata/xlm-idr.json`

**Interfaces:**
- Consumes: `entity.IDR`, `entity.Stroops`, an injected clock.
- Produces: `onramp.PriceReader.LatestXLMIDR(ctx context.Context) (MarketPrice, error)`.
- Produces: `onramp.QuotePolicy.Create(now time.Time, amount entity.IDR, market MarketPrice) (Quote, error)`.

- [ ] **Step 1: Write failing table tests for exact conversion**

```go
func TestQuoteRoundsDownToStroops(t *testing.T) {
    policy := QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 0}
    quote, err := policy.Create(now, 100_000, MarketPrice{IDRPerXLM: "2500", ObservedAt: now})
    if err != nil {
        t.Fatal(err)
    }
    if quote.AssetAmount != 400_000_000 {
        t.Fatalf("stroops = %d", quote.AssetAmount)
    }
}
```

Add stale, malformed decimal, spread, zero, and overflow cases.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/service/onramp ./internal/adapter/coinmarketcap`

Expected: compile failure for missing quote policy/client.

- [ ] **Step 3: Implement rational arithmetic and a bounded HTTP adapter**

Use `math/big.Rat` for rate/spread/division and integer quotient for round-down. Decode CoinMarketCap numbers with `json.Decoder.UseNumber`. Request `/v3/cryptocurrency/quotes/latest?id=512&convert=IDR`, set `X-CMC_PRO_API_KEY`, reject non-2xx and oversized bodies, and map only price/timestamp.

- [ ] **Step 4: Run tests**

Run: `go fmt ./internal/service/onramp ./internal/adapter/coinmarketcap && go test ./internal/service/onramp ./internal/adapter/coinmarketcap`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/service/onramp internal/adapter/coinmarketcap
git commit -m "feat(onramp): add exact XLM IDR quotes"
```

### Task 5: Idempotent order creation, treasury reservation, and Xendit checkout

**Files:**
- Create: `internal/service/onramp/service.go`
- Create: `internal/service/onramp/service_test.go`
- Create: `internal/repository/onramp_repository.go`
- Create: `internal/repository/onramp_repository_test.go`
- Create: `internal/adapter/xendit/client.go`
- Create: `internal/adapter/xendit/client_test.go`
- Create: `internal/adapter/xendit/testdata/qris-created.json`
- Create: `internal/adapter/xendit/testdata/va-created.json`
- Create: `internal/handler/http/onramp_handler.go`
- Create: `internal/handler/http/onramp_handler_test.go`

**Interfaces:**
- Produces: `onramp.TreasuryReader.SpendableBalance(ctx, account) (entity.Stroops, error)`.
- Produces: `onramp.Store.BeginCreate`, `ReserveAndCreate`, `AttachCheckout`, `FailCheckout`, `Get`, and `List`.
- Produces: `onramp.PaymentGateway.CreateCheckout(ctx, CheckoutInput) (Checkout, error)` and `FindCheckout(ctx, providerID) (PaymentState, error)`.
- Produces: `onramp.Service.Create(ctx, Command) (OrderView, replay bool, error)`.

- [ ] **Step 1: Write failing orchestration tests**

Cover valid QRIS/VA, same-key replay, conflicting request hash, insufficient inventory, stale quote, invalid destination, checkout permanent failure release, and checkout timeout held as unknown.

```go
func TestCreateReservesBeforeExposingCheckout(t *testing.T) {
    deps := newCreateFakes()
    service := newOnrampService(t, deps)
    _, _, err := service.Create(context.Background(), validCreateCommand())
    if err != nil {
        t.Fatal(err)
    }
    if deps.store.calls[0] != "reserve-and-create" || deps.gateway.calls[0] != "create-checkout" {
        t.Fatalf("calls = %#v", append(deps.store.calls, deps.gateway.calls...))
    }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/service/onramp`

Expected: compile/test failure for missing service ports and methods.

- [ ] **Step 3: Implement repository transactions and Xendit mapping**

The reservation transaction locks one treasury row, verifies fresh observed balance minus buffer/reservations, inserts order/event/reservation/idempotency records, and commits. Xendit uses Basic auth with the secret as username, API version `2024-11-11`, `session_type: PAY`, `mode: PAYMENT_LINK`, stable order reference, exact IDR integer, and returns the hosted `payment_link_url`.

Repository order queries always include `client_id`. Cross-client reads map to `ErrOrderNotFound`.

- [ ] **Step 4: Implement HTTP request/response mapping**

Require JSON content type, bounded body, `Idempotency-Key`, and API-client middleware. Return exact amounts as strings and safe error envelopes with request ID.

- [ ] **Step 5: Run focused tests**

Run: `go fmt ./internal/service/onramp ./internal/repository ./internal/adapter/xendit ./internal/handler/http && go test ./internal/service/onramp ./internal/repository ./internal/adapter/xendit ./internal/handler/http`

Expected: PASS.

- [ ] **Step 6: Commit**

```text
git add internal/service/onramp internal/repository internal/adapter/xendit internal/handler/http
git commit -m "feat(onramp): create reserved Xendit checkouts"
```

### Task 6: Authenticated callback and atomic settlement intent

**Files:**
- Create: `internal/service/onramp/callback.go`
- Create: `internal/service/onramp/callback_test.go`
- Modify: `internal/repository/onramp_repository.go`
- Modify: `internal/adapter/xendit/client.go`
- Modify: `internal/adapter/xendit/client_test.go`
- Create: `internal/adapter/xendit/testdata/payment-capture.json`
- Create: `internal/handler/http/xendit_callback.go`
- Create: `internal/handler/http/xendit_callback_test.go`

**Interfaces:**
- Extends: `onramp.PaymentGateway.VerifyCallback(raw []byte, token string) (Callback, error)` and `GetPaymentState(ctx, checkoutID) (PaymentState, error)`.
- Extends: `onramp.Store.RecordCallbackReceipt`, `ConfirmPaymentAndEnqueue`, and `CompleteCallback`.

- [ ] **Step 1: Write failing callback tests**

Cover invalid/missing token, malformed/oversized body, duplicate event, mismatched amount/currency/reference/channel/status, paid-after-expiry, and successful atomic transition.

```go
func TestPaidCallbackReplayCreatesOneSettlementIntent(t *testing.T) {
    store := newCallbackStore()
    service := newCallbackService(t, store)
    for range 2 {
        if err := service.Process(context.Background(), validCallback()); err != nil {
            t.Fatal(err)
        }
    }
    if store.paymentConfirmedCount != 1 || store.settlementIntentCount != 1 {
        t.Fatalf("confirmed=%d intents=%d", store.paymentConfirmedCount, store.settlementIntentCount)
    }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/service/onramp ./internal/adapter/xendit ./internal/handler/http`

Expected: compile/test failure for callback APIs.

- [ ] **Step 3: Implement constant-time authentication, provider lookup, and transaction**

Authenticate `x-callback-token` before trusted parsing. Hash raw bytes. Use the Xendit event and `payment_session_id` as event identity. Reconcile through `GET /sessions/{id}` and, when present, its related Payment Request; require exact stored facts.

`ConfirmPaymentAndEnqueue` must lock the order, check `payment_pending`, insert one gateway event, append `payment.confirmed` and `stellar.transfer_requested`, insert one `stellar_transactions` row for purpose `transfer`, and one `stellar.settle_onramp` outbox row in one transaction.

- [ ] **Step 4: Run focused tests**

Run: `go fmt ./internal/service/onramp ./internal/repository ./internal/adapter/xendit ./internal/handler/http && go test ./internal/service/onramp ./internal/repository ./internal/adapter/xendit ./internal/handler/http`

Expected: PASS.

- [ ] **Step 5: Commit**

```text
git add internal/service/onramp internal/repository internal/adapter/xendit internal/handler/http
git commit -m "feat(payments): reconcile Xendit callbacks once"
```

### Task 7: Stellar testnet adapter and settlement worker

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/service/settlement/service.go`
- Create: `internal/service/settlement/service_test.go`
- Create: `internal/repository/settlement_repository.go`
- Create: `internal/repository/settlement_repository_test.go`
- Create: `internal/adapter/stellar/client.go`
- Create: `internal/adapter/stellar/client_test.go`
- Create: `cmd/worker/main.go`

**Interfaces:**
- Produces: `settlement.Store.Lease`, `LoadIntent`, `MarkSubmitted`, `Confirm`, `MarkUnknown`, `FailPermanent`, and `RetryLater`.
- Produces: `settlement.Network.Build(ctx, Transfer) (BuiltTransaction, error)`, `Submit`, and `FindByHash`.
- Produces: `settlement.Service.RunOnce(ctx, workerID) (processed bool, error)`.

- [ ] **Step 1: Add the official Stellar SDK dependency and review the diff**

Run: `go get github.com/stellar/go-stellar-sdk@latest && go mod tidy`

Expected: only the official SDK and its required transitive modules are added.

- [ ] **Step 2: Write failing worker tests**

Cover one lease owner, confirmed transfer, permanent pre-submit failure, timeout/unknown, hash reconciliation before retry, duplicate outbox processing, and reservation consumption only on confirmation.

```go
func TestUnknownSubmissionReconcilesHashBeforeRetry(t *testing.T) {
    network := &fakeNetwork{submitResult: settlement.SubmissionUnknown, findResult: settlement.SubmissionConfirmed}
    store := newSettlementStore()
    service := newSettlementService(t, store, network)
    if _, err := service.RunOnce(context.Background(), "worker-1"); err != nil {
        t.Fatal(err)
    }
    if network.buildCalls != 1 || network.findCalls != 1 || network.submitCalls != 1 {
        t.Fatalf("build=%d submit=%d find=%d", network.buildCalls, network.submitCalls, network.findCalls)
    }
}
```

- [ ] **Step 3: Verify red**

Run: `go test ./internal/service/settlement ./internal/adapter/stellar`

Expected: compile failure for missing settlement packages.

- [ ] **Step 4: Implement native-XLM transaction and repository lease**

Use the configured testnet passphrase, source account sequence from Horizon,
`txnbuild.NativeAsset`, exact `Stroops.String()`, a bounded precondition, and a memo derived from the order ID within Stellar memo limits. Compute/persist hash before submission. Never log the secret or envelope.

Lease with `FOR UPDATE SKIP LOCKED`, bounded lease duration, and attempt count. Confirmation atomically completes the Stellar intent/order/outbox and consumes the reservation.

- [ ] **Step 5: Implement worker lifecycle**

`cmd/worker` loads config, opens PostgreSQL, constructs the signing adapter, loops with a bounded ticker, honors cancellation, and logs one safe error per failed job/process boundary.

- [ ] **Step 6: Run focused tests and tidy**

Run: `go fmt ./internal/service/settlement ./internal/repository ./internal/adapter/stellar ./cmd/worker && go mod tidy && go test ./internal/service/settlement ./internal/repository ./internal/adapter/stellar ./cmd/worker`

Expected: PASS with reviewed `go.mod`/`go.sum` diff.

- [ ] **Step 7: Commit**

```text
git add go.mod go.sum internal/service/settlement internal/repository internal/adapter/stellar cmd/worker
git commit -m "feat(settlement): transfer reserved testnet XLM"
```

### Task 8: Composition, routes, OpenAPI, and synchronized documentation

**Files:**
- Modify: `cmd/api/main.go`
- Modify: `internal/handler/http/router.go`
- Modify: `internal/handler/http/router_test.go`
- Modify: `openapi/openapi.yaml`
- Modify: `openapi/openapi_test.go`
- Modify: `README.md`
- Modify: `documentations/backend/API-DESIGN.md`
- Modify: `documentations/backend/DOMAIN-MODEL.md`
- Modify: `documentations/backend/DATABASE-DESIGN.md`
- Modify: `documentations/backend/ORDER-STATE-MACHINES.md`
- Modify: `documentations/backend/PAYMENT-GATEWAY-INTEGRATION.md`
- Modify: `documentations/backend/STELLAR-ANCHOR-INTEGRATION.md`
- Modify: `documentations/backend/IMPLEMENTATION-PHASES.md`
- Modify: `documentations/backend/BACKEND-BACKLOG.md`

**Interfaces:**
- Consumes all prior constructors and middleware.
- Produces routes `POST/GET/DELETE /v1/api-keys`, `POST /v1/onramps`, `GET /v1/orders`, `GET /v1/orders/{id}`, and `POST /callbacks/payments/xendit`.

- [ ] **Step 1: Write failing router and OpenAPI coverage tests**

```go
func TestRouterRegistersWeek1Routes(t *testing.T) {
    routes := registeredTestRoutes(t)
    for _, route := range []string{
        "POST /v1/api-keys",
        "POST /v1/onramps",
        "GET /v1/orders/:order_id",
        "GET /v1/orders",
        "POST /callbacks/payments/xendit",
    } {
        if !routes[route] {
            t.Errorf("missing route %s", route)
        }
    }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/handler/http ./openapi`

Expected: route/OpenAPI coverage failure.

- [ ] **Step 3: Wire API dependencies and readiness**

Construct API-key, quote, treasury-read, Xendit, on-ramp, callback, handler, and middleware dependencies manually. Readiness checks PostgreSQL and existing object storage; external sandbox/testnet outages remain operation-specific and do not make the process unready.

- [ ] **Step 4: Update OpenAPI and backend documents**

Replace custom-asset examples with native XLM for the accepted on-ramp, document exact statuses/errors/security schemes/idempotency/callback, and mark off-ramp/SEP-24 as deferred. Include every new environment variable without values.

- [ ] **Step 5: Run focused contract tests**

Run: `go fmt ./cmd/api ./internal/handler/... && go test ./cmd/api ./internal/handler/... ./openapi`

Expected: PASS.

- [ ] **Step 6: Commit**

```text
git add cmd/api internal/handler openapi README.md documentations/backend
git commit -m "feat(api): expose the week 1 onramp flow"
```

### Task 9: Final verification and branch review

**Files:**
- Modify only files needed to fix verified defects.

**Interfaces:**
- Consumes the complete branch.
- Produces a clean, reviewable feature branch with no secrets.

- [ ] **Step 1: Run formatting and static checks**

Run: `go fmt ./... && go vet ./...`

Expected: exit 0.

- [ ] **Step 2: Run one full test pass**

Run: `go test ./...`

Expected: exit 0. Real Xendit/Stellar tests remain opt-in and credential-gated.

- [ ] **Step 3: Validate non-Go contracts**

Run: `docker compose config -q && git diff --check`

Expected: exit 0; CRLF notices are acceptable but whitespace errors are not.

- [ ] **Step 4: Review dependency and secret-shaped diff**

Run: `git diff master...HEAD --stat && git diff master...HEAD -- .env.example go.mod go.sum`

Expected: example placeholders only; no reusable credentials or Stellar seeds.

- [ ] **Step 5: Commit final verified fixes if any**

```text
git add cmd internal migrations openapi documentations README.md .env.example go.mod go.sum
git commit -m "fix(onramp): address week 1 verification findings"
```

- [ ] **Step 6: Report manual sandbox evidence commands without running them**

Document how the user supplies local Xendit, CoinMarketCap, and Stellar testnet credentials, runs API and worker, creates QRIS/VA orders, simulates Xendit payment, and confirms the resulting transaction hash. Never print secret values.
