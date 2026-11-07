CREATE TABLE password_recoveries (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
 token_hash CHAR(64) NOT NULL UNIQUE CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 token_ciphertext TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 consumed_at TIMESTAMPTZ,
 CHECK(expires_at>created_at)
);
CREATE INDEX password_recoveries_expiry ON password_recoveries(expires_at);
