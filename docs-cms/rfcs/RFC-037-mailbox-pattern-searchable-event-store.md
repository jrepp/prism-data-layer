---
id: rfc-037
title: "RFC-037: Mailbox Pattern - Searchable Event Store"
sidebar_label: "RFC-037: Mailbox Pattern"
rfc_number: 37
status: Proposed
created: 2025-10-15
updated: 2025-10-15
author: Claude Code
project_id: prism
doc_uuid: 8f7c2a1d-5e6b-4c9a-8d2f-3a1c4b5d6e7f
tags:
  - pattern
  - consumer
  - storage
  - sqlite
  - indexing
related_adrs:
  - ADR-005
related_rfcs:
  - RFC-014
  - RFC-017
  - RFC-033
---

# RFC-037: Mailbox Pattern - Searchable Event Store

## Summary

The **Mailbox Pattern** provides a searchable, persistent event store by consuming messages from a queue and storing them in a structured database with indexed headers and blob bodies. Headers are extracted from event metadata and stored as indexed table columns for efficient querying, while message bodies (which may be encrypted) are stored as opaque blobs.

## Motivation

### Use Cases

1. **Audit Logging**: Store all system events with searchable metadata (user, action, resource) but encrypted PII
2. **Email/Message Archives**: Store communications with searchable headers (from, to, subject, date) and encrypted bodies
3. **Event Sourcing**: Capture all domain events with indexed event types, aggregates, and timestamps
4. **System Observability**: Archive traces, logs, and metrics with searchable dimensions
5. **Compliance**: Retain records with searchable metadata while protecting sensitive payload data

### Problem Statement

Existing patterns lack a unified solution for:
- **Indexed Search**: Query events by metadata without scanning all messages
- **Encrypted Bodies**: Store sensitive payloads securely while maintaining header searchability
- **Schema Evolution**: Handle varying header schemas across different event types
- **Pluggable Storage**: Decouple pattern logic from storage backend (SQLite, PostgreSQL, ClickHouse)

## Design

### Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│                 Mailbox Pattern (Composite)                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌─────────────┐       ┌──────────────┐       ┌─────────────┐│
│  │  Message    │       │   Table      │       │   Table     ││
│  │  Consumer   │──────▶│   Writer     │       │   Reader    ││
│  │  Slot       │       │   Slot       │       │   Slot      ││
│  └─────────────┘       └──────────────┘       └─────────────┘│
│       │                       │                       ▲        │
│       │                       │                       │        │
│       │                       ▼                       │        │
│       │               ┌────────────────┐             │        │
│       │               │   SQLite DB    │─────────────┘        │
│       │               │  (Headers +    │                      │
│       │               │   Blob)        │                      │
│       │               └────────────────┘                      │
│       │                                                        │
│       ▼                                                        │
│  Extract Headers → Index Columns                              │
│  Store Body → Blob Column                                     │
│  Query Interface → Returns MailboxEvent[]                     │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Slot Architecture

The Mailbox Pattern has **three backend slots**:

#### Slot 1: Message Source (Queue Consumer)
- **Interface**: `QueueInterface` or `PubSubInterface`
- **Purpose**: Consume events from messaging backend
- **Implementations**: NATS, Kafka, Redis Streams, RabbitMQ
- **Configuration**: topic, consumer group, batch size

#### Slot 2: Storage Backend (Table Writer)
- **Interface**: `TableWriterInterface` (new)
- **Purpose**: Persist events with indexed headers
- **Implementations**: SQLite, PostgreSQL, ClickHouse
- **Configuration**: table name, indexed columns, retention policy

#### Slot 3: Query Interface (Table Reader)
- **Interface**: `TableReaderInterface` (new)
- **Purpose**: Retrieve stored messages as array of MailboxEvent (header + payload)
- **Implementations**: SQLite, PostgreSQL, ClickHouse (same backends as writer)
- **Configuration**: shared database connection with writer slot

### Message Structure

Messages consumed from the queue follow the standard PubSubMessage format:

