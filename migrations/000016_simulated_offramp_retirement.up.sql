ALTER TABLE stellar_transactions
    DROP CONSTRAINT IF EXISTS stellar_transactions_status_check;

ALTER TABLE stellar_transactions
    ADD CONSTRAINT stellar_transactions_status_check
    CHECK (status IN ('pending', 'submitted', 'confirmed', 'failed', 'unknown', 'simulated'));

ALTER TABLE stellar_transactions
    ADD CONSTRAINT stellar_transactions_simulated_no_chain_evidence_check
CHECK (status <> 'simulated' OR (purpose = 'retirement' AND transaction_hash IS NULL AND ledger_at IS NULL));
