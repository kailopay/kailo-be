# ADR-007: On-chain off-ramp XLM retirement

> Historical decision. [ADR-008](ADR-008-issued-kxlm-and-clawback.md)
> supersedes sink payments for new sell orders. Existing native-XLM intents
> continue to use the behavior documented here.

- Status: Accepted
- Date: 2026-09-29
- Supersedes: ADR-006 retirement simulation only; ADR-006's simulated payout remains active

## Context

The current sell flow verifies an exact native-XLM testnet deposit, then records
retirement as simulated. SOW evidence calls for Stellar transaction hashes,
and sell-side evidence benefits from a distinct transaction after deposit
verification. The API request, deposit instructions, and payout rail can remain
unchanged.

## Decision

1. After the worker verifies a successful deposit's account, memo, and exact
   stroop amount, it submits a native-XLM payment for that amount from the
   configured shared deposit account to
   the canonical all-zero public-key sink
   `GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF`. KailoPay
   holds no signing key for this destination; the implementation submits a
   payment transaction rather than invoking a protocol-level burn operation.
2. `OFFRAMP_DEPOSIT_SECRET` is worker-only and must derive the configured
   `OFFRAMP_DEPOSIT_ACCOUNT`. Worker startup fails on mismatch.
3. Persist the retirement transaction hash before submission using a
   compare-and-set from the pending state. If another worker already persisted
   a hash, reconcile that hash without submitting the local transaction. Clear
   a definitively rejected sequence-conflict hash only if it is still the
   submitted hash; unknown or pending outcomes retain and reconcile it.
4. Move the order to `withdrawal_processing` and enqueue the payout job only
   after Horizon confirms the retirement. The existing deterministic payout
   simulator then completes the order.
5. Public evidence describes confirmed XLM retirement on Stellar testnet and
   continues to state that no real IDR moved. Existing simulated or confirmed
   records keep their recorded evidence; the worker does not backfill burns for
   completed historical orders.
6. Keep the current sell request fields, deposit account, memo, and order
   creation flow unchanged.

## Consequences

- New accepted sell deposits produce a separate confirmed Stellar hash for
  retirement, subject to the worker having enough XLM for the payment, account
  reserve, and transaction fee.
- The testnet payment to the all-zero public-key sink is irreversible for
  KailoPay. Horizon currently reports that testnet account as existing, with
  one master signer of weight 1; the project does not hold its signing key.
- The sandbox payout remains synthetic. No bank account is contacted and no
  real IDR moves.
- Deployment must provision the deposit signer in the worker and never expose
  or log its secret.

## Alternatives considered

- Keep retirement simulated and use only the customer's deposit hash. This
  produces no backend-initiated sell-side transfer evidence.
- Issue a custom Stellar asset and burn it at the issuer. This conflicts with
  ADR-001's native-XLM choice and adds trustline and issuer governance work.
