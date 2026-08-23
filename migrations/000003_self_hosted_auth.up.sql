-- Self-hosted authentication (ADR-002): Argon2id credentials plus
-- single-use email verification and password-reset challenge tokens.

CREATE TABLE auth_credentials (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    password_hash text NOT NULL,
    failed_login_count integer NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
    locked_until timestamptz,
    password_changed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX idx_auth_credentials_user ON auth_credentials(user_id);

CREATE TABLE auth_challenges (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash bytea NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('email_verification', 'password_reset')),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX idx_auth_challenges_token ON auth_challenges(token_hash);
CREATE INDEX idx_auth_challenges_user_purpose ON auth_challenges(user_id, purpose);

-- One account per email address so credentials login and verified-email
-- Google linking have a stable match key.
CREATE UNIQUE INDEX idx_users_email_unique ON users(email) WHERE email IS NOT NULL;
