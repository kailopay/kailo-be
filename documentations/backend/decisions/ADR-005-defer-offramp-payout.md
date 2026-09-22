# ADR-005: Defer the real off-ramp payout rail

- Status: Superseded for the testnet simulator path
- Date: 2026-09-18
- Superseded by: ADR-006 for explicit testnet simulator mode

## Context

The consumer off-ramp flow needs to accept an exact XLM deposit, verify it, and
retire the asset on Stellar testnet. The selected Xendit payout capability is
not available for a real disbursement, and the frontend must not display a
success message for an IDR transfer that did not happen. This ADR remains the
default policy when the simulator is disabled.

## Decision

1. Keep the quote, destination selection token, KYC gate, deposit instruction,
   exact amount/memo matching, and Stellar retirement flow.
2. Do not call a real provider payout adapter from the testnet flow.
3. When `OFFRAMP_PAYOUT_MODE=disabled`, leave the order in
   `withdrawal_processing` after retirement confirmation.
4. The testnet-only simulator and its disclosure are defined in ADR-006; this
   document still governs the disabled/default behavior.

## Consequences

- A payment with the wrong amount or memo is marked `asset_invalid`; the
  frontend can show a failed deposit state.
- An exact payment proceeds to retirement. Once retirement is confirmed with
  the simulator disabled, the order is a durable pending handoff rather than a
  successful fiat payout.
- The worker no longer polls or processes `stellar.pay_offramp` jobs.
- A future real payout adapter must add provider idempotency, reconciliation,
  and a durable success/failure transition before using real IDR.
