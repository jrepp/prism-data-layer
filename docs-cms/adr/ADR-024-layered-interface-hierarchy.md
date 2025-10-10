---
id: adr-024
title: "ADR-024: Layered Interface Hierarchy"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['architecture', 'api-design', 'interfaces', 'use-cases']
---

## Context

Prism needs a coherent interface hierarchy that:
- Starts with basic primitives (sessions, auth, auditing)
- Builds up to use-case-specific operations
- Maintains clean separation of concerns
- Supports multiple backend implementations
- Enables progressive disclosure of complexity

**Interface Layers:**
1. **Session Layer**: Authorization, auditing, connection state
2. **Queue Layer**: Kafka-style message queues
3. **Pub/Sub Layer**: NATS-style publish-subscribe
4. **Paged Reader Layer**: Database pagination and queries
5. **Transact Write Layer**: Two-table transactional writes

## Decision

Implement **layered interface hierarchy** with clear dependencies:

1. **Session as foundation**: All operations require active session
2. **Layer independence**: Each use-case layer operates independently
3. **Composable operations**: Clients can use multiple layers simultaneously
4. **Backend polymorphism**: Each layer supports multiple backend implementations
5. **Protobuf definitions**: All interfaces defined in protobuf

## Rationale

### Layer Hierarchy

```text
                    ┌──────────────────────────────────┐
                    │      Client Applications         │
                    └────────────┬─────────────────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        │                        │                        │
┌───────▼────────┐    ┌──────────▼──────┐    ┌──────────▼──────┐
│ Queue Layer    │    │ PubSub Layer    │    │ Reader Layer    │
│ (Kafka-style)  │    │ (NATS-style)    │    │ (DB pagination) │
└───────┬────────┘    └──────────┬──────┘    └──────────┬──────┘
        │                        │                        │
        │             ┌──────────▼──────┐                │
        │             │ Transact Layer  │                │
        │             │ (2-table write) │                │
        │             └──────────┬──────┘                │
        │                        │                        │
        └────────────────────────┼────────────────────────┘
                                 │
                    ┌────────────▼─────────────┐
                    │     Session Layer        │
                    │  (auth, audit, state)    │
                    └────────────┬─────────────┘
                                 │
                    ┌────────────▼─────────────┐
                    │     Prism Proxy Core     │
                    └──────────────────────────┘
```

### Layer 1: Session Service

**Purpose**: Foundation for all operations - authentication, authorization, auditing, connection state

```protobuf
// proto/prism/session/v1/session_service.proto
syntax = "proto3";

package prism.session.v1;

import "google/protobuf/timestamp.proto";
import "prism/config/v1/client_config.proto";

service SessionService {
  // Create new session with client configuration
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);

  // Close session cleanly
  rpc CloseSession(CloseSessionRequest) returns (CloseSessionResponse);

  // Heartbeat to keep session alive
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

  // Get session info
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
}

message CreateSessionRequest {
  // Authentication credentials
  oneof auth {
    string api_key = 1;
    string jwt_token = 2;
    MutualTLSAuth mtls = 3;
  }

  // Client configuration (named or inline)
  oneof config {
    string config_name = 4;
    prism.config.v1.ClientConfig inline_config = 5;
  }

  // Client metadata
  string client_id = 6;
  string client_version = 7;
}

message CreateSessionResponse {
  // Session token for subsequent requests
  string session_token = 1;

  // Session metadata
  string session_id = 2;
  google.protobuf.Timestamp created_at = 3;
  google.protobuf.Timestamp expires_at = 4;

  // Resolved configuration
  prism.config.v1.ClientConfig config = 5;
}

message CloseSessionRequest {
  string session_token = 1;
  bool force = 2;  // Force close even with pending operations
}

message CloseSessionResponse {
  bool success = 1;
  string message = 2;
}

message HeartbeatRequest {
  string session_token = 1;
}

message HeartbeatResponse {
  google.protobuf.Timestamp server_time = 1;
  int32 ttl_seconds = 2;
}

message MutualTLSAuth {
  bytes client_cert = 1;
}
```

