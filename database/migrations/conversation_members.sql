-- conversation_members: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE conversation_members (
    conversation_id uuid NOT NULL,
    user_id uuid NOT NULL,
    last_read_message_id uuid,
    last_read_sequence bigint DEFAULT 0 NOT NULL,
    joined_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    left_at timestamp with time zone,
    notifications_enabled boolean DEFAULT true NOT NULL,
    history_cleared_through bigint DEFAULT 0 NOT NULL,
    hidden_at timestamp with time zone,
    CONSTRAINT conversation_members_check CHECK (((last_read_message_id IS NULL) = (last_read_sequence = 0))),
    CONSTRAINT conversation_members_history_cleared_through_check CHECK ((history_cleared_through >= 0)),
    CONSTRAINT conversation_members_last_read_sequence_check CHECK ((last_read_sequence >= 0)),
    CONSTRAINT conversation_members_role_check CHECK ((role = ANY (ARRAY['owner'::text, 'admin'::text, 'member'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY conversation_members
    ADD CONSTRAINT conversation_members_pkey PRIMARY KEY (conversation_id, user_id);

CREATE INDEX conversation_members_active_group ON conversation_members USING btree (conversation_id, user_id) WHERE (left_at IS NULL);

CREATE INDEX conversation_members_active_user ON conversation_members USING btree (user_id, conversation_id) WHERE (left_at IS NULL);

CREATE UNIQUE INDEX conversation_members_one_owner ON conversation_members USING btree (conversation_id) WHERE ((role = 'owner'::text) AND (left_at IS NULL));

CREATE INDEX conversation_members_user ON conversation_members USING btree (user_id, conversation_id);

CREATE INDEX conversation_members_visible_user ON conversation_members USING btree (user_id, conversation_id) WHERE ((left_at IS NULL) AND (hidden_at IS NULL));

-- migrate:references

CREATE CONSTRAINT TRIGGER direct_pair_members AFTER INSERT OR DELETE OR UPDATE ON conversation_members DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_direct_pair();

ALTER TABLE ONLY conversation_members
    ADD CONSTRAINT conversation_members_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_members
    ADD CONSTRAINT conversation_members_read_fkey FOREIGN KEY (conversation_id, last_read_message_id, last_read_sequence) REFERENCES conversation_messages(conversation_id, id, sequence) ON DELETE RESTRICT;

ALTER TABLE ONLY conversation_members
    ADD CONSTRAINT conversation_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
