# KailoPay completion report draft

> Historical evidence in this draft records earlier native-XLM testnet flows.
> It does not prove KXLM issuance or protocol burn implemented by ADR-008.
> Submit fresh testnet KXLM buy and sell transaction evidence after issuer
> flags and trustlines have been provisioned.

> Status: Reviewer-aligned draft. It records the reviewer verdict and available
> evidence; it is not a final sponsor acceptance decision.

**Report date:** 2026-09-30

**Release target:** `v0.1.0` sandbox/testnet

## Summary

KailoPay has a public sandbox web app and backend API. The project owner also
confirms that the TypeScript SDK is published and that the Week 1 sandbox flows
were tested. Reviewer feedback approves Deliverable 1 with a note on the Go
backend stack, marks Deliverable 2 Partial, and approves Deliverable 3. The
overall SOW verdict is Partial because the selected native-XLM design does not
implement literal custom-token issuance and issuer-burn. The sandbox buy,
sell-deposit, and retirement flows are demonstrated on testnet; simulated fiat
payout is expected in this environment and no real IDR payout is claimed.

The owner has directed preparation of the `v0.1.0` sandbox release. The owner
provided buy and sell transaction links and three demo video URLs. The reviewer
accepts Deliverable 3 based on the live app, docs, demos, and frontend repository.
The D2 verdict remains Partial under literal SOW wording; use the corrected
transaction map below so the deposit, retirement, and buy hashes are not
confused.

## Reviewer verdict

| SOW item | Verdict from reviewer feedback | Basis |
|---|---|---|
| Deliverable 1: API and payment integration | Approve / Complete, with note | Go is the backend stack substitution; the feedback treats additional evidence as optional. |
| Deliverable 2: Stellar anchor | Partial | The native-XLM testnet flow runs, but there is no custom project-token issuance or issuer-authorized burn. A sink transfer is retirement evidence, not protocol-level burn. |
| Deliverable 3: Web app and demo | Approve / Complete | Feedback confirms `kailopay.com` and references L13-L15 for the app materials plus L08 for the canonical `kailopay/kailo-web` repository. The separate `kailo-fe-v1` URL returned 404 in that review. |
| Overall | Partial | The reviewer recommends approval with a D2 caveat; if the chapter requires literal issue/burn wording, retain the Partial verdict for D2 and overall. |

## Public locations

The project owner confirmed these URLs on 2026-09-25:

| Item | URL | Status |
|---|---|---|
| Web app | [https://kailopay.com](https://kailopay.com) | Owner-confirmed live |
| Backend API | [https://api.kailopay.com](https://api.kailopay.com) | Owner-confirmed live |
| Web frontend source | [kailopay/kailo-web](https://github.com/kailopay/kailo-web) | Reviewer evidence L08 identifies this as the available canonical repository; the separate `kailo-fe-v1` URL returned 404 in that review. |
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
| On/off-ramp API and payment integration | Approve / Complete per reviewer feedback | Go is accepted as the backend stack substitution. The API, OpenAPI contract, and Xendit sandbox session are recorded in the repository; the [buy on-ramp transaction](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) is included below. |
| Stellar testnet anchor | Partial per reviewer feedback | The sandbox uses native XLM, not a project-issued token. The buy settlement, sell deposits, and separate retirement hash are mapped below. Native XLM has no issuer; therefore literal custom-token issuance and issuer-authorized burn are not met. |
| Web app and demo materials | Approve / Complete per reviewer feedback | The feedback approves the live app, docs, demos, and frontend source. Demo recordings: [buy](https://youtu.be/jvUxssZjSzM), [sell](https://youtu.be/fBq_dOv1aGc), and [developer/user dashboard](https://youtu.be/CEVcd4mjYMk). |

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
  not claim a test result for that repository; the D3 approval is recorded from
  the reviewer feedback, which identifies `kailopay/kailo-web` as the canonical
  frontend repository.

## Corrected transaction evidence index

| Evidence reference | Transaction type and direction | Testnet transaction |
|---|---|---|
| L11 | Sell deposit: customer wallet to the off-ramp deposit account, `30.0000000` native XLM. This is not buy evidence; the separate buy is [b4106f0c…](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065). | [45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e](https://stellar.expert/explorer/testnet/tx/45c1d2d17d33359d67afce199039f2b216bd4767ce06c0427c794cc344801b3e) |
| L12 | Sell deposit: customer wallet to the off-ramp deposit account, `35.0000000` native XLM. | [5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a](https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a) |
| Retirement after L12 (separate transaction) | Off-ramp deposit account to the retirement sink, exactly `35.0000000` native XLM, five seconds after L12. This is a distinct hash, not the L12 deposit hash. | [1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e](https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e) |
| Buy on-ramp settlement | Treasury to customer wallet, `24.1411618` native XLM. | [b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065](https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065) |

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
| Final evidence matrix | Updated to match reviewer verdicts: D1 Approve, D2 Partial, D3 Approve; overall Partial | Keep L11/L12 classified as sell deposits, link the separate retirement hash, and use the buy on-ramp hash above for buy evidence. |
| Final security and regression gate | Local format, vet, build, and race suite passed; GitHub Actions build/vet/unit tests, PostgreSQL integration tests, and secret scan passed before tagging | See the [GitHub Actions runs](https://github.com/kailopay/kailo-be/actions) for the release commit. |
| `v0.1.0` release tag | Published | [Tag `v0.1.0`](https://github.com/kailopay/kailo-be/tree/v0.1.0) points to the CI-verified release commit `ecce60a`. |

## Week 4 closeout

The reviewer approves D1 and D3 and marks D2 Partial, making the overall SOW
verdict Partial when literal anchor wording is enforced. Evidence now labels
L11 and L12 as sell deposits, gives the separate retirement hash, and includes
the buy on-ramp hash. The sandbox uses native XLM and does not issue a project
token or perform issuer-burn; a real IDR payout is outside testnet scope.

## Source records

- [Week 1 evidence checklist](WEEK1-EVIDENCE.md)
- [Weeks 1 and 2 implementation checkpoint](WEEK1-2-CHECKPOINT.md)
- [Backend backlog](BACKEND-BACKLOG.md)
- [On-chain off-ramp retirement decision](decisions/ADR-007-onchain-offramp-retirement.md)
- [OpenAPI specification](../../openapi/openapi.yaml)