**Session State:**
- Active sessions tracked server-side
- Idle timeout (default: 5 minutes)
- Max session duration (default: 24 hours)
- Heartbeat keeps session alive
- Clean closure releases resources

### Layer 2: Queue Service

**Purpose**: Kafka-style message queue operations

```protobuf
// proto/prism/queue/v1/queue_service.proto
syntax = "proto3";

package prism.queue.v1;

import "google/protobuf/timestamp.proto";

service QueueService {
  // Publish message to topic
  rpc Publish(PublishRequest) returns (PublishResponse);

  // Subscribe to topic (server streaming)
  rpc Subscribe(SubscribeRequest) returns (stream Message);

  // Acknowledge message processing
  rpc Acknowledge(AcknowledgeRequest) returns (AcknowledgeResponse);

  // Commit offset
  rpc Commit(CommitRequest) returns (CommitResponse);

  // Seek to offset
  rpc Seek(SeekRequest) returns (SeekResponse);
}

message PublishRequest {
  string session_token = 1;
  string topic = 2;
  bytes payload = 3;
  map<string, string> headers = 4;
  optional string partition_key = 5;
}

message PublishResponse {
  string message_id = 1;
  int64 offset = 2;
  int32 partition = 3;
}

message SubscribeRequest {
  string session_token = 1;
  string topic = 2;
  string consumer_group = 3;
  optional int64 start_offset = 4;
}

message Message {
  string message_id = 1;
  bytes payload = 2;
  map<string, string> headers = 3;
  int64 offset = 4;
  int32 partition = 5;
  google.protobuf.Timestamp timestamp = 6;
}

message AcknowledgeRequest {
  string session_token = 1;
  string message_id = 2;
}

message AcknowledgeResponse {
  bool success = 1;
}

message CommitRequest {
  string session_token = 1;
  string topic = 2;
  int32 partition = 3;
  int64 offset = 4;
}

message CommitResponse {
  bool success = 1;
}
```

**Backend Mapping:**
- **Kafka**: Direct mapping to topics/partitions/offsets
- **NATS JetStream**: Stream/consumer/sequence
- **Postgres**: Table-based queue with SKIP LOCKED

### Layer 3: PubSub Service

**Purpose**: NATS-style publish-subscribe with topics and wildcards

```protobuf
// proto/prism/pubsub/v1/pubsub_service.proto
syntax = "proto3";

package prism.pubsub.v1;

import "google/protobuf/timestamp.proto";

service PubSubService {
  // Publish event to topic
  rpc Publish(PublishRequest) returns (PublishResponse);

  // Subscribe to topic pattern (server streaming)
  rpc Subscribe(SubscribeRequest) returns (stream Event);

  // Unsubscribe from topic
  rpc Unsubscribe(UnsubscribeRequest) returns (UnsubscribeResponse);
}

message PublishRequest {
  string session_token = 1;
  string topic = 2;  // e.g., "events.user.created"
  bytes payload = 3;
  map<string, string> metadata = 4;
}

message PublishResponse {
  string event_id = 1;
  google.protobuf.Timestamp published_at = 2;
}

message SubscribeRequest {
  string session_token = 1;
  string topic_pattern = 2;  // e.g., "events.user.*"
  optional string queue_group = 3;  // For load balancing
}

message Event {
  string event_id = 1;
  string topic = 2;
  bytes payload = 3;
  map<string, string> metadata = 4;
  google.protobuf.Timestamp timestamp = 5;
}

message UnsubscribeRequest {
  string session_token = 1;
  string topic_pattern = 2;
}

message UnsubscribeResponse {
  bool success = 1;
}
```

**Backend Mapping:**
- **NATS**: Native subject-based routing with wildcards
- **Kafka**: Topic prefix matching
- **Redis Pub/Sub**: Channel pattern subscription

### Layer 4: Reader Service

**Purpose**: Database-style paged reading and queries

