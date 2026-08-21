# KailoPay Backend

Go backend boilerplate for the KailoPay sandbox/testnet modular monolith.
The module path is `github.com/febry3/kailopay-be`.

## Stack

- Go 1.25+
- Gin for HTTP delivery
- Viper for startup configuration
- PostgreSQL through GORM
- Private profile-image storage through MinIO
- Xendit Payment Requests v3 sandbox checkouts
- CoinMarketCap reference pricing and native XLM on Stellar testnet
- `log/slog` for structured logging

## Layout minimum

```text
cmd/api                 HTTP process composition root
cmd/automigrate         guarded local/test GORM schema bootstrap
internal/entity         domain entities and invariants
internal/handler/http    HTTP transport and router
internal/handler/middleware HTTP middleware
internal/usecase         business workflows / use cases
internal/repository      persistence adapters and repository queries
internal/adapter         external payment/Stellar adapters
internal/platform        config, database lifecycle, and logging
```

Folder capability, provider adapter, dan worker tidak perlu dibuat sampai ada
workflow yang menggunakannya. Struktur Go ini sengaja tumbuh bersama fitur,
bukan membuat seluruh kemungkinan folder di hari pertama.

## Mengapa `platform` terpisah?

`internal` melindungi seluruh kode aplikasi. `internal/platform` mengelompokkan
runtime mechanics yang dipakai lintas capability: membaca config, membuat
connection pool, dan menyiapkan `slog`. Platform bukan tempat business rule;
`cmd/*` menyusun platform, repository, adapter, handler, dan usecase melalui
constructor injection.

Bedakan dua tanggung jawab database:

- `internal/platform/database.go` mengelola lifecycle koneksi GORM dan pool.
- `internal/repository` menerjemahkan operasi bisnis ke query dan
  model persistence.

## Di mana use case?

Use case berada di `internal/usecase`. Setiap capability memakai file yang
jelas, misalnya `auth_usecase.go` atau `order_usecase.go`; tidak perlu membuat
subfolder capability hanya untuk memenuhi diagram folder.

The dependency direction is inward: handlers and adapters depend on usecases,
and usecases depend on entities. Interfaces belong to the package that
consumes them. For this MVP, table models are grouped in `internal/entity`
one file per table; repository queries remain in `internal/repository`.

## Local setup

1. Copy `.env.example` to `.env` and set the Auth0 values.
2. Start the local PostgreSQL and MinIO services:

   ```powershell
   docker compose up -d postgres minio
   ```

   PostgreSQL is available at `localhost:5432`. MinIO exposes its S3 API at
   `http://localhost:9000` and its console at `http://localhost:9001`. The API
   creates the private `kailopay-profile` bucket automatically in local/test.

3. Apply the versioned Week 1 schema:

   ```powershell
   go run ./cmd/migrate
   ```

4. Start the API:

   ```powershell
   go run ./cmd/api
   ```

   Health endpoints are available at `/livez`, `/readyz`, and `/startupz`.
   `/health`, `/healthz`, and `/ready` remain compatibility aliases.

   For live reload during development, run Air instead:

   ```powershell
   air
   ```

   The repository-level `.air.toml` builds `./cmd/api` into the ignored
   `tmp/api.exe` and reloads when Go source or `.env` changes.

5. Run the settlement worker in a second terminal after configuring a funded
   Stellar testnet distribution account and `STELLAR_TREASURY_SECRET`:

   ```powershell
   go run ./cmd/worker
   ```

Developer flow: sign in through Auth0, set `developerEnabled` with
`PATCH /auth/me`, create a one-time `pk_test_` key at `POST /v1/api-keys`, then
use it as `Authorization: Bearer <key>` with `POST /v1/onramps`. The on-ramp
requires `Idempotency-Key`, accepts QRIS or `bri_va`, reserves pre-funded XLM,
and returns Xendit presentation instructions. See `openapi/openapi.yaml`.

Stop the local services with `docker compose down`. Add `-v` only when you
intentionally want to remove the PostgreSQL and MinIO development data.

`cmd/automigrate` refuses environments other than `local` and `test`. Do not
use GORM `AutoMigrate` as a production/shared-database migration mechanism;
production changes must use reviewed, versioned migrations.

Auth profile endpoints are `GET/PATCH /auth/me` and
`GET/PUT/DELETE /auth/me/avatar`. `POST /auth/password/forgot` asks the
configured Auth0 database connection to send its hosted reset email. Configure
an Auth0 post-change-password Action to call
`POST /internal/auth/password-reset-completed` with
`Authorization: Bearer <AUTH_PASSWORD_RESET_WEBHOOK_SECRET>` and JSON
`{"subject":"<event.user.user_id>"}` so existing local sessions are revoked.

## Verification

```powershell
gofmt -w (rg --files -g '*.go')
go vet ./...
go test ./...
go test -race ./...
```

See [backend documentation](documentations/backend/README.md) and
[the agent architecture rules](AGENTS.md) for the full boundary and safety
guidance.
