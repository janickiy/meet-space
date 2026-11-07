-- record_file: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE record_file (
    id bigint NOT NULL,
    record_id bigint NOT NULL,
    uuid uuid DEFAULT gen_random_uuid() NOT NULL,
    file_type character varying(32) NOT NULL,
    bucket character varying(128) NOT NULL,
    object_key text NOT NULL,
    file_name character varying(255),
    mime_type character varying(100) NOT NULL,
    size_bytes bigint,
    duration_sec integer,
    checksum_sha256 character varying(64),
    is_primary boolean DEFAULT false NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    metadata_json jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT record_file_type_check CHECK ((file_type IN ('final_mp4', 'preview_jpg', 'debug_log', 'final_audio', 'tracks_archive')))
);

CREATE SEQUENCE record_file_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE record_file_id_seq OWNED BY record_file.id;

ALTER TABLE ONLY record_file ALTER COLUMN id SET DEFAULT nextval('record_file_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY record_file
    ADD CONSTRAINT record_file_bucket_object_unique UNIQUE (bucket, object_key);

ALTER TABLE ONLY record_file
    ADD CONSTRAINT record_file_pkey PRIMARY KEY (id);

ALTER TABLE ONLY record_file
    ADD CONSTRAINT record_file_uuid_key UNIQUE (uuid);

CREATE INDEX idx_record_file_is_primary ON record_file USING btree (is_primary);

CREATE UNIQUE INDEX idx_record_file_primary_unique ON record_file USING btree (record_id) WHERE (is_primary = true);

CREATE INDEX idx_record_file_record_id ON record_file USING btree (record_id);

CREATE INDEX idx_record_file_record_type ON record_file USING btree (record_id, file_type);

-- migrate:references

CREATE TRIGGER trg_record_file_updated_at BEFORE UPDATE ON record_file FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ONLY record_file
    ADD CONSTRAINT record_file_record_id_fkey FOREIGN KEY (record_id) REFERENCES record(id) ON DELETE CASCADE;
