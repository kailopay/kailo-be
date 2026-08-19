CREATE TABLE users (
    id uuid PRIMARY KEY,
    status text NOT NULL CHECK (status IN ('active', 'disabled')),
    display_name text NOT NULL,
    email text,
    avatar_object_key text,
    email_verified_at timestamptz,
    developer_enabled_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE user_identities (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz NOT NULL,
    last_login_at timestamptz,
    UNIQUE (provider, subject)
);
CREATE INDEX idx_user_identities_user ON user_identities(user_id);

CREATE TABLE auth_transactions (
    id uuid PRIMARY KEY,
    state_hash bytea NOT NULL UNIQUE,
    nonce_hash bytea NOT NULL,
    code_verifier_ciphertext bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_auth_transactions_expiry ON auth_transactions(expires_at);

CREATE TABLE retail_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX idx_retail_sessions_user ON retail_sessions(user_id);

CREATE TABLE api_clients (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id),
    name text NOT NULL,
    environment text NOT NULL CHECK (environment = 'test'),
    status text NOT NULL CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX idx_api_clients_owner ON api_clients(owner_user_id, created_at DESC);
CREATE UNIQUE INDEX idx_api_clients_owner_environment ON api_clients(owner_user_id, environment);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY,
    client_id uuid NOT NULL REFERENCES api_clients(id),
    public_id text NOT NULL UNIQUE,
    prefix text NOT NULL,
    secret_hash bytea NOT NULL,
    created_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX idx_api_keys_client ON api_keys(client_id);

CREATE TABLE treasury_accounts (
    id uuid PRIMARY KEY,
    network text NOT NULL CHECK (network = 'stellar_testnet'),
    public_account text NOT NULL,
    observed_balance_stroops bigint NOT NULL CHECK (observed_balance_stroops >= 0),
    reserved_stroops bigint NOT NULL CHECK (reserved_stroops >= 0),
    operating_buffer_stroops bigint NOT NULL CHECK (operating_buffer_stroops >= 0),
    last_reconciled_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (network, public_account),
    CHECK (reserved_stroops <= observed_balance_stroops)
);

CREATE TABLE orders (
    id uuid PRIMARY KEY,
    client_id uuid REFERENCES api_clients(id),
    created_by_user_id uuid REFERENCES users(id),
    retail_session_id uuid REFERENCES retail_sessions(id),
    direction text NOT NULL CHECK (direction = 'onramp'),
    status text NOT NULL CHECK (status IN (
        'created', 'payment_pending', 'payment_confirmed', 'stellar_processing',
        'completed', 'expired', 'payment_failed', 'stellar_failed', 'cancelled'
    )),
    version integer NOT NULL CHECK (version > 0),
    currency text NOT NULL CHECK (currency = 'IDR'),
    fiat_amount_minor bigint NOT NULL CHECK (fiat_amount_minor > 0),
    asset_code text NOT NULL CHECK (asset_code = 'XLM'),
    asset_issuer text NOT NULL DEFAULT '',
    network text NOT NULL CHECK (network = 'stellar_testnet'),
    asset_amount numeric(30,18) NOT NULL CHECK (asset_amount > 0),
    asset_amount_stroops bigint NOT NULL CHECK (asset_amount_stroops > 0),
    quote_provider text NOT NULL,
    quote_source_at timestamptz NOT NULL,
    quote_rate numeric(30,18) NOT NULL CHECK (quote_rate > 0),
    quote_adjusted_rate numeric(30,18) NOT NULL CHECK (quote_adjusted_rate > 0),
    quote_spread_bps integer NOT NULL CHECK (quote_spread_bps BETWEEN 0 AND 10000),
    quote_expires_at timestamptz NOT NULL,
    payment_method text NOT NULL CHECK (payment_method IN ('qris', 'bank_transfer')),
    gateway_provider text NOT NULL CHECK (gateway_provider = 'xendit'),
    stellar_source text,
    stellar_destination text NOT NULL,
    stellar_memo text,
    withdrawal_destination jsonb,
    expires_at timestamptz,
    failure_code text,
    failure_stage text,
    failure_retryable boolean,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    completed_at timestamptz,
    CHECK (
        (client_id IS NOT NULL AND retail_session_id IS NULL)
        OR
        (client_id IS NULL AND retail_session_id IS NOT NULL AND created_by_user_id IS NOT NULL)
    )
);
CREATE INDEX idx_orders_client_created ON orders(client_id, created_at DESC, id DESC);
CREATE INDEX idx_orders_status_updated ON orders(status, updated_at);

CREATE TABLE treasury_reservations (
    id uuid PRIMARY KEY,
    treasury_id uuid NOT NULL REFERENCES treasury_accounts(id),
    order_id uuid NOT NULL UNIQUE REFERENCES orders(id),
    amount_stroops bigint NOT NULL CHECK (amount_stroops > 0),
    status text NOT NULL CHECK (status IN ('reserved', 'consumed', 'released')),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    released_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((status = 'consumed') = (consumed_at IS NOT NULL)),
    CHECK ((status = 'released') = (released_at IS NOT NULL))
);
CREATE INDEX idx_treasury_reservations_status_expiry ON treasury_reservations(status, expires_at);

CREATE TABLE order_events (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES orders(id),
    aggregate_version integer NOT NULL CHECK (aggregate_version > 0),
    event_type text NOT NULL,
    previous_status text,
    new_status text,
    source text NOT NULL,
    correlation_id text NOT NULL,
    metadata jsonb,
    created_at timestamptz NOT NULL,
    UNIQUE (order_id, aggregate_version)
);

CREATE TABLE payment_checkouts (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES orders(id),
    provider text NOT NULL CHECK (provider = 'xendit'),
    provider_checkout_id text NOT NULL,
    method text NOT NULL CHECK (method IN ('qris', 'bank_transfer')),
    currency text NOT NULL CHECK (currency = 'IDR'),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    status text NOT NULL,
    presentation_reference text,
    expires_at timestamptz,
    metadata jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (provider, provider_checkout_id)
);
CREATE INDEX idx_payment_checkouts_order ON payment_checkouts(order_id, created_at);

CREATE TABLE gateway_events (
    id uuid PRIMARY KEY,
    provider text NOT NULL CHECK (provider = 'xendit'),
    provider_event_id text NOT NULL,
    event_type text NOT NULL,
    checkout_reference text,
    order_reference text,
    payload_hash text NOT NULL,
    signature_verified boolean NOT NULL,
    matching_result text,
    received_at timestamptz NOT NULL,
    processed_at timestamptz,
    processing_status text NOT NULL,
    processing_error text,
    UNIQUE (provider, provider_event_id)
);

CREATE TABLE stellar_transactions (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES orders(id),
    intent_id text NOT NULL UNIQUE,
    purpose text NOT NULL CHECK (purpose = 'transfer'),
    network text NOT NULL CHECK (network = 'stellar_testnet'),
    asset_code text NOT NULL CHECK (asset_code = 'XLM'),
    amount numeric(30,18) NOT NULL CHECK (amount > 0),
    source text,
    destination text,
    memo text,
    transaction_hash text UNIQUE,
    status text NOT NULL CHECK (status IN ('pending', 'submitted', 'confirmed', 'failed', 'unknown')),
    attempt_count integer NOT NULL CHECK (attempt_count >= 0),
    ledger_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (order_id, purpose)
);

CREATE TABLE idempotency_records (
    id uuid PRIMARY KEY,
    client_id uuid NOT NULL REFERENCES api_clients(id),
    operation text NOT NULL,
    key_hash text NOT NULL,
    request_hash text NOT NULL,
    response_status integer,
    response_body jsonb,
    created_resource_id uuid,
    state text NOT NULL CHECK (state IN ('processing', 'completed', 'failed')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (client_id, operation, key_hash)
);

CREATE TABLE outbox_messages (
    id uuid PRIMARY KEY,
    topic text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    lease_owner text,
    lease_until timestamptz,
    attempts integer NOT NULL CHECK (attempts >= 0),
    processed_at timestamptz,
    last_error text
);
CREATE INDEX idx_outbox_available ON outbox_messages(available_at)
    WHERE processed_at IS NULL;

CREATE TABLE webhook_endpoints (
    id uuid PRIMARY KEY,
    client_id uuid NOT NULL REFERENCES api_clients(id),
    url text NOT NULL,
    status text NOT NULL,
    secret_reference text NOT NULL,
    event_types jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    disabled_at timestamptz
);

CREATE TABLE webhook_events (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES orders(id),
    event_type text NOT NULL,
    api_version text NOT NULL,
    canonical_payload jsonb NOT NULL,
    source_order_event_id uuid REFERENCES order_events(id),
    created_at timestamptz NOT NULL
);

CREATE TABLE webhook_attempts (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES webhook_events(id),
    endpoint_id uuid NOT NULL REFERENCES webhook_endpoints(id),
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    status text NOT NULL,
    scheduled_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    http_status integer,
    duration_millis bigint,
    response_body_hash text,
    safe_error text,
    next_attempt_at timestamptz,
    UNIQUE (event_id, endpoint_id, attempt_number)
);
