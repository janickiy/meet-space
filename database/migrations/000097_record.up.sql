-- record: consolidated schema through historical migration 000049.
-- Shared helpers nullable_uuid_text and set_updated_at are defined here.

-- migrate:structure

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE FUNCTION enqueue_recording_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND NEW.platform_conference_id IS NOT NULL AND NEW.status IN ('ready','partial_ready') AND NEW.deleted_at IS NULL THEN
  INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,recording_id,created_at)
  VALUES ('recording.ready',NEW.uuid,1,NEW.platform_conference_id,NEW.uuid,NEW.updated_at)
  ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$;

CREATE FUNCTION nullable_uuid_text(value uuid) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$
	SELECT CASE WHEN value IS NULL THEN NULL ELSE value::text END
$$;

CREATE FUNCTION set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;
$$;

CREATE TABLE record (
    id bigint NOT NULL,
    uuid uuid DEFAULT gen_random_uuid() NOT NULL,
    conference_id uuid NOT NULL,
    requested_by uuid,
    source_type character varying(32) NOT NULL,
    source_session_id character varying(128),
    transport_type character varying(32) NOT NULL,
    status character varying(32) NOT NULL,
    requested_quality character varying(16),
    effective_quality character varying(16),
    quality_mode character varying(16) NOT NULL,
    min_quality character varying(16),
    layout character varying(32),
    audio_mode character varying(32),
    width smallint,
    height smallint,
    fps smallint,
    segment_duration_sec smallint,
    reconnect_grace_sec smallint DEFAULT 10 NOT NULL,
    need_preview boolean DEFAULT true NOT NULL,
    storage_bucket character varying(128),
    storage_object_key text,
    preview_object_key text,
    duration_sec integer,
    size_bytes bigint,
    started_at timestamp with time zone,
    stopped_at timestamp with time zone,
    ended_at timestamp with time zone,
    ended_reason character varying(64),
    error_message text,
    metadata_json jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    worker_id character varying(128),
    mode character varying(24) DEFAULT 'legacy'::character varying NOT NULL,
    platform_conference_id uuid,
    recorder_token uuid,
    recorder_lease_until timestamp with time zone,
    media_started_at timestamp with time zone,
    CONSTRAINT record_audio_mode_check CHECK (((audio_mode IS NULL) OR (audio_mode IN ('mixed', 'separate')))),
    CONSTRAINT record_composite_conference_check CHECK (((((mode)::text = 'legacy'::text) AND (platform_conference_id IS NULL)) OR ((mode IN ('composite', 'audio_only', 'individual_tracks', 'screen_focus')) AND (platform_conference_id IS NOT NULL) AND (platform_conference_id = conference_id)))),
    CONSTRAINT record_effective_quality_check CHECK (((effective_quality IS NULL) OR (effective_quality IN ('1080p', '720p', '480p', '360p')))),
    CONSTRAINT record_layout_check CHECK (((layout IS NULL) OR (layout IN ('speaker', 'grid', 'screen_share')))),
    CONSTRAINT record_min_quality_check CHECK (((min_quality IS NULL) OR (min_quality IN ('1080p', '720p', '480p', '360p')))),
    CONSTRAINT record_quality_mode_check CHECK ((quality_mode IN ('manual', 'auto'))),
    CONSTRAINT record_requested_quality_check CHECK (((requested_quality IS NULL) OR (requested_quality IN ('1080p', '720p', '480p', '360p')))),
    CONSTRAINT record_status_check CHECK ((status IN ('starting', 'recording', 'degraded', 'stopping', 'finalizing', 'uploading', 'ready', 'partial_ready', 'failed', 'cancelled')))
);

CREATE SEQUENCE record_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE record_id_seq OWNED BY record.id;

ALTER TABLE ONLY record ALTER COLUMN id SET DEFAULT nextval('record_id_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY record
    ADD CONSTRAINT record_pkey PRIMARY KEY (id);

ALTER TABLE ONLY record
    ADD CONSTRAINT record_uuid_key UNIQUE (uuid);

CREATE INDEX admin_record_created ON record USING btree (created_at DESC, id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_record_conference_id ON record USING btree (conference_id);

CREATE INDEX idx_record_conference_status ON record USING btree (conference_id, status);

CREATE INDEX idx_record_created_at ON record USING btree (created_at);

CREATE INDEX idx_record_ended_at ON record USING btree (ended_at);

CREATE INDEX idx_record_failed_recent ON record USING btree (updated_at DESC) WHERE (((status)::text = 'failed'::text) AND (platform_conference_id IS NOT NULL) AND (deleted_at IS NULL));

CREATE INDEX idx_record_notifications ON record USING btree (platform_conference_id, uuid) WHERE (((mode)::text = 'composite'::text) AND (status IN ('ready', 'partial_ready')));

CREATE UNIQUE INDEX idx_record_one_active_composite ON record USING btree (platform_conference_id) WHERE (((mode)::text = 'composite'::text) AND (status IN ('starting', 'recording', 'degraded', 'stopping', 'finalizing', 'uploading')));

CREATE UNIQUE INDEX idx_record_one_active_conference ON record USING btree (platform_conference_id) WHERE ((mode IN ('composite', 'audio_only', 'individual_tracks', 'screen_focus')) AND (status IN ('starting', 'recording', 'degraded', 'stopping', 'finalizing', 'uploading')));

CREATE INDEX idx_record_platform_page ON record USING btree (platform_conference_id, created_at DESC, id DESC) WHERE ((deleted_at IS NULL) AND (mode IN ('composite', 'audio_only', 'individual_tracks', 'screen_focus')));

CREATE INDEX idx_record_status ON record USING btree (status);

CREATE INDEX idx_record_worker_id ON record USING btree (worker_id);

-- migrate:references

CREATE TRIGGER trg_record_notification AFTER INSERT OR UPDATE OF status ON record FOR EACH ROW EXECUTE FUNCTION enqueue_recording_notification();

CREATE TRIGGER trg_record_updated_at BEFORE UPDATE ON record FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ONLY record
    ADD CONSTRAINT record_platform_conference_id_fkey FOREIGN KEY (platform_conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;
