-- chat_notification_jobs: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE chat_notification_jobs (
    message_id uuid NOT NULL,
    conference_id uuid NOT NULL,
    cursor_participant_id uuid,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY chat_notification_jobs
    ADD CONSTRAINT chat_notification_jobs_pkey PRIMARY KEY (message_id);

CREATE INDEX chat_notification_jobs_pending ON chat_notification_jobs USING btree (created_at, message_id) WHERE (completed_at IS NULL);

-- migrate:references

ALTER TABLE ONLY chat_notification_jobs
    ADD CONSTRAINT chat_notification_jobs_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_notification_jobs
    ADD CONSTRAINT chat_notification_jobs_message_id_fkey FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE RESTRICT;