```go
type PubSubMessage struct {
    Topic     string               // Event topic/stream
    Payload   []byte               // Message body (may be encrypted)
    Metadata  map[string]string    // Headers to extract and index
    MessageID string               // Unique message identifier
    Timestamp int64                // Event timestamp (Unix epoch millis)
}
```

### Header Extraction and Indexing

The pattern extracts well-known headers from `Metadata` map:

**Standard Indexed Headers**:
- `prism-message-id`: Unique message identifier
- `prism-timestamp`: Event timestamp
- `prism-topic`: Topic/stream name
- `prism-content-type`: Payload content type
- `prism-schema-id`: Schema registry ID (RFC-030)
- `prism-encryption`: Encryption algorithm (if encrypted)
- `prism-correlation-id`: Request correlation ID
- `prism-principal`: User/service identity
- `prism-namespace`: Prism namespace

**Custom Headers**:
Application-specific headers with `x-` prefix are also indexed (configurable).

### Table Schema

Default SQLite table schema:

```sql
CREATE TABLE IF NOT EXISTS mailbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Indexed headers (extracted from metadata)
    message_id TEXT NOT NULL UNIQUE,
    timestamp INTEGER NOT NULL,
    topic TEXT NOT NULL,
    content_type TEXT,
    schema_id TEXT,
    encryption TEXT,
    correlation_id TEXT,
    principal TEXT,
    namespace TEXT,

    -- Custom headers (JSON for flexibility)
    custom_headers TEXT, -- JSON map of x-* headers

    -- Body (opaque blob, may be encrypted)
    body BLOB NOT NULL,

    -- Metadata
    created_at INTEGER NOT NULL,

    -- Indexes for common queries
    INDEX idx_timestamp (timestamp),
    INDEX idx_topic (topic),
    INDEX idx_principal (principal),
    INDEX idx_correlation_id (correlation_id)
);
```

### Backend Interfaces

Two new backend interfaces for structured storage:

#### TableWriterInterface

```go
// TableWriterInterface defines operations for writing structured events
type TableWriterInterface interface {
    // WriteEvent stores an event with indexed headers and body
    WriteEvent(ctx context.Context, event *MailboxEvent) error

    // DeleteOldEvents removes events older than retention period
    DeleteOldEvents(ctx context.Context, olderThan int64) (int64, error)

    // GetTableStats returns storage statistics
    GetTableStats(ctx context.Context) (*TableStats, error)
}
```

#### TableReaderInterface

```go
// TableReaderInterface defines operations for reading structured events
type TableReaderInterface interface {
    // QueryEvents retrieves events matching filter criteria
    // Returns messages as array of MailboxEvent (header + payload)
    QueryEvents(ctx context.Context, filter *EventFilter) ([]*MailboxEvent, error)

    // GetEvent retrieves a single event by message ID
    GetEvent(ctx context.Context, messageID string) (*MailboxEvent, error)

    // GetTableStats returns storage statistics
    GetTableStats(ctx context.Context) (*TableStats, error)
}
```

#### Shared Types

```go
// MailboxEvent represents a structured event for storage
type MailboxEvent struct {
    MessageID     string
    Timestamp     int64
    Topic         string
    ContentType   string
    SchemaID      string
    Encryption    string
    CorrelationID string
    Principal     string
    Namespace     string
    CustomHeaders map[string]string // x-* headers
    Body          []byte             // Opaque blob
}

// EventFilter defines query criteria
type EventFilter struct {
    StartTime     *time.Time
    EndTime       *time.Time
    Topics        []string
    Principals    []string
    CorrelationID *string
    Limit         int
    Offset        int
}

// TableStats provides storage metrics
type TableStats struct {
    TotalEvents   int64
    TotalSizeBytes int64
    OldestEvent   time.Time
    NewestEvent   time.Time
}
```

### Pattern Configuration

YAML configuration for mailbox pattern:

```yaml
namespaces:
  - name: $admin
    pattern: mailbox
    pattern_version: 0.1.0
    description: Store admin events with searchable headers

    slots:
      message_source:
        backend: nats
        interfaces:
          - QueueInterface
        config:
          url: nats://localhost:4222
          subject: admin.events.>
          consumer_group: mailbox-admin
          durable: true

      storage:
        backend: sqlite
        interfaces:
          - TableWriterInterface
        config:
          database_path: /Users/jrepp/.prism/mailbox-admin.db
          table_name: mailbox
          indexed_headers:
            - prism-message-id
            - prism-timestamp
            - prism-topic
            - prism-principal
            - prism-correlation-id
          custom_header_pattern: "x-*"
          retention_days: 90

      query:
        backend: sqlite
        interfaces:
          - TableReaderInterface
        config:
          database_path: /Users/jrepp/.prism/mailbox-admin.db
          table_name: mailbox

    behavior:
      batch_size: 100
      auto_commit: true
      max_retries: 3
      retention_policy:
        max_age_days: 90
        max_size_gb: 10
```

### Pattern Behavior

**Message Processing Flow**:

1. **Consume**: Read message from queue (message_source slot)
2. **Extract**: Parse headers from `Metadata` map
3. **Transform**: Convert to `MailboxEvent` structure
4. **Store**: Write to table with indexed headers and blob body
5. **Commit**: Acknowledge message (if auto_commit enabled)

**Error Handling**:
- Parse errors → Skip message, log warning
- Storage errors → Retry with exponential backoff
- Max retries exceeded → Log to dead letter queue (if configured)

**Retention Policy**:
- Background job deletes events older than `max_age_days`
- Vacuum/compact database when exceeding `max_size_gb`

### Comparison to Alternatives

| Feature                | Mailbox Pattern | Consumer Pattern | Raw SQL          |
|------------------------|----------------|------------------|------------------|
| Indexed Headers        | ✅ Automatic    | ❌ Manual        | ✅ Manual        |
| Encrypted Bodies       | ✅ Supported    | ❌ Not handled   | ✅ Manual        |
| Pluggable Storage      | ✅ Slot-based   | ❌ None          | ❌ Fixed         |
| Schema Evolution       | ✅ JSON custom  | ❌ Not handled   | ⚠️ Migrations    |
| Query API              | ✅ Built-in     | ❌ None          | ✅ SQL           |
| Retention Management   | ✅ Automatic    | ❌ Manual        | ❌ Manual        |

## Implementation Plan

### Phase 1: Core Interfaces (Week 1)
- [ ] Define `TableWriterInterface` in `pkg/plugin/interfaces.go`
- [ ] Define `MailboxEvent`, `EventFilter`, `TableStats` types
- [ ] Add proto definitions for new interfaces

### Phase 2: SQLite Backend (Week 2)
- [ ] Implement SQLite table writer in `pkg/drivers/sqlite/`
- [ ] Create table schema with indexed columns
- [ ] Implement `WriteEvent`, `QueryEvents`, `DeleteOldEvents`
- [ ] Add connection pooling and WAL mode
- [ ] Write unit tests with testcontainers

### Phase 3: Mailbox Pattern (Week 2-3)
- [ ] Create `patterns/mailbox/` directory structure
- [ ] Implement mailbox pattern core logic
- [ ] Implement header extraction and mapping
- [ ] Add retention policy background job
- [ ] Implement `mailbox-runner` command
- [ ] Create `manifest.yaml`

### Phase 4: Integration & Testing (Week 3)
- [ ] Integration tests with NATS + SQLite
- [ ] Test encrypted body handling
- [ ] Test custom header indexing
- [ ] Load test with 100k events/sec
- [ ] Documentation and examples

### Phase 5: $admin Namespace Setup (Week 4)
- [ ] Configure mailbox pattern for `$admin` namespace
- [ ] Set up NATS subscription for `admin.*` topics
- [ ] Deploy with pattern-launcher
- [ ] Verify event capture and search

## Testing Strategy

### Unit Tests
- Header extraction from various metadata formats
- SQLite table writer operations
- Retention policy logic
- Error handling (storage failures, parse errors)

### Integration Tests
- End-to-end: NATS → Mailbox → SQLite → Query
- Encrypted body storage and retrieval
- Custom header indexing
- Concurrent writes (10 goroutines)

