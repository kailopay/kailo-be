# Developer dashboard frontend implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a session-authenticated developer workspace that lets a KailoPay integrator inspect sandbox exchanges, manage test API keys and verified wallets, configure signed webhooks, and understand simulated fee records without changing the existing on-ramp or off-ramp API.

**Architecture:** Keep the existing Next.js App Router and feature-based frontend structure. Put all developer request and response parsing in 'lib/api/developer.ts', keep pages thin, and put interactive dashboard behavior in 'features/developer/'. Use the local session cookie for dashboard management and keep 'pk_test_' keys for trusted server integrations only.

**Tech Stack:** Next.js 16, React 19, TypeScript, Tailwind CSS 4, Node's built-in test runner, same-origin Next rewrites, and the checked-in KailoPay OpenAPI contract.

**Spec:** '../FE-AI-DASHBOARD-IMPLEMENTATION.md', '../../../openapi/openapi.yaml', 'D:/Proyek/kailopay/kailopay-fe/AGENTS.md', and 'D:/Proyek/kailopay/kailopay-fe/documentations/FRONTEND-GUIDE.md'.

The frontend repository root is 'D:/Proyek/kailopay/kailopay-fe'. All frontend paths in this plan are relative to that root. The backend repository is the current repository, 'D:/Proyek/kailopay/kailopay-be'.

## Product meaning

The dashboard belongs to an authenticated account with Developer Mode enabled. It is for a developer who uses KailoPay as an integration service to exchange IDR and Stellar testnet XLM through the existing API.

The dashboard is not the consumer buy and sell screen. Keep these areas separate:

~~~text
/dashboard                         consumer exchange home
/buy and /sell                     consumer on-ramp and off-ramp flows
/developer                         developer overview
/developer/analytics               order and payment analytics
/developer/orders                  account-scoped order records
/developer/revenue                 simulated fee and revenue records
/developer/api-keys                sandbox API-key management
/developer/wallets                 verified Stellar testnet wallet profiles
/developer/webhooks                endpoint and delivery management
~~~

Do not add '/developer/telemetry'. The backend does not expose request counts, endpoint latency, rate-limit usage, or per-key HTTP metrics.

## Backend contract map

Use 'D:/Proyek/kailopay/kailopay-be/openapi/openapi.yaml' as the schema authority. The dashboard calls these routes with the local session cookie:

| Feature | Routes |
| --- | --- |
| Overview | 'GET /v1/developer/overview' |
| Analytics | 'GET /v1/developer/analytics' |
| Revenue | 'GET /v1/developer/revenue/summary', 'GET /v1/developer/revenue/entries' |
| Orders | 'GET /v1/developer/orders' |
| API keys | 'POST /v1/api-keys', 'GET /v1/api-keys', 'DELETE /v1/api-keys/{id}' |
| Wallet profiles | 'GET/POST /v1/developer/wallets', 'PATCH/DELETE /v1/developer/wallets/{id}' |
| Webhook endpoints | 'POST/GET /v1/webhook-endpoints', 'GET/POST/DELETE /v1/webhook-endpoints/{id}' |
| Webhook deliveries | 'GET /v1/webhook-deliveries', 'POST /v1/webhook-deliveries/{id}/replay' |
| Account gate | 'GET /auth/me', 'PATCH /auth/me', 'GET /v1/kyc' |

The dashboard sends 'credentials: "include"' through the existing 'apiRequest' or 'serverApiRequest' helpers. It does not send a 'pk_test_' key to authenticate dashboard management routes.

## Global constraints

- Keep IDR minor units, XLM amounts, exchange rates, cursors, and API identifiers as strings. Never use JavaScript floating-point arithmetic for money or XLM.
- Use 'GET /auth/me' to determine whether the session exists and whether 'user.developer_enabled' is true.
- A developer-management request that returns '403' with 'error: "Developer Mode is required"' returns the user to the Developer Mode setup state.
- Approved Persona sandbox KYC is required to create an API key. KYC is not required to read the dashboard, manage a verified wallet profile, or register a webhook.
- API-key creation returns the complete 'pk_test_' value once. Never store it in browser storage, a URL, shipped JavaScript, logs, or a public repository.
- Webhook creation returns the signing secret once. Never render it again after the one-time dialog closes.
- Wallet registration requires a SEP-10 proof for the submitted Stellar testnet account. Never collect or store a wallet seed or private key.
- Render 'environment: "sandbox"' and 'network: "stellar_testnet"' on developer data views.
- Keep 'simulated' and 'disclosure' fields visible for fee records and payout records. Do not call sandbox revenue withdrawable money or a simulated payout a bank transfer.
- Use opaque 'paging_id' values exactly as returned. Do not parse, increment, or synthesize cursors.
- Send only documented JSON fields. The backend rejects unknown fields.
- Do not call Xendit, Persona, Horizon, federation signing, or webhook delivery directly from the browser. The backend owns those integrations.
- Do not modify the existing request bodies, authentication rules, or polling behavior for 'POST /v1/quotes', 'POST /v1/onramps', 'POST /v1/offramps', 'GET /v1/orders', or 'GET /v1/orders/{order_id}'.

