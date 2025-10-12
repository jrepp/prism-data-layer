---
date: 2025-10-08
deciders: System
doc_uuid: 60fbdbbd-7374-4b88-abe0-815093ff15f8
id: adr-036
project_id: prism-data-layer
status: Proposed
tags:
- configuration
- deployment
- reliability
- operations
title: 'ADR-036: Local SQLite Storage for Namespace Configuration'
---

## Context

Prism proxy instances need to store and query namespace configurations:
- Backend connection strings
- Access pattern settings (consistency, cache TTL, rate limits)
- Feature flags per namespace
- Shadow traffic configuration
- Operational metadata (created_at, updated_at, owner)

### Current State: File-Based Configuration

Currently, configurations are loaded from YAML files:

```yaml
# config/namespaces.yaml
namespaces:
  - name: user-profiles
    backend: postgres
    pattern: keyvalue
    consistency: strong
    connection_string: postgres://db:5432/profiles
```

**Problems with file-based config**:
1. **No transactional updates**: Partial writes on crash leave inconsistent state
2. **No query capabilities**: Can't filter namespaces by backend, SLA, or tags
3. **Slow at scale**: Linear scan through 1000s of namespaces on startup
4. **No versioning**: Can't rollback bad config changes
5. **Admin API complexity**: Must parse YAML, validate, rewrite entire file

### Requirements

- **Fast reads**: Lookup namespace config in &lt;1ms
- **Transactional writes**: Atomic updates prevent corruption
- **Query support**: Filter by backend, tags, status, etc.
- **Version history**: Track config changes over time
- **Embedded**: No external database dependency
- **Durability**: Survive proxy restarts and crashes

## Decision

**Use SQLite as embedded configuration storage** for Prism proxy instances.

### Schema Design

```sql
-- Namespace configuration (primary table)
CREATE TABLE namespaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    backend TEXT NOT NULL,  -- 'postgres', 'redis', etc.
    pattern TEXT NOT NULL,  -- 'keyvalue', 'stream', etc.
    status TEXT NOT NULL DEFAULT 'active',  -- 'active', 'disabled', 'migrating'

    -- Backend configuration (JSON blob)
    backend_config TEXT NOT NULL,

    -- Access pattern settings
    consistency TEXT NOT NULL DEFAULT 'eventual',
    cache_ttl_seconds INTEGER,
    rate_limit_rps INTEGER,

    -- Operational metadata
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    created_by TEXT,

    -- Tags for filtering
    tags TEXT,  -- JSON array: ["production", "high-traffic"]

    CHECK(status IN ('active', 'disabled', 'migrating'))
);

-- Indexes for common queries
CREATE INDEX idx_namespaces_backend ON namespaces(backend);
CREATE INDEX idx_namespaces_status ON namespaces(status);
CREATE INDEX idx_namespaces_pattern ON namespaces(pattern);

-- Configuration change history
CREATE TABLE namespace_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    namespace_id INTEGER NOT NULL,
    operation TEXT NOT NULL,  -- 'create', 'update', 'delete'
    changed_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    changed_by TEXT,
    old_config TEXT,  -- JSON snapshot before change
    new_config TEXT,  -- JSON snapshot after change

    FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
);

CREATE INDEX idx_history_namespace ON namespace_history(namespace_id);
CREATE INDEX idx_history_changed_at ON namespace_history(changed_at);

-- Feature flags per namespace
CREATE TABLE namespace_features (
    namespace_id INTEGER NOT NULL,
    feature_name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),

    PRIMARY KEY(namespace_id, feature_name),
    FOREIGN KEY(namespace_id) REFERENCES namespaces(id) ON DELETE CASCADE
);
```

### File Location

/var/lib/prism/
├── config.db           # Primary SQLite database
├── config.db-wal       # Write-Ahead Log (SQLite WAL mode)
└── config.db-shm       # Shared memory file
```text

### Admin API Integration

```
use rusqlite::{Connection, params};

pub struct ConfigStore {
    conn: Connection,
}

impl ConfigStore {
    pub fn open(path: &Path) -> Result<Self> {
        let conn = Connection::open(path)?;

        // Enable WAL mode for better concurrency
        conn.execute_batch("PRAGMA journal_mode=WAL;")?;
        conn.execute_batch("PRAGMA synchronous=NORMAL;")?;

        Ok(Self { conn })
    }

