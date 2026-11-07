-- push_devices: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE push_devices (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    platform text NOT NULL,
    provider text NOT NULL,
    token_ciphertext text NOT NULL,
    token_fingerprint text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    disabled_at timestamp with time zone,
    CONSTRAINT push_devices_platform_check CHECK ((platform = ANY (ARRAY['web'::text, 'ios'::text, 'android'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY push_devices
    ADD CONSTRAINT push_devices_pkey PRIMARY KEY (id);

ALTER TABLE ONLY push_devices
    ADD CONSTRAINT push_devices_user_id_provider_token_fingerprint_key UNIQUE (user_id, provider, token_fingerprint);

CREATE INDEX idx_push_devices_active ON push_devices USING btree (user_id, id) WHERE ((revoked_at IS NULL) AND (disabled_at IS NULL));

-- migrate:references

ALTER TABLE ONLY push_devices
    ADD CONSTRAINT push_devices_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
