-- conferences: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE FUNCTION enqueue_conference_integration() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE event_name TEXT;
BEGIN
 IF TG_OP='INSERT' AND NEW.status='scheduled' THEN event_name='conference.invited';
 ELSIF TG_OP='UPDATE' AND NEW.status='cancelled' AND OLD.status <> 'cancelled' THEN event_name='conference.cancelled';
 ELSIF TG_OP='UPDATE' AND NEW.status='scheduled' AND NEW.integration_version <> OLD.integration_version THEN event_name='conference.rescheduled';
 ELSE RETURN NEW; END IF;
 INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key)
 VALUES('integrations.conference',NEW.id,NEW.id,NEW.integration_version,jsonb_build_object('event',event_name),
 'conference:'||NEW.id::text||':'||NEW.integration_version::text) ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$;

CREATE FUNCTION version_conference_integration() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.scheduled_at IS DISTINCT FROM OLD.scheduled_at OR NEW.planned_duration_min IS DISTINCT FROM OLD.planned_duration_min
 OR NEW.title IS DISTINCT FROM OLD.title OR (NEW.status='cancelled' AND OLD.status <> 'cancelled') THEN
  NEW.integration_version=OLD.integration_version+1;
  NEW.integration_version_at=clock_timestamp();
 END IF;
 RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

CREATE TABLE conferences (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    title character varying(200) NOT NULL,
    invite_code character varying(32) NOT NULL,
    status character varying(16) DEFAULT 'created'::character varying NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    waiting_room_enabled boolean DEFAULT false NOT NULL,
    scheduled_at timestamp with time zone,
    planned_duration_min integer,
    integration_version bigint DEFAULT 1 NOT NULL,
    integration_version_at timestamp with time zone DEFAULT now() NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, (title)::text)) STORED,
    chat_description character varying(1000) DEFAULT ''::character varying NOT NULL,
    kind text DEFAULT 'meeting'::text NOT NULL,
    CONSTRAINT conferences_invite_code_format CHECK (((invite_code)::text ~ '^[A-Za-z0-9_-]{32}$'::text)),
    CONSTRAINT conferences_kind_check CHECK ((kind = ANY (ARRAY['meeting'::text, 'direct_call'::text]))),
    CONSTRAINT conferences_lifecycle_check CHECK (((((status)::text = 'created'::text) AND (started_at IS NULL) AND (finished_at IS NULL)) OR (((status)::text = 'scheduled'::text) AND (scheduled_at IS NOT NULL) AND (started_at IS NULL) AND (finished_at IS NULL)) OR (((status)::text = 'active'::text) AND (started_at IS NOT NULL) AND (finished_at IS NULL)) OR (((status)::text = 'finished'::text) AND (started_at IS NOT NULL) AND (finished_at IS NOT NULL)) OR (((status)::text = 'cancelled'::text) AND (started_at IS NULL) AND (finished_at IS NOT NULL)))),
    CONSTRAINT conferences_planned_duration_check CHECK (((planned_duration_min IS NULL) OR ((scheduled_at IS NOT NULL) AND ((planned_duration_min >= 1) AND (planned_duration_min <= 1440))))),
    CONSTRAINT conferences_status_check CHECK ((status IN ('created', 'scheduled', 'active', 'finished', 'cancelled'))),
    CONSTRAINT conferences_title_not_empty CHECK ((length(btrim((title)::text)) > 0))
);

-- migrate:constraints

ALTER TABLE ONLY conferences
    ADD CONSTRAINT conferences_invite_code_key UNIQUE (invite_code);

ALTER TABLE ONLY conferences
    ADD CONSTRAINT conferences_pkey PRIMARY KEY (id);

CREATE INDEX admin_conferences_created ON conferences USING btree (created_at DESC, id) WHERE (kind = 'meeting'::text);

CREATE INDEX idx_conferences_owner_created ON conferences USING btree (owner_id, created_at DESC);

CREATE INDEX idx_conferences_scheduled ON conferences USING btree (scheduled_at, id) WHERE ((status)::text = 'scheduled'::text);

CREATE INDEX idx_conferences_search ON conferences USING gin (search_vector);

CREATE INDEX idx_conferences_timeline ON conferences USING btree (COALESCE(finished_at, scheduled_at, created_at) DESC, id DESC);

-- migrate:references

CREATE TRIGGER trg_conference_integration_version BEFORE UPDATE ON conferences FOR EACH ROW EXECUTE FUNCTION version_conference_integration();

CREATE TRIGGER trg_conference_integrations AFTER INSERT OR UPDATE ON conferences FOR EACH ROW EXECUTE FUNCTION enqueue_conference_integration();

CREATE TRIGGER trg_conferences_updated_at BEFORE UPDATE ON conferences FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ONLY conferences
    ADD CONSTRAINT conferences_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE RESTRICT;