    pub fn create_namespace(&self, ns: &Namespace) -> Result<i64> {
        let tx = self.conn.transaction()?;

        // Insert namespace
        tx.execute(
            "INSERT INTO namespaces (name, backend, pattern, backend_config, consistency, created_by)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![ns.name, ns.backend, ns.pattern, ns.backend_config_json, ns.consistency, ns.created_by],
        )?;

        let namespace_id = tx.last_insert_rowid();

        // Record in history
        tx.execute(
            "INSERT INTO namespace_history (namespace_id, operation, new_config, changed_by)
             VALUES (?1, 'create', ?2, ?3)",
            params![namespace_id, ns.to_json(), ns.created_by],
        )?;

        tx.commit()?;

        Ok(namespace_id)
    }

    pub fn get_namespace(&self, name: &str) -> Result<Option<Namespace>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, name, backend, pattern, backend_config, consistency, status, cache_ttl_seconds
             FROM namespaces WHERE name = ?1 AND status = 'active'"
        )?;

        let ns = stmt.query_row(params![name], |row| {
            Ok(Namespace {
                id: row.get(0)?,
                name: row.get(1)?,
                backend: row.get(2)?,
                pattern: row.get(3)?,
                backend_config_json: row.get(4)?,
                consistency: row.get(5)?,
                status: row.get(6)?,
                cache_ttl_seconds: row.get(7)?,
            })
        }).optional()?;

        Ok(ns)
    }

    pub fn list_namespaces_by_backend(&self, backend: &str) -> Result<Vec<Namespace>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, name, backend, pattern, backend_config, consistency
             FROM namespaces WHERE backend = ?1 AND status = 'active'
             ORDER BY name"
        )?;

        let namespaces = stmt.query_map(params![backend], |row| {
            Ok(Namespace {
                id: row.get(0)?,
                name: row.get(1)?,
                backend: row.get(2)?,
                pattern: row.get(3)?,
                backend_config_json: row.get(4)?,
                consistency: row.get(5)?,
                ..Default::default()
            })
        })?.collect::<Result<Vec<_>, _>>()?;

        Ok(namespaces)
    }
}
```text

## Rationale

### Why SQLite?

**Embedded**: No external database to deploy, manage, or monitor
- Prism proxy is self-contained
- Works in containers, bare metal, edge deployments
- Zero operational overhead

**Performance**:
- Reads: &lt;0.1ms for indexed queries
- Writes: &lt;1ms with WAL mode
- Concurrent reads: unlimited (with WAL mode)
- File size: ~10KB per namespace (1000 namespaces = 10MB)

**Reliability**:
- ACID transactions prevent corruption
- WAL mode for crash recovery
- Proven: used by browsers, mobile apps, billions of deployments

**Queryability**:
- SQL enables complex filters: `WHERE backend = 'redis' AND tags LIKE '%production%'`
- Aggregations: `SELECT backend, COUNT(*) FROM namespaces GROUP BY backend`
- Joins: correlate namespace configs with history

**Versioning**:
- `namespace_history` table tracks all changes
- Rollback: restore from `old_config` snapshot
- Audit: who changed what, when

### Compared to Alternatives

**vs File-based YAML**:
- ✅ Transactional updates (YAML = all-or-nothing file rewrite)
- ✅ Query support (YAML = linear scan)
- ✅ Version history (YAML = external version control needed)

**vs etcd/Consul**:
- ✅ No external dependencies (etcd = separate cluster)
- ✅ No network hops (etcd = remote calls)
- ❌ No distributed consensus (etcd = multi-node consistency)
- **When to use etcd**: Multi-instance Prism clusters sharing config (see ADR-037)

**vs PostgreSQL**:
- ✅ Embedded, no separate database (PostgreSQL = external service)
- ✅ Simpler operations (PostgreSQL = backups, replication, etc.)
- ❌ Not distributed (PostgreSQL = HA, replication)
- **When to use PostgreSQL**: Large-scale multi-region deployments

## Alternatives Considered

### 1. Continue with YAML Files

- **Pros**: Simple, human-readable, easy to version control
- **Cons**: No transactions, no queries, slow at scale, no history
- **Rejected because**: Doesn't scale beyond 100s of namespaces

### 2. etcd/Consul for Configuration

- **Pros**: Distributed, HA, multi-instance sharing
- **Cons**: External dependency, operational complexity, network latency
- **Rejected because**: Overkill for single-instance Prism, adds deployment complexity
- **Reconsidered for**: Multi-instance clusters (see ADR-037)

### 3. PostgreSQL as Config Store

- **Pros**: Full SQL, proven at scale, rich ecosystem
- **Cons**: External dependency, separate HA/backup strategy, network latency
- **Rejected because**: Defeats purpose of Prism being self-contained

### 4. Protobuf Binary Files

- **Pros**: Compact, type-safe, fast parsing
- **Cons**: Not human-readable, no SQL queries, no transactions
- **Rejected because**: Gives up queryability and atomicity

## Consequences

### Positive

- **Fast lookups**: O(log n) index scans, &lt;1ms latency
- **Atomic updates**: Transactions prevent config corruption
- **Rich queries**: SQL enables filtering, aggregation, joins
- **Audit trail**: History table tracks all changes
- **Zero dependencies**: Embedded, no external services needed
- **Small footprint**: ~10KB per namespace, 10MB for 1000 namespaces

### Negative

- **Single-instance only**: SQLite file not shareable across proxy instances
- **Write concurrency**: Single writer (WAL mode helps, but still a bottleneck at high write rates)
- **No replication**: Losing the file means losing config (backup strategy needed)
- **File corruption risk**: Rare, but disk corruption can invalidate database

### Neutral

- **Backup strategy**: Must back up SQLite file (simple file copy during WAL checkpoint)
- **Migration from YAML**: Need one-time migration script to import existing configs
- **Multi-instance deployments**: Need different approach (etcd, or Kubernetes CRDs via ADR-037)

## Implementation Notes

### Initialization on Startup

```
pub async fn initialize_config_store() -> Result<ConfigStore> {
    let db_path = Path::new("/var/lib/prism/config.db");

    // Create parent directory
    std::fs::create_dir_all(db_path.parent().unwrap())?;

    let store = ConfigStore::open(db_path)?;

    // Run migrations
    store.migrate()?;

    // Import from YAML if database is empty
    if store.count_namespaces()? == 0 {
        store.import_from_yaml("config/namespaces.yaml").await?;
    }

    Ok(store)
}
```text

### Backup Strategy

```
# Daily backup via cron
#!/bin/bash
# /etc/cron.daily/prism-config-backup

