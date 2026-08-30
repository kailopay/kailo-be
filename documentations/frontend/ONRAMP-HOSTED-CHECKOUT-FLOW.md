# IDR to XLM hosted checkout flow

This document covers one frontend flow only:

```text
Enter IDR amount and Stellar address
  -> create an on-ramp order
  -> redirect to Xendit Hosted Checkout
  -> wait for payment and Stellar settlement
  -> show the final order status
```

The backend runs this flow in sandbox mode and sends XLM on the Stellar
testnet. Show `Sandbox` and `Stellar Testnet` on every screen in this flow.

## 1. Create the order

Collect these values:

- An IDR amount.
- A 56-character Stellar testnet address that starts with `G`.
- An optional Stellar memo with at most 28 characters.

Use `xendit` as the payment method. This opens the Xendit hosted page with all
payment channels activated for the merchant account.

```ts
type CreateOnrampInput = {
  amountMinor: string;
  stellarAccount: string;
  memo?: string | null;
};

type CreateOnrampResponse = {
  order: {
    id: string;
    status: string;
    checkout?: {
      payment_link_url?: string;
      presentation_value?: string;
    };
  };
};

export async function createOnramp(
  apiBaseURL: string,
  apiKey: string,
  input: CreateOnrampInput,
): Promise<CreateOnrampResponse["order"]> {
  const idempotencyKey = crypto.randomUUID();

  const response = await fetch(`${apiBaseURL}/v1/onramps`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
    },
    body: JSON.stringify({
      fiat: {
        currency: "IDR",
        amount_minor: input.amountMinor,
      },
      payment_method: "xendit",
      stellar_destination: {
        account: input.stellarAccount,
        memo: input.memo ?? null,
      },
    }),
  });

  const body = await response.json();

  if (!response.ok) {
    throw new Error(body.error?.message ?? "Could not create the order");
  }

  return body.order;
}
```

Send `amount_minor` as a string. Do not send a JavaScript number. Do not use
floating-point arithmetic for IDR or XLM amounts.

Generate one `Idempotency-Key` for each user purchase intent. If the request
times out, retry with the same key and the same body. Generate a new key only
when the user starts a new purchase.

## 2. Redirect to Xendit

The successful response contains this value:

```text
order.checkout.payment_link_url
```

Redirect the browser with a full navigation:

```ts
const order = await createOnramp(apiBaseURL, apiKey, input);

if (!order.checkout?.payment_link_url) {
  throw new Error("The payment link was not returned");
}

sessionStorage.setItem("kailopay.order_id", order.id);
window.location.assign(order.checkout.payment_link_url);
```

Do not render a QR code or a virtual-account number for a `PAYMENT_LINK`
checkout. Xendit owns the payment screen and displays the available payment
channels there.

The backend may return `presentation_value` with the same URL. Prefer
`payment_link_url` and use `presentation_value` only as a compatibility
fallback.

## 3. Handle the create response

Treat both `201 Created` and `200 OK` as success. A `200` response means that
the backend replayed a request with the same idempotency key.

Handle these responses:

| Response | Frontend action |
|---|---|
| `201` | Save `order.id`, then redirect to `payment_link_url`. |
| `200` | Save `order.id`, then redirect if the order has an active payment link. |
| `202` with `CHECKOUT_PENDING_RECONCILIATION` | Show a pending state. Do not create another order. |
| `400` | Show the validation error. |
| `401` | Ask the user to provide a valid test API key or sign in again. |
| `409` with `IDEMPOTENCY_KEY_REUSED` | Stop and report that the purchase request changed during a retry. |
| `409` with `INSUFFICIENT_LIQUIDITY` | Show that sandbox XLM liquidity is unavailable. |
| `422` | Show that the amount is outside the configured range. |
| `503` | Show a temporary service or quote error and allow a new attempt. |

Use the stable error `code` for UI decisions. Keep `request_id` for support
diagnostics. Do not show raw network or provider error text to the user.

The error shape is:

