-- record_event: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE record_event (
    id bigint NOT NULL,
    record_id bigint NOT NULL,
    segment_id bigint,
    event_type character varying(64) NOT NULL,
    event_source character varying(32) NOT NULL,
    severity character varying(16) DEFAULT 'info'::character varying NOT NULL,
    message text,
    payload_json jsonb,
    correlation_id character varying(128),
    worker_id character varying(128),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT record_event_severity_check CHECK ((severity IN ('info', 'warning', 'error')))
);

CREATE SEQUENCE record_event_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE record_event_id_seq OWNED BY record_event.id;

ALTER TABLE ONLY record_event ALTER COLUMN id SET DEFAULT nextval('record_event_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY record_event
    ADD CONSTRAINT record_event_pkey PRIMARY KEY (id);

CREATE INDEX idx_record_event_correlation_id ON record_event USING btree (correlation_id);

CREATE INDEX idx_record_event_event_type ON record_event USING btree (event_type);

CREATE INDEX idx_record_event_payload_json ON record_event USING gin (payload_json);

CREATE INDEX idx_record_event_record_created_at ON record_event USING btree (record_id, created_at);

CREATE INDEX idx_record_event_severity ON record_event USING btree (severity);

-- migrate:references

ALTER TABLE ONLY record_event
    ADD CONSTRAINT record_event_record_id_fkey FOREIGN KEY (record_id) REFERENCES record(id) ON DELETE CASCADE;

ALTER TABLE ONLY record_event
    ADD CONSTRAINT record_event_segment_id_fkey FOREIGN KEY (segment_id) REFERENCES record_segment(id) ON DELETE SET NULL;
