# KailoPay Backend Agent Guide

## Scope

These instructions apply to the entire repository. KailoPay is a sandbox/testnet
payment and Stellar backend. Correctness, idempotency, auditability, and explicit
failure handling take priority over delivery speed or abstraction purity.

Read the relevant files in `documentations/` before changing behavior. Keep the
architecture, API, domain model, testing strategy, backlog, and implementation
phases synchronized when a decision changes.

## Go philosophy

- Write clear, simple, idiomatic Go. Clear is better than clever.
- Use `gofmt` and `goimports`; do not hand-format around their output.
- Prefer a little duplication over a premature abstraction or unnecessary
  dependency. Extract an abstraction only after its boundary is understood.
- Make the zero value useful where practical, but do not force it when an
  explicit constructor is required to enforce invariants.
- Treat errors as values. Return and handle expected failures; reserve `panic`
  for programmer errors and impossible invariants.
- Keep the public surface small. Export only what another package genuinely
  needs.
- Accept interfaces when substitution is required and return concrete types.
- Do not introduce reflection, `unsafe`, cgo, mutable global state, or implicit
  `init` behavior without a documented, reviewed justification.

Primary style references:

- https://go.dev/doc/effective_go
- https://go.dev/doc/modules/layout
- https://go.dev/blog/package-names
- https://go.dev/wiki/CodeReviewComments
- https://go-proverbs.github.io/

## Target architecture

Use a modular monolith with classic Clean Architecture dependency direction.
Keep one Go module unless an independently versioned public library is actually
required.

```text
cmd/
  api/                         HTTP process composition root
  automigrate/                 Local/test migration composition root
internal/
  entity/                      Business entities, value objects, and invariants
  handler/
    http/                      HTTP handlers, router, and DTOs
    middleware/                HTTP middleware
  usecase/                     Application workflows and consumer-owned ports
  repository/                  Database and external persistence adapters
  adapter/                     External payment/Stellar adapters
  platform/
    config.go                  Configuration parsing and validation
    database.go                Database connection and lifecycle
    logging.go                 Structured logging setup
migrations/                    Versioned PostgreSQL migrations
openapi/                       Public API contract
```

Do not add `src/`, `pkg/`, or generic dumping-ground packages. This repository
builds an application, not a public Go library. Add new directories only when
they represent a cohesive package with a clear responsibility.

## Dependency rules

Dependencies point inward:

```text
delivery and infrastructure adapters -> usecase -> entity
cmd -> all layers only for construction and lifecycle management
```

- `entity` imports no delivery, database, SDK, logging, configuration, or
  framework packages.
- `usecase` may import `entity` and the standard library. It must not import
  PostgreSQL, payment-provider, Stellar, or HTTP implementations.
- `repository` implements persistence ports owned by consumers.
- Keep GORM models and query details inside `repository`; GORM is an
  infrastructure tool, not a domain or use-case contract.
- Use `db.WithContext(ctx)` for repository operations and keep transactions
  explicit at the application boundary. Do not expose `*gorm.DB` to entities,
  use cases, or delivery packages.
- `handler/*` translates protocols into usecase input and maps results and
  errors back to the protocol. It contains no business rules.
- `platform/*` provides reusable runtime mechanics, not domain policy.
- `cmd/*/main.go` parses configuration, constructs dependencies, starts the
  process, and coordinates graceful shutdown. Keep business logic out of it.
- Never create an import cycle to preserve a desired folder diagram. Change the
  boundary instead.

## Interfaces and dependency inversion

Declare an interface in the package that consumes it, normally under
`internal/usecase`. Do not declare provider-owned interfaces merely
to mirror concrete implementations.

For example, if an order use case must create an order and read a user, define
two narrow ports based on those exact needs:

```go
package order

type OrderCreator interface {
	Create(ctx context.Context, order *entity.Order) error
}

type UserReader interface {
	FindByID(ctx context.Context, id entity.UserID) (*entity.User, error)
}
```

Follow these rules:

- Prefer small, behavior-oriented interfaces such as `OrderCreator`,
  `OrderFinder`, or `UserReader` over a broad `Repository` interface.
- A use case that needs two independent capabilities receives two interfaces;
  do not combine them only because one database implements both.
- Do not create `IOrderRepository`, `BaseRepository`, generic CRUD repositories,
  or a repository interface containing every operation for an aggregate.
- Concrete PostgreSQL and gateway types may implement multiple consumer-owned
  interfaces through Go's structural typing.
- Compile-time assertions such as
  `var _ order.OrderCreator = (*OrderRepository)(nil)` are optional and belong
  in the adapter package.
- Do not use pointers to interfaces.
- Introduce an interface only at a real substitution boundary, such as a
  database, external API, clock, ID generator, or focused test seam. Use
  concrete types for ordinary internal collaboration.

## Package and file organization

- Package names are short, lowercase, singular, and meaningful.
- Avoid packages named `util`, `common`, `helper`, `model`, `types`, or
  `interfaces`. The `usecase` directory is the application workflow layer;
  name files by capability, such as `auth_usecase.go` or `payment_usecase.go`.