### Load Tests
- Throughput: 100k events/sec for 10 minutes
- Query performance: 1000 QPS on indexed headers
- Storage growth: 1M events = ~500MB database
- Retention policy: Delete 100k old events &lt;1 second

## Security Considerations

### Encrypted Bodies
- Pattern stores encrypted bodies as-is (opaque blobs)
- No decryption required for indexing headers
- Encryption indicated by `prism-encryption` header

### Access Control
- Namespace-level authorization via Prism auth layer
- SQLite file permissions: 0600 (owner read/write only)
- No direct database access from applications

### PII Handling
- Headers should NOT contain PII (by convention)
- PII must be in encrypted body
- Audit headers: user ID, action, resource (not names/emails)

## Open Questions

1. **PostgreSQL Support**: Should we implement PostgreSQL table writer in Phase 2 or defer?
   - **Decision**: Defer to Phase 6, focus on SQLite first

2. **Query Language**: Expose SQL directly or create filter DSL?
   - **Decision**: Start with `EventFilter` struct, add SQL query API later if needed

3. **Compression**: Should we compress bodies before storage?
   - **Decision**: No automatic compression. Applications can pre-compress and set `content-encoding` header

4. **Partitioning**: How to handle very large mailboxes (&gt;10M events)?
   - **Decision**: Use SQLite ATTACH for time-based partitions (one DB per month)

5. **Custom Index Columns**: Allow dynamic index creation at runtime?
   - **Decision**: No. Indexes defined at configuration time only

## Success Criteria

- ✅ Consume 10k events/sec from NATS with SQLite backend
- ✅ Query indexed headers with &lt;10ms latency (1M events)
- ✅ Support encrypted bodies without header degradation
- ✅ Automatic retention policy deletes old events
- ✅ Zero data loss during pattern restart (durable consumer)
- ✅ Integration with pattern-launcher and prism-admin

## References

- RFC-014: Layered Data Access Patterns (slot architecture)
- RFC-017: Multicast Registry Pattern (slot binding examples)
- RFC-030: Schema Evolution and Validation (schema-id header)
- RFC-033: Claim Check Pattern (large payload handling)
- ADR-005: Backend Plugin Architecture

## Appendix A: Example Queries

**Query by Time Range**:
```sql
SELECT message_id, timestamp, topic, principal
FROM mailbox
WHERE timestamp BETWEEN 1697000000000 AND 1697086400000
ORDER BY timestamp DESC
LIMIT 100;
```

**Query by Principal and Topic**:
```sql
SELECT message_id, timestamp, correlation_id, body
FROM mailbox
WHERE principal = 'user-123'
  AND topic LIKE 'admin.users.%'
ORDER BY timestamp DESC;
```

**Query by Correlation ID (Distributed Trace)**:
```sql
SELECT message_id, timestamp, topic, principal, body
FROM mailbox
WHERE correlation_id = 'trace-abc123'
ORDER BY timestamp ASC;
```

## Appendix B: SQLite Backend Details

**Connection Settings**:
```go
// Optimized SQLite settings for write-heavy workload
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=10000;
PRAGMA temp_store=MEMORY;
PRAGMA mmap_size=30000000000;
```

**Write Performance**:
- Batch inserts: 100 events per transaction
- WAL mode: 10x faster writes vs rollback journal
- Expected: 50k events/sec on SSD (single process)

**Query Performance**:
- Indexed queries: &lt;10ms for 1M events
- Full-text search: Add FTS5 virtual table for body search (if decrypted)
- Explain query plans with `EXPLAIN QUERY PLAN`

## Appendix C: Future Enhancements

### Phase 6: Additional Backends
- PostgreSQL table writer (horizontal scaling)
- ClickHouse table writer (OLAP analytics)
- DynamoDB table writer (serverless)

### Phase 7: Advanced Features
- Full-text search on decrypted bodies (opt-in)
- Time-series aggregations (events per hour/day)
- Materialized views for common queries
- Export to Parquet for data lake integration

### Phase 8: Admin UI
- Web UI for searching mailbox events
- Query builder for non-SQL users
- Event detail view with header/body inspection
- Export results to CSV/JSON
