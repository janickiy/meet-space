-- auth_legacy_tokens: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE auth_legacy_tokens (
    token_hash character(64) NOT NULL,
    user_id uuid NOT NULL,
    session_id uuid,
    revoked_at timestamp with time zone,
    CONSTRAINT auth_legacy_tokens_token_hash_check CHECK ((token_hash ~ '^[a-f0-9]{64}$'::text))
);

-- migrate:constraints

ALTER TABLE ONLY auth_legacy_tokens
    ADD CONSTRAINT auth_legacy_tokens_pkey PRIMARY KEY (token_hash);

-- migrate:references

ALTER TABLE ONLY auth_legacy_tokens
    ADD CONSTRAINT auth_legacy_tokens_session_id_fkey FOREIGN KEY (session_id) REFERENCES auth_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY auth_legacy_tokens
    ADD CONSTRAINT auth_legacy_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
