# Persona KYC sandbox runbook

This runbook covers KailoPay's Persona integration for the `v0.1.0`
sandbox/testnet release. It is an identity-status gate for the demo, not
production KYC/AML, sanctions screening, or regulatory compliance.

## 1. Persona sandbox setup

Create or select the following in the Persona sandbox dashboard:

1. An inquiry template suitable for the demo's supported test identity flow.
2. A Persona sandbox environment.
3. A webhook endpoint pointing to the externally reachable KailoPay callback:
   `/callbacks/kyc/persona`.
4. The webhook event subscriptions for inquiry creation, start, completion,
   review, approval, decline, failure, expiry, and transitions.

Copy the resulting configuration into the local environment without committing
the values:

```text
PERSONA_BASE_URL=https://api.withpersona.com
PERSONA_API_KEY=<sandbox-api-key>
PERSONA_INQUIRY_TEMPLATE_ID=<inquiry-template-id>
PERSONA_ENVIRONMENT_ID=<sandbox-environment-id>
PERSONA_WEBHOOK_SECRET=<webhook-signing-secret>
PERSONA_TIMEOUT=10s
PERSONA_SIGNATURE_TOLERANCE=5m
PERSONA_MAX_RESPONSE_BYTES=1048576
```

`PERSONA_API_KEY` and `PERSONA_WEBHOOK_SECRET` are secrets. The template and
environment IDs are non-secret identifiers, but keep all Persona configuration
out of screenshots and public evidence unless the reviewer explicitly needs
it. `PERSONA_ENVIRONMENT_ID` remains required by this app because the KYC
endpoint passes it to the sandbox embedded client; the Persona inquiry-create
API itself can accept the template ID without that header.

## 2. Local verification flow

Start PostgreSQL and the API with the configured `.env`, then:

1. Register or sign in with a sandbox account and complete local email
   verification.
2. Open the frontend identity-verification route:
   `/verify-identity?return_to=/developer`.
3. Start the inquiry. The backend calls Persona and returns an inquiry ID,
   environment ID, and, when resuming, a short-lived session token.
4. Complete the Persona sandbox test flow in the embedded widget.
5. Wait for the signed Persona callback to reach
   `/callbacks/kyc/persona`.
6. Refresh `GET /v1/kyc` and confirm `status` is `approved`.
7. Create an API key or an order. Before approval, those mutations must return
   `403` with `error.code=KYC_REQUIRED`.

The browser callback is only a refresh signal. KailoPay trusts the verified
server callback and the persisted local status, not an approval value supplied
by browser JavaScript.

## 3. Local callback access

Persona must reach the callback over HTTPS. For local development, expose the
API through an approved tunnel and register the tunnel URL in Persona. Do not
put tunnel credentials, Persona secrets, identity documents, or raw webhook
bodies in the repository.

Check the callback response and safe logs:

- Valid event: `200 {"status":"accepted"}`.
- Unknown inquiry: `503` with `error.code=KYC_INQUIRY_NOT_FOUND` so Persona
  retries; this covers the race before a newly-created inquiry is attached
  locally.
- Invalid signature: `401` with `error.code=INVALID_CALLBACK_SIGNATURE`; no KYC
  state changes. In local mode, `error.details` explains whether the header,
  timestamp, signature, or payload failed. Production omits `details`.
- Conflicting duplicate event: `400` with
  `error.code=CALLBACK_EVENT_CONFLICT`; no second state change.
- Temporary database/provider processing failure: `503` with
  `error.code=CALLBACK_PROCESSING_FAILED`; Persona may retry.

The local diagnostic response is safe to paste into a bug report because it
does not include the webhook secret, signature value, or raw Persona body:

```json
{
  "error": {
    "code": "INVALID_CALLBACK_SIGNATURE",
    "message": "Callback signature verification failed.",
    "retryable": false,
    "details": "invalid kyc webhook: persona webhook signature does not match configured secret"
  },
  "request_id": "req_..."
}
```

## 4. Failure and replay handling

- Duplicate Persona delivery is expected and must be idempotent.
- Event order is not guaranteed. Unknown event statuses are ignored, stale
  events are recorded without changing state, and a newer adverse decision can
  revoke an earlier approval.
- A callback timestamp outside `PERSONA_SIGNATURE_TOLERANCE` is rejected.
- If inquiry creation or resume fails, retry through the frontend; do not create
  a replacement local inquiry manually unless the existing record is terminal.
- If a callback times out after being accepted by the API, query `GET /v1/kyc`
  before retrying the user action.

## 5. Evidence and privacy

Safe evidence includes the release version, local inquiry ID, provider event ID,
normalized status, timestamps, body hash, response status, and request ID.

Never publish:

- Persona API keys, webhook secrets, or session tokens.
- Raw Persona webhook JSON or identity documents.
- Full user identity details, screenshots of documents, or unredacted logs.

The integration stores normalized provider status and correlation metadata in
PostgreSQL. Persona remains the system handling the identity-verification
details; KailoPay's local record is an authorization gate for this sandbox
application.
