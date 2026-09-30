# Week 1 Evidence Checklist

This checklist maps the backend implementation to the Week 1 SOW. Code and
automated test results are kept separate from evidence that requires a real
sandbox or testnet account.

## Checkpoint status

Weeks 1 and 2 are closed as an implementation checkpoint dated 2026-09-16.
The unchecked items are supporting evidence that has not been attached to this
checklist; they do not mean the owner-confirmed testnet flows are unimplemented.
Add those artifacts to the acceptance package if the reviewer requires them.
The simulated fiat-payout record is expected in this testnet scope, where no
real IDR payout occurs.

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
- [x] Verified buy on-ramp settlement: [Stellar Testnet transaction](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065). Horizon confirms a successful `24.1411618` native-XLM payment from the recorded treasury to the customer wallet on 2026-09-29. This is the buy hash; L11 and L12 below are sell deposits.

## Sell flow evidence

- [x] **L11 — sell deposit, not buy:** [Stellar Testnet transaction](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e). This is a `30.0000000` native-XLM payment from the customer wallet to the off-ramp deposit account; its memo starts with `off`. The separate buy on-ramp hash is [b4106f0c…](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065).
- [x] **L12 — sell deposit:** [Stellar Testnet transaction](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a). Horizon confirms a successful `35.0000000` native-XLM payment from the customer wallet to the off-ramp deposit account on 2026-09-30.
- [x] **Separate retirement after L12 — not the L12 transaction:** [Stellar Testnet transaction](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e). Five seconds after the L12 deposit, the deposit account sent exactly `35.0000000` native XLM to the retirement sink with the same memo. This retirement hash is separate from the L12 deposit hash.
- [ ] Attach the authenticated order reference and sanitized webhook delivery record if required for the acceptance package. The public links verify the testnet transfers; the sandbox payout record is expected behavior and no real IDR payout is part of this testnet flow.

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
