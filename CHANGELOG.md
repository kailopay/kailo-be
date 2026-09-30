# Changelog

## v0.1.0 — 2026-09-30

KailoPay sandbox/testnet release. This version is for demonstrations and
integration testing; it is not suitable for real funds.

### Included

- Xendit sandbox checkout sessions for QRIS and BRI Virtual Account.
- Stellar testnet XLM on-ramp and exact-deposit off-ramp retirement.
- SEP-10/24/38 anchor endpoints, public Stellar TOML, and federation lookup.
- Persona sandbox KYC, developer API access, and signed webhook delivery.
- Published TypeScript SDK at [@kailopay/sdk on npm](https://www.npmjs.com/package/@kailopay/sdk).
- Live web app at [kailopay.com](https://kailopay.com) and API at
  [api.kailopay.com](https://api.kailopay.com), as confirmed by the project owner.

### Release checks

- `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test -race ./...` passed.
- GitHub Actions build/vet/unit tests, PostgreSQL 18 integration tests, and
  secret scan passed on the release commit before publishing `v0.1.0`.
- Public GET checks returned HTTP 200 for the app, `/docs/`,
  `/.well-known/stellar.toml`, `/federation?q=alice%2Akailopay`, `/sep24/info`,
  and `/livez`.
- The project owner confirms the Week 1 sandbox flows were tested. The buy hash
  is linked in the evidence checklist; checkout captures, callback logs, and a
  verifiable sell retirement hash are not attached.

### Scope and evidence limitations

- Off-ramp sell processing verifies the XLM deposit and sends a confirmed
  native-XLM payment to a retirement sink. This is not a protocol-level burn;
  the payout remains simulated and does not move IDR.
- The live `stellar.toml` currently advertises localhost service URLs. Set
  `ANCHOR_BASE_URL=https://api.kailopay.com` in production and redeploy the API
  before external wallet discovery.
- The frontend is maintained in a separate repository; this release does not
  claim a frontend test result.
- Owner-provided demo URLs are recorded in the completion report. Checkout and
  callback artifacts, a verifiable sell retirement hash, and final SOW
  acceptance remain open.
