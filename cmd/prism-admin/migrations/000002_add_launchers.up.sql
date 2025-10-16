-- Add launchers table for tracking pattern launcher instances

CREATE TABLE IF NOT EXISTS launchers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    launcher_id TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    region TEXT,
    version TEXT,
    status TEXT CHECK(status IN ('healthy', 'unhealthy', 'unknown')) NOT NULL DEFAULT 'unknown',
    max_processes INTEGER DEFAULT 0,
    available_slots INTEGER DEFAULT 0,
    capabilities TEXT, -- JSON array stored as TEXT
    last_seen TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata TEXT -- JSON stored as TEXT
);

CREATE INDEX IF NOT EXISTS idx_launchers_launcher_id ON launchers(launcher_id);
CREATE INDEX IF NOT EXISTS idx_launchers_status ON launchers(status, last_seen);
CREATE INDEX IF NOT EXISTS idx_launchers_region ON launchers(region);

-- Update schema version
INSERT INTO schema_version (version, description) VALUES (2, 'Add launchers table for pattern launcher tracking');
