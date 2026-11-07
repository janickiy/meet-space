-- administrators: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE administrators (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    username character varying(40) NOT NULL,
    password_hash text NOT NULL,
    name character varying(100) NOT NULL,
    email character varying(254),
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_login_at timestamp with time zone,
    CONSTRAINT administrators_status_check CHECK ((status = ANY (ARRAY['active'::text, 'blocked'::text]))),
    CONSTRAINT administrators_username_check CHECK (((username)::text ~ '^[a-z0-9][a-z0-9.@+_-]{2,39}$'::text))
);

-- migrate:constraints

ALTER TABLE ONLY administrators
    ADD CONSTRAINT administrators_pkey PRIMARY KEY (id);

ALTER TABLE ONLY administrators
    ADD CONSTRAINT administrators_username_key UNIQUE (username);

CREATE INDEX idx_administrators_status_created ON administrators USING btree (status, created_at, id);

-- migrate:references

CREATE TRIGGER trg_administrators_updated_at BEFORE UPDATE ON administrators FOR EACH ROW EXECUTE FUNCTION set_updated_at();
