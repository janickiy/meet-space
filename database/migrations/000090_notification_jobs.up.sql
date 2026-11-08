-- notification_jobs: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE notification_jobs (
    id bigint NOT NULL,
    kind character varying(40) NOT NULL,
    entity_id uuid NOT NULL,
    entity_version bigint NOT NULL,
    conference_id uuid NOT NULL,
    user_id uuid,
    participant_id uuid,
    recording_id uuid,
    admission_state character varying(16),
    cursor_participant_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    processed_at timestamp with time zone,
    CONSTRAINT notification_jobs_admission_state_check CHECK ((admission_state IN ('admitted', 'rejected', 'kicked'))),
    CONSTRAINT notification_jobs_check CHECK (((((kind)::text = 'admission.decided'::text) AND (user_id IS NOT NULL) AND (participant_id IS NOT NULL) AND (admission_state IS NOT NULL) AND (recording_id IS NULL)) OR (((kind)::text = 'recording.ready'::text) AND (recording_id IS NOT NULL) AND (user_id IS NULL) AND (participant_id IS NULL) AND (admission_state IS NULL)))),
    CONSTRAINT notification_jobs_entity_version_check CHECK ((entity_version > 0)),
    CONSTRAINT notification_jobs_kind_check CHECK ((kind IN ('admission.decided', 'recording.ready')))
);

CREATE SEQUENCE notification_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE notification_jobs_id_seq OWNED BY notification_jobs.id;

ALTER TABLE ONLY notification_jobs ALTER COLUMN id SET DEFAULT nextval('notification_jobs_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY notification_jobs
    ADD CONSTRAINT notification_jobs_kind_entity_id_entity_version_key UNIQUE (kind, entity_id, entity_version);

ALTER TABLE ONLY notification_jobs
    ADD CONSTRAINT notification_jobs_pkey PRIMARY KEY (id);

CREATE INDEX idx_notification_jobs_pending ON notification_jobs USING btree (available_at, id) WHERE (processed_at IS NULL);

-- migrate:references

ALTER TABLE ONLY notification_jobs
    ADD CONSTRAINT notification_jobs_participant_id_fkey FOREIGN KEY (participant_id) REFERENCES conference_participants(id) ON DELETE RESTRICT;

ALTER TABLE ONLY notification_jobs
    ADD CONSTRAINT notification_jobs_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES record(uuid) ON DELETE RESTRICT;

ALTER TABLE ONLY notification_jobs
    ADD CONSTRAINT notification_jobs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
