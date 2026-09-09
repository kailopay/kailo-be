DROP INDEX IF EXISTS idx_payment_checkouts_provider_request;

ALTER TABLE payment_checkouts
    DROP COLUMN IF EXISTS provider_payment_request_id;
