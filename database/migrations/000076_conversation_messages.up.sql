-- conversation_messages: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conversation_messages (
    id uuid NOT NULL,
    sequence bigint NOT NULL,
    conversation_id uuid NOT NULL,
    sender_user_id uuid NOT NULL,
    client_request_id uuid NOT NULL,
    request_fingerprint character(64) NOT NULL,
    text text DEFAULT ''::text NOT NULL,
    reply_to_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    deleted_at timestamp with time zone,
    kind text DEFAULT 'text'::text NOT NULL,
    call_id uuid,
    CONSTRAINT conversation_message_call_shape CHECK (((kind = 'call'::text) = (call_id IS NOT NULL))),
    CONSTRAINT conversation_messages_kind_check CHECK ((kind = ANY (ARRAY['text'::text, 'call'::text]))),
    CONSTRAINT conversation_messages_text_check CHECK ((char_length(text) <= 4000)),
    CONSTRAINT conversation_messages_version_check CHECK ((version > 0))
);

CREATE SEQUENCE conversation_messages_sequence_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE conversation_messages_sequence_seq OWNED BY conversation_messages.sequence;

ALTER TABLE ONLY conversation_messages ALTER COLUMN sequence SET DEFAULT nextval('conversation_messages_sequence_seq'::regclass);

-- migrate:constraints

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_call_id_key UNIQUE (call_id);

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_id_key UNIQUE (conversation_id, id);

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_id_sequence_key UNIQUE (conversation_id, id, sequence);

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_sender_user_id_client_key UNIQUE (conversation_id, sender_user_id, client_request_id);

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX conversation_messages_cursor ON conversation_messages USING btree (conversation_id, sequence DESC);

CREATE INDEX conversation_messages_unread ON conversation_messages USING btree (conversation_id, sequence) INCLUDE (sender_user_id) WHERE (deleted_at IS NULL);

-- migrate:references

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_call_id_fkey FOREIGN KEY (call_id) REFERENCES direct_calls(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_reply_to_id_fkey FOREIGN KEY (conversation_id, reply_to_id) REFERENCES conversation_messages(conversation_id, id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_messages
    ADD CONSTRAINT conversation_messages_conversation_id_sender_user_id_fkey FOREIGN KEY (conversation_id, sender_user_id) REFERENCES conversation_members(conversation_id, user_id) ON DELETE RESTRICT;
