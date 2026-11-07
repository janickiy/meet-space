-- auth_session_tokens: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE auth_session_tokens (
    token_hash character(64) NOT NULL,
    session_id uuid NOT NULL,
    CONSTRAINT auth_session_tokens_token_hash_check CHECK ((token_hash ~ '^[a-f0-9]{64}$'::text))
);

-- migrate:constraints

ALTER TABLE ONLY auth_session_tokens
    ADD CONSTRAINT auth_session_tokens_pkey PRIMARY KEY (token_hash);

CREATE INDEX idx_auth_session_tokens_session ON auth_session_tokens USING btree (session_id);

-- migrate:references

ALTER TABLE ONLY auth_session_tokens
    ADD CONSTRAINT auth_session_tokens_session_id_fkey FOREIGN KEY (session_id) REFERENCES auth_sessions(id) ON DELETE CASCADE;
