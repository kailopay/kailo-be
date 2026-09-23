# ADR-005: Defer the real off-ramp payout rail

- Status: Superseded for the testnet simulator path
- Date: 2026-09-18
- Superseded by: ADR-006 for the current hard-coded testnet simulation path

## Context

The consumer off-ramp flow needs to accept and verify an exact XLM deposit. The
selected Xendit payout capability is not available for a real disbursement, and
the frontend must not display a success message for an IDR transfer that did
not happen. ADR-006 now defines the current testnet simulation path; this ADR
records the earlier deferred-payout decision only.

## Decision

1. Keep the quote, destination selection token, KYC gate, deposit instruction,
   and exact amount/memo matching.
2. Do not call a real provider payout adapter from the testnet flow.
3. The current testnet simulator, including its no-burn behavior and public
   disclosure, is defined by ADR-006; there is no disabled runtime mode.

## Consequences

- A payment with the wrong amount or memo is marked `asset_invalid`; the
  frontend can show a failed deposit state.
- An exact payment proceeds to the simulated retirement and payout path defined
  by ADR-006; it is not a successful on-chain retirement or fiat payout.
- The worker no longer polls or processes `stellar.pay_offramp` jobs.
- A future real payout adapter must add provider idempotency, reconciliation,
  and a durable success/failure transition before using real IDR.
