-- ldap_sync_runs: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE ldap_sync_runs (
    singleton boolean DEFAULT true NOT NULL,
    last_requested_at timestamp with time zone,
    requested_revision bigint DEFAULT 0 NOT NULL,
    job_id uuid,
    lease_token uuid,
    status text DEFAULT 'idle'::text NOT NULL,
    last_started_at timestamp with time zone,
    last_completed_at timestamp with time zone,
    last_succeeded_at timestamp with time zone,
    error_code character varying(64) DEFAULT ''::character varying NOT NULL,
    imported integer DEFAULT 0 NOT NULL,
    updated integer DEFAULT 0 NOT NULL,
    disabled integer DEFAULT 0 NOT NULL,
    total integer DEFAULT 0 NOT NULL,
    CONSTRAINT ldap_sync_runs_singleton_check CHECK (singleton),
    CONSTRAINT ldap_sync_runs_status_check CHECK ((status = ANY (ARRAY['idle'::text, 'queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY ldap_sync_runs
    ADD CONSTRAINT ldap_sync_runs_pkey PRIMARY KEY (singleton);

-- migrate:references

INSERT INTO ldap_sync_runs (singleton) VALUES (true);
