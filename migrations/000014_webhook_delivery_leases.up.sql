ALTER TABLE webhook_attempts
    ADD COLUMN lease_owner text,
    ADD COLUMN lease_until timestamptz;

CREATE INDEX idx_webhook_attempts_lease_until
    ON webhook_attempts (lease_until);