DB_PATH=/var/lib/prism/config.db
BACKUP_DIR=/var/backups/prism

# Wait for WAL checkpoint
sqlite3 $DB_PATH "PRAGMA wal_checkpoint(TRUNCATE);"

# Copy database file
cp $DB_PATH $BACKUP_DIR/config-$(date +%Y%m%d).db

# Retain last 30 days
find $BACKUP_DIR -name "config-*.db" -mtime +30 -delete
```text

### Read-Heavy Optimization

```
// Use connection pool for concurrent reads
use r2d2_sqlite::SqliteConnectionManager;
use r2d2::Pool;

pub struct ConfigStore {
    pool: Pool<SqliteConnectionManager>,
}

impl ConfigStore {
    pub fn open(path: &Path) -> Result<Self> {
        let manager = SqliteConnectionManager::file(path)
            .with_init(|conn| {
                conn.execute_batch("PRAGMA journal_mode=WAL;")?;
                conn.execute_batch("PRAGMA query_only=ON;")?;  // Read-only for pooled connections
                Ok(())
            });

        let pool = Pool::builder()
            .max_size(10)  // 10 concurrent read connections
            .build(manager)?;

        Ok(Self { pool })
    }
}
```text

### Multi-Instance Deployment (Future)

For multi-instance Prism deployments (ADR-034 sharding, ADR-037 Kubernetes):

**Option 1**: Each instance has own SQLite, sync via Kubernetes ConfigMaps
**Option 2**: Use etcd for distributed config, fall back to SQLite cache
**Option 3**: Kubernetes CRDs as source of truth, SQLite as local cache

See ADR-037 for full multi-instance strategy.

## References

- [SQLite in Production](https://www.sqlite.org/whentouse.html)
- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [rusqlite Documentation](https://docs.rs/rusqlite/)
- ADR-037: Kubernetes Operator (multi-instance config sync)
- ADR-033: Capability API (reads from config store)

## Revision History

- 2025-10-08: Initial draft proposing SQLite for local config storage

```