-- conference_chat_pins: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conference_chat_pins (
    conference_id uuid NOT NULL,
    user_id uuid NOT NULL,
    message_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY conference_chat_pins
    ADD CONSTRAINT conference_chat_pins_pkey PRIMARY KEY (conference_id, user_id, message_id);

CREATE INDEX conference_chat_pins_recent ON conference_chat_pins USING btree (conference_id, user_id, created_at DESC, message_id DESC);

-- migrate:references

ALTER TABLE ONLY conference_chat_pins
    ADD CONSTRAINT conference_chat_pins_conference_id_message_id_fkey FOREIGN KEY (conference_id, message_id) REFERENCES chat_messages(conference_id, id) ON DELETE RESTRICT;

ALTER TABLE ONLY conference_chat_pins
    ADD CONSTRAINT conference_chat_pins_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
