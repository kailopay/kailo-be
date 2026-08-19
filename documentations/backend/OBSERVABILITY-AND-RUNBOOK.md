# Observability and Runbook

## 1. Objectives

Observability must answer:

- What happened to an order and why?
- Did a gateway callback authenticate and reconcile?
- Was a Stellar transaction submitted, confirmed, failed, or left unknown?
- Was a developer webhook delivered or exhausted?
- Is the API/worker ready, and which external dependency is degraded?

The sandbox requires actionable diagnostics and evidence, not a production 24/7 SRE program.

## 2. Structured logging

Use JSON logs with consistent fields:

| Field | Purpose |
|---|---|
| `timestamp`, `level`, `message` | Basic event |
| `service`, `process`, `release_version`, `environment` | Deployment identity |
| `request_id`, `correlation_id` | Request/job trace |
| `route`, `method`, `status`, `duration` | HTTP outcome using bounded route templates |
| `order_id`, `client_id` | Business correlation |
| `provider`, `provider_reference`, `gateway_event_id` | Payment correlation |
| `stellar_intent_id`, `stellar_tx_hash` | Testnet correlation |
| `webhook_event_id`, `webhook_attempt_id` | Delivery correlation |
| `error_code`, `retryable`, `duration_ms` | Outcome diagnostics |

Redact secret-bearing headers/fields centrally. Do not log raw unrestricted request/callback/response bodies.

For local development, use `LOG_FORMAT=text` for readable terminal output. Keep
`LOG_LEVEL=info` to avoid debug noise. HTTP access logs use severity by status:
successful requests are `DEBUG`, client errors are `WARN`, and server errors are
`ERROR`. Production should use `LOG_FORMAT=json`; raise the level to `debug` only
for a short, controlled investigation.

## 3. Metrics

Minimum metrics:

### HTTP

- Request count by route template, method, status class.
- Duration histogram by route template.
- Authentication failures and rate-limit count.

### Orders

- Orders created/completed/failed/expired by direction.
- Current non-terminal orders by state.
- State-transition count and transition rejection count.
- End-to-end completion duration by direction.

### Payment gateway

- Checkout creation outcome/duration by method.
- Callback received, authentication failed, unmatched, mismatched, duplicate, processed.
- Payout outcome/duration.
- Unknown operations awaiting reconciliation.

### Stellar

- Submission outcome/duration by purpose.
- Unknown submissions and reconciliation outcomes.
- Processing-state age.
- Sequence or insufficient-balance/trustline errors.

### Webhooks/workers

- Outbox backlog count/oldest age.
- Job attempts, retry count, exhausted count.
- Webhook delivery outcome/duration by status class.
- Worker lease/poll errors.

Avoid high-cardinality IDs as metric labels; keep IDs in logs/traces.

## 4. Health and readiness

`GET /livez` (also `/health` and `/healthz` aliases):

- Confirms the process is alive.
- Performs no database or external dependency check, avoiding restart loops
  during a dependency outage.
- Returns only a minimal status; it does not expose config, dependency URLs, or
  secrets.

`GET /readyz` (also `/ready` alias):

- Runs the configured readiness checks with a bounded timeout.
- Returns `503` when the database check fails or times out.
- Keeps dependency error details in structured logs, not in the public body.

`GET /startupz`:

- Returns `503` until one-time process initialization completes.
- Returns `200` after required startup dependencies have initialized.
- Worker processes may expose a separate readiness/heartbeat contract.

External gateway or Stellar degradation should be reported through metrics/status diagnostics and operation errors. Decide whether it fails readiness based on whether routing traffic would worsen the incident; document the choice.

## 5. Dashboard/review queries

At minimum, provide safe queries or a lightweight dashboard to find:

- Orders by ID/status/direction and age.
- Order event timeline.
- Payment checkout and callback matching results.
- Stellar intents/transactions by order and state.
- Outbox messages and webhook attempts by order/event.
- Non-terminal orders older than expected thresholds.

These tools must not expose secrets or require a public admin endpoint.

## 6. General incident workflow

