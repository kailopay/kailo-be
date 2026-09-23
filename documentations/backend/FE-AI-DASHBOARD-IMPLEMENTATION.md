# Build the Week 3 developer dashboard

This guide tells the frontend AI agent how to build the developer dashboard
against the KailoPay backend. The dashboard manages sandbox and Stellar testnet
integrations. It does not show production earnings or bank settlement.

Use the checked-in [OpenAPI contract](../../openapi/openapi.yaml) for the exact
schemas. Use this guide for page behavior and user-facing rules.

## Keep the scope clear

Build these dashboard areas:

- overview;
- order analytics;
- orders;
- sandbox fee and revenue records;
- API keys;
- verified wallet profiles;
- webhook endpoints and delivery history.

Do not build an API request telemetry page. The backend does not expose request
counts, endpoint latency, rate-limit usage, or per-key HTTP metrics. The
analytics endpoints describe orders, exchange activity, payment methods, and
webhook delivery health.

Do not change the existing on-ramp or off-ramp integration. These routes remain
the integration contract:

```text
POST /v1/quotes
POST /v1/onramps
POST /v1/offramps
GET  /v1/orders
GET  /v1/orders/{order_id}
```

## Complete the account checks first

Use the local session cookie for every dashboard request. Set
`credentials: "include"` in `fetch` or the equivalent client configuration.

1. Call `GET /auth/me` when the app loads.
2. Read `user.developer_enabled` from the response.
3. If Developer Mode is disabled, show the Developer Mode setup state.
4. Send `PATCH /auth/me` with `{"developer_enabled":true}` when the user
   enables Developer Mode.
5. Call `GET /v1/kyc` to read the Persona sandbox KYC status.
6. Allow API-key creation only after the KYC status is approved.

Every developer-management route requires Developer Mode. The backend returns
`403` with `error: "Developer Mode is required"` when the user disables it or
has not enabled it yet. Handle this response by returning the user to the
Developer Mode setup state.

KYC is an API-key creation prerequisite. It is not a prerequisite for reading
the dashboard, managing a verified wallet profile, or registering a webhook.
Wallet registration still requires a matching SEP-10 testnet proof.

## Map pages to backend routes

| Page | Backend route | Use |
| --- | --- | --- |
| Overview | `GET /v1/developer/overview` | Counts, exchange totals, completion health, webhook health, and recent orders |
| Analytics | `GET /v1/developer/analytics` | Hour, day, or week order and payment-method charts |
| Revenue summary | `GET /v1/developer/revenue/summary` | Sandbox gross, fee, platform, developer, and net totals |
| Revenue entries | `GET /v1/developer/revenue/entries` | Cursor-paginated immutable financial records |
| Orders | `GET /v1/developer/orders` | Account-wide order history and provider correlation |
| API keys | `POST/GET /v1/api-keys`, `DELETE /v1/api-keys/{id}` | Create, list, and revoke server-side test keys |
| Wallet profiles | `GET/POST/PATCH/DELETE /v1/developer/wallets` | Manage verified Stellar testnet wallet hints |
| Webhook endpoints | `POST/GET /v1/webhook-endpoints`, `GET/POST/DELETE /v1/webhook-endpoints/{id}` | Register, inspect, test, and disable endpoints |
| Webhook deliveries | `GET /v1/webhook-deliveries`, `POST /v1/webhook-deliveries/{id}/replay` | Inspect attempts and replay exhausted deliveries |

The dashboard routes scope data from the session user. A `client_id` filter
can narrow results only to a client owned by that user.

## Build the overview and analytics pages

Call `GET /v1/developer/overview` for the summary cards. The response includes:

- `total_orders`, `active_orders`, `completed_orders`, and `failed_orders`;
- `buy_orders` and `sell_orders`;
- `gross_idr_minor`, `fee_idr_minor`, and `net_idr_minor` as decimal strings;
- `asset_volume` as an exact XLM string;
- `success_rate` and `average_completion_seconds`;
- `pending_webhook_deliveries` and `exhausted_webhook_deliveries`;
- `recent_orders`.

Call `GET /v1/developer/analytics` for charts. The `bucket` query accepts
`hour`, `day`, or `week`. The default is `day`. Each bucket includes order
counts, buy and sell counts, exact amounts, completion and failure counts,
average completion time, payment methods, and webhook delivery counts.

The default date range is the previous 30 days. The maximum range is 366 days.
Send `from` and `to` as RFC 3339 timestamps when the user selects a range.
Supported filters are `client_id`, `currency`, `direction`, `status`, and
`payment_method`.

Use an empty chart state when `buckets` is empty. Do not turn an empty response
into a failed request.

## Build the orders page

Call `GET /v1/developer/orders` with the selected filters. Use `paging_id` for
the next page and `limit` for the page size. The backend caps a page at 100
items.

The order record can include:

- order ID, client ID, and client name;
- `direction` and `status`;
- fiat currency and exact fiat amount;
- exact XLM amount and payment method;
- gateway provider and gateway reference;
- Stellar intent ID and transaction hash;
- SEP-24 transaction ID and quote ID;
- wallet account;
- payout reference and payout disclosure;
- latest event, timestamps, and failure code;
- the immutable financial entry when one exists.

