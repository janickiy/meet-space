-- ldap_directory_state: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE ldap_directory_state (
    singleton boolean DEFAULT true NOT NULL,
    last_snapshot_at timestamp with time zone,
    CONSTRAINT ldap_directory_state_singleton_check CHECK (singleton)
);

-- migrate:constraints

ALTER TABLE ONLY ldap_directory_state
    ADD CONSTRAINT ldap_directory_state_pkey PRIMARY KEY (singleton);

-- migrate:references

INSERT INTO ldap_directory_state (singleton) VALUES (true);