## Review focus

- Developer Mode disabled after the page loads. The UI must stop treating a '403' response as an empty dashboard and must show the setup state. Test in Task 2.
- One-time API-key and webhook-secret values. The UI must show each value only in a controlled copy dialog and must not persist it. Test in Tasks 5 and 6.
- Exact decimal strings and simulated financial data. The UI must format values for display without converting them to 'number', and it must keep the sandbox disclosure next to totals. Test in Tasks 1 and 4.
- Opaque cursor pagination and empty collections. The UI must preserve filters while requesting the next page and must render an empty state instead of an error. Test in Tasks 3 and 4.
- Webhook replay eligibility. The UI must show replay only for 'exhausted' deliveries and must keep the delivery unchanged after a '409'. Test in Task 6.

---

### Task 1: Add the typed developer API boundary

**Files:**
- Modify: 'lib/api/types.ts'
- Create: 'lib/api/developer.ts'
- Test: 'lib/api/developer.test.mts'

**Interfaces:**
- 'DeveloperFilters' contains optional 'client_id', 'from', 'to', 'currency', 'direction', 'status', 'payment_method', 'limit', and 'paging_id' fields.
- 'parseDeveloperOverview(payload: unknown): DeveloperOverview', 'parseWebhookDelivery(payload: unknown): WebhookDelivery', and 'buildDeveloperQuery(filters): string' are exported boundary helpers for tests and page loaders.
- 'getDeveloperOverview(filters?: DeveloperFilters): Promise<DeveloperOverview>' calls 'GET /v1/developer/overview'.
- 'getDeveloperAnalytics(filters: DeveloperFilters & { bucket?: "hour" | "day" | "week" }): Promise<DeveloperAnalyticsResponse>' calls 'GET /v1/developer/analytics'.
- 'getDeveloperRevenueSummary(filters?: DeveloperFilters): Promise<DeveloperRevenueSummary>' calls 'GET /v1/developer/revenue/summary'.
- 'listDeveloperRevenueEntries(filters?: DeveloperFilters): Promise<DeveloperRevenueEntriesResponse>' calls 'GET /v1/developer/revenue/entries'.
- 'listDeveloperOrders(filters?: DeveloperFilters): Promise<DeveloperOrdersResponse>' calls 'GET /v1/developer/orders'.
- 'listDeveloperWallets(): Promise<DeveloperWallet[]>', 'registerDeveloperWallet(input): Promise<DeveloperWallet>', 'updateDeveloperWallet(id, input): Promise<DeveloperWallet>', and 'revokeDeveloperWallet(id): Promise<void>' wrap the wallet routes.
- 'listWebhookEndpoints(): Promise<WebhookEndpoint[]>', 'createWebhookEndpoint(input): Promise<{ endpoint: WebhookEndpoint; secret: string }>', 'getWebhookEndpoint(id): Promise<WebhookEndpoint>', 'queueWebhookTest(id): Promise<WebhookQueuedResponse>', and 'disableWebhookEndpoint(id): Promise<void>' wrap endpoint management.
- 'listWebhookDeliveries(filters): Promise<WebhookDeliveriesResponse>' and 'replayWebhookDelivery(id): Promise<void>' wrap delivery inspection.

- [ ] **Step 1: Write the failing boundary tests**

Add tests that parse one valid payload for each collection and reject a malformed payload. Include these assertions:

