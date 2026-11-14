-- Keep the original author name when a message is forwarded to another chat.
ALTER TABLE conversation_messages
    ADD COLUMN forwarded_sender_name text,
    ADD CONSTRAINT conversation_messages_forwarded_sender_name_check
        CHECK (forwarded_sender_name IS NULL OR char_length(forwarded_sender_name) BETWEEN 1 AND 100);
