# KailoPay D2 Evidence Guide

This page explains the Stellar part of KailoPay in plain language and shows how to read the transaction evidence. It is intended as a project handoff for someone who has not worked on the codebase.

## What the project does

KailoPay is a sandbox payment application. A user can create a buy or sell order in the web app, while the backend coordinates the order, payment status, webhook notifications, and Stellar operations. The fiat side is simulated in this sandbox; it does not move real Indonesian rupiah.

Deliverable 2 covers the Stellar anchor behavior. The backend now supports the KXLM asset flow: after a buy is confirmed, the issuer can send KXLM to the user's Stellar wallet; for a sell, the backend checks the KXLM deposit details and the issuer can claw back the accepted amount. The sell endpoint and request JSON shape remain the same. The implementation is recorded in the backend repository at commit `a55ad13` and described in:

- ADR-008: https://github.com/kailopay/kailo-be/blob/main/documentations/backend/decisions/ADR-008-issued-kxlm-and-clawback.md
- Testnet setup guide: https://github.com/kailopay/kailo-be/blob/main/documentations/backend/KXLM-TESTNET-SETUP.md

The backend stores issuer information for orders and retains support for records that use native XLM. This is covered by migrations `000017` and `000018`.

## How to read the transaction links

These Stellar Expert links point to real testnet transactions related to the buy and sell flows:

- Buy flow: https://stellar.expert/explorer/testnet/tx/b4106f0c9fecc7d7a00b7715e5a99d2e156dfdb11b29c7954a8edb1bcb8f1065 — a 24.1411618 XLM transfer from the treasury to a customer's Stellar wallet.
- Sell flow: https://stellar.expert/explorer/testnet/tx/5ebd7e80ebad47de8becf74d271537944a95d40ed1965ddc3ced608c9fccd93a — a 35 XLM deposit from a customer's wallet to the anchor deposit account.
- Sell retirement: https://stellar.expert/explorer/testnet/tx/1b7b8c3a23e248bdac3ef5a16f414472c65513530909269e4892d17f0691ce3e — a 35 XLM transfer from the deposit account to the sink account.

The asset shown in these three transactions is native XLM. They let a reader inspect the buy transfer, sell deposit, and retirement steps on Stellar testnet. They should not be described as KXLM issuance or KXLM clawback transactions.

## Which hashes demonstrate KXLM issue and clawback?

A Stellar transaction hash is produced when a transaction is submitted to the network. It cannot be calculated in advance from the code, wallet, or order. For D2, the relevant KXLM transaction records are:

1. **KXLM issuance after buy:** the successful transaction in which the KXLM issuer sends the purchased amount to the customer's wallet.
2. **KXLM clawback after sell:** the successful transaction in which the issuer claws back the accepted KXLM deposit.

For each operation, copy the full transaction hash from the successful Stellar submission result or find it in the transaction's Stellar Expert page. A Stellar transaction hash is a 64-character hexadecimal value. The explorer URL format is:

```text
https://stellar.expert/explorer/testnet/tx/<actual-64-character-transaction-hash>
```

Replace the angle-bracketed part only with the hash returned for that specific successful transaction. Do not reuse the buy transfer hash as the issuance hash, or the sell deposit hash as the clawback hash: each submitted on-chain operation has its own hash. A link is useful evidence when its explorer page shows the expected asset, source, destination, amount, operation, and successful result.

## How D2 fits into the project

D2 is marked **Complete for backend implementation**: the repository contains the KXLM issuance and clawback behavior, the sell request contract is preserved, and the setup and design decisions are documented. The transaction links above provide concrete testnet examples for the XLM buy and sell movements. Read the on-chain asset and operation on each page to understand exactly what it proves.

This is a sandbox project. It demonstrates the application and Stellar integration in testnet; it is not a production payout service, and the fiat payout is simulated.
