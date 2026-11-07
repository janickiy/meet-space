-- Keep the existing unique login and length limit while allowing email-shaped logins.
ALTER TABLE administrators DROP CONSTRAINT administrators_username_check;
ALTER TABLE administrators ADD CONSTRAINT administrators_username_check
 CHECK (username ~ '^[a-z0-9][a-z0-9.@+_-]{2,39}$');
