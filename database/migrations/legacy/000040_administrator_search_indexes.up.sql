-- pg_trgm is already required by migration 29. One multicolumn GIN index per
-- search surface supports the original per-field OR predicates without joining
-- fields together or repeating ILIKE checks for common, nonselective searches.
CREATE INDEX IF NOT EXISTS admin_users_search_trgm
    ON users USING gin (display_name gin_trgm_ops,email gin_trgm_ops)
    WHERE guest_conference_id IS NULL AND password_hash<>'';
CREATE INDEX IF NOT EXISTS admin_audit_search_trgm
    ON administrator_audit_log USING gin (username gin_trgm_ops,action gin_trgm_ops,object_id gin_trgm_ops,ip gin_trgm_ops);
