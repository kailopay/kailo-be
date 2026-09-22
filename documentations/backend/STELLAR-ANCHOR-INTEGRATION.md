# Stellar Anchor Integration

> The Week 1 settlement foundation sends native XLM from a pre-funded Stellar
> testnet distribution wallet after Xendit payment reconciliation. The Week 2
> slice adds off-ramp and the authenticated SEP-24/federation surfaces on top
> of the same order and settlement invariants.

## 1. Scope and network

All `v0.1.0` operations use **Stellar testnet**. The release demonstrates:

- A configured test asset and processing accounts.
- On-ramp issuance or distribution after confirmed sandbox payment.
- Off-ramp asset receipt, validation, and burn/retirement.
- Testnet transaction hashes linked to orders.
- Canonical SEP-10-authenticated SEP-24 deposit and withdrawal flows for classic
  Stellar wallets, including retail-session linking and Persona approval.
- SEP-38 indicative and wallet-owned firm quotes for the supported testnet pair.
- Clearly labelled Persona sandbox KYC verification flow; it is not a production
  compliance decision.
- Public `stellar.toml` and federation configuration/service.

Mainnet, production custody, reserves, liquidity, and regulated issuance are Future scope.

## 2. Account and asset roles

The Phase 0 ADR must define:

- Asset code and supported precision.
- Issuer account.
- Distribution/processing account.
- Off-ramp deposit account.
- Burn/retirement method: return to issuer, clawback when intentionally configured, or another testnet-safe method.
- Account trustline setup and authorization flags if used.

Preferred minimal model:

- Issuer secret is used only by the settlement worker and injected through managed secrets.
- Distribution/processing account submits routine transfers and receives off-ramp assets.
- Public account IDs may be documented; secret seeds are never stored in PostgreSQL, source, logs, screenshots, or evidence.

## 3. Stellar port

```ts
interface StellarSettlement {
  validateAccount(input: AccountInput): Promise<AccountValidation>;
  issueOrTransfer(input: IssueAssetInput): Promise<SubmissionResult>;
  findDeposit(input: DepositLookup): Promise<DepositResult>;
  retireAsset(input: RetireAssetInput): Promise<SubmissionResult>;
  findTransaction(input: TransactionLookup): Promise<TransactionResult>;
}
```

The adapter owns Horizon/RPC/SDK types, sequence handling, fee configuration, transaction construction, signing, submission, and network-result mapping.

## 4. On-ramp settlement

1. Verified payment moves order to `payment_confirmed`.
2. In the same database transaction, create a unique issuance intent and outbox message.
3. Worker validates destination and required trustline/account conditions.
4. Worker builds a transaction using the configured testnet network passphrase.
5. Include a deterministic order correlation value in a memo or documented alternative.
6. Sign only inside the settlement adapter using injected testnet secret material.
7. Submit and persist the result.
8. If successful, record transaction hash and move order to `completed`.
9. If timeout/unknown, query network history using hash/source/sequence/memo before rebuilding or resubmitting.

Exactly one active issuance intent per order is enforced in PostgreSQL.

## 5. Off-ramp deposit and retirement

Deposit instructions include:

- Processing account.
- Exact asset code and issuer.
- Exact amount.
- Required memo/order correlation or muxed-account mechanism.
- Expiry and testnet warning.

A deposit is accepted only when:

- Transaction is successful on testnet.
- Destination is the configured account.
- Asset code and issuer match.
- Amount matches the order's documented tolerance policy.
- Memo/muxed/correlation identifies the order.
- Transaction hash has not been assigned to another order.

After acceptance, persist a retirement intent. Burn/retirement must be confirmed on testnet before sandbox payout begins.

Unexpected/wrong deposits are not silently credited. Record safe evidence and follow the sandbox exception procedure.

## 5.1 Testnet payout simulator

Set `OFFRAMP_PAYOUT_MODE=simulated` to enable the worker-owned sandbox payout
adapter. It is accepted only with the Stellar testnet passphrase. After a
confirmed retirement, the worker leases a `payout.simulate_offramp` outbox job,
creates one deterministic `sandbox_bank_transfer` payout reference, records
`payout.simulated`, and advances the order to `completed`.

The simulator accepts any non-empty synthetic destination reference within the
existing 200-character limit. It does not validate, contact, or transfer to a
bank account, and every public payout/SEP-24 response says that no real IDR
moved. If the mode is `disabled`, the order remains safely in
`withdrawal_processing` with the payout intent pending; no false success is
returned.

## 6. Sequence numbers and concurrent submissions

