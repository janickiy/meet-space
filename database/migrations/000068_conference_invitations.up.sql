-- conference_invitations: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conference_invitations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conference_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    email character varying(254) NOT NULL,
    user_id uuid,
    status character varying(16) DEFAULT 'queued'::character varying NOT NULL,
    error_code character varying(64) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    sent_at timestamp with time zone,
    CONSTRAINT conference_invitations_email_check CHECK (((email)::text = lower(btrim((email)::text)))),
    CONSTRAINT conference_invitations_status_check CHECK ((status IN ('queued', 'sent', 'failed', 'skipped')))
);

-- migrate:constraints

ALTER TABLE ONLY conference_invitations
    ADD CONSTRAINT conference_invitations_conference_id_email_key UNIQUE (conference_id, email);

ALTER TABLE ONLY conference_invitations
    ADD CONSTRAINT conference_invitations_pkey PRIMARY KEY (id);

CREATE INDEX idx_conference_invitations_user ON conference_invitations USING btree (conference_id, user_id);

-- migrate:references

ALTER TABLE ONLY conference_invitations
    ADD CONSTRAINT conference_invitations_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conference_invitations
    ADD CONSTRAINT conference_invitations_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conference_invitations
    ADD CONSTRAINT conference_invitations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
