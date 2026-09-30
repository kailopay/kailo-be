# KailoPay completion report draft

> Status: Draft. This report records available project facts and owner
> confirmations. It is not a final acceptance report.

**Report date:** 2026-09-30

**Release target:** `v0.1.0` sandbox/testnet

## Summary

KailoPay has a public sandbox web app and backend API. The project owner also
confirms that the TypeScript SDK is published and that the Week 1 sandbox flows
were tested. The backend test suite passed on the current working tree. Within
the selected native-XLM testnet scope, the buy settlement and sell deposit plus
on-chain retirement are implemented and verified. A simulated fiat-payout
record is the expected sandbox outcome; no real IDR payout is expected or
claimed on Stellar testnet.

The owner has directed preparation of the `v0.1.0` sandbox release. The owner
provided buy and sell transaction links and three demo video URLs. Checkout
captures and sanitized callback logs have not been attached to this report, so
the evidence package and formal SOW acceptance remain open. The remaining
evidence items are documentation and acceptance work, not missing testnet
payout functionality.

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
| Stellar testnet anchor | Implemented for the selected native-XLM testnet scope; buy settlement and sell retirement verified | Horizon confirms the [buy transfer](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065), the [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a), and the matching [35 XLM retirement transfer](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e). The chosen design distributes treasury-funded native XLM and retires off-ramp XLM through the verified sink transfer. Native XLM has no issuer, so custom-token issuance and issuer-authorized burn are outside this chosen design. The simulated fiat-payout record is expected testnet behavior; it does not represent or claim a real IDR transfer. |
| Web app and demo materials | App is live; demo URLs provided by owner | The app URL is owner-confirmed and the SDK is published. Demo recordings: [buy](https://youtu.be/jvUxssZjSzM), [sell](https://youtu.be/fBq_dOv1aGc), and [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). Final user and reviewer guides remain open. |

## Implementation and tests

- The backend uses Go, Gin, GORM, and PostgreSQL. The repository README records
  Go as the accepted substitution for the SOW's Node.js/TypeScript backend.
  Link the formal approval record here if it is held outside the repository.
- The Xendit adapter creates sandbox Payment Sessions for QRIS and BRI Virtual
  Account checkout.
- The on-ramp worker transfers pre-funded native XLM on Stellar testnet.
- The off-ramp requires an exact XLM deposit, amount, and memo. The live testnet
  record shows the [35 XLM deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a), followed five seconds later by the exact-amount [retirement transfer](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e) with the same memo. The workflow then records its sandbox payout result. This is the expected testnet behavior: no bank is contacted and no real IDR moves.
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
| Week 1 checkout, callback, and settlement evidence | Owner confirms the sandbox flows were tested; buy and sell Stellar transfers are verified; checkout captures and callback records are not attached here | [Buy settlement](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) · [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a) · [sell retirement](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e). Add checkout captures and sanitized Xendit callback logs to the acceptance package if required. |
| Formal buy and sell end-to-end records | On-chain hashes are verified; webhook and order-correlation artifacts are not attached here | [Buy transaction](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) · [sell deposit](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a) · [sell retirement](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e) · [buy demo](https://youtu.be/jvUxssZjSzM) · [sell demo](https://youtu.be/fBq_dOv1aGc) · [dashboard demo](https://youtu.be/CEVcd4mjYMk). Attach sanitized webhook logs and the authenticated order reference if the reviewer requires those records. |
| Fiat payout rail | No real fiat payout is expected in testnet scope | The sandbox records a payout outcome to complete the test flow; it does not contact a bank or move real IDR. A production payout integration is a separate future scope. |
| Demo recording | Three video URLs provided by owner | [Buy](https://youtu.be/jvUxssZjSzM) · [sell](https://youtu.be/fBq_dOv1aGc) · [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). The links are recorded as owner-provided evidence; video contents have not been independently reviewed. |
| Final evidence matrix | Draft only | Confirm each evidence link and mark each deliverable Present, Partial, or Missing. |
| Final security and regression gate | Local format, vet, build, and race suite passed; GitHub Actions build/vet/unit tests, PostgreSQL integration tests, and secret scan passed before tagging | See the [GitHub Actions runs](https://github.com/kailopay/kailo-be/actions) for the release commit. |
| `v0.1.0` release tag | Published | [Tag `v0.1.0`](https://github.com/kailopay/kailo-be/tree/v0.1.0) points to the CI-verified release commit `ecce60a`. |

## Week 4 closeout

The `v0.1.0` sandbox software scope is implemented, and the native-XLM buy and
sell retirement transfers are verified on testnet. A real IDR payout is not
part of this testnet deliverable; the sandbox payout record is expected
behavior. The release notes state these environment limits. To close the
acceptance package, attach checkout and callback artifacts if required and
record that the accepted anchor interpretation is treasury-funded native XLM
with sink retirement, rather than custom-token issuance and issuer-burn.

## Source records

- [Week 1 evidence checklist](WEEK1-EVIDENCE.md)
- [Weeks 1 and 2 implementation checkpoint](WEEK1-2-CHECKPOINT.md)
- [Backend backlog](BACKEND-BACKLOG.md)
- [On-chain off-ramp retirement decision](decisions/ADR-007-onchain-offramp-retirement.md)
- [OpenAPI specification](../../openapi/openapi.yaml)
