-- Rename logins containing @ or + before rollback; never rename accounts implicitly.
ALTER TABLE administrators DROP CONSTRAINT administrators_username_check;
ALTER TABLE administrators ADD CONSTRAINT administrators_username_check
 CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,39}$');
