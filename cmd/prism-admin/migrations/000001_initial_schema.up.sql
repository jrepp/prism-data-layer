-- Initial schema for prism-admin storage

-- Namespaces table
CREATE TABLE IF NOT EXISTS namespaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata TEXT -- JSON stored as TEXT for SQLite compatibility
);

CREATE INDEX IF NOT EXISTS idx_namespaces_name ON namespaces(name);

-- Proxies table (last known state)
CREATE TABLE IF NOT EXISTS proxies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    proxy_id TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    version TEXT,
    status TEXT CHECK(status IN ('healthy', 'unhealthy', 'unknown')) NOT NULL DEFAULT 'unknown',
    last_seen TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata TEXT -- JSON stored as TEXT
);

CREATE INDEX IF NOT EXISTS idx_proxies_proxy_id ON proxies(proxy_id);
CREATE INDEX IF NOT EXISTS idx_proxies_status ON proxies(status, last_seen);

-- Patterns table (active connections)
CREATE TABLE IF NOT EXISTS patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern_id TEXT NOT NULL,
    pattern_type TEXT NOT NULL,
    proxy_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    status TEXT CHECK(status IN ('active', 'stopped', 'error')) NOT NULL DEFAULT 'active',
    config TEXT, -- JSON stored as TEXT
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proxy_id) REFERENCES proxies(proxy_id) ON DELETE CASCADE,
    FOREIGN KEY (namespace) REFERENCES namespaces(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_patterns_namespace ON patterns(namespace);
CREATE INDEX IF NOT EXISTS idx_patterns_proxy ON patterns(proxy_id);
CREATE INDEX IF NOT EXISTS idx_patterns_pattern_id ON patterns(pattern_id);

-- Audit log table
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    namespace TEXT,
    method TEXT,
    path TEXT,
    status_code INTEGER,
    request_body TEXT, -- JSON stored as TEXT
    response_body TEXT, -- JSON stored as TEXT
    error TEXT,
    duration_ms INTEGER,
    client_ip TEXT,
    user_agent TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_namespace ON audit_logs(namespace);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user);

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

INSERT INTO schema_version (version, description) VALUES (1, 'Initial schema with namespaces, proxies, patterns, and audit logs');
