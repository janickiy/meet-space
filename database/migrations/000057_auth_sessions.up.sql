-- auth_sessions: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE auth_sessions (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone
);

-- migrate:constraints

ALTER TABLE ONLY auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (id);

CREATE INDEX idx_auth_sessions_user ON auth_sessions USING btree (user_id);

-- migrate:references

ALTER TABLE ONLY auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