```json
{
  "error": {
    "code": "AMOUNT_OUT_OF_RANGE",
    "message": "The requested amount is outside the supported range."
  },
  "request_id": "..."
}
```

## 4. Track the order after payment

Save the order ID before redirecting. When the user returns to the app or opens
the order screen, request:

```text
GET /v1/orders/{orderId}
```

Poll every 3 to 5 seconds while the order is `payment_pending`. Stop polling
when the order reaches a terminal status.

```ts
const terminalStatuses = new Set([
  "completed",
  "expired",
  "payment_failed",
  "stellar_failed",
  "cancelled",
]);

async function waitForOrder(
  apiBaseURL: string,
  apiKey: string,
  orderId: string,
): Promise<void> {
  for (;;) {
    const response = await fetch(`${apiBaseURL}/v1/orders/${orderId}`, {
      headers: { Authorization: `Bearer ${apiKey}` },
    });

    if (!response.ok) {
      throw new Error("Could not read the order");
    }

    const { order } = await response.json();
    renderOrderStatus(order);

    if (terminalStatuses.has(order.status)) {
      return;
    }

    await new Promise((resolve) => setTimeout(resolve, 4000));
  }
}
```

Use the backend order status as the source of truth. Do not mark an order as
paid because the user visited the Xendit return page. The backend confirms the
payment through the Xendit callback and server-side reconciliation.

## 5. Map statuses to the UI

| Status | Meaning | UI behavior |
|---|---|---|
| `created` | The order exists and the checkout is being established. | Show a short preparing state. |
| `payment_pending` | The hosted checkout is ready or payment is not complete. | Show a button to reopen the payment link and the quote expiry countdown. |
| `payment_confirmed` | Xendit payment is confirmed. | Show that payment was received. |
| `stellar_processing` | The worker is sending XLM on Stellar testnet. | Show a sending state. |
| `completed` | The XLM transfer is complete. | Show success and link to the Stellar testnet transaction. |
| `expired` | The payment window expired without a completed payment. | Show expiry and offer a new purchase. |
| `payment_failed` | Xendit rejected the checkout permanently. | Show the failure and offer a new purchase. |
| `stellar_failed` | Payment succeeded, but the XLM transfer failed. | Tell the user not to pay again. Show the order ID and support path. |
| `cancelled` | The order was cancelled. | Show the cancellation and offer a new purchase. |

The success transaction URL is:

```text
https://stellar.expert/lumen/testnet/tx/{stellar_transaction_hash}
```

The frontend must not expose or calculate the XLM amount with floating-point
math. Keep `asset.amount` as a string such as `40.0000000`.

## 6. Optional restricted payment methods

Use these values only when the user explicitly chooses one payment channel:

```json
{ "payment_method": "qris" }
```

```json
{ "payment_method": "bri_va" }
```

These methods still return a hosted payment link. The Xendit page is limited to
the requested channel. The redirect and polling code does not change.

## 7. Browser and key requirements

- Use a same-origin reverse proxy or a development-server proxy. The backend
  does not send CORS headers.
- Keep a `pk_test_` key in memory only in the developer playground. Never
  commit, bundle, persist, or log it.
- Use a server-side BFF for a production frontend so the browser does not hold
  the API key.
- Display `Sandbox` and `Stellar Testnet` on the order and checkout screens.
- Do not let a user submit another order while the current order is waiting for
  reconciliation.

## 8. Acceptance checklist

- [ ] The form serializes IDR as `fiat.amount_minor`, a string.
- [ ] The request sends `payment_method: "xendit"`.
- [ ] The request includes a new `Idempotency-Key`.
- [ ] The frontend stores the returned order ID.
- [ ] The frontend redirects to `checkout.payment_link_url`.
- [ ] The frontend does not render QR or VA data for `PAYMENT_LINK`.
- [ ] The frontend polls `GET /v1/orders/{id}` after payment.
- [ ] The frontend uses order status, not the redirect result, to decide whether payment succeeded.
- [ ] The frontend shows sandbox and Stellar testnet labels.
- [ ] The frontend handles payment and Stellar failure without asking the user to pay again.