~~~ts
test("keeps developer money as exact strings", () => {
  const result = parseDeveloperOverview({
    environment: "sandbox",
    network: "stellar_testnet",
    from: "2026-09-01T00:00:00Z",
    to: "2026-10-01T00:00:00Z",
    total_orders: 1,
    active_orders: 0,
    completed_orders: 1,
    failed_orders: 0,
    buy_orders: 1,
    sell_orders: 0,
    gross_idr_minor: "100000",
    asset_volume: "40.0000000",
    fee_idr_minor: "0",
    net_idr_minor: "100000",
    success_rate: 1,
    average_completion_seconds: 12,
    pending_webhook_deliveries: 0,
    exhausted_webhook_deliveries: 0,
    recent_orders: [],
  });

  assert.equal(result.gross_idr_minor, "100000");
  assert.equal(result.asset_volume, "40.0000000");
});
~~~

Also test that 'parseDeveloperOverview' rejects 'gross_idr_minor: 100000', that 'parseWebhookDelivery' preserves 'status: "exhausted"', and that 'buildDeveloperQuery({ paging_id: "opaque-2", limit: 20 })' returns 'paging_id=opaque-2&limit=20' without decoding or changing the cursor.

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- lib/api/developer.test.mts
~~~

Expected result: the test fails because the developer response types, parsers, and request wrappers do not exist yet.

- [ ] **Step 3: Add the wire types and boundary parsers**

Add these types to 'lib/api/types.ts': 'DeveloperOverview', 'DeveloperAnalyticsResponse', 'DeveloperAnalyticsBucket', 'DeveloperRevenueSummary', 'DeveloperRevenueEntry', 'DeveloperRevenueEntriesResponse', 'DeveloperOrder', 'DeveloperOrdersResponse', 'DeveloperWallet', 'WebhookEventType', 'WebhookEndpoint', 'WebhookQueuedResponse', 'WebhookDeliveryStatus', 'WebhookDelivery', 'WebhookDeliveriesResponse', and the wallet and webhook request types.

Keep exact string fields as strings. Use the existing 'ApiError', 'isRecord', 'stringField', and 'numberField' helpers from 'lib/api/client.ts' at the boundary. Do not add 'any', unchecked casts, or UI-specific fields to the wire types.

- [ ] **Step 4: Add request wrappers and query serialization**

Implement 'lib/api/developer.ts' with 'apiRequest'. Serialize only defined query values through 'URLSearchParams'. Use 'PATCH' for wallet updates, explicit 'POST' for webhook tests with no body, and treat '204' as 'void'.

Use the exact request bodies below:

~~~ts
await apiRequest("/v1/developer/wallets", {
  method: "POST",
  body: {
    client_id,
    network: "stellar_testnet",
    wallet_account,
    label,
    is_primary,
    sep10_token,
  },
});

await apiRequest("/v1/webhook-endpoints", {
  method: "POST",
  body: { url, event_types },
});
~~~

Do not put 'sep10_token' or a webhook secret into a returned list type after the request completes.

- [ ] **Step 5: Run the focused tests and verify that they pass**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- lib/api/developer.test.mts
~~~

Expected result: all parser and query-serialization tests pass.

### Task 2: Add the Developer Mode gate and developer shell

**Files:**
- Modify: 'app/(session)/developer/page.tsx'
- Create: 'app/(session)/developer/layout.tsx'
- Create: 'app/(session)/developer/api-keys/page.tsx'
- Create: 'app/(session)/developer/analytics/page.tsx'
- Create: 'app/(session)/developer/orders/page.tsx'
- Create: 'app/(session)/developer/revenue/page.tsx'
- Create: 'app/(session)/developer/wallets/page.tsx'
- Create: 'app/(session)/developer/webhooks/page.tsx'
- Create: 'features/developer/developer-shell.tsx'
- Create: 'features/developer/developer-access-state.tsx'
- Create: 'features/developer/developer-access.ts'
- Test: 'features/developer/developer-access.test.mts'

**Interfaces:**
- 'getDeveloperAccessState(user: User): "enabled" | "setup"' returns 'enabled' only when 'user.developer_enabled' is true.
- 'DeveloperShell' receives '{ children: React.ReactNode; developerEnabled: boolean }' and renders the developer navigation without adding a telemetry link.
- Every developer page checks the session before fetching developer data. A missing session redirects to '/login'.

- [ ] **Step 1: Write the failing access-state tests**

Test these cases:

~~~ts
test("disabled users get the setup state", () => {
  assert.equal(
    getDeveloperAccessState({ developer_enabled: false } as User),
    "setup",
  );
});