```protobuf
// proto/prism/reader/v1/reader_service.proto
syntax = "proto3";

package prism.reader.v1;

import "google/protobuf/struct.proto";

service ReaderService {
  // Read pages of data (server streaming)
  rpc Read(ReadRequest) returns (stream Page);

  // Query with filters (server streaming)
  rpc Query(QueryRequest) returns (stream Row);

  // Count matching records
  rpc Count(CountRequest) returns (CountResponse);
}

message ReadRequest {
  string session_token = 1;
  string collection = 2;
  int32 page_size = 3;
  optional string cursor = 4;  // Continuation token
  repeated string fields = 5;  // Projection
}

message Page {
  repeated Row rows = 1;
  optional string next_cursor = 2;
  bool has_more = 3;
}

message QueryRequest {
  string session_token = 1;
  string collection = 2;
  Filter filter = 3;
  repeated Sort sort = 4;
  int32 page_size = 5;
  optional string cursor = 6;
}

message Filter {
  oneof filter {
    FieldFilter field = 1;
    CompositeFilter composite = 2;
  }
}

message FieldFilter {
  string field = 1;
  Operator op = 2;
  google.protobuf.Value value = 3;

  enum Operator {
    OPERATOR_UNSPECIFIED = 0;
    OPERATOR_EQUALS = 1;
    OPERATOR_NOT_EQUALS = 2;
    OPERATOR_GREATER_THAN = 3;
    OPERATOR_LESS_THAN = 4;
    OPERATOR_IN = 5;
    OPERATOR_CONTAINS = 6;
  }
}

message CompositeFilter {
  LogicalOperator op = 1;
  repeated Filter filters = 2;

  enum LogicalOperator {
    LOGICAL_OPERATOR_UNSPECIFIED = 0;
    LOGICAL_OPERATOR_AND = 1;
    LOGICAL_OPERATOR_OR = 2;
  }
}

message Sort {
  string field = 1;
  Direction direction = 2;

  enum Direction {
    DIRECTION_UNSPECIFIED = 0;
    DIRECTION_ASC = 1;
    DIRECTION_DESC = 2;
  }
}

message Row {
  map<string, google.protobuf.Value> fields = 1;
}

message CountRequest {
  string session_token = 1;
  string collection = 2;
  optional Filter filter = 3;
}

message CountResponse {
  int64 count = 1;
}
```

**Backend Mapping:**
- **Postgres**: SQL queries with LIMIT/OFFSET
- **SQLite**: Same as Postgres
- **DynamoDB**: Query with pagination tokens
- **Neptune**: Gremlin queries with pagination

### Layer 5: Transact Service

**Purpose**: Transactional writes across two tables (inbox/outbox pattern)

