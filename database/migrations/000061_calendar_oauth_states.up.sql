-- calendar_oauth_states: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE calendar_oauth_states (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    state_hash text NOT NULL,
    verifier_ciphertext text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone
);

-- migrate:constraints

ALTER TABLE ONLY calendar_oauth_states
    ADD CONSTRAINT calendar_oauth_states_pkey PRIMARY KEY (id);

ALTER TABLE ONLY calendar_oauth_states
    ADD CONSTRAINT calendar_oauth_states_state_hash_key UNIQUE (state_hash);

CREATE INDEX idx_calendar_oauth_expiry ON calendar_oauth_states USING btree (expires_at);

-- migrate:references

ALTER TABLE ONLY calendar_oauth_states
    ADD CONSTRAINT calendar_oauth_states_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