Show `environment: "sandbox"` and `network: "stellar_testnet"` on the page.
Keep `payout_disclosure` visible for off-ramp orders. The current simulation
also does not retire XLM on-chain; it requires a verified deposit, and the
disclosure must remain visible beside the payout reference.

## Build the fee and revenue pages

Use `GET /v1/developer/revenue/summary` for summary cards and
`GET /v1/developer/revenue/entries` for the table. Amount fields are strings.
Keep them as strings until a display formatter converts them to IDR text.

The current policy is `sandbox-zero-fee-v1`. The response marks the figures as
simulated and includes a disclosure. Display that disclosure near the totals:

```text
Sandbox figures are simulated estimates. No real fiat revenue or payout is settled.
```

Do not label the current developer or platform revenue as withdrawable money.

## Build API-key management

Use the session routes:

```text
POST   /v1/api-keys
GET    /v1/api-keys
DELETE /v1/api-keys/{id}
```

Create a key with a name between 1 and 100 characters:

```json
{
  "name": "Checkout service"
}
```

The create response contains the complete `pk_test_` key. Show it once in a
copy dialog and explain that the backend never returns it again. Never put the
complete key in browser storage, frontend JavaScript, a URL, or a public
repository. The user must copy it to a trusted server integration.

The list response contains safe metadata only:

- key ID and client ID;
- client name, environment, and status;
- safe key prefix;
- creation time, last-used time, and revocation time.

Revoking a key returns `204`. Treat a revoked key as unusable and update the
list after the response.

## Build wallet profile settings

Use these routes:

```text
GET    /v1/developer/wallets
POST   /v1/developer/wallets
PATCH  /v1/developer/wallets/{id}
DELETE /v1/developer/wallets/{id}
```

The create request includes a Stellar testnet account, a label, an optional
client ID, a primary flag, and a SEP-10 bearer proof. The proof account must
match the submitted wallet account. The backend stores the public account and
never stores a wallet seed or private key.

Allow the user to edit the label and primary flag. Use revoke for removal. Do
not show a wallet seed field because the backend does not accept or store one.

## Build webhook settings

Register a public HTTPS endpoint:

```json
{
  "url": "https://developer.example.test/kailopay/events",
  "event_types": [
    "order.payment_confirmed",
    "order.completed",
    "order.failed"
  ]
}
```

The backend rejects non-HTTPS URLs, private hosts, embedded credentials, and
unknown event types. An empty `event_types` array subscribes to every supported
order event.

The create response returns the signing secret once:

```json
{
  "endpoint": {"id": "endpoint-uuid"},
  "secret": "whsec_one_time_value"
}
```

Tell the user to save the secret in the receiving service. The dashboard cannot
read it again. There is no separate rotation endpoint. Disable the endpoint and
register a new one when the secret must change.

Use the test action to queue a synthetic event. The test does not create an
order or move money. The worker must be running before the receiving service
gets the request.

The delivery table supports these statuses:

```text
in_flight, delivered, retry_scheduled, failed, exhausted, superseded
```

Show the HTTP status, attempt number, schedule, completion time, duration, and
safe error when present. Show a replay button only for an `exhausted` delivery.
Replay uses the same immutable event ID, so the receiving service must still
deduplicate events.

Webhook consumers verify the raw request body. The backend sends:

```text
KailoPay-Timestamp
KailoPay-Event-Id
KailoPay-Signature: v1=<hex HMAC-SHA256>
```

The signing input is the timestamp, a period, and the exact raw JSON body. The
consumer compares the HMAC with constant-time comparison and rejects an old
timestamp before processing the event.

## Handle common states

| Status | Frontend behavior |
| --- | --- |
| `401` | Send the user to the login flow. |
| `403` with `Developer Mode is required` | Show Developer Mode setup. |
| `400` | Show the field or filter error and keep the current page state. |
| `404` | Show that the resource is no longer available to this user. |
| `409` on webhook replay | Keep the delivery unchanged. It is not eligible for replay. |
| `500` | Show a retry action and keep the last successful data. |

The backend returns initialized empty arrays for dashboard collections. Render
empty states for `recent_orders`, `buckets`, `entries`, `orders`, `api_keys`,
`wallets`, `endpoints`, and `deliveries`.

## Keep the sandbox disclosures visible

The backend operates on Stellar testnet XLM. The off-ramp simulator requires an
exact verified deposit, records retirement without an on-chain burn, accepts
synthetic payout references, and does not send a real bank transfer.
Payment checkout redirects do not prove payment confirmation. Poll the order
until the backend reports the final state.

Do not call Xendit, Persona, Horizon, federation signing, or webhook delivery
from the browser. The backend owns those integrations.

## Suggested build order

1. Add one session-aware API client with `credentials: "include"`.
2. Add account, Developer Mode, and KYC setup states.
3. Add overview cards and recent orders.
4. Add analytics filters and charts.
5. Add orders and revenue tables with cursor pagination.
6. Add API-key creation and the one-time secret dialog.
7. Add wallet profile settings.
8. Add webhook configuration, test delivery, attempts, and replay.
9. Add the existing buy and sell flows without changing their request bodies.

## Backend references

- [OpenAPI contract](../../openapi/openapi.yaml)
- [Backend API design](API-DESIGN.md)
- [Frontend capabilities](FRONTEND-CAPABILITIES.md)
- [Backend README](README.md)
- [Webhook design](WEBHOOK-DESIGN.md)
