DROP INDEX IF EXISTS idx_webhook_events_client_created;

DELETE FROM outbox_messages
WHERE topic = 'webhook.deliver'
  AND aggregate_id IN (SELECT id FROM webhook_events WHERE is_test = true);

DELETE FROM webhook_attempts
WHERE event_id IN (SELECT id FROM webhook_events WHERE is_test = true);

DELETE FROM webhook_events WHERE is_test = true;

ALTER TABLE webhook_events
    DROP COLUMN IF EXISTS is_test,
    DROP COLUMN IF EXISTS client_id,
    ALTER COLUMN order_id SET NOT NULL;
