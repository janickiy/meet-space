-- participant_sessions: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE participant_sessions (
    id uuid NOT NULL,
    conference_id uuid NOT NULL,
    participant_id uuid NOT NULL,
    user_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    status character varying(16) NOT NULL,
    connected_at timestamp with time zone NOT NULL,
    disconnected_at timestamp with time zone,
    last_seen_at timestamp with time zone NOT NULL,
    media_sequence bigint DEFAULT 0 NOT NULL,
    microphone_enabled boolean DEFAULT false NOT NULL,
    camera_enabled boolean DEFAULT false NOT NULL,
    screen_sharing boolean DEFAULT false NOT NULL,
    CONSTRAINT participant_sessions_dates CHECK (((last_seen_at >= connected_at) AND ((((status)::text = 'connected'::text) AND (disconnected_at IS NULL)) OR (((status)::text = 'disconnected'::text) AND (disconnected_at IS NOT NULL) AND (disconnected_at >= last_seen_at))))),
    CONSTRAINT participant_sessions_media_sequence_check CHECK (((media_sequence >= 0) AND (media_sequence <= '9007199254740991'::bigint))),
    CONSTRAINT participant_sessions_status CHECK ((status IN ('connected', 'disconnected')))
);

-- migrate:constraints

ALTER TABLE ONLY participant_sessions
    ADD CONSTRAINT participant_sessions_connection_id_key UNIQUE (connection_id);

ALTER TABLE ONLY participant_sessions
    ADD CONSTRAINT participant_sessions_pkey PRIMARY KEY (id);

CREATE INDEX idx_participant_sessions_connected ON participant_sessions USING btree (connected_at) WHERE ((status)::text = 'connected'::text);

CREATE INDEX idx_participant_sessions_member ON participant_sessions USING btree (participant_id, connected_at DESC);

-- migrate:references

ALTER TABLE ONLY participant_sessions
    ADD CONSTRAINT participant_sessions_identity_fk FOREIGN KEY (participant_id, conference_id, user_id) REFERENCES conference_participants(id, conference_id, user_id) ON DELETE CASCADE;
