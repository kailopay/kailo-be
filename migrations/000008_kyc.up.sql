CREATE TABLE kyc_inquiries (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider = 'persona'),
    provider_inquiry_id text,
    provider_request_key text NOT NULL UNIQUE,
    provider_status text,
    status text NOT NULL CHECK (status IN (
        'creating', 'created', 'pending', 'pending_review',
        'approved', 'declined', 'failed', 'expired'
    )),
    provider_event_at timestamptz,
    last_provider_event_id text,
    started_at timestamptz,
    completed_at timestamptz,
    approved_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX idx_kyc_inquiries_provider_inquiry
    ON kyc_inquiries(provider_inquiry_id)
    WHERE provider_inquiry_id IS NOT NULL;
CREATE UNIQUE INDEX idx_kyc_inquiries_active_user
    ON kyc_inquiries(user_id)
    WHERE status IN ('creating', 'created', 'pending', 'pending_review');
CREATE INDEX idx_kyc_inquiries_user_updated
    ON kyc_inquiries(user_id, updated_at DESC, id DESC);

CREATE TABLE kyc_provider_events (
    id uuid PRIMARY KEY,
    provider text NOT NULL CHECK (provider = 'persona'),
    provider_event_id text NOT NULL,
    inquiry_id uuid NOT NULL REFERENCES kyc_inquiries(id),
    event_type text NOT NULL,
    provider_event_at timestamptz NOT NULL,
    payload_hash text NOT NULL,
    received_at timestamptz NOT NULL,
    UNIQUE (provider, provider_event_id)
);
