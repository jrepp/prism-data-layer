---
date: 2025-10-15
deciders: Engineering Team
doc_uuid: 8f3c4d2a-9b5e-4f1c-a2d7-3e8f9c1d5b4a
id: adr-054
project_id: prism-data-layer
status: Accepted
tags:
- admin
- database
- sqlite
- storage
- cli
- audit
title: 'ADR-054: SQLite Storage for prism-admin Local State'
---

## Context

The `prism-admin` CLI tool needs to persist operational state including:
- **Namespaces**: Configured namespaces and their settings
- **Proxy registry**: Last known proxies, their health status, and connection information
- **Pattern registry**: Active patterns connected to proxies
- **Audit log**: Complete record of all API interactions with the admin API

Currently, prism-admin is stateless and relies on querying live proxy instances. This creates issues:
- No historical data when proxies are down
- No audit trail of administrative actions
- Cannot track namespace configuration over time
- Difficult to debug past issues

We need a lightweight, embedded storage solution that requires zero external dependencies for local development and testing while supporting optional external database URNs for production deployments.

## Decision

Use SQLite as the default embedded storage backend for prism-admin with support for alternative database URNs via the `-db` flag:

```bash
# Default: Creates ~/.prism/admin.db
prism-admin server

# Custom SQLite location
prism-admin server -db sqlite:///path/to/admin.db

# PostgreSQL for production
prism-admin server -db postgresql://user:pass@host:5432/prism_admin
```

**Schema Design:**

```sql
-- Namespaces table
CREATE TABLE namespaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSON
);

-- Proxies table (last known state)
CREATE TABLE proxies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    proxy_id TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    version TEXT,
    status TEXT CHECK(status IN ('healthy', 'unhealthy', 'unknown')),
    last_seen TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSON
);

-- Patterns table (active connections)
CREATE TABLE patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern_id TEXT NOT NULL,
    pattern_type TEXT NOT NULL,
    proxy_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    status TEXT CHECK(status IN ('active', 'stopped', 'error')),
    config JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proxy_id) REFERENCES proxies(proxy_id),
    FOREIGN KEY (namespace) REFERENCES namespaces(name)
);

-- Audit log table
CREATE TABLE audit_logs (
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
    request_body JSON,
    response_body JSON,
    error TEXT,
    duration_ms INTEGER,
    client_ip TEXT,
    user_agent TEXT
);

-- Indexes for common queries
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_namespace ON audit_logs(namespace);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_proxies_status ON proxies(status, last_seen);
CREATE INDEX idx_patterns_namespace ON patterns(namespace);
CREATE INDEX idx_patterns_proxy ON patterns(proxy_id);
```

## Rationale

**Why SQLite as default:**
- Zero configuration: Works out-of-the-box with no setup
- Zero external dependencies: Embedded in the Go binary
- Cross-platform: Works on macOS, Linux, Windows
- Excellent for local development and testing
- Sufficient performance for admin workloads (writes are infrequent)
- Battle-tested reliability
- Built-in JSON support for flexible metadata storage

**Why support external database URNs:**
- Production deployments may require PostgreSQL for high availability
- Allows multiple prism-admin instances to share state
- Enables centralized audit logging
- Supports compliance requirements for audit log retention

### Alternatives Considered

1. **PostgreSQL only**
   - Pros: Production-ready, handles high concurrency
   - Cons: Requires external setup, overkill for local dev, increases friction
   - Rejected because: Developer experience suffers, local testing becomes complex

2. **JSON files**
   - Pros: Simple, human-readable
   - Cons: No transactional integrity, poor query performance, no concurrent access
   - Rejected because: Audit logs grow quickly, queries would be slow

3. **Embedded key-value store (BoltDB/BadgerDB)**
   - Pros: Fast, embedded, good for key-value access
   - Cons: Poor support for complex queries, no SQL, harder to inspect data
   - Rejected because: Audit log queries require filtering, joins, aggregations

4. **Redis**
   - Pros: Fast, supports various data structures
   - Cons: Requires external service, not embedded, persistence not primary use case
   - Rejected because: Not suitable for audit logs, requires external dependency

## Consequences

### Positive

- **Zero-config local development**: Developers can use prism-admin immediately
- **Audit compliance**: Complete trail of all administrative actions
- **Historical visibility**: View past proxy and pattern states even when offline
- **Debugging capability**: Troubleshoot issues using historical data
- **Flexibility**: Supports both embedded (SQLite) and external (PostgreSQL) databases
- **Standard tooling**: Can inspect/backup database with standard SQL tools
- **JSON columns**: Flexible schema for metadata without migrations

