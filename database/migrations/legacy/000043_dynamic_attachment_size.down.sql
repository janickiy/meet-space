-- Never discard existing large attachments when rolling back the safety ceiling.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM chat_attachments WHERE size > 10485760)
 OR EXISTS(SELECT 1 FROM conversation_attachments WHERE size > 10485760) THEN
  RAISE EXCEPTION 'cannot restore 10 MiB ceiling while larger attachment metadata exists';
 END IF;
END $$;
ALTER TABLE chat_attachments DROP CONSTRAINT chat_attachments_size_check;
ALTER TABLE chat_attachments ADD CONSTRAINT chat_attachments_size_check CHECK(size BETWEEN 1 AND 10485760);
ALTER TABLE conversation_attachments DROP CONSTRAINT conversation_attachments_size_check;
ALTER TABLE conversation_attachments ADD CONSTRAINT conversation_attachments_size_check CHECK(size BETWEEN 1 AND 10485760);
