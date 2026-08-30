# ADR-001: Xendit and Native XLM for the First On-Ramp

- Status: accepted
- Date: 2026-08-19
- Decision owners: KailoPay project owner and backend developer

## Context

KailoPay needs one production-shaped sandbox on-ramp that accepts an IDR
payment, proves the payment exactly once, and transfers value over Stellar
testnet. The original documentation left both the Indonesian payment provider
and Stellar asset model unresolved.

The first release must remain explicit that Xendit test payments and Stellar
testnet XLM have no real monetary settlement relationship. CoinMarketCap is a
reference price source, not an exchange or source of executable liquidity.

## Decision

1. Use Xendit Payment Sessions in test mode with `mode=PAYMENT_LINK`.
2. Use the hosted Xendit checkout so customers can choose any activated payment
   channel. Support optional `QRIS` and `BRI_VIRTUAL_ACCOUNT` restrictions for
   clients that explicitly request them.
3. Authenticate payment webhooks with the configured `x-callback-token`, then
   reconcile successful callbacks through the Payment Session status API and,
   when present, the related Payment Request status API.
4. Use native XLM on Stellar testnet instead of issuing a custom asset.
5. Hold testnet XLM in a pre-funded treasury distribution account.
6. Reserve XLM inventory before exposing a payment checkout. A paid order
   consumes its reservation; an expired unpaid order releases it.
7. Use CoinMarketCap XLM/IDR market data for an indicative, time-limited
   sandbox quote. Keep pricing behind a consumer-owned interface.
8. Run Stellar submission in a separate worker. Payment callbacks only commit
   the payment fact and one durable settlement intent.

## Consequences

- Customers do not need a custom-asset trustline, but their destination must be
  an existing Stellar testnet account that can receive native XLM.
- The treasury bears inventory and reference-price risk. A configurable spread
  is recorded in each quote; the Week 1 default is zero basis points.
- CoinMarketCap downtime stops new orders but does not affect existing locked
  orders.
- Insufficient unreserved XLM stops checkout creation before IDR can be paid.
- Xendit Payment Session and Stellar timeouts are unknown outcomes and require reconciliation
  before retry.
- A future exchange or liquidity-provider adapter may replenish treasury, but
  it is not part of customer settlement or Week 1.
- Custom assets, off-ramp retirement, payout, SEP-24, federation, and external
  developer webhooks remain later-phase work.

## Alternatives considered

### Buy XLM after every payment

Rejected for the first release. The customer could pay successfully while the
exchange purchase or withdrawal fails, and settlement latency would depend on
an additional external system.

### Issue a custom KIDR test asset

Deferred. It demonstrates issuance and redemption but adds issuer governance,
trustlines, reserves, market-making, and retirement behavior that are not
needed to prove an XLM on-ramp.

### Persist payment confirmation without testnet settlement in Week 1

Rejected for this implementation. It matches the narrow documented Week 1
gate, but does not provide the production-shaped end-to-end flow requested by
the project owner. The minimum safe native-XLM settlement worker is therefore
pulled forward from Week 2.

## References

- [Xendit Create Session API](https://docs.xendit.co/apidocs/create-session)
- [Xendit Payment Session webhook](https://docs.xendit.co/apidocs/webhook-notification-sent-defined-webhook-url-updates-payment-session)
- [Xendit payment channels](https://docs.xendit.co/docs/en/available-payment-channels)
- [Xendit payment channels](https://docs.xendit.co/docs/en/available-payment-channels)
- [Stellar transaction error handling](https://developers.stellar.org/docs/data/apis/horizon/api-reference/errors/error-handling)
- [CoinMarketCap latest quotes](https://coinmarketcap.com/api/documentation/guides/get-latest-crypto-prices)
