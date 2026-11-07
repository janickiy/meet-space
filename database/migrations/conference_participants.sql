-- conference_participants: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE FUNCTION enqueue_admission_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.user_id IS NOT NULL AND NEW.admission_decided_at IS NOT NULL AND NEW.admission_state IN ('admitted','rejected','kicked') THEN
        INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,user_id,participant_id,admission_state,created_at)
        VALUES ('admission.decided',NEW.id,NEW.admission_version,NEW.conference_id,NEW.user_id,NEW.id,NEW.admission_state,NEW.admission_decided_at)
        ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION enqueue_enrollment_integration() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF current_setting('meetrix.explicit_invitation',TRUE)='true' THEN RETURN NEW; END IF;
 IF NEW.user_id IS NOT NULL AND NEW.role <> 'owner' AND NEW.status NOT IN ('kicked','rejected') THEN
  INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key)
  SELECT 'integrations.event',NEW.id,NEW.conference_id,NEW.user_id,c.integration_version,jsonb_build_object('event','conference.invited'),
   'enrollment:'||NEW.id::text||':'||c.integration_version::text FROM conferences c
  WHERE c.id=NEW.conference_id AND c.status='scheduled' ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$;

CREATE TABLE conference_participants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conference_id uuid NOT NULL,
    user_id uuid,
    display_name character varying(100) NOT NULL,
    role character varying(16) NOT NULL,
    status character varying(16) DEFAULT 'left'::character varying NOT NULL,
    joined_at timestamp with time zone,
    left_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    microphone_enabled boolean DEFAULT false NOT NULL,
    camera_enabled boolean DEFAULT false NOT NULL,
    screen_sharing boolean DEFAULT false NOT NULL,
    microphone_blocked boolean DEFAULT false NOT NULL,
    camera_blocked boolean DEFAULT false NOT NULL,
    screen_blocked boolean DEFAULT false NOT NULL,
    media_policy_version bigint DEFAULT 1 NOT NULL,
    admission_state character varying(16) DEFAULT 'admitted'::character varying NOT NULL,
    admission_decided_at timestamp with time zone,
    admission_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT conference_participants_admission_check CHECK (((admission_state IN ('waiting', 'admitted', 'rejected', 'kicked')) AND (admission_version > 0) AND (((status)::text <> 'joined'::text) OR ((admission_state)::text = 'admitted'::text)) AND (((status)::text <> 'waiting'::text) OR ((admission_state)::text = 'waiting'::text)) AND (((status)::text <> 'rejected'::text) OR ((admission_state)::text = 'rejected'::text)) AND (((status)::text <> 'kicked'::text) OR ((admission_state)::text = 'kicked'::text)))),
    CONSTRAINT conference_participants_identity_check CHECK ((((user_id IS NULL) AND ((role)::text = 'guest'::text)) OR ((user_id IS NOT NULL) AND (role IN ('owner', 'co_host', 'participant'))))),
    CONSTRAINT conference_participants_joined_check CHECK ((((status)::text <> 'joined'::text) OR ((joined_at IS NOT NULL) AND (left_at IS NULL)))),
    CONSTRAINT conference_participants_left_check CHECK (((left_at IS NULL) OR ((joined_at IS NOT NULL) AND (left_at >= joined_at)))),
    CONSTRAINT conference_participants_role_check CHECK ((role IN ('owner', 'co_host', 'participant', 'guest'))),
    CONSTRAINT conference_participants_status_check CHECK ((status IN ('joined', 'left', 'waiting', 'rejected', 'kicked')))
);

-- migrate:constraints

ALTER TABLE ONLY conference_participants
    ADD CONSTRAINT conference_participants_membership_unique UNIQUE (conference_id, user_id);

ALTER TABLE ONLY conference_participants
    ADD CONSTRAINT conference_participants_pkey PRIMARY KEY (id);

CREATE INDEX idx_conference_participants_conference ON conference_participants USING btree (conference_id);

CREATE UNIQUE INDEX idx_conference_participants_single_owner ON conference_participants USING btree (conference_id) WHERE ((role)::text = 'owner'::text);

CREATE INDEX idx_conference_participants_user ON conference_participants USING btree (user_id);

CREATE UNIQUE INDEX idx_participant_session_identity ON conference_participants USING btree (id, conference_id, user_id);

CREATE INDEX idx_participants_admission_decisions ON conference_participants USING btree (admission_decided_at, id) WHERE (admission_decided_at IS NOT NULL);

CREATE INDEX idx_participants_history ON conference_participants USING btree (conference_id, created_at, id);

CREATE INDEX idx_participants_notification_fanout ON conference_participants USING btree (conference_id, id) WHERE ((user_id IS NOT NULL) AND ((admission_state)::text = 'admitted'::text) AND (status IN ('joined', 'left')));

CREATE INDEX idx_participants_user_admission ON conference_participants USING btree (user_id, admission_state, conference_id);

CREATE INDEX idx_participants_waiting ON conference_participants USING btree (conference_id, created_at, id) WHERE (((admission_state)::text = 'waiting'::text) AND ((status)::text = 'waiting'::text));

-- migrate:references

CREATE TRIGGER trg_conference_participants_updated_at BEFORE UPDATE ON conference_participants FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_enrollment_integrations AFTER INSERT ON conference_participants FOR EACH ROW EXECUTE FUNCTION enqueue_enrollment_integration();

CREATE TRIGGER trg_participants_notification AFTER INSERT OR UPDATE OF admission_state, admission_version, admission_decided_at ON conference_participants FOR EACH ROW EXECUTE FUNCTION enqueue_admission_notification();

ALTER TABLE ONLY conference_participants
    ADD CONSTRAINT conference_participants_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conference_participants
    ADD CONSTRAINT conference_participants_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
