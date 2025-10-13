---
date: 2025-10-07
deciders: Core Team
doc_uuid: 3efefb72-d17b-4a34-984e-22e523efba65
id: adr-029
project_id: prism-data-layer
status: Accepted
tags:
- protobuf
- protocols
- observability
- debugging
title: Protocol Recording with Protobuf Tagging
---

## Context

Prism handles complex distributed protocols (Queue, PubSub, Transact) across multiple services. Need to:
- Record protocol interactions for debugging
- Trace multi-step operations
- Reconstruct failure scenarios
- Audit protocol compliance
- Enable replay for testing

**Requirements:**
- Capture protocol messages without code changes
- Tag messages for categorization and filtering
- Support sampling (don't record everything)
- Queryable storage
- Privacy-aware (PII handling)

## Decision

Use **Protobuf custom options for protocol recording tags**:

1. **Custom option `(prism.protocol)`**: Tag messages for recording
2. **Recording levels**: `NONE`, `METADATA`, `FULL`
3. **Sampling policy**: Configurable per message type
4. **Storage backend**: Pluggable (file, database, S3)
5. **Query interface**: Filter by tags, time, session, operation

## Rationale

### Why Custom Protobuf Options

Protobuf options allow **declarative metadata** on messages:
- No code changes needed
- Centralized configuration
- Type-safe
- Code generation aware
- Version controlled

### Protocol Option Definition

```protobuf
// proto/prism/options.proto
syntax = "proto3";

package prism;

import "google/protobuf/descriptor.proto";

// Protocol recording options
extend google.protobuf.MessageOptions {
  ProtocolOptions protocol = 50100;
}

message ProtocolOptions {
  // Recording level for this message type
  RecordingLevel recording = 1;

  // Protocol category
  string category = 2;  // "queue", "pubsub", "transact", "session"

  // Operation name
  string operation = 3;  // "publish", "subscribe", "write", "commit"

  // Sampling rate (0.0 - 1.0)
  float sample_rate = 4 [default = 1.0];

  // Include in protocol trace
  bool trace = 5 [default = true];

  // Tags for filtering
  repeated string tags = 6;
}

enum RecordingLevel {
  RECORDING_LEVEL_UNSPECIFIED = 0;
  RECORDING_LEVEL_NONE = 1;        // Don't record
  RECORDING_LEVEL_METADATA = 2;    // Record metadata only (no payload)
  RECORDING_LEVEL_FULL = 3;        // Record complete message
  RECORDING_LEVEL_SAMPLED = 4;     // Sample based on sample_rate
}
```

### Tagged Message Examples

**Queue Protocol:**
```protobuf
// proto/prism/queue/v1/queue.proto
import "prism/options.proto";

message PublishRequest {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_FULL
    category: "queue"
    operation: "publish"
    sample_rate: 0.1  // Record 10% of publish requests
    tags: ["write", "producer"]
  };

  string session_token = 1;
  string topic = 2;
  bytes payload = 3;
  // ...
}

message PublishResponse {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_METADATA
    category: "queue"
    operation: "publish_response"
    tags: ["write", "producer"]
  };

  string message_id = 1;
  int64 offset = 2;
  // ...
}

message Message {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_SAMPLED
    category: "queue"
    operation: "message_delivery"
    sample_rate: 0.05  // Record 5% of messages
    tags: ["read", "consumer"]
  };

  string message_id = 1;
  bytes payload = 2;
  // ...
}
```

**Transaction Protocol:**
```protobuf
// proto/prism/transact/v1/transact.proto
message WriteRequest {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_FULL
    category: "transact"
    operation: "write"
    sample_rate: 1.0  // Record all transactions
    trace: true
    tags: ["transaction", "write", "critical"]
  };

  DataWrite data = 1;
  MailboxWrite mailbox = 2;
  // ...
}

message TransactionStarted {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_METADATA
    category: "transact"
    operation: "begin"
    tags: ["transaction", "lifecycle"]
  };

  string transaction_id = 1;
  // ...
}
```

### Recording Infrastructure

**Protocol Recorder Interface:**
```rust
// proxy/src/protocol/recorder.rs
use prost::Message;

#[async_trait]
pub trait ProtocolRecorder: Send + Sync {
    async fn record(&self, entry: ProtocolEntry) -> Result<()>;
    async fn query(&self, filter: ProtocolFilter) -> Result<Vec<ProtocolEntry>>;
}

pub struct ProtocolEntry {
    pub id: String,
    pub timestamp: Timestamp,
    pub session_id: Option<String>,
    pub category: String,
    pub operation: String,
    pub message_type: String,
    pub recording_level: RecordingLevel,
    pub metadata: HashMap<String, String>,
    pub payload: Option<Vec<u8>>,  // Only if FULL recording
    pub tags: Vec<String>,
}

pub struct ProtocolFilter {
    pub start_time: Option<Timestamp>,
    pub end_time: Option<Timestamp>,
    pub session_id: Option<String>,
    pub category: Option<String>,
    pub operation: Option<String>,
    pub tags: Vec<String>,
}
```

**Interceptor for Recording:**
```rust
// proxy/src/protocol/interceptor.rs
pub struct RecordingInterceptor {
    recorder: Arc<dyn ProtocolRecorder>,
    sampler: Arc<Sampler>,
}

impl Interceptor for RecordingInterceptor {
    fn call(&mut self, req: Request<()>) -> Result<Request<()>, Status> {
        let message_type = req.extensions().get::<MessageType>().unwrap();

        // Get protocol options from generated code
        let options = get_protocol_options(message_type);

        // Check if should record
        if !should_record(&options, &self.sampler) {
            return Ok(req);
        }

        // Extract metadata
        let metadata = extract_metadata(&req);

        // Get payload based on recording level
        let payload = match options.recording {
            RecordingLevel::Full => Some(req.get_ref().encode_to_vec()),
            _ => None,
        };

        // Record
        let entry = ProtocolEntry {
            id: Uuid::new_v4().to_string(),
            timestamp: Utc::now(),
            session_id: metadata.get("session_id").cloned(),
            category: options.category.clone(),
            operation: options.operation.clone(),
            message_type: message_type.clone(),
            recording_level: options.recording,
            metadata,
            payload,
            tags: options.tags.clone(),
        };

        tokio::spawn(async move {
            recorder.record(entry).await.ok();
        });

        Ok(req)
    }
}
```

**Sampling Logic:**
```rust
pub struct Sampler {
    rng: ThreadRng,
}

impl Sampler {
    fn should_sample(&self, sample_rate: f32) -> bool {
        if sample_rate >= 1.0 {
            return true;
        }
        if sample_rate <= 0.0 {
            return false;
        }

        self.rng.gen::<f32>() < sample_rate
    }
}

fn should_record(options: &ProtocolOptions, sampler: &Sampler) -> bool {
    match options.recording {
        RecordingLevel::None => false,
        RecordingLevel::Metadata | RecordingLevel::Full => true,
        RecordingLevel::Sampled => sampler.should_sample(options.sample_rate),
        RecordingLevel::Unspecified => false,
    }
}
```

### Storage Backends

**File Storage:**
```rust
pub struct FileProtocolRecorder {
    path: PathBuf,
}

impl ProtocolRecorder for FileProtocolRecorder {
    async fn record(&self, entry: ProtocolEntry) -> Result<()> {
        let json = serde_json::to_string(&entry)?;
        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&self.path)?;
        writeln!(file, "{}", json)?;
        Ok(())
    }
}
```

**PostgreSQL Storage:**
```rust
pub struct PostgresProtocolRecorder {
    pool: PgPool,
}

impl ProtocolRecorder for PostgresProtocolRecorder {
    async fn record(&self, entry: ProtocolEntry) -> Result<()> {
        sqlx::query(
            r#"
            INSERT INTO protocol_recordings
            (id, timestamp, session_id, category, operation, message_type,
             recording_level, metadata, payload, tags)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
            "#
        )
        .bind(&entry.id)
        .bind(&entry.timestamp)
        .bind(&entry.session_id)
        .bind(&entry.category)
        .bind(&entry.operation)
        .bind(&entry.message_type)
        .bind(&entry.recording_level)
        .bind(&entry.metadata)
        .bind(&entry.payload)
        .bind(&entry.tags)
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    async fn query(&self, filter: ProtocolFilter) -> Result<Vec<ProtocolEntry>> {
        let mut query = QueryBuilder::new(
            "SELECT * FROM protocol_recordings WHERE 1=1"
        );

        if let Some(start) = filter.start_time {
            query.push(" AND timestamp >= ").push_bind(start);
        }
        if let Some(category) = filter.category {
            query.push(" AND category = ").push_bind(category);
        }
        if !filter.tags.is_empty() {
            query.push(" AND tags && ").push_bind(&filter.tags);
        }

        query.push(" ORDER BY timestamp DESC LIMIT 1000");

        let entries = query
            .build_query_as::<ProtocolEntry>()
            .fetch_all(&self.pool)
            .await?;

        Ok(entries)
    }
}
```

### Query Interface

**CLI Tool:**
```bash
# Query protocol recordings
prism-admin protocol query \
  --category queue \
  --operation publish \
  --session abc123 \
  --start "2025-10-07T00:00:00Z" \
  --tags write,producer

# Replay protocol sequence
prism-admin protocol replay \
  --session abc123 \
  --start "2025-10-07T12:00:00Z" \
  --end "2025-10-07T12:05:00Z"
```

**gRPC Admin API:**
```protobuf
service AdminService {
  // Query protocol recordings
  rpc QueryProtocol(QueryProtocolRequest) returns (stream ProtocolEntry);

  // Replay protocol sequence
  rpc ReplayProtocol(ReplayProtocolRequest) returns (stream ReplayEvent);
}

message QueryProtocolRequest {
  optional google.protobuf.Timestamp start_time = 1;
  optional google.protobuf.Timestamp end_time = 2;
  optional string session_id = 3;
  optional string category = 4;
  optional string operation = 5;
  repeated string tags = 6;
  int32 limit = 7;
}
```

### Privacy Considerations

**PII in Protocol Messages:**
```protobuf
message UserProfile {
  option (prism.protocol) = {
    recording: RECORDING_LEVEL_METADATA  // Don't record full payload
    category: "data"
    operation: "user_profile"
    tags: ["pii", "sensitive"]
  };

  string user_id = 1;
  string email = 2 [(prism.pii) = "email"];  // Flagged as PII
  string name = 3 [(prism.pii) = "name"];
}
```

**Automatic PII Scrubbing:**
```rust
fn scrub_pii(entry: &mut ProtocolEntry) {
    if entry.tags.contains(&"pii".to_string()) {
        // Scrub payload if contains PII
        if let Some(payload) = &mut entry.payload {
            *payload = scrub_pii_from_bytes(payload);
        }

        // Scrub metadata
        for (key, value) in &mut entry.metadata {
            if is_pii_field(key) {
                *value = "[REDACTED]".to_string();
            }
        }
    }
}
```

### Configuration

```yaml
# proxy/config.yaml
protocol_recording:
  enabled: true
  backend: postgres
  postgres:
    connection_string: postgres://...
    table: protocol_recordings

  # Override recording levels
  overrides:
    - message_type: "prism.queue.v1.Message"
      recording: RECORDING_LEVEL_NONE  # Disable for performance
    - category: "transact"
      recording: RECORDING_LEVEL_FULL  # Always record transactions

  # Global sampling
  default_sample_rate: 0.1  # 10% by default

  # Retention
  retention_days: 30
  auto_cleanup: true
```

### Alternatives Considered

1. **Application-level logging**
   - Pros: Simple, already exists
   - Cons: Not structured, hard to query, scattered
   - Rejected: Need structured protocol-specific recording

2. **Network packet capture**
   - Pros: Captures everything, no code changes
   - Cons: Binary parsing, performance impact, storage intensive
   - Rejected: Too low-level, hard to query

3. **OpenTelemetry spans**
   - Pros: Standard, integrates with tracing
   - Cons: Not protocol-specific, limited queryability
   - Deferred: Use for tracing, protocol recording for detailed protocol analysis

## Consequences

### Positive

- **Declarative**: Protocol recording via protobuf tags
- **Type-safe**: Options validated at compile time
- **Queryable**: Structured storage enables filtering
- **Sampling**: Control recording overhead
- **Privacy-aware**: PII handling built-in
- **Debuggable**: Reconstruct protocol sequences

### Negative

- **Storage overhead**: Recording consumes storage
- **Performance impact**: Interceptor adds latency (mitigated by async)
- **Complexity**: Another system to manage

### Neutral

- **Retention policy**: Must configure cleanup
- **Query performance**: Depends on storage backend

## Implementation Notes

### Code Generation

Extract protocol options in build:
```rust
// build.rs
fn main() {
    // Generate protocol option extractors
    prost_build::Config::new()
        .type_attribute(".", "#[derive(serde::Serialize)]")
        .compile_protos(&["proto/prism/queue/v1/queue.proto"], &["proto/"])
        .unwrap();
}
```

### Database Schema

```sql
CREATE TABLE protocol_recordings (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    session_id TEXT,
    category TEXT NOT NULL,
    operation TEXT NOT NULL,
    message_type TEXT NOT NULL,
    recording_level TEXT NOT NULL,
    metadata JSONB,
    payload BYTEA,
    tags TEXT[],

    -- Indexes for querying
    INDEX idx_timestamp ON protocol_recordings(timestamp),
    INDEX idx_session ON protocol_recordings(session_id),
    INDEX idx_category ON protocol_recordings(category),
    INDEX idx_tags ON protocol_recordings USING GIN(tags)
);

-- Retention policy
CREATE INDEX idx_retention ON protocol_recordings(timestamp)
WHERE timestamp < NOW() - INTERVAL '30 days';
```

## References

- [Protobuf Options](https://protobuf.dev/programming-guides/proto3/#options)
- ADR-003: Protobuf as Single Source of Truth
- ADR-008: Observability Strategy

## Revision History

- 2025-10-07: Initial draft and acceptance