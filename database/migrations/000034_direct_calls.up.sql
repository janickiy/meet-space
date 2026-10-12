ALTER TABLE conferences ADD COLUMN kind TEXT NOT NULL DEFAULT 'meeting' CHECK(kind IN ('meeting','direct_call'));
CREATE TABLE direct_calls (
 id UUID PRIMARY KEY,
 conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
 conference_id UUID UNIQUE REFERENCES conferences(id) ON DELETE RESTRICT,
 caller_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 callee_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 caller_client_id UUID NOT NULL,
 accepted_by_client_id UUID,
 client_request_id UUID NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('ringing','accepted','declined','cancelled','ended','missed')),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 accepted_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 caller_lease_until TIMESTAMPTZ,
 callee_lease_until TIMESTAMPTZ,
 UNIQUE(caller_user_id,client_request_id),
 CHECK(caller_user_id<>callee_user_id),
 CHECK((conference_id IS NULL)=(accepted_at IS NULL)),
 CHECK((accepted_at IS NULL)=(accepted_by_client_id IS NULL)),
 CHECK((status IN ('ringing','accepted'))=(ended_at IS NULL)),
 CHECK(status NOT IN ('accepted','ended') OR accepted_at IS NOT NULL),
 CHECK(status NOT IN ('ringing','declined','cancelled','missed') OR accepted_at IS NULL),
 CHECK(expires_at>created_at),
 CHECK(ended_at IS NULL OR ended_at>=created_at),
 FOREIGN KEY(conversation_id,caller_user_id) REFERENCES conversation_members(conversation_id,user_id),
 FOREIGN KEY(conversation_id,callee_user_id) REFERENCES conversation_members(conversation_id,user_id)
);
CREATE INDEX direct_calls_expiring ON direct_calls(expires_at) WHERE status='ringing';
CREATE INDEX direct_calls_leases ON direct_calls(caller_lease_until,callee_lease_until) WHERE status='accepted';
CREATE TABLE direct_call_slots (
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
 call_id UUID NOT NULL REFERENCES direct_calls(id) ON DELETE RESTRICT
);
CREATE INDEX direct_call_slots_call ON direct_call_slots(call_id);
ALTER TABLE conversation_messages ADD COLUMN kind TEXT NOT NULL DEFAULT 'text' CHECK(kind IN ('text','call'));
ALTER TABLE conversation_messages ADD COLUMN call_id UUID UNIQUE REFERENCES direct_calls(id) ON DELETE RESTRICT;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_message_call_shape CHECK((kind='call')=(call_id IS NOT NULL));
