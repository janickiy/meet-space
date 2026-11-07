-- calendar_connections: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE calendar_connections (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    calendar_id text DEFAULT 'primary'::text NOT NULL,
    status text NOT NULL,
    access_ciphertext text DEFAULT ''::text NOT NULL,
    refresh_ciphertext text DEFAULT ''::text NOT NULL,
    scopes text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT calendar_connections_status_check CHECK ((status = ANY (ARRAY['connected'::text, 'revoked'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY calendar_connections
    ADD CONSTRAINT calendar_connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY calendar_connections
    ADD CONSTRAINT calendar_connections_user_id_provider_calendar_id_key UNIQUE (user_id, provider, calendar_id);

-- migrate:references

ALTER TABLE ONLY calendar_connections
    ADD CONSTRAINT calendar_connections_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
