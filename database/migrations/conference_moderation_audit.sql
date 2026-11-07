-- conference_moderation_audit: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conference_moderation_audit (
    id bigint NOT NULL,
    conference_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    participant_id uuid NOT NULL,
    action character varying(16) NOT NULL,
    blocked boolean,
    role character varying(16),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE conference_moderation_audit_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE conference_moderation_audit_id_seq OWNED BY conference_moderation_audit.id;

ALTER TABLE ONLY conference_moderation_audit ALTER COLUMN id SET DEFAULT nextval('conference_moderation_audit_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY conference_moderation_audit
    ADD CONSTRAINT conference_moderation_audit_pkey PRIMARY KEY (id);

-- migrate:references

ALTER TABLE ONLY conference_moderation_audit
    ADD CONSTRAINT conference_moderation_audit_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES conference_participants(id);

ALTER TABLE ONLY conference_moderation_audit
    ADD CONSTRAINT conference_moderation_audit_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE CASCADE;

ALTER TABLE ONLY conference_moderation_audit
    ADD CONSTRAINT conference_moderation_audit_participant_id_fkey FOREIGN KEY (participant_id) REFERENCES conference_participants(id);