### Negative

- **SQLite limitations in production**:
  - Single-writer limitation (but admin writes are infrequent)
  - No network access (but can use external DB URN for multi-instance)
- **Schema migrations**: Need to manage database schema versions
- **Disk usage**: Audit logs grow over time, need rotation policy
- **Backup complexity**: Need to document backup procedures for both SQLite and PostgreSQL

### Neutral

- Database URN parsing adds configuration complexity
- Need to support two database drivers (sqlite3 and pgx)
- Must test both SQLite and PostgreSQL code paths

## Implementation Notes

### Database Driver Selection

**SQLite**: Use `modernc.org/sqlite` (pure Go, no CGO required)
- Avoids CGO cross-compilation issues
- Fully compatible with SQLite file format
- Excellent performance for admin workloads

**PostgreSQL**: Use `github.com/jackc/pgx/v5` (pure Go)
- Best-in-class PostgreSQL driver
- Native Go implementation

### Default Database Location

```go
func defaultDatabasePath() string {
    homeDir, _ := os.UserHomeDir()
    prismDir := filepath.Join(homeDir, ".prism")
    os.MkdirAll(prismDir, 0700)
    return filepath.Join(prismDir, "admin.db")
}
```

### Migration Strategy

Use `golang-migrate/migrate` with embedded migrations:

```go
//go:embed migrations/*.sql
var migrations embed.FS

func runMigrations(db *sql.DB, dbType string) error {
    driver, _ := sqlite.WithInstance(db, &sqlite.Config{})
    m, _ := migrate.NewWithDatabaseInstance(
        "embed://migrations",
        dbType,
        driver,
    )
    return m.Up()
}
```

### Audit Logging Middleware

Wrap all gRPC/HTTP handlers with audit logging:

```go
func AuditMiddleware(store *Storage) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Capture request body
            var bodyBytes []byte
            if r.Body != nil {
                bodyBytes, _ = io.ReadAll(r.Body)
                r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
            }

            // Wrap response writer to capture status code
            rec := &responseRecorder{ResponseWriter: w, statusCode: 200}

            // Execute handler
            next.ServeHTTP(rec, r)

            // Log audit entry
            store.LogAudit(context.Background(), &AuditEntry{
                Timestamp:   start,
                Action:      r.Method + " " + r.URL.Path,
                Method:      r.Method,
                Path:        r.URL.Path,
                StatusCode:  rec.statusCode,
                DurationMs:  time.Since(start).Milliseconds(),
                ClientIP:    r.RemoteAddr,
                UserAgent:   r.UserAgent(),
                RequestBody: json.RawMessage(bodyBytes),
            })
        })
    }
}
```

### Database URN Parsing

```go
func ParseDatabaseURN(urn string) (*DatabaseConfig, error) {
    if urn == "" {
        return &DatabaseConfig{
            Type: "sqlite",
            Path: defaultDatabasePath(),
        }, nil
    }

    // Parse sqlite:///path/to/db
    if strings.HasPrefix(urn, "sqlite://") {
        path := strings.TrimPrefix(urn, "sqlite://")
        return &DatabaseConfig{Type: "sqlite", Path: path}, nil
    }

    // Parse postgresql://... or postgres://...
    if strings.HasPrefix(urn, "postgres") {
        return &DatabaseConfig{Type: "postgresql", URN: urn}, nil
    }

    return nil, fmt.Errorf("unsupported database URN: %s", urn)
}
```

### Audit Log Retention

Implement configurable retention policy:

```sql
-- Delete audit logs older than 90 days (default)
DELETE FROM audit_logs WHERE timestamp < datetime('now', '-90 days');
```

Run as cron job or on prism-admin startup.

## References

- [ADR-036: SQLite Config Storage](/adr/adr-036) - Proxy config storage pattern
- [ADR-040: Go Binary Admin CLI](/adr/adr-040) - Admin CLI architecture
- [ADR-027: Admin API gRPC](/adr/adr-027) - Admin API design
- [SQLite JSON Functions](https://www.sqlite.org/json1.html)
- [golang-migrate](https://github.com/golang-migrate/migrate)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite)
- [pgx PostgreSQL driver](https://github.com/jackc/pgx)

## Revision History

- 2025-10-15: Initial draft
- 2025-10-15: Accepted - zero-config local storage for prism-admin
