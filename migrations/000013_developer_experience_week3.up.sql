CREATE TABLE order_financials (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
    client_id uuid REFERENCES api_clients(id),
    direction text NOT NULL CHECK (direction IN ('onramp', 'offramp')),
    environment text NOT NULL CHECK (environment = 'test'),
    network text NOT NULL CHECK (network = 'stellar_testnet'),
    currency text NOT NULL CHECK (currency = 'IDR'),
    asset_code text NOT NULL CHECK (asset_code = 'XLM'),
    asset_issuer text NOT NULL DEFAULT '',
    asset_amount numeric(30,18) NOT NULL CHECK (asset_amount > 0),
    asset_amount_stroops bigint NOT NULL CHECK (asset_amount_stroops > 0),
    gross_amount_minor bigint NOT NULL CHECK (gross_amount_minor > 0),
    fee_amount_minor bigint NOT NULL CHECK (fee_amount_minor >= 0),
    platform_revenue_minor bigint NOT NULL CHECK (platform_revenue_minor >= 0),
    developer_revenue_minor bigint NOT NULL CHECK (developer_revenue_minor >= 0),
    net_amount_minor bigint NOT NULL CHECK (net_amount_minor >= 0),
    fee_currency text NOT NULL CHECK (fee_currency = 'IDR'),
    fee_policy_version text NOT NULL,
    source text NOT NULL,
    quote_spread_bps integer NOT NULL CHECK (quote_spread_bps BETWEEN 0 AND 10000),
    simulated boolean NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (gross_amount_minor = fee_amount_minor + net_amount_minor),
    CHECK (platform_revenue_minor + developer_revenue_minor <= fee_amount_minor)
);
CREATE INDEX idx_order_financials_client_created
    ON order_financials(client_id, created_at DESC, order_id DESC);
CREATE INDEX idx_order_financials_direction_created
    ON order_financials(direction, created_at DESC);

CREATE TABLE developer_wallets (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    client_id uuid REFERENCES api_clients(id),
    network text NOT NULL CHECK (network = 'stellar_testnet'),
    wallet_account text NOT NULL,
    label text NOT NULL,
    is_primary boolean NOT NULL,
    verification_method text NOT NULL CHECK (verification_method = 'sep10'),
    verified_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'revoked')),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (user_id, network, wallet_account),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);
CREATE INDEX idx_developer_wallets_user_updated
    ON developer_wallets(user_id, updated_at DESC);
CREATE UNIQUE INDEX idx_developer_wallet_primary
    ON developer_wallets(user_id, network)
    WHERE is_primary AND status = 'active';