test("enabled users get the developer workspace", () => {
  assert.equal(
    getDeveloperAccessState({ developer_enabled: true } as User),
    "enabled",
  );
});
~~~

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run 'cd D:/Proyek/kailopay/kailopay-fe; npm test -- features/developer/developer-access.test.mts'.

Expected result: the test fails because the access helper does not exist.

- [ ] **Step 3: Implement the shell and setup state**

Keep the existing consumer routes available when Developer Mode is disabled. Show a setup panel with a Developer Mode toggle and a link to '/profile'. Do not call a developer data route while the user is in the setup state.

The shell navigation must contain 'Overview', 'Analytics', 'Orders', 'Revenue', 'API keys', 'Wallets', and 'Webhooks'. Do not add 'Telemetry', 'API usage', or 'Rate limits' because no backend contract supports them.

When the toggle is enabled, call 'PATCH /auth/me' with exactly '{ developer_enabled: true }', then refresh the session state. When a developer route returns '403' with 'Developer Mode is required', render the same setup state and preserve the last successful page data until the user acts.

- [ ] **Step 4: Convert the existing '/developer' page to the workspace entry point**

Use 'getServerSession' and 'serverApiRequest'. When the session is enabled, '/developer' loads the overview. When the session is disabled, it renders the setup state. Move the existing API-key panel to '/developer/api-keys' so API-key management has a stable route and navigation item.

- [ ] **Step 5: Run the focused tests and lint**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- features/developer/developer-access.test.mts
npm run lint
~~~

Expected result: focused tests pass and lint exits with code 0.

### Task 3: Build overview and analytics

**Files:**
- Create: 'features/developer/overview-panel.tsx'
- Create: 'features/developer/analytics-panel.tsx'
- Create: 'features/developer/analytics-filters.tsx'
- Create: 'features/developer/dashboard-format.ts'
- Test: 'features/developer/dashboard-format.test.mts'
- Modify: 'app/(session)/developer/page.tsx'
- Modify: 'app/(session)/developer/analytics/page.tsx'

**Interfaces:**
- 'formatIdrMinor(value: string): string' formats an IDR minor-unit string without changing its stored value.
- 'formatXlm(value: string): string' formats an XLM string without using 'parseFloat'.
- 'defaultDeveloperRange(now: Date): { from: string; to: string }' returns the previous 30 days as UTC RFC 3339 timestamps.
- 'OverviewPanel' renders summary cards, recent orders, sandbox badges, and webhook health.
- 'AnalyticsPanel' renders server-provided buckets for 'hour', 'day', or 'week' and an empty state when 'buckets' is empty.

- [ ] **Step 1: Write formatting and empty-state tests**

Test that 'formatIdrMinor("100000")' displays 'Rp100.000', 'formatXlm("40.0000000")' preserves the seven decimal places in the displayed value, and neither helper accepts a 'number' parameter. Test that an empty bucket array produces an empty-chart state rather than an error.

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run 'cd D:/Proyek/kailopay/kailopay-fe; npm test -- features/developer/dashboard-format.test.mts'.

Expected result: the test fails because the formatters and range helper do not exist.

- [ ] **Step 3: Implement overview cards**

Load 'GET /v1/developer/overview' with the default 30-day range. Render 'total_orders', 'active_orders', 'completed_orders', 'failed_orders', 'buy_orders', 'sell_orders', 'success_rate', and 'average_completion_seconds'.

Render 'gross_idr_minor', 'fee_idr_minor', 'net_idr_minor', and 'asset_volume' as exact display strings. Render 'pending_webhook_deliveries' and 'exhausted_webhook_deliveries' as webhook health cards. Render 'recent_orders' as a table or list with an explicit empty state.

- [ ] **Step 4: Implement analytics filters and charts**

Send 'bucket=hour', 'bucket=day', or 'bucket=week'. Support 'from', 'to', 'client_id', 'currency', 'direction', 'status', and 'payment_method'. Reject a local range longer than 366 days before sending it, and serialize selected dates as UTC RFC 3339 values.

Use the server-provided 'payment_methods', completion counts, failure counts, and webhook counts. Do not infer revenue or request telemetry from chart data.

- [ ] **Step 5: Implement loading, error, and disabled states**

Keep the last successful overview or analytics data while a refresh is pending. Show a retry action for '500'. Show the Developer Mode setup state for the documented '403'. Show the filter error inline for '400'. Send the user to login for '401'.

