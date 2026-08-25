# ADR-003: Off-Ramp Retirement and Payout Model

- Status: accepted
- Date: 2026-08-24
- Decision owners: KailoPay project owner and backend developer
- Resolves: BE-002 (Stellar asset/retirement model) and BE-003 (off-ramp
  sandbox payout evidence fallback)

## Context

ADR-001 chose native XLM instead of issuing a custom asset, which removed the
usual retirement mechanisms available to issuers (lock issuer keys, clawback,
trustline removal). Week 2 must still demonstrate an off-ramp where a received
asset is verifiably retired before any withdrawal step begins, per
ORDER-STATE-MACHINES §3, and must produce honest payout evidence when the
payment gateway's sandbox offers no usable disbursement rail.

## Decision

1. **Retirement by burn-address transfer.** After an off-ramp deposit is
   verified (exact account, memo, amount match on Stellar testnet), the worker
   sends the deposited XLM from the deposit processing account to the standard
   unspendable burn address `GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWH4`.
   The transaction is recorded as a `stellar_transactions` row with purpose
   `retirement`; the order may only enter `withdrawal_processing` after that
   row is confirmed on Horizon.
2. **Worker-only deposit signing key.** The deposit processing account's
   secret key is loaded through the same dedicated loader pattern as the
   treasury secret (`LoadWorkerDepositSecret`). The API process never receives
   it.
3. **Simulated payout with explicit reference.** After confirmed retirement,
   the platform records a deterministic simulated payout —
   `reference_id = payout_<orderID>` in a new `offramp_payouts` table with
   state `completed` — and completes the order. Every public surface words
   this as "sandbox simulation; no IDR was transferred." No real bank details
   are accepted; the withdrawal destination is a synthetic token stored as
   jsonb.
4. **Deposits are matched exactly or rejected.** A deposit qualifies only when:
   the payment is a successful native-XLM payment *to* the configured deposit
   account, its memo equals `off<orderID>`, its stroop amount matches the
   order exactly, and its transaction hash is not already assigned to another
   order. Correlated-but-mismatched payments move the order to
   `asset_invalid` with a safe reason; uncorrelated payments are ignored.

## Consequences

- Retirement is irreversible on testnet and produces independent Horizon
  evidence (a second transaction hash distinct from the deposit hash).
- The deposit account is a hot account requiring a funded signer in the
  worker; losing its key strands orders in `retirement_processing` for manual
  recovery (consistent with existing unknown-outcome semantics).
- The simulated payout keeps the demo self-contained (no disbursement API
  activation) but must never be presented as real IDR movement — UI, API
  responses, and evidence records carry the disclosure.
- If a real sandbox disbursement rail is enabled later, only the payout job
  changes; the retirement gate stays.

## Alternatives considered

### Return-to-treasury instead of burn

Rejected: weaker retirement evidence (inventory return looks identical to any
treasury consolidation), mixes accounting, and provides no distinct on-chain
proof of retirement.

### Real Xendit disbursement in sandbox

Deferred: requires disbursement activation and introduces provider risk into
the retirement→payout chain before the core flow is proven. The simulated
path is the documented BE-003 fallback; the adapter seam allows upgrading.

### Issuing a KIDR asset to enable true burns

Rejected for this release: reintroduces issuer governance, trustlines, and
market-making concerns ADR-001 deliberately avoided.