```protobuf
// proto/prism/transact/v1/transact_service.proto
syntax = "proto3";

package prism.transact.v1;

import "google/protobuf/struct.proto";

service TransactService {
  // Single transactional write
  rpc Write(WriteRequest) returns (WriteResponse);

  // Streaming transaction
  rpc Transaction(stream TransactRequest) returns (stream TransactResponse);
}

message WriteRequest {
  string session_token = 1;

  // Data table write
  DataWrite data = 2;

  // Mailbox table write
  MailboxWrite mailbox = 3;

  // Transaction options
  TransactionOptions options = 4;
}

message DataWrite {
  string table = 1;
  map<string, google.protobuf.Value> record = 2;
  WriteMode mode = 3;

  enum WriteMode {
    WRITE_MODE_UNSPECIFIED = 0;
    WRITE_MODE_INSERT = 1;
    WRITE_MODE_UPDATE = 2;
    WRITE_MODE_UPSERT = 3;
  }
}

message MailboxWrite {
  string mailbox_id = 1;
  bytes message = 2;
  map<string, string> metadata = 3;
}

message TransactionOptions {
  IsolationLevel isolation = 1;
  int32 timeout_ms = 2;

  enum IsolationLevel {
    ISOLATION_LEVEL_UNSPECIFIED = 0;
    ISOLATION_LEVEL_READ_COMMITTED = 1;
    ISOLATION_LEVEL_SERIALIZABLE = 2;
  }
}

message WriteResponse {
  string transaction_id = 1;
  bool committed = 2;
  DataWriteResult data_result = 3;
  MailboxWriteResult mailbox_result = 4;
}

message DataWriteResult {
  int64 rows_affected = 1;
  map<string, google.protobuf.Value> generated_values = 2;
}

message MailboxWriteResult {
  string message_id = 1;
  int64 sequence = 2;
}

// For streaming transactions
message TransactRequest {
  oneof request {
    BeginTransaction begin = 1;
    WriteRequest write = 2;
    CommitTransaction commit = 3;
    RollbackTransaction rollback = 4;
  }
}

message BeginTransaction {
  string session_token = 1;
  TransactionOptions options = 2;
}

message CommitTransaction {}

message RollbackTransaction {}

message TransactResponse {
  oneof response {
    TransactionStarted started = 1;
    WriteResponse write_result = 2;
    TransactionCommitted committed = 3;
    TransactionRolledBack rolled_back = 4;
  }
}

message TransactionStarted {
  string transaction_id = 1;
}

message TransactionCommitted {
  bool success = 1;
}

message TransactionRolledBack {
  string reason = 1;
}
```

**Backend Mapping:**
- **Postgres**: Native transactions with two-table writes
- **SQLite**: Same as Postgres
- **DynamoDB**: TransactWriteItems with two items

### Cross-Layer Concepts

**Session Token Propagation:**
All layers require session token in metadata or request:

```rust
// Server: extract session from request
async fn validate_session(&self, token: &str) -> Result<Session, Status> {
    self.session_store
        .get(token)
        .await
        .ok_or_else(|| Status::unauthenticated("invalid session token"))
}

// All service methods start with validation
async fn publish(&self, req: Request<PublishRequest>) -> Result<Response<PublishResponse>, Status> {
    let req = req.into_inner();
    let session = self.validate_session(&req.session_token).await?;

    // Use session for authorization, auditing, routing
    // ...
}
```

**Auditing:**
Session layer provides audit hooks:

```rust
struct AuditLog {
    session_id: String,
    operation: String,
    resource: String,
    timestamp: Timestamp,
    success: bool,
}

// Logged for all operations
self.audit_logger.log(AuditLog {
    session_id: session.id,
    operation: "queue.publish",
    resource: format!("topic:{}", req.topic),
    timestamp: Utc::now(),
    success: true,
});
```

### Alternatives Considered

1. **Monolithic service with all operations**
   - Pros: Simple, single service
   - Cons: Tight coupling, hard to evolve independently
   - Rejected: Violates separation of concerns

2. **Backend-specific services (KafkaService, PostgresService)**
   - Pros: Clear backend mapping
   - Cons: Leaks implementation, prevents backend swapping
   - Rejected: Violates abstraction goal

3. **Single generic DataService**
   - Pros: Ultimate flexibility
   - Cons: No type safety, unclear semantics
   - Rejected: Too generic, loses use-case clarity

## Consequences

### Positive

- **Clear separation**: Each layer has distinct purpose
- **Progressive disclosure**: Clients use only what they need
- **Independent evolution**: Layers evolve independently
- **Backend polymorphism**: Multiple backends per layer
- **Type safety**: Protobuf enforces correct usage
- **Session foundation**: All operations audited and authorized

### Negative

- **Multiple services**: More gRPC services to manage
- **Session overhead**: All requests must validate session
- **Complexity**: More interfaces to learn

### Neutral

- **Service discovery**: Clients must know which service to use
- **Version management**: Each layer versions independently

## References

- ADR-022: Dynamic Client Configuration
- ADR-023: gRPC-First Interface Design
- [Inbox/Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- Netflix Data Gateway Architecture

## Revision History

- 2025-10-07: Initial draft and acceptance
