DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM idempotency_records WHERE retail_user_id IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot roll back consumer order ownership while retail idempotency records exist';
    END IF;
END $$;

DROP INDEX idx_orders_retail_user_created;
DROP INDEX idx_idempotency_retail_user;
DROP INDEX idx_idempotency_api_client;

ALTER TABLE idempotency_records
    DROP CONSTRAINT idempotency_records_one_owner_check;

ALTER TABLE idempotency_records
    DROP COLUMN retail_user_id;

ALTER TABLE idempotency_records
    ALTER COLUMN client_id SET NOT NULL;

ALTER TABLE idempotency_records
    ADD CONSTRAINT idempotency_records_client_id_operation_key_hash_key
    UNIQUE (client_id, operation, key_hash);
