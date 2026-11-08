-- folder_conversations: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE folder_conversations (
    folder_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY folder_conversations
    ADD CONSTRAINT folder_conversations_pkey PRIMARY KEY (folder_id, conversation_id);

CREATE INDEX idx_folder_conversations_target ON folder_conversations USING btree (conversation_id, folder_id);

-- migrate:references

ALTER TABLE ONLY folder_conversations
    ADD CONSTRAINT folder_conversations_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE;

ALTER TABLE ONLY folder_conversations
    ADD CONSTRAINT folder_conversations_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE;
