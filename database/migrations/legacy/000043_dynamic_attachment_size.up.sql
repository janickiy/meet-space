-- The live application policy enforces the configured limit; the database keeps
-- the absolute safety ceiling for both conference and personal attachments.
ALTER TABLE chat_attachments DROP CONSTRAINT chat_attachments_size_check;
ALTER TABLE chat_attachments ADD CONSTRAINT chat_attachments_size_check CHECK(size BETWEEN 1 AND 104857600);
ALTER TABLE conversation_attachments DROP CONSTRAINT conversation_attachments_size_check;
ALTER TABLE conversation_attachments ADD CONSTRAINT conversation_attachments_size_check CHECK(size BETWEEN 1 AND 104857600);