- [ ] **Step 6: Run focused tests and lint**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- features/developer/dashboard-format.test.mts
npm run lint
~~~

Expected result: tests pass and lint exits with code 0.

### Task 4: Build orders and sandbox revenue views

**Files:**
- Create: 'features/developer/orders-panel.tsx'
- Create: 'features/developer/revenue-panel.tsx'
- Create: 'features/developer/developer-filters.tsx'
- Create: 'features/developer/cursor-pagination.tsx'
- Modify: 'app/(session)/developer/orders/page.tsx'
- Modify: 'app/(session)/developer/revenue/page.tsx'
- Test: 'features/developer/developer-filters.test.mts'

**Interfaces:**
- 'OrdersPanel' accepts 'DeveloperOrdersResponse' and renders a cursor-paginated account-wide order table.
- 'RevenuePanel' accepts 'DeveloperRevenueSummary' and 'DeveloperRevenueEntriesResponse' and renders summary cards plus entries.
- 'CursorPagination' accepts '{ pagingId: string | null; onNext(): void; disabled: boolean }' and never edits the cursor.

- [ ] **Step 1: Write filter and cursor tests**

Test that changing 'direction', 'status', 'payment_method', 'from', or 'to' clears the old 'paging_id'. Test that clicking next reuses the returned 'paging_id' with the active filters. Test that 'paging_id: null' or an empty result hides the next-page action.

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run 'cd D:/Proyek/kailopay/kailopay-fe; npm test -- features/developer/developer-filters.test.mts'.

Expected result: the test fails because the filter state and cursor component do not exist.

- [ ] **Step 3: Implement the orders table**

Call 'GET /v1/developer/orders' with the active filters and 'limit' up to 100. Show the order ID, client name, direction, status, IDR amount, XLM amount, payment method, gateway reference, Stellar transaction hash, SEP-24 transaction ID, wallet account, latest event, timestamps, and failure code when present.

Show 'payout_reference', 'payout_simulated', and 'payout_disclosure' for off-ramp rows. Keep the disclosure visible:

~~~text
Sandbox figures are simulated estimates. No real fiat revenue or payout is settled.
~~~

Do not invent 'GET /v1/developer/orders/{id}'. If a row needs more detail, expand the row using the already loaded 'DeveloperOrder' or use an existing order route only when its documented authentication and ownership rules allow it.

- [ ] **Step 4: Implement the revenue views**

Call 'GET /v1/developer/revenue/summary' for totals and 'GET /v1/developer/revenue/entries' for the immutable entries table. Display 'gross_idr_minor', 'fee_idr_minor', 'platform_revenue_minor', 'developer_revenue_minor', and 'net_idr_minor' as strings. Display 'simulated' and 'disclosure' next to the totals. Keep 'sandbox-zero-fee-v1' visible as the policy version when it appears in an entry.

Do not add withdrawal, settlement, export-to-bank, or production-revenue actions. The backend does not provide them.

- [ ] **Step 5: Add empty and failure states**

Render empty states for 'orders' and 'entries'. Preserve the selected filters during retries. Treat a '409' or '404' as a resource or eligibility message, not as a generic empty response. Keep 'request_id' available in support copy without exposing raw provider diagnostics.

- [ ] **Step 6: Run focused tests and lint**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- features/developer/developer-filters.test.mts
npm run lint
~~~

Expected result: tests pass and lint exits with code 0.

### Task 5: Finish API-key and verified-wallet management

**Files:**
- Modify: 'features/developer/api-keys-panel.tsx'
- Modify: 'app/(session)/developer/api-keys/page.tsx'
- Create: 'features/developer/api-key-secret-dialog.tsx'
- Create: 'features/developer/wallets-panel.tsx'
- Create: 'features/developer/sep10-wallet-proof.ts'
- Modify: 'app/(session)/developer/wallets/page.tsx'
- Test: 'features/developer/api-key-secret.test.mts'
- Test: 'features/developer/sep10-wallet-proof.test.mts'

**Interfaces:**
- 'ApiKeySecretDialog' accepts '{ key: string; onClose(): void }' and does not write the key to storage.
- 'createWalletProof(walletAccount: string, signer: WalletSigner): Promise<string>' obtains a challenge from 'GET /auth?account=...', signs it through the wallet adapter, and exchanges it through 'POST /auth'.
- 'WalletSigner' is '{ signTransaction(transaction: string): Promise<string> }'; it never exposes a private key to the frontend.

