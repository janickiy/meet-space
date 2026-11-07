-- conversation_message_pins: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conversation_message_pins (
    conversation_id uuid NOT NULL,
    message_id uuid NOT NULL,
    pinned_by_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY conversation_message_pins
    ADD CONSTRAINT conversation_message_pins_pkey PRIMARY KEY (conversation_id, message_id);

-- migrate:references

ALTER TABLE ONLY conversation_message_pins
    ADD CONSTRAINT conversation_message_pins_conversation_id_message_id_fkey FOREIGN KEY (conversation_id, message_id) REFERENCES conversation_messages(conversation_id, id) ON DELETE CASCADE;

ALTER TABLE ONLY conversation_message_pins
    ADD CONSTRAINT conversation_message_pins_conversation_id_pinned_by_user_i_fkey FOREIGN KEY (conversation_id, pinned_by_user_id) REFERENCES conversation_members(conversation_id, user_id) ON DELETE RESTRICT;
