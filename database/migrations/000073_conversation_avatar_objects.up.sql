-- conversation_avatar_objects: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conversation_avatar_objects (
    id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    object_key text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    checksum character(64) NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    cleanup_after timestamp with time zone DEFAULT (clock_timestamp() + '00:10:00'::interval) NOT NULL,
    cleanup_token uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT conversation_avatar_objects_check CHECK ((object_key = ((('avatars/conversations/'::text || (conversation_id)::text) || '/'::text) || (id)::text))),
    CONSTRAINT conversation_avatar_objects_check1 CHECK (((state = 'cleaning'::text) = (cleanup_token IS NOT NULL))),
    CONSTRAINT conversation_avatar_objects_checksum_check CHECK ((checksum ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT conversation_avatar_objects_content_type_check CHECK ((content_type = ANY (ARRAY['image/jpeg'::text, 'image/png'::text]))),
    CONSTRAINT conversation_avatar_objects_size_check CHECK (((size >= 1) AND (size <= 2097152))),
    CONSTRAINT conversation_avatar_objects_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'active'::text, 'retired'::text, 'cleaning'::text, 'cleaned'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY conversation_avatar_objects
    ADD CONSTRAINT conversation_avatar_objects_object_key_key UNIQUE (object_key);

ALTER TABLE ONLY conversation_avatar_objects
    ADD CONSTRAINT conversation_avatar_objects_pkey PRIMARY KEY (id);

CREATE INDEX conversation_avatar_cleaned_retention ON conversation_avatar_objects USING btree (updated_at, id) WHERE (state = 'cleaned'::text);

CREATE INDEX conversation_avatar_cleanup ON conversation_avatar_objects USING btree (cleanup_after, id) WHERE (state = ANY (ARRAY['pending'::text, 'retired'::text, 'cleaning'::text]));

CREATE INDEX conversation_avatar_current ON conversation_avatar_objects USING btree (conversation_id) WHERE (state = 'active'::text);

-- migrate:references

ALTER TABLE ONLY conversation_avatar_objects
    ADD CONSTRAINT conversation_avatar_objects_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE RESTRICT;
