-- chat_messages: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE chat_messages (
    id uuid NOT NULL,
    sequence bigint NOT NULL,
    conference_id uuid NOT NULL,
    sender_user_id uuid NOT NULL,
    client_request_id uuid NOT NULL,
    request_fingerprint character(64) NOT NULL,
    text text DEFAULT ''::text NOT NULL,
    reply_to_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT chat_messages_text_check CHECK ((char_length(text) <= 4000)),
    CONSTRAINT chat_messages_version_check CHECK ((version > 0))
);

CREATE SEQUENCE chat_messages_sequence_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE chat_messages_sequence_seq OWNED BY chat_messages.sequence;

ALTER TABLE ONLY chat_messages ALTER COLUMN sequence SET DEFAULT nextval('chat_messages_sequence_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY chat_messages
    ADD CONSTRAINT chat_messages_conference_id_sender_user_id_client_request_i_key UNIQUE (conference_id, sender_user_id, client_request_id);

ALTER TABLE ONLY chat_messages
    ADD CONSTRAINT chat_messages_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX chat_messages_conference_id_id ON chat_messages USING btree (conference_id, id);

CREATE UNIQUE INDEX chat_messages_conference_sequence ON chat_messages USING btree (conference_id, sequence DESC);

CREATE INDEX chat_messages_text_search ON chat_messages USING gin (text gin_trgm_ops) WHERE (deleted_at IS NULL);

CREATE INDEX chat_messages_unread ON chat_messages USING btree (conference_id, sequence) INCLUDE (sender_user_id) WHERE (deleted_at IS NULL);

-- migrate:references

ALTER TABLE ONLY chat_messages
    ADD CONSTRAINT chat_messages_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_messages
    ADD CONSTRAINT chat_messages_reply_to_id_fkey FOREIGN KEY (reply_to_id) REFERENCES chat_messages(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_messages
    ADD CONSTRAINT chat_messages_sender_user_id_fkey FOREIGN KEY (sender_user_id) REFERENCES users(id) ON DELETE RESTRICT;
