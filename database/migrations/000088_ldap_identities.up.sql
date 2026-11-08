-- ldap_identities: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE ldap_identities (
    provider_id text DEFAULT 'ldap'::text NOT NULL,
    external_id text NOT NULL,
    user_id uuid NOT NULL,
    login text NOT NULL,
    last_seen_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT ldap_identities_provider_id_check CHECK ((provider_id = 'ldap'::text))
);

-- migrate:constraints

ALTER TABLE ONLY ldap_identities
    ADD CONSTRAINT ldap_identities_pkey PRIMARY KEY (provider_id, external_id);

ALTER TABLE ONLY ldap_identities
    ADD CONSTRAINT ldap_identities_user_id_key UNIQUE (user_id);

CREATE INDEX ldap_identities_last_seen_idx ON ldap_identities USING btree (last_seen_at);

-- migrate:references

ALTER TABLE ONLY ldap_identities
    ADD CONSTRAINT ldap_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
