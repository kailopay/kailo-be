-- Week 2 off-ramp (ADR-003): allow offramp orders, their lifecycle states,
-- deposit/retirement transaction purposes, and the simulated payout record.

ALTER TABLE orders DROP CONSTRAINT orders_direction_check;
ALTER TABLE orders ADD CONSTRAINT orders_direction_check
    CHECK (direction IN ('onramp', 'offramp'));

ALTER TABLE orders DROP CONSTRAINT orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status IN (
    'created', 'payment_pending', 'payment_confirmed', 'stellar_processing',
    'completed', 'expired', 'payment_failed', 'stellar_failed', 'cancelled',
    'asset_pending', 'asset_received', 'asset_invalid',
    'retirement_processing', 'withdrawal_processing',
    'retirement_failed', 'withdrawal_failed'
));

-- Off-ramp orders carry no payment gateway checkout.
ALTER TABLE orders ALTER COLUMN payment_method DROP NOT NULL;
ALTER TABLE orders ALTER COLUMN gateway_provider DROP NOT NULL;

-- The fiat side of an off-ramp is the withdrawal, not a Stellar destination;
-- the deposit flow instead uses stellar_source plus a required memo.
ALTER TABLE orders ALTER COLUMN stellar_destination DROP NOT NULL;

ALTER TABLE orders ADD COLUMN withdrawal_method text
    CONSTRAINT orders_withdrawal_method_check CHECK (withdrawal_method = 'sandbox_bank_transfer');

ALTER TABLE stellar_transactions DROP CONSTRAINT stellar_transactions_purpose_check;
ALTER TABLE stellar_transactions ADD CONSTRAINT stellar_transactions_purpose_check
    CHECK (purpose IN ('transfer', 'deposit', 'retirement'));

CREATE TABLE offramp_payouts (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
    method text NOT NULL CHECK (method = 'sandbox_bank_transfer'),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    reference_id text NOT NULL UNIQUE,
    state text NOT NULL CHECK (state IN ('pending', 'completed', 'failed')),
    completed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
