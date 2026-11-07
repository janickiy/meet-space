-- administrator_audit_log: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE FUNCTION deny_administrator_audit_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'administrator audit log is append-only'; END $$;

CREATE TABLE administrator_audit_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    administrator_id uuid,
    username character varying(40) DEFAULT ''::character varying NOT NULL,
    action character varying(80) NOT NULL,
    object_type character varying(60) DEFAULT ''::character varying NOT NULL,
    object_id character varying(100) DEFAULT ''::character varying NOT NULL,
    ip character varying(64) DEFAULT ''::character varying NOT NULL,
    user_agent character varying(500) DEFAULT ''::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY administrator_audit_log
    ADD CONSTRAINT administrator_audit_log_pkey PRIMARY KEY (id);

CREATE INDEX admin_audit_search_trgm ON administrator_audit_log USING gin (username gin_trgm_ops, action gin_trgm_ops, object_id gin_trgm_ops, ip gin_trgm_ops);

CREATE INDEX idx_administrator_audit_action ON administrator_audit_log USING btree (action, created_at DESC);

CREATE INDEX idx_administrator_audit_actor ON administrator_audit_log USING btree (administrator_id, created_at DESC);

CREATE INDEX idx_administrator_audit_created ON administrator_audit_log USING btree (created_at DESC, id);

-- migrate:references

CREATE TRIGGER administrator_audit_immutable BEFORE DELETE OR UPDATE OR TRUNCATE ON administrator_audit_log FOR EACH STATEMENT EXECUTE FUNCTION deny_administrator_audit_mutation();
