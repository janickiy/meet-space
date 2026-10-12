-- Roll back only before calls have been used: preserving call history is required.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM direct_calls) THEN RAISE EXCEPTION 'cannot remove durable call history'; END IF;
END $$;
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_message_call_shape;
ALTER TABLE conversation_messages DROP COLUMN call_id;
ALTER TABLE conversation_messages DROP COLUMN kind;
DROP TABLE direct_call_slots;
DROP TABLE direct_calls;
ALTER TABLE conferences DROP COLUMN kind;
