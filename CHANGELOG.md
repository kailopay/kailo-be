# Changelog

## v0.1.0 — 2026-09-25

KailoPay sandbox/testnet release. This version is for demonstrations and
integration testing; it is not suitable for real funds.

### Included

- Xendit sandbox checkout sessions for QRIS and BRI Virtual Account.
- Stellar testnet XLM on-ramp and exact-deposit off-ramp verification.
- SEP-10/24/38 anchor endpoints, public Stellar TOML, and federation lookup.
- Persona sandbox KYC, developer API access, and signed webhook delivery.
- Published TypeScript SDK at [@kailopay/sdk on npm](https://www.npmjs.com/package/@kailopay/sdk).
- Live web app at [kailopay.com](https://kailopay.com) and API at
  [api.kailopay.com](https://api.kailopay.com), as confirmed by the project owner.

### Verification recorded for this release candidate

- `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test -race ./...` passed.
- PostgreSQL 18 repository integration tests passed with
  `go test ./internal/repository/ -count=1`.
- Public GET checks returned HTTP 200 for the app, `/docs/`,
  `/.well-known/stellar.toml`, `/federation?q=alice%2Akailopay`, `/sep24/info`,
  and `/livez`.
- The project owner confirms the Week 1 sandbox flows were tested. Individual
  checkout captures, callback logs, and transaction hashes are not attached to
  this repository.

### Scope and evidence limitations

- Off-ramp settlement is simulated. The flow verifies the XLM deposit, then
  records simulated retirement and payout; it does not burn XLM or move IDR.
- The frontend is maintained in a separate repository; this release does not
  claim a frontend test result.
- The demo recording, formal buy/sell evidence package, and written acceptance
  of the off-ramp scope deviation are not linked here. This release is not final
  SOW acceptance.
- Confirm the GitHub Actions secret scan on the release commit before creating
  the public `v0.1.0` tag.
