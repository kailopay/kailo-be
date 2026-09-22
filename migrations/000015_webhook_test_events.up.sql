ALTER TABLE webhook_events
    ALTER COLUMN order_id DROP NOT NULL,
    ADD COLUMN client_id uuid REFERENCES api_clients(id),
    ADD COLUMN is_test boolean NOT NULL DEFAULT false;

CREATE INDEX idx_webhook_events_client_created
    ON webhook_events (client_id, created_at DESC);