Stellar source-account sequence numbers make concurrent submission risky. For `v0.1.0` use one of:

- A single serialized settlement queue per signing account, recommended for the sandbox.
- Strict account-level database/advisory lock around transaction build and submission.

Do not prebuild multiple transactions from a stale sequence. On `tx_bad_seq` or unknown outcome, reload account/network state and reconcile the prior intent before retry.

## 7. Retry and reconciliation

Classify outcomes:

- `confirmed`: successful ledger result and hash.
- `failed_permanent`: deterministic transaction failure requiring changed input/state.
- `retryable`: transient network/rate condition with no submitted effect.
- `unknown`: request may have reached the network; reconciliation required.

Reconciliation reads the network, compares source, sequence, hash, memo, asset, amount, and destination, then updates the existing intent. It never creates an unrelated replacement transaction without resolving the prior intent.

## 8. SEP-10 and SEP-24 wallet bridge

The sandbox supports classic Stellar accounts through SEP-10. Contract-account
authentication through SEP-45 is intentionally not advertised. `ANCHOR_BASE_URL`
must point to the public API origin; it is intentionally separate from
`AUTH_EMAIL_LINK_BASE_URL`, which points to the frontend.

The wallet first requests and signs a challenge, then exchanges it for the
short-lived bearer JWT. The JWT is the only authentication accepted by the
canonical wallet-owned SEP-24 routes:

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/auth?account=G...` | None | Create a SEP-10 challenge transaction |
| `POST` | `/auth` | None | Exchange a signed challenge for a bearer JWT |
| `POST` | `/sep24/transactions/deposit/interactive` | SEP-10 bearer JWT | Create a wallet-owned interactive deposit session |
| `POST` | `/sep24/transactions/withdraw/interactive` | SEP-10 bearer JWT | Create a wallet-owned interactive withdrawal session |
| `GET` | `/sep24/transactions?limit=...` | SEP-10 bearer JWT | List recent wallet-owned transaction mappings |
| `GET` | `/sep24/transaction?id=...` | SEP-10 bearer JWT | Read a wallet-owned mapping by protocol or external transaction ID |
| `GET`/`POST` | `/sep24/interactive/{id}` | Opaque browser token then KailoPay session | Link the wallet to a retail user, pass KYC, and complete the session |

Minimum behavior:

- Both initiation endpoints require `Idempotency-Key` and accept the standard
  `multipart/form-data` request shape as well as the JSON equivalent.
- Deposit requests support `asset_code=XLM`, `account`, `memo`, and the
  sandbox-specific `amount_minor` IDR input required by the quote workflow.
- Withdrawal requests support `asset_code=XLM`, the exact XLM `amount`, and an
  optional `quote_id`. The interactive page collects a non-empty synthetic
  `destination_token`; it never accepts or contacts a real bank account.
- `/sep24/info` labels deposit bounds as `idr_minor` and withdrawal bounds as
  `XLM`; the distinction is intentional because deposit quotes are fiat
  denominated in this sandbox.
- Initiation creates a durable interactive session before any order exists. The
  browser URL contains only a short-lived opaque token; the server stores its
  hash. The browser must establish a KailoPay retail session, link that user to
  the SEP-10 wallet, and pass the approved Persona KYC gate before the existing
  on-ramp/off-ramp use case creates an order.
- A successful order is correlated in `sep24_transactions` using a stable
  protocol transaction ID, wallet account, order ID, direction, quote ID, and
  Stellar/external transaction IDs. Ownership is checked against the SEP-10
  wallet; the linked retail session is required for completion.
- Transaction polling loads the current order state and ignores any
  client-supplied status. The interactive endpoint returns an escaped HTML
  sandbox page by default and a JSON projection when requested.
- Deposit initiation and transaction projections include the sandbox extension
  `payment_link_url` when the hosted payment checkout is available. Withdrawal
  projections do not include this field.
- A provider checkout timeout leaves the mapped transaction in
  `pending_external` until reconciliation; it is never presented as a fresh
  user-transfer state.

Legacy `/sep24/deposit` and `/sep24/withdraw` aliases remain for existing local
clients and continue to use the existing order-principal middleware. They do
not change the public `/v1/onramps` or `/v1/offramps` contracts.

## 8.1 SEP-38 quote server

The anchor also advertises a public quote server through the
`ANCHOR_QUOTE_SERVER` field in `stellar.toml`. It uses the same market source,
spread, freshness checks, and exact amount arithmetic as the first-party
`/v1/quotes` endpoint, but uses SEP-38 asset identifiers:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/sep38/info` | List `iso4217:IDR` and `stellar:native` plus delivery methods |
| `GET` | `/sep38/prices?sell_asset=...&buy_asset=...` | Return an indicative pair price |
| `GET` | `/sep38/price?...&sell_amount=...` or `buy_amount=...` | Calculate an amount-specific result |
| `POST` | `/sep38/quote` | Create a SEP-10 wallet-owned firm quote |
| `GET` | `/sep38/quote/{id}` | Retrieve an unexpired, unconsumed firm quote |

