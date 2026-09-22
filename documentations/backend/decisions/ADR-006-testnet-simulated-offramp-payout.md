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

1. Add a worker-owned payout simulator selected explicitly by
   `OFFRAMP_PAYOUT_MODE=simulated`.
2. Accept any non-empty synthetic destination reference within the existing
   200-character limit. Never validate or contact a bank account.
3. Enqueue `payout.simulate_offramp` only after confirmed Stellar retirement.
   Record one deterministic `sandbox_bank_transfer` payout row per order,
   append `payout.simulated`, update SEP-24 external transaction correlation,
   and move the order to `completed` in one idempotent transaction.
4. Require the Stellar testnet network. `disabled` remains safe and leaves the
   order in `withdrawal_processing`.
5. Every public order and SEP-24 payout projection carries a clear disclosure:
   no real IDR moved.

## Consequences

- The external wallet demo can reach a deterministic completed sandbox state,
  including a payout reference and retry-safe evidence.
- The completion is not a bank transfer, financial settlement, or production
  payout claim. UI and API clients must keep the disclosure visible.
- A future real adapter must replace only the payout worker boundary while
  preserving the retirement gate, idempotency, reconciliation, and explicit
  provider success/failure evidence.
