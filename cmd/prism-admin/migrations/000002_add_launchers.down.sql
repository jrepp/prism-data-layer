-- Rollback launchers table

DROP INDEX IF EXISTS idx_launchers_region;
DROP INDEX IF EXISTS idx_launchers_status;
DROP INDEX IF EXISTS idx_launchers_launcher_id;
DROP TABLE IF EXISTS launchers;

DELETE FROM schema_version WHERE version = 2;
