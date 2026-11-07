-- administrator_sessions: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE administrator_sessions (
    id uuid NOT NULL,
    administrator_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT administrator_sessions_token_hash_check CHECK ((token_hash ~ '^[a-f0-9]{64}$'::text))
);

-- migrate:constraints

ALTER TABLE ONLY administrator_sessions
    ADD CONSTRAINT administrator_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY administrator_sessions
    ADD CONSTRAINT administrator_sessions_token_hash_key UNIQUE (token_hash);

CREATE INDEX idx_administrator_sessions_expiry ON administrator_sessions USING btree (expires_at);

CREATE INDEX idx_administrator_sessions_owner ON administrator_sessions USING btree (administrator_id, created_at);

-- migrate:references

ALTER TABLE ONLY administrator_sessions
    ADD CONSTRAINT administrator_sessions_administrator_id_fkey FOREIGN KEY (administrator_id) REFERENCES administrators(id) ON DELETE CASCADE;
