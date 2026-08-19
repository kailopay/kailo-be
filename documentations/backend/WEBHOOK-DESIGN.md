# Developer Webhook Design

## 1. Delivery semantics

KailoPay developer webhooks use **at-least-once delivery**. Consumers must deduplicate by stable event ID. Event ordering is not globally guaranteed; `created_at` and the order's current status/version help consumers reconcile.

Outgoing developer webhooks are distinct from inbound payment-gateway callbacks.

## 2. Public event catalog

Minimum `v0.1.0` events:

| Event type | Emitted when |
|---|---|
| `order.created` | On-ramp or off-ramp order is durably created |
| `order.payment_pending` | Sandbox checkout is available |
| `order.payment_confirmed` | Gateway payment is verified and reconciled |
| `order.asset_received` | Valid off-ramp test asset is received |
| `order.processing` | External settlement/retirement/withdrawal is processing |
| `order.completed` | All required external evidence for success is durable |
| `order.failed` | Order reaches a terminal failure state |
| `order.expired` | Required user action was not completed before expiry |

Avoid exposing every internal event. Public events describe stable integration-relevant lifecycle changes.

## 3. Event envelope

```json
{
  "id": "evt_01J...",
  "object": "event",
  "type": "order.completed",
  "api_version": "2026-08-01",
  "created_at": "2026-08-18T12:10:00Z",
  "environment": "sandbox",
  "data": {
    "object": {
      "id": "01J...",
      "object": "order",
      "direction": "onramp",
      "status": "completed",
      "stellar_transaction_hash": "..."
    }
  }
}
```

Payloads contain the public order representation or a documented stable subset, never secret/private provider fields.

## 4. Endpoint registration

Registration requirements:

- HTTPS URL for hosted review; local development may use explicitly allowed tunnel tooling.
- Resolve and validate URL against SSRF rules before activation.
- Reject loopback, link-local, private network, metadata-service, embedded-credential, and unsupported-port destinations unless an explicit safe local-development mode is active.
- Re-resolve and revalidate at delivery time to reduce DNS rebinding risk.
- Issue/show signing secret once; store encrypted or by secret reference.
- Support disable/revoke without deleting delivery history.

## 5. Signing

Recommended request headers:

```http
KailoPay-Event-Id: evt_...
KailoPay-Timestamp: 1787055000
KailoPay-Signature: v1=<hex_hmac_sha256>
Content-Type: application/json
```

Canonical signed bytes:

```text
<timestamp>.<exact_raw_request_body>
```

Consumer verification:

1. Read raw body bytes.
2. Reject timestamps outside documented tolerance.
3. Compute HMAC-SHA256 with endpoint secret.
4. Constant-time compare against an allowed `v1` signature.
5. Deduplicate by event ID.
6. Return `2xx` quickly, then process asynchronously.

Secret rotation is P1 if time allows; at minimum endpoint recreation must issue a new secret.

## 6. Transactional production

When an order event requiring a public webhook is committed:

1. Create a public webhook event/outbox message in the same database transaction.
2. Worker resolves active subscribed endpoints.
3. Create/lease one attempt per event and endpoint.
4. Sign exact serialized bytes and send with strict timeouts and redirects disabled.
5. Persist HTTP status/duration and schedule retry if eligible.

This prevents an order transition from being committed without durable webhook intent.

## 7. Retry policy

Suggested sandbox schedule: immediate, 1 minute, 5 minutes, 30 minutes, and 2 hours. Make the schedule configurable and document the final values.

Retry:

- Network errors/timeouts.
- `408`, `409` only if documented as retryable by the consumer contract, `425`, `429`, and `5xx`.

Do not retry automatically:

- Successful `2xx`.
- Most `3xx` because redirects are disabled.
- Permanent `4xx` such as `400`, `401`, `403`, `404`, `410`, or `422`.

After the final attempt, mark delivery exhausted and retain diagnostic evidence. A manual sandbox replay must create a new attempt for the same event ID, not a new business event.

## 8. Timeout and response handling

- Short connect and total timeouts; consumers should acknowledge quickly.
- Do not follow redirects.
- Limit DNS resolution, response size, and downloaded body.
- Store only status, duration, selected safe headers, response hash, and a short redacted diagnostic.
- Never log authorization/cookie headers or unrestricted response bodies.

## 9. Ordering and duplicates

- Event ID is stable across attempts.
- A consumer may receive a later state before an earlier retry.
- Payload includes creation time and current order status/version.
- Consumer documentation instructs retrieval of current order state when uncertain.
- KailoPay never promises exactly-once or globally ordered delivery.

## 10. Test endpoint and evidence

The developer UI/API should allow a controlled test event or use a real sandbox order. Evidence includes:

- Endpoint registration with safe destination.
- Signature verification result.
- Successful delivery attempt.
- Retry after an intentional temporary failure.
- Duplicate delivery using the same event ID.
- Sanitized attempt logs mapped to an order.

## 11. Acceptance checks

- Event is durable when the corresponding order transition commits.
- Invalid/private/metadata destinations are rejected.
- Request signature validates using documented sample code.
- Retry schedule is bounded and respects retryable responses.
- Duplicate delivery preserves event ID.
- Disabling an endpoint prevents new attempts without erasing audit history.
- Payload contains no API keys, signing secrets, Stellar secret seeds, or raw gateway credentials.