- Avoid package-name stutter in capability-specific types; use names such as
  `AuthUsecase` rather than repeating the capability name unnecessarily.
- Keep related declarations and their tests together. Split files by cohesive
  responsibility, not by arbitrary line count.
- Co-locate tests as `*_test.go`. Put fixtures in `testdata/`.
- Keep request/response DTOs at the delivery boundary. Do not add JSON, SQL, or
  provider SDK concerns to entities.
- Do not let PostgreSQL row structs or provider response objects escape their
  adapter packages.

## Functions, context, and errors

- Put `context.Context` first and propagate it through every blocking or
  external operation. Do not store a context in a struct.
- Give every external call a timeout or an inherited deadline.
- Handle errors immediately and keep the successful path minimally indented.
- Wrap errors with useful operation context using `%w`. Use `errors.Is` and
  `errors.As` for classification.
- Error strings are lowercase and have no trailing punctuation.
- Do not log and return the same error at the same layer. Add context while
  returning; log once at the delivery or process boundary.
- Use typed or sentinel errors only when callers need stable classification.
- Avoid naked returns in non-trivial functions.
- Defer resource cleanup immediately after successful acquisition and handle a
  cleanup error when it affects correctness.

## Domain and data rules

- Keep order state transitions inside entities or deterministic domain policy;
  adapters report facts and must not mutate arbitrary order fields.
- Represent IDR using integer minor units. Never use floating point for money.
- Represent Stellar quantities using exact decimal or stroop-based arithmetic
  according to the configured asset precision.
- Use typed IDs, statuses, and value objects when they prevent invalid states.
- Start enums with an explicit unknown or invalid zero value.
- Validate transport syntax at delivery boundaries and enforce business
  invariants in entities/use cases.
- Use UTC timestamps and inject a clock where time affects behavior.
- Return initialized empty slices at JSON boundaries when the public contract
  requires `[]`; otherwise a nil slice is idiomatic internally.

## Transactions and external effects

- A PostgreSQL transaction may atomically persist local state, domain events,
  idempotency records, and outbox messages.
- Never hold a database transaction open while calling a payment provider,
  Stellar, or a developer webhook.
- Coordinate external effects through durable intent, transactional outbox,
  stable idempotency keys, bounded retries, and reconciliation.
- Treat timeouts after sending an external request as unknown outcomes. Query or
  reconcile before retrying an operation that can move value.
- Do not combine repository interfaces merely to obtain a transaction. Model a
  transaction boundary explicitly when a use case needs one.
- Limit queues, batches, goroutines, retries, buffers, and connection pools.
  Every goroutine must have an owner and a termination path.

## Testing expectations

- Write table-driven unit tests for domain invariants, state transitions,
  validation, error classification, and use-case orchestration.
- Use small handwritten fakes that implement only the consumer-owned interface
  under test. Do not build a global mock repository framework.
- Test PostgreSQL repositories and migrations against an ephemeral PostgreSQL
  instance, including constraints, transactions, leases, and concurrency.
- Add contract tests for payment and Stellar adapters using recorded or sandbox
  behavior without committing secrets.
- Test callback replay, idempotency, unknown outcomes, reconciliation, and
  duplicate job delivery explicitly.
- Run race-enabled tests for concurrent workers and shared state.
- A bug fix must include a regression test when the behavior can be tested.

Once the Go module exists, the normal verification baseline is:

```text
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

Run `go mod tidy` only when imports or tool dependencies change, and review its
diff. Do not add or upgrade dependencies without explaining the need.

## Security and observability

- Never commit, print, or return private keys, full API keys, gateway secrets,
  callback secrets, or raw sensitive provider payloads.
- Use `crypto/rand` for keys and tokens and constant-time comparison where
  appropriate.
- Use parameterized SQL and explicit transaction boundaries.
- Use `AutoMigrate` only for local/test bootstrap. Production/shared databases
  require reviewed, versioned migrations and an approval gate.
- Configure GORM logging through the injected `slog` logger, and never emit
  SQL parameters, credentials, or raw sensitive provider payloads.
- Use structured logs with request, correlation, order, provider, and Stellar
  intent identifiers. Do not log secret or sensitive values.
- Public errors are stable and sanitized; internal logs retain safe diagnostic
  context.
- Webhook delivery must enforce the documented SSRF policy.

## Change discipline

- Work in vertical slices: delivery contract, use case, entity behavior,
  adapter, persistence, tests, observability, and documentation.
- Preserve the modular-monolith boundary. Do not introduce microservices,
  queues, frameworks, code generation, or dependency-injection containers
  without demonstrated need and an approved architecture decision.
- Prefer manual constructor injection.
- When architecture, behavior, API contracts, or operational assumptions
  change, update the affected documents in the same change.
- Do not silently resolve a documented Phase 0 decision, such as the selected
  payment provider, Go version, migration tool, router, or PostgreSQL driver.
  Record the decision first.