For `/sep38/price`, send exactly one of `sell_amount` or `buy_amount`. IDR is
represented as integer minor units and native XLM has up to seven decimal
places. These endpoints calculate prices only: they do not reserve liquidity,
create an order, or replace the SEP-24 transaction-initiation flow. Firm quotes
are separate, short-lived records owned by the SEP-10 wallet and can be
consumed once by a matching SEP-24 initiation.

Suggested mapping:

| KailoPay state | SEP-facing meaning |
|---|---|
| `created`, `payment_pending`, `asset_pending` | pending user transfer/action |
| `payment_confirmed`, `asset_received` | transfer received / processing |
| `stellar_processing`, `retirement_processing`, `withdrawal_processing` | in progress |
| `checkout_unknown` failure metadata | pending_external until reconciliation |
| `completed` | completed |
| Terminal failure/expiry | error/expired with safe message |

The implementation follows the transaction vocabulary in the [SEP-24
specification](https://github.com/stellar/stellar-protocol/blob/master/ecosystem/sep-0024.md)
and covers SEP-10 challenge exchange, wallet-owned history/lookup, interactive
linking, firm quote ownership, and simulator disclosure with unit and HTTP
tests. SEP-45 contract-account authentication remains future scope.

## 9. Persona sandbox KYC gate

KailoPay uses Persona's hosted/embedded sandbox inquiry flow as the identity
status gate for the anchor and developer paths. The backend creates or resumes
the inquiry, returns only the embedded-flow inputs, and accepts approval only
after a verified Persona callback.

- Configure the Persona inquiry template, environment, API key, and webhook
  secret through environment variables; see `PERSONA-KYC-RUNBOOK.md`.
- The browser opens Persona with the inquiry ID or short-lived resume session
  token and never receives the Persona API key or webhook secret.
- The backend stores normalized status, provider IDs, timestamps, and a body
  hash, not raw identity documents or raw provider payloads.
- Duplicate and out-of-order callbacks are safe; an approved inquiry cannot be
  downgraded by a stale event.
- API-key and order creation are rejected until local status is `approved`.
- This is a sandbox integration and does not provide production KYC/AML,
  sanctions screening, or regulatory compliance.

## 10. `stellar.toml`

The public file must:

- Be served from the expected `/.well-known/stellar.toml` location.
- Use HTTPS and valid CORS/content type where required.
- Advertise only implemented testnet endpoints and supported test asset.
- Include transfer server/SEP-24, quote-server/SEP-38, and federation URLs as
  applicable.
- Avoid mainnet or production claims.
- Validate with a documented tool/manual procedure before release.

## 11. Federation

Provide the smallest valid documented configuration/service required by the SOW. It may resolve a controlled demo address to a Stellar testnet account and memo.

Requirements:

- Public HTTPS endpoint and valid response format.
- Synthetic demo identities only.
- No use as a production identity directory.
- Tests for valid lookup, unknown name, invalid query, and cross-environment safety.

## 12. Key security

- Testnet secret keys are injected at runtime and scoped to the worker.
- API process should not hold signing keys unless the deployment cannot isolate worker configuration; document the compromise if so.
- Never expose secret seeds in exceptions, telemetry, process arguments, repository, or evidence.
- Use separate testnet accounts from any personal or production account.
- Rotate/re-fund demo accounts if exposure is suspected.
- Production HSM/custody and multi-approval signing are Future requirements.

## 13. Stellar acceptance evidence

- Public testnet account and asset configuration.
- Successful on-ramp transaction hash and explorer link.
- Successful off-ramp receipt and burn/retirement transaction hashes.
- Order-to-transaction correlation visible in safe logs/database evidence.
- Public valid `stellar.toml`.
- Public federation test.
- SEP-24 deposit and withdrawal discovery/interactive bridge, including
  authenticated order ownership, persisted transaction mapping, current-state
  polling, wallet linking, and Persona sandbox KYC-gate disclosure.
- SEP-38 firm quote ownership/expiry/consumption evidence and a testnet
  off-ramp simulator record that explicitly says no real IDR moved.
