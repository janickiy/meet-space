-- background_jobs: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE background_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kind character varying(64) NOT NULL,
    entity_id uuid NOT NULL,
    conference_id uuid NOT NULL,
    user_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    dedup_key character varying(255) NOT NULL,
    state character varying(16) DEFAULT 'queued'::character varying NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 5 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    lease_token uuid,
    lease_until timestamp with time zone,
    error_code character varying(64),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT background_jobs_attempts_check CHECK ((attempts >= 0)),
    CONSTRAINT background_jobs_check CHECK ((((state)::text = 'processing'::text) = ((lease_token IS NOT NULL) AND (lease_until IS NOT NULL)))),
    CONSTRAINT background_jobs_max_attempts_check CHECK (((max_attempts >= 1) AND (max_attempts <= 10))),
    CONSTRAINT background_jobs_payload_check CHECK ((octet_length((payload)::text) <= 32768)),
    CONSTRAINT background_jobs_state_check CHECK ((state IN ('queued', 'processing', 'done', 'failed', 'skipped'))),
    CONSTRAINT background_jobs_version_check CHECK ((version > 0))
);

-- migrate:constraints

ALTER TABLE ONLY background_jobs
    ADD CONSTRAINT background_jobs_dedup_key_key UNIQUE (dedup_key);

ALTER TABLE ONLY background_jobs
    ADD CONSTRAINT background_jobs_pkey PRIMARY KEY (id);

CREATE INDEX idx_background_jobs_available ON background_jobs USING btree (kind, available_at, id) WHERE (state IN ('queued', 'processing'));

CREATE INDEX idx_background_jobs_entity ON background_jobs USING btree (entity_id, kind, version);

CREATE INDEX idx_background_jobs_failed_recent ON background_jobs USING btree (updated_at DESC) WHERE ((state)::text = 'failed'::text);

CREATE INDEX idx_background_jobs_retention ON background_jobs USING btree (finished_at, id) WHERE (finished_at IS NOT NULL);

CREATE UNIQUE INDEX idx_ldap_sync_one_pending ON background_jobs USING btree (kind) WHERE (((kind)::text = 'directory.sync'::text) AND (state IN ('queued', 'processing')));

-- migrate:references


