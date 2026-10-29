-- Deleted speech content cannot be reconstructed. Restore a pre-upgrade backup
-- together with the matching application release instead of a partial rollback.
DO $$ BEGIN
 RAISE EXCEPTION 'Speech feature removal is irreversible; restore a pre-upgrade database backup';
END $$;
