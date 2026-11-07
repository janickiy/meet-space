-- Separate identities and opaque sessions: user capabilities never authorize this console.
CREATE TABLE administrators (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 username VARCHAR(40) NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,39}$'),
 password_hash TEXT NOT NULL,
 name VARCHAR(100) NOT NULL,
 email VARCHAR(254),
 status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','blocked')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_login_at TIMESTAMPTZ
);
CREATE TRIGGER trg_administrators_updated_at BEFORE UPDATE ON administrators FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX idx_administrators_status_created ON administrators(status,created_at,id);
CREATE TABLE administrator_sessions (
 id UUID PRIMARY KEY,
 administrator_id UUID NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
 token_hash CHAR(64) NOT NULL UNIQUE CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_administrator_sessions_owner ON administrator_sessions(administrator_id,created_at);
CREATE INDEX idx_administrator_sessions_expiry ON administrator_sessions(expires_at);
-- No actor FK: deleting an account must never mutate historical audit events.
CREATE TABLE administrator_audit_log (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 administrator_id UUID,
 username VARCHAR(40) NOT NULL DEFAULT '',
 action VARCHAR(80) NOT NULL,
 object_type VARCHAR(60) NOT NULL DEFAULT '',
 object_id VARCHAR(100) NOT NULL DEFAULT '',
 ip VARCHAR(64) NOT NULL DEFAULT '',
 user_agent VARCHAR(500) NOT NULL DEFAULT '',
 metadata JSONB NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_administrator_audit_created ON administrator_audit_log(created_at DESC,id);
CREATE INDEX idx_administrator_audit_actor ON administrator_audit_log(administrator_id,created_at DESC);
CREATE INDEX idx_administrator_audit_action ON administrator_audit_log(action,created_at DESC);
CREATE FUNCTION deny_administrator_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'administrator audit log is append-only'; END $$;
CREATE TRIGGER administrator_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON administrator_audit_log FOR EACH STATEMENT EXECUTE FUNCTION deny_administrator_audit_mutation();
CREATE TABLE administrator_login_limits (
 key_hash CHAR(64) PRIMARY KEY,
 window_start TIMESTAMPTZ NOT NULL,
 attempts INTEGER NOT NULL
);
CREATE INDEX idx_administrator_login_limit_expiry ON administrator_login_limits(window_start);
ALTER TABLE users ADD COLUMN is_blocked BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN auth_invalid_before TIMESTAMPTZ;
CREATE INDEX idx_users_admin_status_created ON users(is_blocked,created_at DESC,id) WHERE password_hash<>'';
