-- Cover bounded administrative lists without changing application data.
CREATE INDEX IF NOT EXISTS admin_conferences_created ON conferences(created_at DESC,id) WHERE kind='meeting';
CREATE INDEX IF NOT EXISTS admin_record_created ON record(created_at DESC,id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS admin_users_registered ON users(created_at DESC,id) WHERE guest_conference_id IS NULL AND password_hash<>'';
