-- recording_outbox: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE recording_outbox (
    id bigint NOT NULL,
    record_id uuid NOT NULL,
    command_type character varying(16) NOT NULL,
    reason character varying(64) DEFAULT ''::character varying NOT NULL,
    claimed_until timestamp with time zone,
    claim_token uuid,
    published_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recording_outbox_command_type_check CHECK ((command_type IN ('record.start', 'record.stop')))
);

CREATE SEQUENCE recording_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE recording_outbox_id_seq OWNED BY recording_outbox.id;

ALTER TABLE ONLY recording_outbox ALTER COLUMN id SET DEFAULT nextval('recording_outbox_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY recording_outbox
    ADD CONSTRAINT recording_outbox_pkey PRIMARY KEY (id);

ALTER TABLE ONLY recording_outbox
    ADD CONSTRAINT recording_outbox_record_id_command_type_key UNIQUE (record_id, command_type);

CREATE INDEX idx_recording_outbox_pending ON recording_outbox USING btree (id) WHERE (published_at IS NULL);

-- migrate:references

ALTER TABLE ONLY recording_outbox
    ADD CONSTRAINT recording_outbox_record_id_fkey FOREIGN KEY (record_id) REFERENCES record(uuid) ON DELETE CASCADE;
