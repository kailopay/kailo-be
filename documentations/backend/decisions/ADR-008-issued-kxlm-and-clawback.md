# ADR-008: Issue and burn the KXLM sandbox asset

- Status: Accepted
- Date: 2026-09-30
- Supersedes: ADR-001's native-XLM asset choice and ADR-007's sink-payment behavior for new sell orders

## Context

The SOW evidence requires an issued token after a confirmed buy and a protocol
burn after a confirmed sell deposit. Native XLM cannot be issued or burned by
KailoPay. The existing payment, quote, sell-order, and payout workflows should
remain intact.

## Decision

1. Use the configured Stellar treasury account as the testnet issuer of the
   custom `KXLM` credit asset. Its existing worker-only secret signs issuance
   payments and issuer clawback operations. The worker must verify the signing
   key matches `STELLAR_TREASURY_ACCOUNT`.
2. After a sandbox payment is confirmed, issue the quoted amount of `KXLM` to
   the customer's trustline. The existing XLM/IDR market rate is a reference
   for sandbox quantity only; KXLM is not native XLM, is not redeemable for XLM,
   and has no claim of backing or real-world value.
3. Sell requests keep the existing route and JSON shape, with `asset.code`
   set to `KXLM`. The deposit account must hold a KXLM trustline. After the
   exact successful KXLM deposit is verified, the issuer clawbacks the amount
   from that account. The Stellar clawback operation destroys the issued
   balance; it does not send the asset to a sink.
4. The issuer must have `AUTH_REVOCABLE` and `AUTH_CLAWBACK_ENABLED` set before
   any relevant trustlines are created. Customer wallets and the deposit
   account must establish KXLM trustlines before they can receive KXLM.
5. Existing native-XLM orders and their transaction records are not rewritten.
   Legacy pending retirement intents continue to reconcile and use their
   recorded native-XLM sink transfer; new orders use KXLM issuance/clawback.
6. Fiat payout remains simulated in the sandbox. Public disclosures must not
   claim that real IDR moved.

## Consequences

- API request paths and JSON field structure stay stable, while asset code and
  `stellar.toml` identify the issued KXLM asset and issuer.
- Issuance and clawback require a funded issuer account for Stellar fees and
  pre-provisioned trustlines with clawback enabled.
- Testnet issuer setup is an explicit deployment prerequisite. Secrets remain
  worker-only and must not appear in source, database, logs, or evidence.
- Existing historical evidence remains valid for the transactions it
  describes, but only new KXLM flows demonstrate protocol-level issuance and
  burn.

## References

- https://developers.stellar.org/docs/tokens/how-to-issue-an-asset
- https://developers.stellar.org/docs/build/guides/transactions/clawbacks
