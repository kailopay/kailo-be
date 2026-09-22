CREATE TABLE sep10_challenges (
    id uuid PRIMARY KEY,
    challenge_hash bytea NOT NULL UNIQUE,
    account text NOT NULL,
    home_domain text NOT NULL,
    network text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_sep10_challenges_expiry
    ON sep10_challenges(expires_at)
    WHERE consumed_at IS NULL;

CREATE TABLE sep38_quotes (
    id uuid PRIMARY KEY,
    quote_id text NOT NULL UNIQUE,
    wallet_account text NOT NULL,
    sell_asset text NOT NULL,
    buy_asset text NOT NULL,
    sell_amount text NOT NULL,
    buy_amount text NOT NULL,
    price text NOT NULL,
    spread_bps integer NOT NULL CHECK (spread_bps BETWEEN 0 AND 10000),
    delivery_method text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX idx_sep38_quotes_owner_expiry
    ON sep38_quotes(wallet_account, expires_at DESC, quote_id DESC);

CREATE TABLE sep24_interactive_sessions (
    id uuid PRIMARY KEY,
    transaction_id text NOT NULL UNIQUE CHECK (char_length(transaction_id) BETWEEN 1 AND 255),
    kind text NOT NULL CHECK (kind IN ('deposit', 'withdraw', 'withdrawal')),
    wallet_account text NOT NULL,
    request_hash bytea NOT NULL,
    request_payload jsonb NOT NULL,
    asset_code text NOT NULL,
    asset_amount text,
    fiat_amount_minor bigint,
    payment_method text,
    destination_token bytea,
    quote_id text,
    linked_user_id uuid REFERENCES users(id),
    linked_session_id uuid REFERENCES retail_sessions(id),
    kyc_status text NOT NULL CHECK (kyc_status IN ('pending', 'approved', 'completed', 'expired')),
    order_id uuid UNIQUE REFERENCES orders(id),
    expires_at timestamptz NOT NULL,
    completed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (wallet_account, transaction_id)
);
CREATE INDEX idx_sep24_interactive_wallet_expiry
    ON sep24_interactive_sessions(wallet_account, expires_at DESC, transaction_id DESC);
CREATE INDEX idx_sep24_interactive_user
    ON sep24_interactive_sessions(linked_user_id, updated_at DESC)
    WHERE linked_user_id IS NOT NULL;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS wallet_account text,
    ADD COLUMN IF NOT EXISTS quote_id text;
CREATE INDEX IF NOT EXISTS idx_orders_wallet_created
    ON orders(wallet_account, created_at DESC, id DESC)
    WHERE wallet_account IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orders_quote_id
    ON orders(quote_id)
    WHERE quote_id IS NOT NULL;

ALTER TABLE sep24_transactions
    ADD COLUMN IF NOT EXISTS wallet_account text,
    ADD COLUMN IF NOT EXISTS quote_id text,
    ADD COLUMN IF NOT EXISTS stellar_transaction_id text,
    ADD COLUMN IF NOT EXISTS external_transaction_id text;
CREATE INDEX IF NOT EXISTS idx_sep24_transactions_wallet_created
    ON sep24_transactions(wallet_account, created_at DESC, id DESC)
    WHERE wallet_account IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sep24_transactions_stellar_id
    ON sep24_transactions(stellar_transaction_id)
    WHERE stellar_transaction_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_sep24_transactions_external_id
    ON sep24_transactions(external_transaction_id)
    WHERE external_transaction_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_offramp_payouts_reference_id
    ON offramp_payouts(reference_id);
