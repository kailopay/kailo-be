DROP TABLE IF EXISTS offramp_payouts;

ALTER TABLE stellar_transactions DROP CONSTRAINT stellar_transactions_purpose_check;
ALTER TABLE stellar_transactions ADD CONSTRAINT stellar_transactions_purpose_check
    CHECK (purpose = 'transfer');

ALTER TABLE orders ALTER COLUMN stellar_destination SET NOT NULL;
ALTER TABLE orders DROP CONSTRAINT orders_withdrawal_method_check;
ALTER TABLE orders DROP COLUMN withdrawal_method;
ALTER TABLE orders ALTER COLUMN payment_method SET NOT NULL;
ALTER TABLE orders ALTER COLUMN gateway_provider SET NOT NULL;
DELETE FROM orders WHERE direction <> 'onramp';
ALTER TABLE orders DROP CONSTRAINT orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status IN (
    'created', 'payment_pending', 'payment_confirmed', 'stellar_processing',
    'completed', 'expired', 'payment_failed', 'stellar_failed', 'cancelled'
));
ALTER TABLE orders DROP CONSTRAINT orders_direction_check;
ALTER TABLE orders ADD CONSTRAINT orders_direction_check CHECK (direction = 'onramp');
