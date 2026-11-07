CREATE TABLE ldap_sync_runs (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    last_requested_at TIMESTAMPTZ NULL,
    requested_revision BIGINT NOT NULL DEFAULT 0,
    job_id UUID NULL,
    lease_token UUID NULL,
    status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle','queued','running','succeeded','failed')),
    last_started_at TIMESTAMPTZ NULL,
    last_completed_at TIMESTAMPTZ NULL,
    last_succeeded_at TIMESTAMPTZ NULL,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    imported INTEGER NOT NULL DEFAULT 0,
    updated INTEGER NOT NULL DEFAULT 0,
    disabled INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0
);
INSERT INTO ldap_sync_runs(singleton) VALUES(TRUE);
CREATE UNIQUE INDEX idx_ldap_sync_one_pending ON background_jobs(kind)
    WHERE kind='directory.sync' AND state IN ('queued','processing');
