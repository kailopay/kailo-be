DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM stellar_transactions WHERE status = 'simulated') THEN
        RAISE EXCEPTION 'cannot roll back while simulated retirement evidence exists';
    END IF;
END;
$$;

ALTER TABLE stellar_transactions
    DROP CONSTRAINT IF EXISTS stellar_transactions_simulated_no_chain_evidence_check;

ALTER TABLE stellar_transactions
    DROP CONSTRAINT IF EXISTS stellar_transactions_status_check;

ALTER TABLE stellar_transactions
    ADD CONSTRAINT stellar_transactions_status_check
CHECK (status IN ('pending', 'submitted', 'confirmed', 'failed', 'unknown'));
