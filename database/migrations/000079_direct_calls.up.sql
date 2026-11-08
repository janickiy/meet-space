-- direct_calls: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE direct_calls (
    id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    conference_id uuid,
    caller_user_id uuid NOT NULL,
    callee_user_id uuid NOT NULL,
    caller_client_id uuid NOT NULL,
    accepted_by_client_id uuid,
    client_request_id uuid NOT NULL,
    status text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    ended_at timestamp with time zone,
    caller_lease_until timestamp with time zone,
    callee_lease_until timestamp with time zone,
    CONSTRAINT direct_calls_check CHECK ((caller_user_id <> callee_user_id)),
    CONSTRAINT direct_calls_check1 CHECK (((conference_id IS NULL) = (accepted_at IS NULL))),
    CONSTRAINT direct_calls_check2 CHECK (((accepted_at IS NULL) = (accepted_by_client_id IS NULL))),
    CONSTRAINT direct_calls_check3 CHECK (((status = ANY (ARRAY['ringing'::text, 'accepted'::text])) = (ended_at IS NULL))),
    CONSTRAINT direct_calls_check4 CHECK (((status <> ALL (ARRAY['accepted'::text, 'ended'::text])) OR (accepted_at IS NOT NULL))),
    CONSTRAINT direct_calls_check5 CHECK (((status <> ALL (ARRAY['ringing'::text, 'declined'::text, 'cancelled'::text, 'missed'::text])) OR (accepted_at IS NULL))),
    CONSTRAINT direct_calls_check6 CHECK ((expires_at > created_at)),
    CONSTRAINT direct_calls_check7 CHECK (((ended_at IS NULL) OR (ended_at >= created_at))),
    CONSTRAINT direct_calls_status_check CHECK ((status = ANY (ARRAY['ringing'::text, 'accepted'::text, 'declined'::text, 'cancelled'::text, 'ended'::text, 'missed'::text]))),
    CONSTRAINT direct_calls_version_check CHECK ((version > 0))
);

-- migrate:constraints

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_caller_user_id_client_request_id_key UNIQUE (caller_user_id, client_request_id);

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_conference_id_key UNIQUE (conference_id);

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_pkey PRIMARY KEY (id);

CREATE INDEX direct_calls_expiring ON direct_calls USING btree (expires_at) WHERE (status = 'ringing'::text);

CREATE INDEX direct_calls_leases ON direct_calls USING btree (caller_lease_until, callee_lease_until) WHERE (status = 'accepted'::text);

-- migrate:references

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_callee_user_id_fkey FOREIGN KEY (callee_user_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_caller_user_id_fkey FOREIGN KEY (caller_user_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_conversation_id_callee_user_id_fkey FOREIGN KEY (conversation_id, callee_user_id) REFERENCES conversation_members(conversation_id, user_id);

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_conversation_id_caller_user_id_fkey FOREIGN KEY (conversation_id, caller_user_id) REFERENCES conversation_members(conversation_id, user_id);

ALTER TABLE ONLY direct_calls
    ADD CONSTRAINT direct_calls_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE RESTRICT;
