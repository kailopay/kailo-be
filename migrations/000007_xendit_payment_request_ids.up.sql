ALTER TABLE payment_checkouts
    ADD COLUMN provider_payment_request_id text;

CREATE UNIQUE INDEX idx_payment_checkouts_provider_request
    ON payment_checkouts(provider, provider_payment_request_id)
    WHERE provider_payment_request_id IS NOT NULL;
