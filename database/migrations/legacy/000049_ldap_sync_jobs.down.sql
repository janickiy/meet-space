DELETE FROM background_jobs WHERE kind='directory.sync';
DROP INDEX IF EXISTS idx_ldap_sync_one_pending;
DROP TABLE IF EXISTS ldap_sync_runs;
