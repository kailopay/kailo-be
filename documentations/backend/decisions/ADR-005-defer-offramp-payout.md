# ADR-005: Defer the off-ramp payout rail

- Status: Accepted
- Date: 2026-09-18
- Supersedes: the payout-completion part of ADR-003 for the current release

## Context

The consumer off-ramp flow needs to accept an exact XLM deposit, verify it,
and retire the asset on Stellar testnet. The selected Xendit payout capability
is not available for this release, and the frontend must not display a success
message for an IDR transfer that did not happen.

## Decision

1. Keep the quote, destination selection token, KYC gate, deposit instruction,
   exact amount/memo matching, and Stellar retirement flow.
2. Do not enqueue `stellar.pay_offramp` after confirmed retirement.
3. Leave the order in `withdrawal_processing` after retirement confirmation.
   The API omits `payout` and does not return a completed/success response for
   this step.
4. Keep the payout schema and historical payout records readable for
   compatibility, but do not create new simulated payout records.

## Consequences

- A payment with the wrong amount or memo is marked `asset_invalid`; the
  frontend can show a failed deposit state.
- An exact payment proceeds to retirement. Once retirement is confirmed, the
  order is a durable pending handoff rather than a successful fiat payout.
- The worker no longer polls or processes `stellar.pay_offramp` jobs.
- A future payout adapter must add provider idempotency, reconciliation, and a
  durable success/failure transition before changing this state behavior.
