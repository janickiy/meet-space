-- notifications: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE FUNCTION enqueue_external_notification() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.type='chat.message' THEN RETURN NEW; END IF;
 INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,payload,dedup_key)
 SELECT 'integrations.delivery',NEW.id,(NEW.payload->>'conferenceId')::uuid,NEW.user_id,
 jsonb_build_object('notificationId',NEW.id,'channel',channel),
 'delivery:'||NEW.id::text||':'||channel FROM (VALUES ('email'),('push')) channels(channel)
 WHERE channel <> 'email' OR NOT (NEW.type='conference.invited' AND NEW.payload ? 'invitationId')
 ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$;

CREATE TABLE notifications (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    type character varying(40) NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    payload jsonb NOT NULL,
    dedup_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone,
    published_at timestamp with time zone,
    CONSTRAINT notifications_type_check CHECK ((type IN ('conference.soon', 'admission.decided', 'recording.ready', 'conference.invited', 'conference.rescheduled', 'conference.cancelled', 'chat.message'))),
    CONSTRAINT notifications_version_check CHECK ((version = 1))
);

-- migrate:constraints

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_user_id_dedup_key_key UNIQUE (user_id, dedup_key);

CREATE INDEX idx_notifications_pending ON notifications USING btree (created_at, id) WHERE (published_at IS NULL);

CREATE INDEX idx_notifications_unread ON notifications USING btree (user_id) WHERE (read_at IS NULL);

CREATE INDEX idx_notifications_user_cursor ON notifications USING btree (user_id, created_at DESC, id DESC);

-- migrate:references

CREATE TRIGGER trg_notification_external AFTER INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION enqueue_external_notification();

ALTER TABLE ONLY notifications
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