- [ ] **Step 1: Write one-time secret tests**

Test that the API-key create flow stores the complete 'pk_test_' value only in component state, clears it when the dialog closes, and renders the one-time warning. Test that a key-list response contains only the safe prefix and never a complete key.

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run 'cd D:/Proyek/kailopay/kailopay-fe; npm test -- features/developer/api-key-secret.test.mts'.

Expected result: the test fails because the secret dialog behavior is not implemented.

- [ ] **Step 3: Implement API-key management**

Keep the existing 'POST /v1/api-keys', 'GET /v1/api-keys', and 'DELETE /v1/api-keys/{id}' calls. Before create, load 'GET /v1/kyc'. Allow creation only when 'status === "approved"'. For every other KYC status, show the existing verification action or status explanation.

After a successful create, show the complete key once with a copy button and a clear instruction to place it in a trusted server integration. After closing the dialog, set the state to 'null'. Never use 'localStorage', 'sessionStorage', URL query parameters, or a persistent global store for the key.

- [ ] **Step 4: Write and run SEP-10 proof tests**

Test that the wallet proof helper calls these exact operations in order:

~~~text
GET  /auth?account=<wallet_account>
wallet.signTransaction(challenge.transaction)
POST /auth with { transaction: signedTransaction }
~~~

The helper returns the short-lived 'token' string and does not retain it after the registration request finishes. Test that a signing failure stops registration and that the submitted wallet account remains the account used for the challenge.

- [ ] **Step 5: Implement wallet profile settings**

The create form sends 'network: "stellar_testnet"', the public 'wallet_account', 'label', optional 'client_id', 'is_primary', and the short-lived 'sep10_token'. Allow editing only 'label' and 'is_primary'. Use 'DELETE' to revoke a wallet. Never render a seed, private key, or editable secret field.

Show 'verification_method: "sep10"', 'verified_at', 'status', and the public account. Keep the wallet list session-scoped.

- [ ] **Step 6: Run focused tests and lint**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- features/developer/api-key-secret.test.mts features/developer/sep10-wallet-proof.test.mts
npm run lint
~~~

Expected result: tests pass and lint exits with code 0.

### Task 6: Build webhook endpoint and delivery management

**Files:**
- Create: 'features/developer/webhooks-panel.tsx'
- Create: 'features/developer/webhook-endpoint-form.tsx'
- Create: 'features/developer/webhook-secret-dialog.tsx'
- Create: 'features/developer/webhook-deliveries-table.tsx'
- Modify: 'app/(session)/developer/webhooks/page.tsx'
- Test: 'features/developer/webhooks.test.mts'

**Interfaces:**
- 'WebhookEndpointForm' submits '{ url: string; event_types?: WebhookEventType[] }'.
- 'WebhookSecretDialog' accepts '{ secret: string; onClose(): void }' and clears the secret on close.
- 'WebhookDeliveriesTable' accepts 'WebhookDelivery[]' and exposes replay only when 'status === "exhausted"'.

- [ ] **Step 1: Write webhook behavior tests**

Test these cases:

~~~ts
assert.equal(canReplayWebhookDelivery({ status: "exhausted" }), true);
assert.equal(canReplayWebhookDelivery({ status: "failed" }), false);
assert.equal(canReplayWebhookDelivery({ status: "delivered" }), false);
~~~

Also test that endpoint creation accepts only the supported event values, that an empty event list means all supported order events, and that a '409' replay response leaves the delivery status unchanged.

- [ ] **Step 2: Run the focused tests and verify that they fail**

Run 'cd D:/Proyek/kailopay/kailopay-fe; npm test -- features/developer/webhooks.test.mts'.

Expected result: the test fails because webhook-specific UI helpers do not exist.

- [ ] **Step 3: Implement endpoint registration and one-time secret handling**

Validate the URL before submission. Require 'https://', reject embedded credentials, and leave private-host enforcement to the backend. Use the supported events exactly:

~~~text
order.created
order.payment_pending
order.payment_confirmed
order.asset_received
order.processing
order.completed
order.failed
order.expired
~~~

After 'POST /v1/webhook-endpoints' returns, show 'response.secret' once. The endpoint list contains metadata only. To rotate a secret, disable the endpoint and register a new one because there is no rotation route.

- [ ] **Step 4: Implement test delivery, delivery history, and replay**

