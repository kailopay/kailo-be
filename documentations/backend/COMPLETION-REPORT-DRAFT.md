# KailoPay completion report draft

> Status: Draft. This report records available project facts and owner
> confirmations. It is not a final acceptance report.

**Report date:** 2026-09-25

**Release target:** `v0.1.0` sandbox/testnet

## Summary

KailoPay has a public sandbox web app and backend API. The project owner also
confirms that the TypeScript SDK is published and that the Week 1 sandbox flows
were tested. The backend test suite passed on the current working tree.

The owner has directed preparation of the `v0.1.0` sandbox release. This report
keeps unlinked test records, the demo recording, written acceptance of the
simulated off-ramp, and final SOW acceptance visible as open items. The release
must not be read as proof that those items are complete.

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
`/livez`. A request to `/federation` without its required `q` parameter returns
404 by design.

## SOW deliverables

| Deliverable | Current status | Evidence and remaining work |
|---|---|---|
| On/off-ramp API and payment integration | Implemented; Week 1 flows tested per project owner | The API, OpenAPI contract, and Xendit sandbox session are recorded in the repository. Link the QRIS and BRI Virtual Account checkout captures, the sanitized callback log, and the settlement hash to the [Week 1 evidence checklist](WEEK1-EVIDENCE.md). |
| Stellar testnet anchor | Implemented with a documented off-ramp simulation | SEP-24, Persona sandbox KYC, `stellar.toml`, and federation routes are in the backend. Link the on-ramp and sell deposit transaction hashes, webhook logs, and public route checks. The current sell flow records simulated retirement and payout. It does not burn XLM or move IDR. Attach the Chapter Lead's acceptance of this SOW deviation if that approval exists. |
| Web app and demo materials | App is live; final demo materials are pending | The app URL is owner-confirmed. The SDK is published. Add the demo recording, final user and reviewer guides, and evidence links. |

## Implementation and tests

- The backend uses Go, Gin, GORM, and PostgreSQL. The repository README records
  Go as the accepted substitution for the SOW's Node.js/TypeScript backend.
  Link the formal approval record here if it is held outside the repository.
- The Xendit adapter creates sandbox Payment Sessions for QRIS and BRI Virtual
  Account checkout.
- The on-ramp worker transfers pre-funded native XLM on Stellar testnet.
- The off-ramp requires an exact XLM deposit, amount, and memo. The worker then
  records simulated retirement and a deterministic sandbox payout reference.
  The public response must disclose that no XLM was retired on-chain and no IDR
  moved.
- `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test -race ./...`
  passed on the release candidate on 2026-09-25.
- The repository integration suite passed with a temporary PostgreSQL 18
  database using `go test ./internal/repository/ -count=1`.
- The local CI-equivalent checks do not replace the GitHub secret scan. Confirm
  the `secret scan` job on the release commit before tagging.
- The project owner confirms that Week 1 sandbox flows were tested. The run
  records are not linked to this report yet.
- The live frontend is maintained in a separate repository. This report does
  not claim a test result for that repository.

## Evidence matrix

| Evidence | Status | Next action |
|---|---|---|
| Public web app URL | Present per project owner | Add this report to the final release package. |
| Public API and anchor route checks | Owner-confirmed; `/docs/`, Stellar TOML, documented federation lookup, SEP-24 info, and `/livez` returned HTTP 200 on 2026-09-25 | Preserve response captures if the reviewer requires raw artifacts. |
| Public SDK package | Present per project owner | Link package acceptance checks for methods, retries, amount handling, webhook verification, and examples. |
| Week 1 checkout, callback, and settlement evidence | Tested per project owner; individual records not linked | Attach checkout captures, sanitized callback logs, and the testnet transaction hash. |
| Formal buy and sell end-to-end records | Not linked in this repository | Add timestamps, inputs, expected and actual results, order references, logs, and transaction hashes. Link existing records if they are stored elsewhere. |
| Off-ramp simulation approval | Not linked in this repository | Attach the written scope acceptance or record the approver and decision. |
| Demo recording | Not found in the inspected repositories | Record and link a buy and sell walkthrough with sandbox disclosures. |
| Final evidence matrix | Draft only | Confirm each evidence link and mark each deliverable Present, Partial, or Missing. |
| Final security and regression gate | Local format, vet, build, race, and PostgreSQL integration checks passed; secret scan is pending | Confirm the GitHub Actions secret scan against the release commit. |
| `v0.1.0` release tag | Pending | Record the tag and GitHub release URL after publication. |

## Week 4 closeout

The release notes, README, and verification results are recorded. Formal SOW
acceptance remains partial until the E2E artifacts, demo, and off-ramp scope
acceptance are attached. The release notes state the sandbox limitations.

## Source records

- [Week 1 evidence checklist](WEEK1-EVIDENCE.md)
- [Weeks 1 and 2 implementation checkpoint](WEEK1-2-CHECKPOINT.md)
- [Backend backlog](BACKEND-BACKLOG.md)
- [Testnet-simulated off-ramp decision](decisions/ADR-006-testnet-simulated-offramp-payout.md)
- [OpenAPI specification](../../openapi/openapi.yaml)
