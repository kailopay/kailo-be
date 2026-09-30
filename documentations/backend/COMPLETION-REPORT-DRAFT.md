# KailoPay completion report draft

> Status: Draft. This report records available project facts and owner
> confirmations. It is not a final acceptance report.

**Report date:** 2026-09-30

**Release target:** `v0.1.0` sandbox/testnet

## Summary

KailoPay has a public sandbox web app and backend API. The project owner also
confirms that the TypeScript SDK is published and that the Week 1 sandbox flows
were tested. The backend test suite passed on the current working tree.

The owner has directed preparation of the `v0.1.0` sandbox release. The owner
provided buy and sell transaction links and three demo video URLs. Checkout
captures, sanitized callback logs, and final SOW acceptance remain open; this
report does not claim final acceptance.

## Public locations

The project owner confirmed these URLs on 2026-09-25:

| Item | URL | Status |
|---|---|---|
| Web app | [https://kailopay.com](https://kailopay.com) | Owner-confirmed live |
| Backend API | [https://api.kailopay.com](https://api.kailopay.com) | Owner-confirmed live |
| TypeScript SDK | [@kailopay/sdk on npm](https://www.npmjs.com/package/@kailopay/sdk) | Owner-confirmed published |
| Source repository | [kailopay/kailo-be](https://github.com/kailopay/kailo-be) | Week 1 checklist records it reachable on 2026-09-09 |

Public route checks on 2026-09-30 returned HTTP 200 for the web app, `/docs/`,
`/.well-known/stellar.toml`, `/federation?q=alice%2Akailopay`, `/sep24/info`, and
`/livez`. The live `stellar.toml` now advertises public HTTPS URLs at
`api.kailopay.com` for SEP-24, SEP-38, federation, and web authentication. A
request to `/federation` without its required `q` parameter returns 404 by
design.

## SOW deliverables

| Deliverable | Current status | Evidence and remaining work |
|---|---|---|
| On/off-ramp API and payment integration | Implemented; Week 1 flows tested per project owner | The API, OpenAPI contract, and Xendit sandbox session are recorded in the repository. The [verified on-ramp settlement](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) is linked in the [Week 1 evidence checklist](WEEK1-EVIDENCE.md); QRIS/BRI checkout captures and sanitized callback logs remain to attach. |
| Stellar testnet anchor | Live native-XLM retirement verified; SOW wording and evidence remain partial | Horizon confirms the [buy transfer](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065), the [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a), and the matching [35 XLM retirement transfer](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e). Native XLM goes to a retirement sink, not an issuer-burn operation; the payout remains simulated and moves no real IDR. |
| Web app and demo materials | App is live; demo URLs provided by owner | The app URL is owner-confirmed and the SDK is published. Demo recordings: [buy](https://youtu.be/jvUxssZjSzM), [sell](https://youtu.be/fBq_dOv1aGc), and [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). Final user and reviewer guides remain open. |

## Implementation and tests

- The backend uses Go, Gin, GORM, and PostgreSQL. The repository README records
  Go as the accepted substitution for the SOW's Node.js/TypeScript backend.
  Link the formal approval record here if it is held outside the repository.
- The Xendit adapter creates sandbox Payment Sessions for QRIS and BRI Virtual
  Account checkout.
- The on-ramp worker transfers pre-funded native XLM on Stellar testnet.
- The off-ramp requires an exact XLM deposit, amount, and memo. The live testnet
  record shows the [35 XLM deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a), followed five seconds later by the exact-amount [retirement transfer](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e) with the same memo. The payout remains simulated; the public response discloses that no real IDR moved.
- On 2026-09-30, local `gofmt -l .`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...` passed on the current working tree.
- GitHub Actions build/vet/unit tests, PostgreSQL integration tests, and secret
  scan passed on the release commit before tagging.
- The project owner confirms that Week 1 sandbox flows were tested. The run
  records are not linked to this report yet.
- The live frontend is maintained in a separate repository. This report does
  not claim a test result for that repository.

## Evidence matrix

| Evidence | Status | Next action |
|---|---|---|
| Public web app URL | Present per project owner | Add this report to the final release package. |
| Public API and anchor route checks | On 2026-09-30, the app, `/docs/`, federation lookup, SEP-24 info, `/livez`, and Stellar TOML returned HTTP 200; TOML now advertises HTTPS URLs at `api.kailopay.com` | Public discovery URLs are verified. Capture responses in the release evidence package if raw artifacts are required. |
| Public SDK package | Present per project owner | Link package acceptance checks for methods, retries, amount handling, webhook verification, and examples. |
| Week 1 checkout, callback, and settlement evidence | Buy and sell Stellar transfers are now verified; checkout captures and callback records remain missing | [Buy settlement](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) · [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a) · [sell retirement](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e). Attach checkout captures and sanitized Xendit callback logs. |
| Formal buy and sell end-to-end records | Buy, sell deposit, and retirement hashes verified; webhook and order-correlation records remain incomplete | [Buy transaction](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) · [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a) · [sell retirement](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e) · [buy demo](https://youtu.be/jvUxssZjSzM) · [sell demo](https://youtu.be/fBq_dOv1aGc) · [dashboard demo](https://youtu.be/CEVcd4mjYMk). Attach sanitized webhook logs and the authenticated order reference. |
| Real payout rail | Out of sandbox scope | Sell orders complete through the disclosed sandbox payout; no bank transfer or real IDR is claimed. Production disbursement remains a roadmap item. |
| Demo recording | Three video URLs provided by owner | [Buy](https://youtu.be/jvUxssZjSzM) · [sell](https://youtu.be/fBq_dOv1aGc) · [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). The links are recorded as owner-provided evidence; video contents have not been independently reviewed. |
| Final evidence matrix | Draft only | Confirm each evidence link and mark each deliverable Present, Partial, or Missing. |
| Final security and regression gate | Local format, vet, build, and race suite passed; GitHub Actions build/vet/unit tests, PostgreSQL integration tests, and secret scan passed before tagging | See the [GitHub Actions runs](https://github.com/kailopay/kailo-be/actions) for the release commit. |
| `v0.1.0` release tag | Published | [Tag `v0.1.0`](https://github.com/kailopay/kailo-be/tree/v0.1.0) points to the CI-verified release commit `ecce60a`. |

## Week 4 closeout

The release notes, README, verification results, and owner-provided demo URLs
are recorded. Formal SOW acceptance remains partial until the webhook and
checkout artifacts and remaining SOW evidence are attached. The release notes
state the sandbox limitations.

## Source records

- [Week 1 evidence checklist](WEEK1-EVIDENCE.md)
- [Weeks 1 and 2 implementation checkpoint](WEEK1-2-CHECKPOINT.md)
- [Backend backlog](BACKEND-BACKLOG.md)
- [On-chain off-ramp retirement decision](decisions/ADR-007-onchain-offramp-retirement.md)
- [OpenAPI specification](../../openapi/openapi.yaml)
