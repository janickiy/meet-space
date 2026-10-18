-- Общие пины для личных и групповых чатов.  История сообщений сохраняется.
CREATE TABLE conversation_message_pins (
 conversation_id UUID NOT NULL,
 message_id UUID NOT NULL,
 pinned_by_user_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(conversation_id,message_id),
 FOREIGN KEY(conversation_id,message_id) REFERENCES conversation_messages(conversation_id,id) ON DELETE CASCADE,
 FOREIGN KEY(conversation_id,pinned_by_user_id) REFERENCES conversation_members(conversation_id,user_id) ON DELETE RESTRICT
);
