# ADR-006: Testnet-simulated off-ramp payout

- Status: Accepted
- Date: 2026-09-22
- Supersedes: the simulator prohibition in ADR-005

## Context

KailoPay needs a deterministic end-to-end off-ramp demonstration for an
external Stellar wallet, but the selected provider does not expose a real
sandbox bank disbursement rail. Marking an order as a successful fiat payout
would be misleading, while leaving every testnet order pending prevents an
automated wallet flow from proving its final state.

## Decision

1. The current release hard-codes simulated off-ramp settlement in the worker;
   there is no runtime payout-mode environment setting.
2. Continue requiring an exact, successful XLM deposit to the configured
   account with the order's expected amount and memo. No deposit means no
   progress, and mismatched deposits remain invalid.
3. After the deposit is verified, record the retirement intent as `simulated`
   without submitting a burn-address transfer or inventing a transaction hash
   or ledger time. Persist the state transition and enqueue
   `payout.simulate_offramp` atomically.
4. Record one deterministic `sandbox_bank_transfer` payout row per order,
   append `payout.simulated`, update SEP-24 external transaction correlation,
   and move the order to `completed` in one idempotent transaction.
5. Reconcile any retirement hash that was already persisted before simulation
   was deployed. Never replace an uncertain submitted operation with a
   simulated success. Recover exhausted retirement jobs only when the order is
   still processing and the retirement intent has no transaction hash.
6. Every public order and SEP-24 payout projection carries a clear disclosure.
   New simulated retirements state that XLM was not retired on-chain and no
   real IDR moved; previously submitted, confirmed retirement hashes are shown
   as on-chain testnet evidence while still disclosing that no real IDR moved.

## Consequences

- The external wallet demo can reach a deterministic completed sandbox state
  only after an exact on-chain XLM deposit, including a payout reference and
  retry-safe simulation evidence.
- For new orders, the retirement record is explicitly simulated and has no
  retirement hash or ledger timestamp; the deposit transaction remains
  independently verifiable. Historical confirmed retirements retain their
  actual hash and use a matching disclosure.
- The completion is not a bank transfer, financial settlement, or production
  payout claim. UI and API clients must keep the disclosure visible.
- This behavior is sandbox-only and must not be represented as on-chain XLM
  retirement or real fiat settlement. A future production flow needs a real
  asset-retirement operation and payout adapter with idempotency,
  reconciliation, and explicit provider success/failure evidence.
