-- Consumer orders are owned by a verified retail user/session instead of an
-- API client. Keep API-client records intact while adding the second scope.
ALTER TABLE idempotency_records
    ALTER COLUMN client_id DROP NOT NULL;

ALTER TABLE idempotency_records
    ADD COLUMN retail_user_id uuid REFERENCES users(id);

ALTER TABLE idempotency_records
    DROP CONSTRAINT idempotency_records_client_id_operation_key_hash_key;

ALTER TABLE idempotency_records
    ADD CONSTRAINT idempotency_records_one_owner_check CHECK (
        (client_id IS NOT NULL AND retail_user_id IS NULL)
        OR
        (client_id IS NULL AND retail_user_id IS NOT NULL)
    );

CREATE UNIQUE INDEX idx_idempotency_api_client
    ON idempotency_records(client_id, operation, key_hash)
    WHERE client_id IS NOT NULL;

CREATE UNIQUE INDEX idx_idempotency_retail_user
    ON idempotency_records(retail_user_id, operation, key_hash)
    WHERE retail_user_id IS NOT NULL;

CREATE INDEX idx_orders_retail_user_created
    ON orders(created_by_user_id, created_at DESC, id DESC)
    WHERE client_id IS NULL AND retail_session_id IS NOT NULL;
