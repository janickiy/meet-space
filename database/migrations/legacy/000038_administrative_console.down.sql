-- Destructive rollback: export the administrators/audit tables before explicitly applying.
DROP TABLE IF EXISTS administrator_login_limits;
DROP TABLE IF EXISTS administrator_sessions;
DROP TABLE IF EXISTS administrator_audit_log;
DROP FUNCTION IF EXISTS deny_administrator_audit_mutation();
DROP TABLE IF EXISTS administrators;
DROP INDEX IF EXISTS idx_users_admin_status_created;
ALTER TABLE users DROP COLUMN IF EXISTS auth_invalid_before;
ALTER TABLE users DROP COLUMN IF EXISTS is_blocked;
