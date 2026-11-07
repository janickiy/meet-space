-- users: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email character varying(254) NOT NULL,
    password_hash text NOT NULL,
    display_name character varying(100),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    guest_conference_id uuid,
    avatar_version uuid,
    is_blocked boolean DEFAULT false NOT NULL,
    auth_invalid_before timestamp with time zone,
    email_verification_required boolean DEFAULT false NOT NULL,
    email_verified_at timestamp with time zone,
    auth_source text DEFAULT 'local'::text NOT NULL,
    deleted_at timestamp with time zone,
    ldap_disabled boolean DEFAULT false NOT NULL,
    auth_version bigint DEFAULT 0 NOT NULL,
    CONSTRAINT users_auth_source_check CHECK ((auth_source = ANY (ARRAY['local'::text, 'ldap'::text]))),
    CONSTRAINT users_guest_not_admin CHECK (((guest_conference_id IS NULL) OR (is_admin = false))),
    CONSTRAINT users_normalized_email CHECK (((email)::text = lower(btrim((email)::text))))
);

-- migrate:constraints

ALTER TABLE ONLY users
    ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE INDEX admin_users_registered ON users USING btree (created_at DESC, id) WHERE ((guest_conference_id IS NULL) AND (deleted_at IS NULL));

CREATE INDEX admin_users_search_trgm ON users USING gin (display_name gin_trgm_ops, email gin_trgm_ops) WHERE ((guest_conference_id IS NULL) AND (deleted_at IS NULL));

CREATE INDEX idx_users_admin_status_created ON users USING btree (((is_blocked OR ldap_disabled)), created_at DESC, id) WHERE (deleted_at IS NULL);

-- migrate:references

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