1. Record time, release version, environment, reporter, and observed order/reference.
2. Confirm sandbox/testnet environment before any action.
3. Inspect current order, immutable event history, external intents, and correlated logs.
4. Classify whether the issue is input, provider, Stellar, database, deployment, or application logic.
5. For unknown external outcomes, reconcile before retry.
6. Use only documented idempotent recovery commands/procedures.
7. Capture sanitized evidence and final resolution.
8. Add or update a regression test for application defects.

## 7. Runbook: API unavailable

Checks:

- `/livez`, `/readyz`, and `/startupz` (plus compatibility aliases).
- Deployment status and release version.
- Startup/configuration errors.
- Database connectivity and migration version.
- Resource exhaustion or crash loop.

Recovery:

- Correct configuration through secret/config management.
- Apply missing tested migration through the one-shot migration process.
- Restart/rollback to the last verified release.
- Verify health, readiness, one authenticated read, and no duplicated workers/jobs.

Never print the full environment or database URL during diagnosis.

## 8. Runbook: payment confirmed but asset not delivered

1. Find order and verified gateway event.
2. Confirm amount/currency/reference reconciliation succeeded.
3. Inspect issuance intent, outbox message, and worker attempts.
4. If a transaction hash exists, query Stellar testnet and reconcile it.
5. If submission outcome is unknown, query source account/sequence/memo/history before retry.
6. Retry only the existing idempotent intent after proving no successful prior effect.
7. Confirm final order transition and developer webhook.

Do not manually create an unrelated Stellar transfer as a shortcut; it breaks auditability and can duplicate settlement.

## 9. Runbook: asset received but withdrawal incomplete

1. Verify deposit transaction matches asset, issuer, amount, destination, memo/order, and testnet.
2. Inspect retirement intent and transaction outcome.
3. Reconcile unknown retirement before resubmission.
4. Confirm retirement evidence exists before payout processing.
5. Inspect payout intent/provider status or approved simulation result.
6. Reconcile unknown provider outcome before retry.
7. Update order through the application recovery path, not direct SQL state edits.

## 10. Runbook: duplicate callback

- Confirm provider event/fingerprint unique constraint identified the duplicate.
- Verify only one payment-confirmed event and settlement intent exist.
- Return provider-compatible acknowledgement.
- If a duplicate effect occurred, stop processing and preserve all evidence; do not delete records. Treat it as a high-severity correctness defect.

## 11. Runbook: webhook delivery failure

- Validate endpoint is active and passes SSRF policy.
- Inspect attempt status, HTTP code, duration, and safe error.
- Respect retry policy; do not retry permanent `4xx` blindly.
- For manual replay, retain the same public event ID and add an attempt.
- Ask consumer to verify signature using raw body/timestamp and deduplicate by event ID.

## 12. Runbook: suspected secret exposure

1. Stop publishing/sharing the affected artifact.
2. Identify secret type and affected environment without copying it into tickets/logs.
3. Revoke/rotate gateway, API, webhook, database, or testnet credentials.
4. Replace/fund new testnet accounts if a Stellar seed was exposed.
5. Scrub public evidence/repository through an approved history-remediation process where necessary.
6. Verify old credentials no longer work.
7. Document root cause and prevention without reproducing the secret.

## 13. Deployment and rollback checklist

Before deploy:

- CI tests/scans pass.
- Environment asserts sandbox/testnet.
- Backup/restore point exists where supported.
- Migration reviewed and tested from the current version.
- Release version and OpenAPI/docs are aligned.

Deploy:

1. Run migration once.
2. Start API and verify readiness.
3. Start worker with one active consumer initially.
4. Run smoke tests: health, auth, create test order/controlled integration check.
5. Monitor error/outbox/non-terminal metrics.

Rollback:

- Roll back application only when schema remains backward compatible.
- Do not reverse destructive migrations without a tested plan.
- Pause workers if new jobs are unsafe for the old release.
- Reconcile in-flight external operations before replay.

## 14. Evidence continuity

Because live dependencies may be unavailable during review, retain:

- Sanitized screenshots and request/response examples.
- Order IDs and immutable event histories.
- Provider sandbox references and callback-processing logs.
- Stellar testnet hashes and explorer links.
- Webhook event/attempt logs.
- Release tag, OpenAPI, test result, and demo recording.

Evidence complements live URLs; it does not justify claiming behavior that never completed.
