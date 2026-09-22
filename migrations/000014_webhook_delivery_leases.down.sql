DROP INDEX IF EXISTS idx_webhook_attempts_lease_until;

ALTER TABLE webhook_attempts
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS lease_until;
