# KXLM Stellar testnet setup

This guide provisions the issued KXLM asset required by the backend on
Stellar testnet. KXLM is a sandbox demonstration asset. It is not native XLM,
is not redeemable for XLM, and has no real-world value. IDR payout remains
simulated.

## 1. Configure the issuer account

Use the public account configured as `STELLAR_TREASURY_ACCOUNT` as the KXLM
issuer. Its matching secret must be available only to the worker as
`STELLAR_TREASURY_SECRET`; never put the secret in source, the database, API
responses, logs, or submitted evidence.

Before creating KXLM trustlines, set the issuer's `AUTH_REVOCABLE` and
`AUTH_CLAWBACK_ENABLED` account flags. Stellar requires these flags before the
relevant trustlines are created. Enable only the account flags required for
this testnet demonstration.

## 2. Prepare destination trustlines

Each customer wallet that receives KXLM and the configured off-ramp deposit
account must create an authorized KXLM trustline referencing both:

- Asset code: `KXLM`
- Asset issuer: the configured `STELLAR_TREASURY_ACCOUNT`

Create these trustlines after the issuer flags are enabled. Confirm Horizon
reports the matching asset code and issuer, `is_authorized: true`, and
`is_clawback_enabled: true`. The backend checks these properties before it
creates a buy checkout or sell order.

## 3. Configure and migrate the backend

Set the issuer public account in the API and worker configuration. Keep the
treasury secret worker-only. Apply migrations `000017` and `000018` before
deploying the updated API and worker. The first migration records the issuer on
Stellar transaction intents; the second permits both historical `XLM` and new
`KXLM` financial records.

The worker needs enough native XLM on the issuer account to pay Stellar
transaction fees and maintain reserves. The amount of KXLM minted for a buy
does not consume native-XLM inventory.

## 4. Verify the flows

1. Create a sandbox buy for a wallet with the configured KXLM trustline.
2. Complete the sandbox payment and confirm a successful `KXLM` payment from
   the issuer to that wallet on Stellar testnet.
3. Create a sell order using the existing `/v1/offramps` request structure and
   `asset.code: "KXLM"`.
4. Send the exact KXLM amount to the configured deposit account with the
   returned memo.
5. Confirm the deposit is accepted and the issuer's clawback transaction
   destroys the exact amount from the deposit account.
6. Record both transaction hashes and the sandbox payout disclosure. A
   completed payout remains simulated and does not move real IDR.

Existing native-XLM orders and pending retirement intents are historical data.
The worker continues reconciling previously persisted native-XLM retirement
intents with the legacy deposit signer; these records are not converted to
KXLM.

## References

- [Stellar asset basics](https://developers.stellar.org/docs/learn/fundamentals/stellar-data-structures/assets)
- [Stellar clawbacks](https://developers.stellar.org/docs/build/guides/transactions/clawbacks)
- [SEP-38 asset identifiers](https://developers.stellar.org/docs/build/apps/wallet/sep38)
