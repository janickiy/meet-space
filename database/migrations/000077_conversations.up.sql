-- conversations: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE FUNCTION check_direct_pair() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE ids UUID[]; cid UUID; kind TEXT; lo UUID; hi UUID; gone TIMESTAMPTZ; total BIGINT; owners BIGINT;
BEGIN
 IF TG_TABLE_NAME='conversations' THEN
  IF TG_OP='DELETE' THEN ids:=ARRAY[OLD.id]; ELSE ids:=ARRAY[NEW.id]; END IF;
 ELSIF TG_OP='INSERT' THEN ids:=ARRAY[NEW.conversation_id];
 ELSIF TG_OP='DELETE' THEN ids:=ARRAY[OLD.conversation_id];
 ELSE ids:=ARRAY[OLD.conversation_id,NEW.conversation_id]; END IF;
 FOREACH cid IN ARRAY ids LOOP
  SELECT type,user_low_id,user_high_id,deleted_at INTO kind,lo,hi,gone FROM conversations WHERE id=cid;
  IF NOT FOUND THEN CONTINUE; END IF;
  IF EXISTS(SELECT 1 FROM conversation_members m JOIN users u ON u.id=m.user_id WHERE m.conversation_id=cid AND u.guest_conference_id IS NOT NULL) THEN
   RAISE EXCEPTION 'conversation requires registered accounts' USING ERRCODE='23514';
  END IF;
  IF kind='direct' THEN
   IF (SELECT COUNT(*) FROM conversation_members WHERE conversation_id=cid)<>2
    OR EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=cid AND (user_id NOT IN(lo,hi) OR left_at IS NOT NULL OR role<>'member')) THEN
    RAISE EXCEPTION 'direct conversation requires exactly its two active registered accounts' USING ERRCODE='23514';
   END IF;
  ELSE
   SELECT count(*),count(*) FILTER(WHERE role='owner') INTO total,owners FROM conversation_members WHERE conversation_id=cid AND left_at IS NULL;
   IF total>100 OR (gone IS NULL AND (total<1 OR owners<>1)) OR (gone IS NOT NULL AND total<>0) THEN
    RAISE EXCEPTION 'active group requires one owner and at most 100 members' USING ERRCODE='23514';
   END IF;
  END IF;
 END LOOP;
 RETURN NULL;
END $$;

CREATE TABLE conversations (
    id uuid NOT NULL,
    type text DEFAULT 'direct'::text NOT NULL,
    user_low_id uuid,
    user_high_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    last_message_at timestamp with time zone,
    last_message_id uuid,
    name text,
    description text DEFAULT ''::text NOT NULL,
    created_by uuid,
    deleted_at timestamp with time zone,
    metadata_version bigint DEFAULT 1 NOT NULL,
    avatar_key text,
    avatar_version uuid,
    group_create_request_id uuid,
    group_request_fingerprint character(64),
    CONSTRAINT conversations_check CHECK ((user_low_id < user_high_id)),
    CONSTRAINT conversations_group_shape_check CHECK ((((type = 'direct'::text) AND (user_low_id IS NOT NULL) AND (user_high_id IS NOT NULL) AND (user_low_id < user_high_id) AND (name IS NULL) AND (created_by IS NULL) AND (deleted_at IS NULL) AND (avatar_key IS NULL) AND (avatar_version IS NULL) AND (group_create_request_id IS NULL) AND (group_request_fingerprint IS NULL)) OR ((type = 'group'::text) AND (user_low_id IS NULL) AND (user_high_id IS NULL) AND (created_by IS NOT NULL) AND (name IS NOT NULL) AND ((char_length(name) >= 1) AND (char_length(name) <= 50)) AND (name = btrim(name)) AND (char_length(description) <= 200) AND (group_create_request_id IS NOT NULL) AND (group_request_fingerprint IS NOT NULL) AND (group_request_fingerprint ~ '^[a-f0-9]{64}$'::text) AND ((avatar_key IS NULL) = (avatar_version IS NULL))))),
    CONSTRAINT conversations_kind_check CHECK ((type = ANY (ARRAY['direct'::text, 'group'::text]))),
    CONSTRAINT conversations_metadata_version_check CHECK ((metadata_version > 0))
);

-- migrate:constraints

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_group_request_unique UNIQUE (created_by, group_create_request_id);

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_user_low_id_user_high_id_key UNIQUE (user_low_id, user_high_id);

CREATE INDEX conversations_activity ON conversations USING btree (COALESCE(last_message_at, created_at) DESC, id DESC);

CREATE INDEX conversations_avatar_reference ON conversations USING btree (avatar_version) WHERE (avatar_version IS NOT NULL);

-- migrate:references

CREATE CONSTRAINT TRIGGER direct_pair_conversation AFTER INSERT OR UPDATE ON conversations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_direct_pair();

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversation_last_message_fkey FOREIGN KEY (id, last_message_id) REFERENCES conversation_messages(conversation_id, id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_user_high_id_fkey FOREIGN KEY (user_high_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY conversations
    ADD CONSTRAINT conversations_user_low_id_fkey FOREIGN KEY (user_low_id) REFERENCES users(id) ON DELETE RESTRICT;
