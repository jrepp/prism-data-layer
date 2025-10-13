---
author: Platform Team
created: 2025-10-09
doc_uuid: 6b98adc6-65e5-4d25-ae1d-dbc54473da2d
id: rfc-023
project_id: prism-data-layer
status: Proposed
tags:
- plugin
- snapshotter
- streaming
- pagination
- object-storage
- write-only
- buffering
title: Publish Snapshotter Plugin - Write-Only Event Buffering with Pagination
updated: 2025-10-09
---

# RFC-023: Publish Snapshotter Plugin - Write-Only Event Buffering with Pagination

## Summary

Define a **publish snapshotter plugin** that provides write-only event capture with intelligent buffering, pagination, and durable storage. The snapshotter buffers N events for a writer, commits pages when size/time thresholds are reached, and publishes page metadata to an index. Supports multiple storage backends (object storage, local files) and serialization formats (protobuf, NDJSON). Session disconnects trigger safe page writes with no data loss.

**Key Features**:
1. **Write-only API**: Satisfies PubSub publish interface only (no subscription)
2. **Intelligent buffering**: Buffer N events per writer with configurable thresholds
3. **Page-based commits**: Write pages when size/time limits reached
4. **Durable storage**: Object storage (S3, MinIO) or local filesystem
5. **Index publishing**: Side channel publishes page metadata for discovery
6. **Session safety**: Disconnects flush buffered pages automatically
7. **Format flexibility**: Protobuf or NDJSON serialization

## Motivation

### Problem

Current streaming patterns require:
1. **Continuous consumers**: Data lost if no consumer is actively reading
2. **Complex buffering**: Application-level buffering adds complexity
3. **No replay**: Historical event replay requires separate CDC/WAL patterns
4. **Session fragility**: Connection drops lose buffered events

**Use Cases Requiring Snapshotter**:
- **Audit logging**: Write-only event capture with guaranteed durability
- **Event archival**: Store events for later analysis without active consumers
- **Data lake ingestion**: Buffer events and write large files to S3/MinIO
- **Session recording**: Capture user activity across sessions
- **Metrics collection**: Buffer high-volume metrics and batch write
- **Change capture**: Snapshot database changes to object storage

### Goals

1. **Durability**: Zero data loss even on session disconnect or plugin crash
2. **Efficiency**: Write large pages (MB-scale) instead of tiny messages
3. **Discoverability**: Index tracks all pages for query/replay
4. **Flexibility**: Support multiple storage backends and formats
5. **Simplicity**: Single pattern handles buffering, pagination, and indexing

## Architecture Overview

### Component Diagram

```text
┌────────────────────────────────────────────────────────────────┐
│                    Snapshotter Architecture                     │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Client (Writer)                                         │  │
│  │  - Publishes events via PubSub API                       │  │
│  └────────────────┬─────────────────────────────────────────┘  │
│                   │                                             │
│                   │ gRPC (PubSubBasicInterface)                 │
│                   ▼                                             │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Snapshotter Plugin                                      │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │  Event Buffer (per writer)                         │  │  │
│  │  │  - Buffer N events (default: 1000)                 │  │  │
│  │  │  - Track size (bytes) and age (duration)           │  │  │
│  │  │  - Flush on: size limit, time limit, disconnect    │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │  Page Writer                                       │  │  │
│  │  │  - Serialize to protobuf or NDJSON                 │  │  │
│  │  │  - Compress with gzip/zstd (optional)              │  │  │
│  │  │  - Write to storage backend                        │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │  Index Publisher                                   │  │  │
│  │  │  - Publish page metadata (key, size, event count)  │  │  │
│  │  │  - Enable discovery and replay                     │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  └─────────────────┬────────────────────┬───────────────────┘  │
│                    │                    │                       │
│        Storage Slot│                    │Index Slot             │
│                    ▼                    ▼                       │
│  ┌──────────────────────┐   ┌──────────────────────┐           │
│  │  Storage Backend     │   │  Index Backend       │           │
│  │  - S3/MinIO          │   │  - KeyValue (Redis)  │           │
│  │  - Local filesystem  │   │  - TimeSeries (DB)   │           │
│  │  - Azure Blob        │   │  - Search (Elastic)  │           │
│  └──────────────────────┘   └──────────────────────┘           │
└────────────────────────────────────────────────────────────────┘
```

## Backend Interface Decomposition

Following MEMO-006 principles, the snapshotter uses **two backend slots**:

