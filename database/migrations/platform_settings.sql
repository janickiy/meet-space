-- platform_settings: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE platform_settings (
    singleton boolean DEFAULT true NOT NULL,
    revision bigint DEFAULT 0 NOT NULL,
    "values" jsonb DEFAULT '{}'::jsonb NOT NULL,
    smtp_password_ciphertext text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    ldap_bind_password_ciphertext text DEFAULT ''::text NOT NULL,
    CONSTRAINT platform_settings_revision_check CHECK ((revision >= 0)),
    CONSTRAINT platform_settings_singleton_check CHECK (singleton),
    CONSTRAINT platform_settings_values_check CHECK ((jsonb_typeof("values") = 'object'::text))
);

-- migrate:constraints

ALTER TABLE ONLY platform_settings
    ADD CONSTRAINT platform_settings_pkey PRIMARY KEY (singleton);

-- migrate:references

INSERT INTO platform_settings (singleton) VALUES (true);
