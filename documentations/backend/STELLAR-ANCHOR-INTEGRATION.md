# Stellar Anchor Integration

## 1. Scope and network

All `v0.1.0` operations use **Stellar testnet**. The release demonstrates:

- A configured test asset and processing accounts.
- On-ramp issuance or distribution after confirmed sandbox payment.
- Off-ramp asset receipt, validation, and burn/retirement.
- Testnet transaction hashes linked to orders.
- SEP-24 deposit and withdrawal skeleton.
- Clearly labelled KYC stub.
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

## 8. SEP-24 skeleton

Minimum components:

- Anchor discovery points clients to the transfer server and interactive endpoints.
- Deposit initiation creates or maps to a KailoPay on-ramp order.
- Withdrawal initiation creates or maps to an off-ramp order and returns deposit instructions.
- Interactive pages display the KYC stub, sandbox status, and order state.
- Transaction-status mapping remains consistent with internal states.

Suggested mapping:

| KailoPay state | SEP-facing meaning |
|---|---|
| `created`, `payment_pending`, `asset_pending` | pending user transfer/action |
| `payment_confirmed`, `asset_received` | transfer received / processing |
| `stellar_processing`, `retirement_processing`, `withdrawal_processing` | in progress |
| `completed` | completed |
| Terminal failure/expiry | error/expired with safe message |

The implementation must verify the exact SEP-24 field/status vocabulary against the selected Stellar specification during development and cover it with contract tests.

## 9. KYC stub

The KYC screen exists only to demonstrate the interactive flow:

- Use synthetic fields/data.
- Display “KYC simulation - no identity verification performed.”
- Do not upload or retain real identity documents.
- Do not use the result for a production compliance decision.
- Persist only a stub status/reference needed for the demo.

## 10. `stellar.toml`

The public file must:

- Be served from the expected `/.well-known/stellar.toml` location.
- Use HTTPS and valid CORS/content type where required.
- Advertise only implemented testnet endpoints and supported test asset.
- Include transfer server/SEP-24 and federation URLs as applicable.
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
- SEP-24 deposit and withdrawal demo with KYC-stub warning.
