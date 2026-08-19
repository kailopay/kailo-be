# Payment Gateway Integration

## 1. Scope

`v0.1.0` selects Xendit Payment Requests v3 using development credentials. The Week 1 adapter supports:

- QRIS sandbox checkout.
- At least one bank-transfer/virtual-account sandbox checkout.
- Authenticated payment callbacks.
- Payment status lookup/reconciliation.
- Server-side Payment Request status reconciliation.

The selected API version is `2024-11-11`; QRIS uses `QRIS` and bank transfer
uses `BRI_VIRTUAL_ACCOUNT`. Domain and service behavior remain provider-neutral.

## 2. Provider port

```ts
interface PaymentGateway {
  createCheckout(input: CreateCheckoutInput): Promise<CheckoutResult>;
  verifyCallback(input: RawCallback): Promise<VerifiedGatewayEvent>;
  getPayment(input: PaymentLookup): Promise<PaymentStatusResult>;
  initiatePayout(input: PayoutInput): Promise<PayoutResult>;
  getPayout(input: PayoutLookup): Promise<PayoutStatusResult>;
}
```

The interface uses KailoPay types. Provider response objects are mapped inside the adapter and never enter order-domain code.

## 3. Checkout creation

Input must include:

- Stable KailoPay order ID and provider idempotency/external ID.
- `IDR` and exact expected amount.
- Selected method (`qris` or `bank_transfer`).
- Callback/return URLs derived from trusted configuration.
- Expiry consistent with the order.
- Minimal synthetic customer metadata only when required by the sandbox.

Processing:

1. Validate order is `created` and route is supported.
2. Persist the order, idempotency record, and XLM reservation atomically.
3. The API calls Xendit synchronously after that transaction commits, using the order ID as `reference_id`.
4. Persist provider checkout ID, method-specific presentation data, amount, expiry, and sanitized metadata.
5. Move order to `payment_pending`.

If the provider outcome is unknown, reconcile by the external ID before retrying.

## 4. Callback endpoint

Recommended route: provider-specific infrastructure route such as `/callbacks/payments/{provider}`. Do not expose provider choice in public order-domain endpoints.

Callback processing order:

1. Enforce HTTPS at the edge, method, content type, payload-size, and timeout limits.
2. Capture request ID and exact raw bytes needed for signature verification.
3. Verify provider authentication/signature/token before parsing into a trusted event.
4. Derive or read stable provider event ID and compute payload hash.
5. Insert the callback receipt under a unique constraint.
6. Return the prior safe result for an already processed event.
7. Match provider checkout/reference to exactly one KailoPay order.
8. Reconcile currency, amount, method, and successful provider state.
9. Apply the legal order transition and write outbox events in one transaction.
10. Return the provider-required acknowledgement promptly.

Do not initiate Stellar network calls inside the callback request. The callback commits durable intent; a worker performs settlement.

## 5. Callback authenticity

Xendit authenticates the webhook with `x-callback-token`. KailoPay compares the
configured token in constant time, stores the exact-body digest, then retrieves
`GET /v3/payment_requests/{payment_request_id}` before moving value.

General requirements:

- Use raw request bytes where the signature algorithm requires them.
- Compare MAC/signatures in constant time.
- Use credentials only from secret storage.
- Reject missing/invalid authentication before order lookup/state mutation.
- Prevent algorithm/header confusion by allowing only the configured mechanism.
- Record authentication result and safe failure reason; never log the shared secret or full sensitive body.
- If provider guidance recommends server-side status lookup, perform it before accepting value movement.

## 6. Payment reconciliation

A callback is accepted as payment confirmation only when all available fields match:

- Active configured provider.
- Known provider checkout/external ID.
- Expected KailoPay order.
- Currency `IDR`.
- Exact expected amount.
- Supported payment method.
- Provider status equivalent to settled/paid, not merely pending/authorized.
- Event not previously processed for another order.

Mismatch handling:

- Persist a sanitized mismatch record.
- Do not move the order to `payment_confirmed`.
- Increment a security/operations metric.
- Return the provider-safe acknowledgement/error behavior documented for the adapter.
- Expose the event in the demo operator diagnostic query/runbook.

## 7. Duplicate and out-of-order events

- Unique provider event IDs prevent double processing.
- Payment reference has a uniqueness policy preventing reuse across orders.
- A later `paid` event may supersede `pending`; a later `pending` event cannot regress `paid`.
- Events arriving after a terminal order state are retained but do not mutate it.
- A payment reported after order expiry is escalated for reconciliation rather than automatically settled.

## 8. Off-ramp payout

After verified asset receipt and confirmed burn/retirement:

1. Persist one payout intent with stable order/external ID.
2. Call the provider's sandbox disbursement/payout capability if available.
3. Store provider payout ID, state, exact IDR amount, and safe destination reference.
4. Reconcile unknown outcomes before retry.
5. Complete the order only after provider success evidence or the stakeholder-approved sandbox simulation outcome.

If a full sandbox payout is unavailable:

- Use an explicitly named `simulated`/`sandbox_instruction_created` result.
- Never show wording such as “IDR sent” unless provider evidence supports it.
- Include the limitation in UI, API response, demo, test record, and Completion Report.

## 9. Error classification

| Class | Example | Retry? | Order behavior |
|---|---|---:|---|
| Validation/permanent | Unsupported method, invalid destination token | No | Safe terminal failure or reject before creation |
| Authentication/configuration | Invalid credential, signature config error | No automatic retry | Alert/stop affected processing; order remains safely pending |
| Rate/temporary provider | `429`, transient `5xx`, timeout | Bounded | Keep processing/unknown; schedule retry/reconciliation |
| Unknown outcome | Client timeout after request sent | Reconcile first | Never assume failed |
| Business rejection | Payout rejected | Usually no | `withdrawal_failed` with safe reason |

## 10. Adapter selection checklist

Before choosing Xendit or Midtrans, verify and record:

- Sandbox account and credential availability.
- QRIS sandbox creation and completion procedure.
- Bank-transfer/virtual-account sandbox procedure.
- Callback authentication and replay behavior.
- Stable event/reference identifiers.
- Status lookup APIs.
- Idempotency support.
- Payout/disbursement sandbox behavior.
- Expiry, refund, late-event, and error behavior.
- Public documentation links and SDK/runtime compatibility.

## 11. Tests and evidence

Required automated tests:

- Provider adapter mapping using sanitized fixtures.
- Valid and invalid callback authentication.
- Amount/currency/reference mismatch.
- Duplicate and out-of-order callback replay.
- Checkout timeout with reconciliation.
- Payout success, permanent failure, and unknown outcome.

Required SOW evidence:

- Real provider sandbox QRIS checkout.
- Real provider sandbox bank-transfer checkout.
- Verified callback receipt and order transition logs.
- Off-ramp provider/simulation reference with limitation disclosure.
- No production credential or real customer/payment data.
