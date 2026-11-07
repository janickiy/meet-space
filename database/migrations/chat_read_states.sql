-- chat_read_states: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE chat_read_states (
    conference_id uuid NOT NULL,
    user_id uuid NOT NULL,
    last_read_message_id uuid NOT NULL,
    last_read_sequence bigint NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY chat_read_states
    ADD CONSTRAINT chat_read_states_pkey PRIMARY KEY (conference_id, user_id);

-- migrate:references

ALTER TABLE ONLY chat_read_states
    ADD CONSTRAINT chat_read_states_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_read_states
    ADD CONSTRAINT chat_read_states_last_read_message_id_fkey FOREIGN KEY (last_read_message_id) REFERENCES chat_messages(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_read_states
    ADD CONSTRAINT chat_read_states_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