### Slot 1: Storage Backend

**Purpose**: Durable page storage (object storage or files)

**Required Interface**: `storage_object` (new interface)

```protobuf
// proto/interfaces/storage_object.proto
syntax = "proto3";
package prism.interfaces.storage;

// Object storage operations (S3-like)
service StorageObjectInterface {
  rpc PutObject(PutObjectRequest) returns (PutObjectResponse);
  rpc GetObject(GetObjectRequest) returns (stream GetObjectResponse);
  rpc DeleteObject(DeleteObjectRequest) returns (DeleteObjectResponse);
  rpc ListObjects(ListObjectsRequest) returns (stream ObjectMetadata);
  rpc HeadObject(HeadObjectRequest) returns (ObjectMetadata);
}

message PutObjectRequest {
  string bucket = 1;        // Bucket/container name
  string key = 2;           // Object key (path)
  bytes data = 3;           // Object data (chunked for large objects)
  string content_type = 4;  // MIME type (e.g., "application/protobuf")
  map<string, string> metadata = 5;  // User metadata
  bool is_final_chunk = 6;  // True for last chunk in multipart
}

message PutObjectResponse {
  string etag = 1;          // ETag for verification
  int64 size = 2;           // Total object size
  string version_id = 3;    // Version ID (if versioning enabled)
}

message GetObjectRequest {
  string bucket = 1;
  string key = 2;
  int64 offset = 3;         // Byte offset (for range reads)
  int64 limit = 4;          // Bytes to read (0 = all)
}

message GetObjectResponse {
  bytes data = 1;           // Chunked data
  bool is_final_chunk = 2;
}

message DeleteObjectRequest {
  string bucket = 1;
  string key = 2;
  string version_id = 3;    // Optional version to delete
}

message DeleteObjectResponse {
  bool deleted = 1;
}

message ListObjectsRequest {
  string bucket = 1;
  string prefix = 2;        // Key prefix filter
  int32 max_keys = 3;       // Max results (0 = all)
  string continuation_token = 4;  // Pagination token
}

message ObjectMetadata {
  string key = 1;
  int64 size = 2;
  int64 last_modified = 3;  // Unix timestamp
  string etag = 4;
  string content_type = 5;
  map<string, string> metadata = 6;
}

message HeadObjectRequest {
  string bucket = 1;
  string key = 2;
}
```

