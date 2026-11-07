-- conference_chat_preferences: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conference_chat_preferences (
    conference_id uuid NOT NULL,
    user_id uuid NOT NULL,
    notifications_enabled boolean DEFAULT true NOT NULL,
    left_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY conference_chat_preferences
    ADD CONSTRAINT conference_chat_preferences_pkey PRIMARY KEY (conference_id, user_id);

CREATE INDEX conference_chat_preferences_user_left ON conference_chat_preferences USING btree (user_id, conference_id) WHERE (left_at IS NOT NULL);

-- migrate:references

ALTER TABLE ONLY conference_chat_preferences
    ADD CONSTRAINT conference_chat_preferences_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE CASCADE;

ALTER TABLE ONLY conference_chat_preferences
    ADD CONSTRAINT conference_chat_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
