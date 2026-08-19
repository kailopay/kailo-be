# KailoPay Backend

Go backend boilerplate for the KailoPay sandbox/testnet modular monolith.
The module path is `github.com/febry3/kailopay-be`.

## Stack

- Go 1.25+
- Gin for HTTP delivery
- Viper for startup configuration
- PostgreSQL through GORM
- `log/slog` for structured logging

## Layout minimum

```text
cmd/api                 HTTP process composition root
cmd/automigrate         guarded local/test GORM schema bootstrap
internal/entity         domain entities and invariants
internal/handler/http    HTTP transport and router
internal/handler/middleware HTTP middleware
internal/usecase         business workflows / use cases
internal/repository      persistence adapters and GORM models
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
consumes them. GORM models stay in `internal/repository` and do
not become domain entities or use-case contracts.

## Local setup

1. Copy `.env.example` to `.env` and set `DATABASE_DSN`.
2. Start a local PostgreSQL database.
3. Run the guarded schema bootstrap:

   ```powershell
   go run ./cmd/automigrate
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

`cmd/automigrate` refuses environments other than `local` and `test`. Do not
use GORM `AutoMigrate` as a production/shared-database migration mechanism;
production changes must use reviewed, versioned migrations.

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
