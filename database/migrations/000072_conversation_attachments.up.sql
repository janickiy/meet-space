-- conversation_attachments: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conversation_attachments (
    id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    owner_user_id uuid NOT NULL,
    client_request_id uuid NOT NULL,
    filename character varying(720) NOT NULL,
    mime_type character varying(80) NOT NULL,
    size bigint NOT NULL,
    checksum character(64) DEFAULT ''::bpchar NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    message_id uuid,
    object_key text DEFAULT ''::text NOT NULL,
    upload_token uuid,
    upload_lease_until timestamp with time zone,
    uploaded_at timestamp with time zone,
    cleaned_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT conversation_attachments_check CHECK ((((status)::text = 'attached'::text) = (message_id IS NOT NULL))),
    CONSTRAINT conversation_attachments_size_check CHECK (((size >= 1) AND (size <= 104857600))),
    CONSTRAINT conversation_attachments_status_check CHECK ((status IN ('pending', 'ready', 'attached', 'expired')))
);

-- migrate:constraints

ALTER TABLE ONLY conversation_attachments
    ADD CONSTRAINT conversation_attachments_conversation_id_owner_user_id_clie_key UNIQUE (conversation_id, owner_user_id, client_request_id);

ALTER TABLE ONLY conversation_attachments
    ADD CONSTRAINT conversation_attachments_pkey PRIMARY KEY (id);

CREATE INDEX conversation_attachments_cleanup ON conversation_attachments USING btree (expires_at, id) WHERE (cleaned_at IS NULL);

CREATE INDEX conversation_attachments_message ON conversation_attachments USING btree (message_id) WHERE (message_id IS NOT NULL);

CREATE INDEX conversation_attachments_owner_pending ON conversation_attachments USING btree (conversation_id, owner_user_id) WHERE (status IN ('pending', 'ready'));

-- migrate:references

ALTER TABLE ONLY conversation_attachments
    ADD CONSTRAINT conversation_attachments_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_attachments
    ADD CONSTRAINT conversation_attachments_conversation_id_message_id_fkey FOREIGN KEY (conversation_id, message_id) REFERENCES conversation_messages(conversation_id, id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_attachments
    ADD CONSTRAINT conversation_attachments_conversation_id_owner_user_id_fkey FOREIGN KEY (conversation_id, owner_user_id) REFERENCES conversation_members(conversation_id, user_id) ON DELETE RESTRICT;
