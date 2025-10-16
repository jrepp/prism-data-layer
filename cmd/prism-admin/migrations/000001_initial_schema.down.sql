-- Rollback initial schema

DROP INDEX IF EXISTS idx_audit_logs_user;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_resource;
DROP INDEX IF EXISTS idx_audit_logs_namespace;
DROP INDEX IF EXISTS idx_audit_logs_timestamp;
DROP TABLE IF EXISTS audit_logs;

DROP INDEX IF EXISTS idx_patterns_pattern_id;
DROP INDEX IF EXISTS idx_patterns_proxy;
DROP INDEX IF EXISTS idx_patterns_namespace;
DROP TABLE IF EXISTS patterns;

DROP INDEX IF EXISTS idx_proxies_status;
DROP INDEX IF EXISTS idx_proxies_proxy_id;
DROP TABLE IF EXISTS proxies;

DROP INDEX IF EXISTS idx_namespaces_name;
DROP TABLE IF EXISTS namespaces;

DROP TABLE IF EXISTS schema_version;
