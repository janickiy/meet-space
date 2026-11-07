-- Откат без преобразования корпоративных аккаунтов в локальные: такие аккаунты блокируются.
UPDATE users SET is_blocked=true, auth_invalid_before=clock_timestamp() WHERE auth_source='ldap';
UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp())
WHERE user_id IN (SELECT id FROM users WHERE auth_source='ldap');
DROP TABLE ldap_directory_state;
DROP TABLE ldap_identities;
DROP INDEX idx_users_admin_status_created;
DROP INDEX admin_users_registered;
DROP INDEX admin_users_search_trgm;
ALTER TABLE users DROP COLUMN auth_version, DROP COLUMN ldap_disabled, DROP COLUMN deleted_at, DROP COLUMN auth_source;
CREATE INDEX idx_users_admin_status_created ON users(is_blocked,created_at DESC,id) WHERE password_hash<>'';
CREATE INDEX admin_users_registered ON users(created_at DESC,id) WHERE guest_conference_id IS NULL AND password_hash<>'';
CREATE INDEX admin_users_search_trgm ON users USING gin(display_name gin_trgm_ops,email gin_trgm_ops) WHERE guest_conference_id IS NULL AND password_hash<>'';
