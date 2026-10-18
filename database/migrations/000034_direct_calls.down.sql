-- Откат возможен только до того момента, когда были использованы вызовы: необходимо сохранить
-- историю вызовов.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM direct_calls) THEN RAISE EXCEPTION 'cannot remove durable call history'; END IF;
END $$;
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_message_call_shape;
ALTER TABLE conversation_messages DROP COLUMN call_id;
ALTER TABLE conversation_messages DROP COLUMN kind;
DROP TABLE direct_call_slots;
DROP TABLE direct_calls;
ALTER TABLE conferences DROP COLUMN kind;