**Backends implementing `storage_object`**:
- S3 (AWS)
- MinIO (self-hosted S3-compatible)
- Azure Blob Storage
- Google Cloud Storage
- Local filesystem (file:// protocol)

### Slot 2: Index Backend

**Purpose**: Track page metadata for discovery and replay

**Required Interfaces**: `keyvalue_basic` OR `timeseries_basic` OR `document_query`

**Option 1: KeyValue-based index** (simple, fast lookups)
```yaml
index:
  backend: redis
  interface: keyvalue_basic
  key_pattern: "snapshot:{writer_id}:{timestamp}:{sequence}"
  value: JSON metadata (page key, size, event count, start/end times)
```

**Option 2: TimeSeries-based index** (time-range queries)
```yaml
index:
  backend: clickhouse
  interface: timeseries_basic
  schema:
    - writer_id: string
    - page_key: string
    - page_size: int64
    - event_count: int64
    - start_time: timestamp
    - end_time: timestamp
```

**Option 3: Document-based index** (rich querying)
```yaml
index:
  backend: elasticsearch
  interface: document_query
  document:
    writer_id: string
    page_key: string
    page_size: int64
    event_count: int64
    start_time: timestamp
    end_time: timestamp
    content_type: string
    compression: string
```

## Snapshotter Plugin API

### PubSub Interface (Write-Only)

The snapshotter implements **only** the `Publish` method from `PubSubBasicInterface`:

```protobuf
// Implements subset of proto/interfaces/pubsub_basic.proto
service PubSubBasicInterface {
  rpc Publish(PublishRequest) returns (PublishResponse);
  // Subscribe NOT implemented (snapshotter is write-only)
}

message PublishRequest {
  string topic = 1;         // Writer identifier (session ID, user ID, etc.)
  bytes payload = 2;        // Event data
  map<string, string> attributes = 3;  // Event metadata
}

message PublishResponse {
  string message_id = 1;    // Unique message ID within buffer
  int64 sequence = 2;       // Sequence number in current page
}
```

### Snapshotter Configuration

```yaml
# Configuration for snapshotter plugin
plugin: snapshotter
version: v1.0.0

# Buffering configuration
buffer:
  max_events: 1000          # Flush after N events
  max_size_bytes: 10485760  # Flush after 10MB
  max_age_seconds: 300      # Flush after 5 minutes
  flush_on_disconnect: true # Flush buffer on session close

# Page configuration
page:
  format: protobuf          # "protobuf" or "ndjson"
  compression: gzip         # "none", "gzip", "zstd"
  target_size_mb: 10        # Target page size (soft limit)
  include_metadata: true    # Include event metadata in page

# Storage slot configuration
storage:
  backend: minio
  interface: storage_object
  config:
    endpoint: "minio:9000"
    bucket: "event-snapshots"
    access_key: "${MINIO_ACCESS_KEY}"
    secret_key: "${MINIO_SECRET_KEY}"
    key_template: "snapshots/{year}/{month}/{day}/{writer_id}/{timestamp}_{sequence}.pb.gz"
    # Local filesystem alternative:
    # backend: filesystem
    # path: "/var/lib/prism/snapshots"

# Index slot configuration
index:
  backend: redis
  interface: keyvalue_basic
  config:
    connection: "redis://localhost:6379/0"
    key_prefix: "snapshot:"
    ttl_days: 90            # Index entries expire after 90 days
    # Optional: publish to multiple indexes
    secondary:
      - backend: elasticsearch
        interface: document_query
        index: "event-snapshots"
```

## Page Format Specifications

### Protobuf Page Format

```protobuf
// proto/snapshotter/page.proto
syntax = "proto3";
package prism.snapshotter;

message EventPage {
  PageMetadata metadata = 1;
  repeated Event events = 2;
}

message PageMetadata {
  string writer_id = 1;         // Writer identifier
  int64 page_sequence = 2;      // Page sequence number for this writer
  int64 start_time = 3;         // Unix timestamp of first event
  int64 end_time = 4;           // Unix timestamp of last event
  int64 event_count = 5;        // Number of events in page
  int64 page_size_bytes = 6;    // Uncompressed page size
  string format_version = 7;    // Schema version (e.g., "v1")
  string compression = 8;       // "none", "gzip", "zstd"
  map<string, string> tags = 9; // User-defined tags
}

message Event {
  string event_id = 1;          // Unique event ID
  int64 timestamp = 2;          // Event timestamp
  bytes payload = 3;            // Event data
  map<string, string> attributes = 4;  // Event metadata
  int64 sequence = 5;           // Sequence within page
}
```

### NDJSON Page Format

```json
// Each line is a JSON object
{"metadata":{"writer_id":"user-123","page_sequence":42,"start_time":1696800000,"end_time":1696800300,"event_count":1000,"page_size_bytes":1048576,"format_version":"v1","compression":"gzip","tags":{"environment":"production","region":"us-west-2"}}}
{"event_id":"evt-001","timestamp":1696800000,"payload":"base64-encoded-data","attributes":{"type":"user.login"},"sequence":0}
{"event_id":"evt-002","timestamp":1696800001,"payload":"base64-encoded-data","attributes":{"type":"user.click"},"sequence":1}
...
{"event_id":"evt-1000","timestamp":1696800300,"payload":"base64-encoded-data","attributes":{"type":"user.logout"},"sequence":999}
```

**NDJSON Benefits**:
- Human-readable for debugging
- Line-by-line streaming processing
- Works with standard Unix tools (grep, awk, jq)
- No schema required

**Protobuf Benefits**:
- Smaller file sizes (30-50% vs NDJSON)
- Faster serialization/deserialization
- Schema evolution with backward compatibility
- Binary safety (no encoding issues)

## Page Lifecycle

### 1. Event Buffering

```text
Writer publishes event
    ↓
Buffer event in memory
    ↓
Check flush conditions:
  - max_events reached?
  - max_size_bytes reached?
  - max_age_seconds exceeded?
  - session disconnect?
    ↓
If YES → Flush page
If NO → Await next event
```

### 2. Page Flush

```text
Flush triggered
    ↓
Serialize events to format (protobuf/NDJSON)
    ↓
Compress with gzip/zstd (optional)
    ↓
Generate page key from template:
  snapshots/2025/10/09/user-123/1696800000_42.pb.gz
    ↓
Write to storage backend (PutObject)
    ↓
Publish page metadata to index
    ↓
Clear buffer
```

### 3. Index Publishing

```text
Page written successfully
    ↓
Generate index entry:
  key: snapshot:user-123:1696800000:42
  value: {
    "page_key": "snapshots/2025/10/09/user-123/1696800000_42.pb.gz",
    "writer_id": "user-123",
    "page_sequence": 42,
    "start_time": 1696800000,
    "end_time": 1696800300,
    "event_count": 1000,
    "page_size_bytes": 1048576,
    "storage_size_bytes": 524288,  // Compressed size
    "format": "protobuf",
    "compression": "gzip"
  }
    ↓
Publish to index backend (KeyValue.Set or TimeSeries.Insert)
    ↓
Index entry available for query
```

## Session Disconnect Handling

**Critical Requirement**: No data loss on disconnect.

```go
// Pseudo-code for session disconnect
func (s *Snapshotter) OnSessionClose(writerID string) error {
    // Get writer's buffer
    buffer := s.getBuffer(writerID)

    // Flush buffer even if small
    if buffer.Len() > 0 {
        page := s.serializePage(buffer)
        pageKey := s.generatePageKey(writerID, buffer.StartTime, buffer.Sequence)

        // Write to storage (blocking, no timeout)
        if err := s.storage.PutObject(pageKey, page); err != nil {
            // CRITICAL: Log error and retry until success
            s.logger.Error("Failed to flush page on disconnect, retrying...", err)
            return s.retryPutObject(pageKey, page, maxRetries)
        }

        // Publish index entry
        s.publishIndexEntry(pageKey, buffer.Metadata())

        // Clear buffer
        buffer.Clear()
    }

    return nil
}
```

**Guarantees**:
1. **Flush before disconnect**: Buffer flushed synchronously before session closes
2. **Retry on failure**: Storage writes retry until success (with backoff)
3. **Index eventual consistency**: Index updated best-effort (can lag storage)

## Query and Replay

### Query Index by Writer

```bash
# Using Redis KeyValue backend
redis-cli KEYS "snapshot:user-123:*"

# Using TimeSeries backend (ClickHouse)
SELECT page_key, event_count, start_time, end_time
FROM event_snapshots
WHERE writer_id = 'user-123'
  AND start_time >= unix_timestamp('2025-10-01')
  AND end_time <= unix_timestamp('2025-10-31')
ORDER BY start_time ASC
```

### Replay Events from Pages

```python
# Python replay example
import boto3
import gzip
from prism_pb2 import EventPage

s3 = boto3.client('s3')

def replay_writer_events(writer_id, start_time, end_time):
    # Query index for page keys
    page_keys = query_index(writer_id, start_time, end_time)

    for page_key in page_keys:
        # Download page from S3
        obj = s3.get_object(Bucket='event-snapshots', Key=page_key)
        compressed_data = obj['Body'].read()

        # Decompress
        data = gzip.decompress(compressed_data)

        # Deserialize
        page = EventPage()
        page.ParseFromString(data)

        # Process events
        for event in page.events:
            yield {
                'event_id': event.event_id,
                'timestamp': event.timestamp,
                'payload': event.payload,
                'attributes': dict(event.attributes)
            }

# Replay all events for user-123 in October 2025
for event in replay_writer_events('user-123', 1727740800, 1730419200):
    print(f"Event {event['event_id']} at {event['timestamp']}")
```

## Implementation Examples

### MemStore + Local Filesystem (Development)

```yaml
plugin: snapshotter
buffer:
  max_events: 100
  max_size_bytes: 1048576  # 1MB
  max_age_seconds: 60

page:
  format: ndjson
  compression: none

storage:
  backend: filesystem
  interface: storage_object
  config:
    base_path: "/tmp/prism-snapshots"
    key_template: "{writer_id}/{date}/{sequence}.ndjson"

index:
  backend: memstore
  interface: keyvalue_basic
  config:
    connection: "mem://local"
```

### MinIO + Redis (Production)

```yaml
plugin: snapshotter
buffer:
  max_events: 10000
  max_size_bytes: 10485760  # 10MB
  max_age_seconds: 300

page:
  format: protobuf
  compression: gzip

storage:
  backend: minio
  interface: storage_object
  config:
    endpoint: "minio.prod.internal:9000"
    bucket: "event-snapshots"
    access_key_env: "MINIO_ACCESS_KEY"
    secret_key_env: "MINIO_SECRET_KEY"
    key_template: "snapshots/{year}/{month}/{day}/{writer_id}/{timestamp}_{sequence}.pb.gz"
    enable_versioning: true

index:
  backend: redis
  interface: keyvalue_basic
  config:
    connection: "redis://redis.prod.internal:6379/0"
    key_prefix: "snapshot:"
    ttl_days: 90
    cluster_mode: true
```

### S3 + ClickHouse (Large Scale)

```yaml
plugin: snapshotter
buffer:
  max_events: 50000
  max_size_bytes: 52428800  # 50MB
  max_age_seconds: 600

page:
  format: protobuf
  compression: zstd
  target_size_mb: 50

storage:
  backend: s3
  interface: storage_object
  config:
    region: "us-west-2"
    bucket: "company-event-snapshots"
    key_template: "events/{year}/{month}/{day}/{writer_id}/{timestamp}_{sequence}.pb.zst"
    storage_class: "INTELLIGENT_TIERING"

index:
  backend: clickhouse
  interface: timeseries_basic
  config:
    connection: "clickhouse://clickhouse.prod.internal:9000/events"
    table: "event_snapshots"
    partitioning: "toYYYYMM(start_time)"
```

## Performance Characteristics

### Buffer Memory Usage

```text
Memory per writer = max_events × avg_event_size + overhead

Example with 10,000 events × 1KB each:
  Buffer size: ~10MB per active writer
  With 1,000 concurrent writers: ~10GB RAM
```

**Optimization**: Implement buffer eviction policy (LRU) if writer count exceeds memory limits.

### Page Write Throughput

```text
Events/sec per writer = max_events / max_age_seconds
Page writes/sec = concurrent_writers × (1 / max_age_seconds)

Example:
  10,000 events per page
  300 second max age
  1,000 concurrent writers

  Events/sec: 10,000 / 300 = 33 events/sec per writer
  Page writes/sec: 1,000 / 300 = 3.3 pages/sec = ~200 pages/min
```

### Storage Growth

```text
Daily storage = events_per_day × avg_event_size × (1 - compression_ratio)

Example with 1B events/day × 1KB each × 50% compression:
  Raw: 1TB/day
  Compressed: 500GB/day
  Monthly: 15TB
  Yearly: 180TB
```

**Cost Optimization**:
- Use S3 Intelligent Tiering (auto-moves to cheaper tiers)
- Set lifecycle policies (delete after 90 days, archive to Glacier)
- Implement page compaction (merge small pages periodically)

## Comparison to Alternatives

### vs. Standard PubSub

| Feature | Snapshotter | Standard PubSub |
|---------|-------------|-----------------|
| **Durability** | Guaranteed (written to storage) | Best-effort (lost if no consumer) |
| **Replay** | Full replay from storage | Limited (depends on retention) |
| **Buffering** | Intelligent page-based | Fixed message queue |
| **Storage cost** | Object storage (cheap) | Message broker (expensive) |
| **Latency** | Higher (buffered writes) | Lower (immediate delivery) |
| **Consumer coupling** | Decoupled (no active consumer needed) | Coupled (requires active subscriber) |

**Use Snapshotter When**:
- Durability > latency
- Events may need replay months later
- No active consumer at write time
- Cost-sensitive (object storage cheaper than Kafka/Redis)

### vs. Event Sourcing

| Feature | Snapshotter | Event Sourcing |
|---------|-------------|----------------|
| **Purpose** | Event capture and archival | Event-driven state management |
| **Replay** | Full event replay | Rebuild state from events |
| **State** | Stateless (no aggregates) | Stateful (aggregates, projections) |
| **Complexity** | Simple (write pages) | Complex (CQRS, projections) |
| **Query** | Index-based | Projection-based |

**Use Snapshotter When**:
- Don't need event sourcing complexity
- Just want durable event log
- Replay is occasional, not continuous

### vs. Database CDC

| Feature | Snapshotter | CDC (Debezium) |
|---------|-------------|----------------|
| **Source** | Application events | Database changes |
| **Coupling** | Decoupled from DB | Tightly coupled to DB |
| **Format** | Flexible (protobuf/NDJSON) | Database-specific |
| **Schema** | User-defined | Database schema |
| **Performance** | No DB overhead | Reads WAL/binlog |

**Use Snapshotter When**:
- Events come from application, not database
- Want control over format and schema
- Don't want database-specific tooling

## Operational Considerations

### Monitoring Metrics

```yaml
# Prometheus metrics
snapshotter_buffer_size_bytes{writer_id}      # Current buffer size per writer
snapshotter_buffer_event_count{writer_id}     # Events in buffer per writer
snapshotter_buffer_age_seconds{writer_id}     # Age of oldest event in buffer
snapshotter_page_writes_total                 # Total pages written
snapshotter_page_write_duration_seconds       # Time to write page
snapshotter_page_size_bytes                   # Page sizes (histogram)
snapshotter_index_publish_errors_total        # Index publish failures
snapshotter_storage_errors_total              # Storage backend errors
snapshotter_session_disconnects_total         # Disconnect-triggered flushes
```

### Health Checks

```bash
# Check buffer health
GET /health/buffers
Response: {
  "active_writers": 1234,
  "total_buffered_events": 5432100,
  "total_buffer_size_bytes": 5368709120,
  "oldest_buffer_age_seconds": 250,
  "writers_exceeding_age_limit": 12
}

# Check storage backend
GET /health/storage
Response: {
  "backend": "minio",
  "status": "healthy",
  "last_write_success": "2025-10-09T14:23:15Z",
  "write_success_rate": 0.9995,
  "avg_write_latency_ms": 45
}

# Check index backend
GET /health/index
Response: {
  "backend": "redis",
  "status": "healthy",
  "last_publish_success": "2025-10-09T14:23:15Z",
  "publish_success_rate": 0.999,
  "index_entry_count": 45678
}
```

### Failure Recovery

**Buffer Loss Prevention**:
1. **Periodic checkpoints**: Write buffer state to disk every 60 seconds
2. **Crash recovery**: Reload buffers from checkpoint on restart
3. **WAL option**: Optional write-ahead log for zero data loss (with performance cost)

**Storage Backend Failure**:
1. **Retry with backoff**: Exponential backoff up to 5 minutes
2. **Dead letter queue**: Move failed pages to DLQ after max retries
3. **Alert on failure**: Page writes to monitoring/alerting

**Index Backend Failure**:
1. **Best-effort**: Index publish failures don't block page writes
2. **Retry queue**: Failed index publishes queued for retry
3. **Reconciliation job**: Periodic job scans storage and rebuilds missing index entries

## Related Backend Interfaces

### New Interface: `storage_object`

**Add to MEMO-006 interface catalog**:

```yaml
# Backend interfaces
StorageObject (5 operations):
  - storage_object.proto - Object storage (PutObject, GetObject, DeleteObject, ListObjects, HeadObject)

# Backends implementing storage_object
- S3 (AWS)
- MinIO
- Azure Blob Storage
- Google Cloud Storage
- Local filesystem
```

### Existing Interfaces Used

**Index Slot Options**:
1. `keyvalue_basic` - Simple key-value lookups (Redis, DynamoDB)
2. `timeseries_basic` - Time-range queries (ClickHouse, TimescaleDB)
3. `document_query` - Rich querying (Elasticsearch, MongoDB)

## Open Questions

1. **Should snapshotter support batch publish API?**
   - **Proposal**: Add `BatchPublish` RPC for bulk event submission
   - **Trade-off**: More efficient but more complex client code

2. **Should page compaction be automatic or manual?**
   - **Proposal**: Optional background job merges small pages (&lt;1MB) into larger pages
   - **Benefit**: Reduces object count, improves replay performance

3. **Should index be updated synchronously or asynchronously?**
   - **Proposal**: Async by default (don't block page write), sync option for critical use cases
   - **Trade-off**: Async has eventual consistency delay

4. **Should snapshotter support multi-region replication?**
   - **Proposal**: Optional cross-region page replication (S3 cross-region replication)
   - **Use case**: Disaster recovery, compliance requirements

5. **Should page format support schema evolution?**
   - **Proposal**: Protobuf with schema registry (Confluent Schema Registry compatible)
   - **Benefit**: Track schema versions, enable backward/forward compatibility

## Related Documents

- [RFC-008: Proxy Plugin Architecture](/rfc/rfc-008-proxy-plugin-architecture) - Plugin system overview
- [MEMO-006: Backend Interface Decomposition](/memos/memo-006-backend-interface-decomposition-schema-registry) - Interface design principles
- [RFC-009: Distributed Reliability Patterns](/rfc/rfc-009-distributed-reliability-patterns) - Related patterns (Event Sourcing, CDC)
- [RFC-014: Layered Data Access Patterns](/rfc/rfc-014-layered-data-access-patterns) - PubSub pattern spec

## Revision History

- 2025-10-09: Initial RFC defining snapshotter plugin with interface decomposition, storage/index slots, and format options