-- record_segment: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE record_segment (
    id bigint NOT NULL,
    record_id bigint NOT NULL,
    uuid uuid DEFAULT gen_random_uuid() NOT NULL,
    seq_no integer NOT NULL,
    status character varying(32) NOT NULL,
    local_path text,
    object_key text,
    file_name character varying(255),
    mime_type character varying(100),
    started_at timestamp with time zone,
    ended_at timestamp with time zone,
    duration_ms integer,
    size_bytes bigint,
    checksum_sha256 character varying(64),
    effective_quality character varying(16),
    width smallint,
    height smallint,
    fps smallint,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT record_segment_effective_quality_check CHECK (((effective_quality IS NULL) OR (effective_quality IN ('1080p', '720p', '480p', '360p')))),
    CONSTRAINT record_segment_status_check CHECK ((status IN ('writing', 'closed', 'uploaded', 'failed', 'skipped')))
);

CREATE SEQUENCE record_segment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE record_segment_id_seq OWNED BY record_segment.id;

ALTER TABLE ONLY record_segment ALTER COLUMN id SET DEFAULT nextval('record_segment_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY record_segment
    ADD CONSTRAINT record_segment_pkey PRIMARY KEY (id);

ALTER TABLE ONLY record_segment
    ADD CONSTRAINT record_segment_record_seq_unique UNIQUE (record_id, seq_no);

ALTER TABLE ONLY record_segment
    ADD CONSTRAINT record_segment_uuid_key UNIQUE (uuid);

CREATE INDEX idx_record_segment_created_at ON record_segment USING btree (created_at);

CREATE INDEX idx_record_segment_record_id ON record_segment USING btree (record_id);

CREATE INDEX idx_record_segment_record_status ON record_segment USING btree (record_id, status);

CREATE INDEX idx_record_segment_status ON record_segment USING btree (status);

CREATE INDEX idx_record_segment_updated_at ON record_segment USING btree (updated_at);

-- migrate:references

CREATE TRIGGER trg_record_segment_updated_at BEFORE UPDATE ON record_segment FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ONLY record_segment
    ADD CONSTRAINT record_segment_record_id_fkey FOREIGN KEY (record_id) REFERENCES record(id) ON DELETE CASCADE;
