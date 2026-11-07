-- chat_attachments: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE chat_attachments (
    id uuid NOT NULL,
    conference_id uuid NOT NULL,
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
    CONSTRAINT chat_attachments_check CHECK ((((status)::text = 'attached'::text) = (message_id IS NOT NULL))),
    CONSTRAINT chat_attachments_size_check CHECK (((size >= 1) AND (size <= 104857600))),
    CONSTRAINT chat_attachments_status_check CHECK ((status IN ('pending', 'ready', 'attached', 'expired')))
);

-- migrate:constraints

ALTER TABLE ONLY chat_attachments
    ADD CONSTRAINT chat_attachments_conference_id_owner_user_id_client_request_key UNIQUE (conference_id, owner_user_id, client_request_id);

ALTER TABLE ONLY chat_attachments
    ADD CONSTRAINT chat_attachments_pkey PRIMARY KEY (id);

CREATE INDEX chat_attachments_cleanup ON chat_attachments USING btree (expires_at, id) WHERE (cleaned_at IS NULL);

CREATE INDEX chat_attachments_message ON chat_attachments USING btree (message_id) WHERE (message_id IS NOT NULL);

CREATE INDEX chat_attachments_owner_pending ON chat_attachments USING btree (conference_id, owner_user_id) WHERE (status IN ('pending', 'ready'));

-- migrate:references

ALTER TABLE ONLY chat_attachments
    ADD CONSTRAINT chat_attachments_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_attachments
    ADD CONSTRAINT chat_attachments_message_id_fkey FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE RESTRICT;

ALTER TABLE ONLY chat_attachments
    ADD CONSTRAINT chat_attachments_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT;
