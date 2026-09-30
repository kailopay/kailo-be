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

Public route smoke checks on 2026-09-25 returned HTTP 200 for `/docs/`,
`/.well-known/stellar.toml`, `/federation?q=alice%2Akailopay`, `/sep24/info`, and
`/livez`. The live `stellar.toml` currently returns localhost URLs for SEP-24,
SEP-38, federation, and web authentication. Set
`ANCHOR_BASE_URL=https://api.kailopay.com` in production and redeploy the API.
A request to `/federation` without its required `q` parameter returns 404 by
design.

## SOW deliverables

| Deliverable | Current status | Evidence and remaining work |
|---|---|---|
| On/off-ramp API and payment integration | Implemented; Week 1 flows tested per project owner | The API, OpenAPI contract, and Xendit sandbox session are recorded in the repository. Link the QRIS and BRI Virtual Account checkout captures, the sanitized callback log, and the settlement hash to the [Week 1 evidence checklist](WEEK1-EVIDENCE.md). |
| Stellar testnet anchor | Backend retirement path implemented locally; live release and evidence remain partial | The owner-provided [buy transaction](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e) has a 64-character hash. The [sell URL](https://stellar.expert/explorer/testnet/tx/21189560457244672#21189560457244673) is not a verifiable transaction hash. Native XLM is sent to a retirement sink; this is not a protocol-level burn. The payout remains simulated and does not move real IDR. |
| Web app and demo materials | App is live; demo URLs provided by owner | The app URL is owner-confirmed and the SDK is published. Demo recordings: [buy](https://youtu.be/jvUxssZjSzM), [sell](https://youtu.be/fBq_dOv1aGc), and [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). Final user and reviewer guides remain open. |

## Implementation and tests

- The backend uses Go, Gin, GORM, and PostgreSQL. The repository README records
  Go as the accepted substitution for the SOW's Node.js/TypeScript backend.
  Link the formal approval record here if it is held outside the repository.
- The Xendit adapter creates sandbox Payment Sessions for QRIS and BRI Virtual
  Account checkout.
- The on-ramp worker transfers pre-funded native XLM on Stellar testnet.
- The off-ramp requires an exact XLM deposit, amount, and memo. The worker then
  confirms an exact-amount testnet retirement and records a deterministic
  sandbox payout reference. The owner provided the [sell flow transaction link](https://stellar.expert/explorer/testnet/tx/21189560457244672#21189560457244673).
  The public response discloses that no real IDR moved.
- On 2026-09-30, local `gofmt -l .`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...` passed on the current working tree.
- GitHub Actions must pass build, vet, unit race tests, PostgreSQL integration
  race tests, and the secret scan on the exact release commit before tagging.
- The project owner confirms that Week 1 sandbox flows were tested. The run
  records are not linked to this report yet.
- The live frontend is maintained in a separate repository. This report does
  not claim a test result for that repository.

## Evidence matrix

| Evidence | Status | Next action |
|---|---|---|
| Public web app URL | Present per project owner | Add this report to the final release package. |
| Public API and anchor route checks | `/docs/`, Stellar TOML, documented federation lookup, SEP-24 info, and `/livez` returned HTTP 200 on 2026-09-25; live TOML advertises localhost URLs | Set `ANCHOR_BASE_URL=https://api.kailopay.com` in production, redeploy, and verify the published URLs. |
| Public SDK package | Present per project owner | Link package acceptance checks for methods, retries, amount handling, webhook verification, and examples. |
| Week 1 checkout, callback, and settlement evidence | Week 1 flows were tested per owner; buy hash is linked; checkout and callback records are not | [Buy flow transaction](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e). Attach checkout captures and sanitized callback logs. |
| Formal buy and sell end-to-end records | Buy transaction and demo URLs are linked; sell transaction URL is not a valid hash | [Buy transaction](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e) · [buy demo](https://youtu.be/jvUxssZjSzM); [unverified sell URL](https://stellar.expert/explorer/testnet/tx/21189560457244672#21189560457244673) · [sell demo](https://youtu.be/fBq_dOv1aGc). [Developer/user dashboard demo](https://youtu.be/CEVcd4mjYMk). Attach a confirmed retirement hash, checkout/callback/webhook records, timestamps, and order references. |
| Real payout rail | Out of sandbox scope | Sell orders complete through the disclosed sandbox payout; no bank transfer or real IDR is claimed. Production disbursement remains a roadmap item. |
| Demo recording | Three video URLs provided by owner | [Buy](https://youtu.be/jvUxssZjSzM) · [sell](https://youtu.be/fBq_dOv1aGc) · [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). The links are recorded as owner-provided evidence; video contents have not been independently reviewed. |
| Final evidence matrix | Draft only | Confirm each evidence link and mark each deliverable Present, Partial, or Missing. |
| Final security and regression gate | Local format, vet, build, and race suite passed; GitHub Actions on the release commit is pending | Run and record CI build, vet, unit/PostgreSQL race tests, and secret scan before tagging. |
| `v0.1.0` release tag | Pending publication | Record the tag and GitHub release URL after publication. |

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
