-- email_verifications: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE email_verifications (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    token_ciphertext text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    CONSTRAINT email_verifications_check CHECK ((expires_at > created_at)),
    CONSTRAINT email_verifications_token_hash_check CHECK ((token_hash ~ '^[a-f0-9]{64}$'::text))
);

-- migrate:constraints

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_user_id_key UNIQUE (user_id);

CREATE INDEX email_verifications_expiry ON email_verifications USING btree (expires_at);

-- migrate:references

ALTER TABLE ONLY email_verifications
    ADD CONSTRAINT email_verifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
