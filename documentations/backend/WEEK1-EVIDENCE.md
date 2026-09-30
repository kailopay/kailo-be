# Week 1 Evidence Checklist

This checklist maps the backend implementation to the Week 1 SOW. Code and
automated test results are kept separate from evidence that requires a real
sandbox or testnet account.

## Checkpoint status

Weeks 1 and 2 are closed as an implementation checkpoint dated 2026-09-16.
The unchecked items in this file remain open evidence gates. They are not
implementation claims, and they must be completed before formal release
acceptance.

See [Weeks 1 and 2 backend checkpoint](WEEK1-2-CHECKPOINT.md) for the complete
status, Week 2 result, and frontend handoff.

## Owner-confirmed test update

On 2026-09-25, the project owner confirmed that the Week 1 sandbox flows were
tested. On 2026-09-29, the owner provided buy and sell testnet transaction URLs
and three demo video URLs. Checkout screenshots and sanitized callback logs
are still pending. See the [completion report draft](COMPLETION-REPORT-DRAFT.md)
for the linked transactions, demos, and remaining evidence.

## Repository and contract

- [x] Public GitHub repository URL: `https://github.com/kailopay/kailo-be` (verified reachable on 2026-09-09).
- [x] `LICENSE` is present.
- [x] Go/Gin/GORM is the accepted backend implementation stack for this
  repository; the SOW's Node.js/TypeScript wording is treated as a stack
  substitution, not a second backend to build.
- [x] Initial OpenAPI contract: `openapi/openapi.yaml`

## API and payment gateway

- [x] Quote preview endpoint: `POST /v1/quotes`
- [x] On-ramp order creation endpoint: `POST /v1/onramps`
- [x] Off-ramp order creation endpoint: `POST /v1/offramps`
- [x] Order query endpoints: `GET /v1/orders` and `GET /v1/orders/{id}`
- [x] Xendit callback endpoint: `POST /callbacks/payments/xendit`
- [x] Hosted Xendit checkout supports unrestricted merchant channels through
  `payment_method: "xendit"`.
- [x] QRIS-only and BRI Virtual Account-only restrictions are supported through
  `payment_method: "qris"` and `payment_method: "bri_va"`.
- [x] Real Xendit sandbox Payment Session created on 2026-09-09:
  reference `kailopay-sow-week1-e5c893af78f7`, session
  `ps-6aa1885dd9fcab275ea45b62`, status `ACTIVE`, checkout URL
  `https://dev.xen.to/4kd9oCM_`.
- [ ] Screenshot or recording showing QRIS and BRI Virtual Account on the
  hosted Xendit page.
- [ ] Sanitized callback request/response log from the real sandbox.

## PostgreSQL and idempotency

- [x] Versioned migrations apply to PostgreSQL.
- [x] API keys use the `pk_test_` prefix and are persisted as hashes.
- [x] Callback receipts use provider/event uniqueness and payload matching.
- [x] Payment confirmation uses a PostgreSQL order lock, durable settlement
  intent, and transactional outbox.
- [x] PostgreSQL integration suite passed against disposable database
  `kailopay_codex_week1_test_20260909` on 2026-09-09.
- [x] Duplicate/concurrent callback and settlement tests passed with one
  durable settlement intent and one outbox message.

## Stellar testnet

- [x] Stellar testnet adapter and settlement worker are present.
- [x] Funded Stellar testnet treasury account: `GDGLOWQIRDAURHIXNJUDEON43AMMZWOWER3LDAVEIDDDFFVGV5CZSEE7`
  with `10000.0000000` native XLM observed from Horizon on 2026-09-09.
- [x] Owner-provided buy flow transaction proving settlement transfer: [Stellar Testnet Explorer](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e).

## Sell flow evidence

- [ ] Confirmed off-ramp retirement transaction hash and explorer link. The
  owner-provided [sell flow URL](https://stellar.expert/explorer/testnet/tx/21189560457244672#21189560457244673)
  contains numeric IDs rather than a 64-character Stellar transaction hash, so
  it does not verify the deposit or the backend retirement transaction.
- [ ] Capture the deposit hash, confirmed retirement hash, order reference, and
  payout-simulation disclosure together after deploying the on-chain retirement
  worker.

## Demo video links (owner-provided)

- [x] Buy flow demo: [YouTube](https://youtu.be/jvUxssZjSzM).
- [x] Sell flow demo: [YouTube](https://youtu.be/fBq_dOv1aGc).
- [x] Developer/user dashboard demo: [YouTube](https://youtu.be/CEVcd4mjYMk).

## Reproducible verification

```powershell
go run ./cmd/migrate
go test ./...
go test -race ./...
go vet ./...
go build -buildvcs=false ./...
go test ./openapi -run TestDocumentValidatesAuthContract -count=1
```

Recorded verification on 2026-09-09:

- `go test ./...`: PASS against the disposable PostgreSQL database.
- `go test -race -count=1 ./...`: PASS against the disposable PostgreSQL database.
- `go vet ./...`: PASS.
- `go build -buildvcs=false ./...`: PASS.
- `go test ./openapi -run TestDocumentValidatesAuthContract -count=1`: PASS.

Run repository integration tests only against the CI PostgreSQL service or a
disposable local database. Do not add secrets, private keys, full API keys,
raw sensitive provider payloads, or fabricated checkout URLs and transaction
hashes to this file.