Use 'POST /v1/webhook-endpoints/{id}' to queue a synthetic test. Explain that the test does not create an order or move money. Load 'GET /v1/webhook-deliveries' with 'endpoint_id', 'event_id', 'status', 'from', 'to', 'limit', and 'paging_id' filters.

Render 'in_flight', 'delivered', 'retry_scheduled', 'failed', 'exhausted', and 'superseded' distinctly. Show HTTP status, attempt number, scheduled time, completion time, duration, next attempt time, and safe error. Show a replay button only for 'exhausted'. Replay uses the same immutable event ID, so the UI must not claim that it created a new business event.

- [ ] **Step 5: Add common webhook failure states**

For '401', redirect to login. For '403', show Developer Mode setup. For '404', show that the endpoint or delivery is no longer available to this user. For '409' on replay, keep the current row and show that only exhausted deliveries can be replayed. For '500', keep the last successful table and show retry.

- [ ] **Step 6: Run focused tests and lint**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test -- features/developer/webhooks.test.mts
npm run lint
~~~

Expected result: tests pass and lint exits with code 0.

### Task 7: Update frontend documentation and verify the complete workspace

**Files:**
- Modify: 'documentations/FRONTEND-GUIDE.md'
- Modify: 'README.md'
- Test: existing API, feature, and formatting tests under 'lib/' and 'features/'

**Interfaces:**
- The documentation links to 'kailopay-be/documentations/backend/FE-AI-DASHBOARD-IMPLEMENTATION.md' and this plan.
- The public frontend route map lists only implemented dashboard routes.

- [ ] **Step 1: Document the route and permission model**

Document that '/developer/*' is session-authenticated and requires Developer Mode. State that API keys authenticate trusted server integrations, not the dashboard browser. State that KYC gates API-key creation only.

- [ ] **Step 2: Document the feature boundaries**

Document overview, analytics, orders, revenue, wallet profiles, webhooks, one-time secrets, simulated sandbox disclosures, and the absence of API telemetry. State that buy and sell API request bodies remain unchanged.

- [ ] **Step 3: Run the full verification baseline**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
npm test
npm run lint
npm run build
~~~

Expected result: all tests pass, ESLint exits with code 0, and the Next.js production build completes.

- [ ] **Step 4: Run the manual acceptance checks**

Verify these flows against the sandbox backend:

1. Sign in with Developer Mode disabled. '/developer' shows setup, while '/dashboard', '/buy', and '/sell' remain available.
2. Enable Developer Mode. The navigation exposes Overview, Analytics, Orders, Revenue, API keys, Wallets, and Webhooks, with no telemetry page.
3. Load an account with no orders. Overview, analytics, orders, and revenue render empty states.
4. Create an API key after approved KYC. The complete key appears once, copying works, and reopening the page shows only the prefix.
5. Register a testnet wallet through SEP-10. The wallet list shows public metadata only.
6. Register a webhook, copy its one-time secret, queue a synthetic test, inspect the delivery, and replay an exhausted delivery.
7. Create one existing on-ramp and one existing off-ramp order. Confirm their request bodies, idempotency behavior, checkout or deposit presentation, and polling behavior are unchanged.

- [ ] **Step 5: Inspect the diff**

Run:

~~~powershell
cd D:/Proyek/kailopay/kailopay-fe
git diff --check
git status --short
git diff --stat
~~~

Confirm that the diff adds developer dashboard behavior only, does not add API telemetry, does not embed secrets, and does not change the existing on-ramp or off-ramp request contract.

## Definition of done

The frontend agent can mark this plan complete only when all of the following are true:

- '/developer' is a working overview for a session user with Developer Mode enabled.
- Disabled Developer Mode produces a setup state instead of a blank page or an unauthorized data error.
- Overview and analytics use the backend filters and exact string amounts.
- Orders and revenue use cursor pagination and preserve sandbox disclosures.
- API-key creation is KYC-gated and reveals the complete key once only.
- Wallet registration uses a matching SEP-10 proof and stores no secret wallet material in the browser or backend request history.
- Webhook registration, one-time secret display, test delivery, delivery history, disabling, and exhausted replay work against the documented routes.
- No API telemetry UI exists.
- Existing consumer buy and sell flows still use the same on-ramp and off-ramp API contract.
- 'npm test', 'npm run lint', and 'npm run build' pass in 'kailopay-fe'.
