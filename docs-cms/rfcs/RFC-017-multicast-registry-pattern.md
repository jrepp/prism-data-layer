---
title: "RFC-017: Multicast Registry Pattern"
status: Draft
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [patterns, client-api, registry, pubsub, service-discovery, composition]
---

# RFC-017: Multicast Registry Pattern

**Status**: Draft
**Author**: Platform Team
**Created**: 2025-10-09
**Updated**: 2025-10-09

## Abstract

The **Multicast Registry Pattern** is a composite client pattern that combines identity management, metadata storage, and selective broadcasting. It enables applications to:

1. **Register identities** with rich metadata (presence, capabilities, location, etc.)
2. **Enumerate registered identities** with metadata filtering
3. **Multicast publish** messages to all or filtered subsets of registered identities

This pattern emerges from multiple use cases (service discovery, IoT command-and-control, presence systems, microservice coordination) that share common requirements but have been implemented ad-hoc across different systems.

**Key Innovation**: The pattern is **schematized with pluggable backend slots**, allowing the same client API to be backed by different storage and messaging combinations depending on scale, consistency, and durability requirements.

## Motivation

### Problem Statement

Modern distributed applications frequently need to:
- Track a dynamic set of participants (services, devices, users, agents)
- Store metadata about each participant (status, capabilities, location, version)
- Send messages to all participants or filtered subsets
- Discover participants based on metadata queries

**Current approaches are fragmented**:

1. **Service Discovery Only** (Consul, etcd, Kubernetes Service):
   - ✅ Identity registration and enumeration
   - ❌ No native multicast messaging
   - 🔧 Applications must implement pub/sub separately

2. **Pure Pub/Sub** (Kafka, NATS):
   - ✅ Multicast messaging
   - ❌ No built-in identity registry with metadata
   - 🔧 Applications must maintain subscriber lists separately

3. **Ad-Hoc Solutions** (Redis Sets + Pub/Sub):
   - ✅ Can combine primitives
   - ❌ Application-specific, not reusable
   - ❌ Error-prone consistency between registry and messaging

4. **Heavy Frameworks** (Akka Cluster, Orleans):
   - ✅ Complete solution
   - ❌ Language/framework lock-in
   - ❌ Complex operational overhead

### Goals

1. **Unified Pattern**: Single client API for register → enumerate → multicast workflow
2. **Metadata-Rich**: First-class support for identity metadata and filtering
3. **Schematized Composition**: Define backend "slots" that can be filled with different implementations
4. **Backend Flexibility**: Same pattern works with Redis, PostgreSQL+Kafka, NATS, or other combinations
5. **Semantic Clarity**: Clear guarantees about consistency, durability, and delivery
6. **Operational Simplicity**: Prism handles coordination between registry and pub/sub backends

### Use Cases

#### Use Case 1: Microservice Coordination

```yaml
# Service registry with broadcast notifications
pattern: multicast-registry
identity_schema:
  service_name: string
  version: string
  endpoint: string
  health_status: enum[healthy, degraded, unhealthy]
  capabilities: array[string]

operations:
  - register: Service announces itself with metadata
  - enumerate: Discovery service lists all healthy services
  - multicast: Config service broadcasts new feature flags to all services
```

**Example**:
```python
# Service A registers
registry.register(
    identity="payment-service-instance-42",
    metadata={
        "service_name": "payment-service",
        "version": "2.3.1",
        "endpoint": "http://" + "10.1.2.42:8080",
        "health_status": "healthy",
        "capabilities": ["credit-card", "paypal", "stripe"]
    },
    ttl=30  # Heartbeat required every 30s
)

# API Gateway enumerates healthy services
services = registry.enumerate(
    filter={"service_name": "payment-service", "health_status": "healthy"}
)

# Config service broadcasts to all services
registry.multicast(
    filter={"service_name": "*"},  # All services
    message={"type": "config_update", "feature_flags": {...}}
)
```

#### Use Case 2: IoT Command-and-Control

```yaml
# Device registry with command broadcast
pattern: multicast-registry
identity_schema:
  device_id: string
  device_type: enum[sensor, actuator, gateway]
  location: geo_point
  firmware_version: string
  battery_level: float
  last_seen: timestamp

operations:
  - register: Device registers on connect
  - enumerate: Dashboard lists devices by location
  - multicast: Control plane sends firmware update command to all v1.0 devices
```

**Example**:
```python
# IoT device registers
registry.register(
    identity="sensor-temp-floor2-room5",
    metadata={
        "device_type": "sensor",
        "location": {"lat": 37.7749, "lon": -122.4194},
        "firmware_version": "1.0.3",
        "battery_level": 0.78,
        "capabilities": ["temperature", "humidity"]
    }
)

# Dashboard enumerates devices in building
devices = registry.enumerate(
    filter={"location.building": "HQ", "battery_level.lt": 0.2}
)

# Send firmware update to all v1.0 devices
registry.multicast(
    filter={"firmware_version.startswith": "1.0"},
    message={"command": "update_firmware", "url": "https://..."}
)
```

#### Use Case 3: Presence System

```yaml
# User presence with room-based broadcast
pattern: multicast-registry
identity_schema:
  user_id: string
  display_name: string
  status: enum[online, away, busy, offline]
  current_room: string
  client_version: string
  last_activity: timestamp

operations:
  - register: User joins with status
  - enumerate: Show room participant list
  - multicast: Send message to all users in room
```

**Example**:
```python
# User joins chat room
registry.register(
    identity="user-alice-session-abc123",
    metadata={
        "user_id": "alice",
        "display_name": "Alice",
        "status": "online",
        "current_room": "engineering",
        "client_version": "web-2.0"
    }
)

# Enumerate users in room
participants = registry.enumerate(
    filter={"current_room": "engineering", "status": "online"}
)

# Broadcast message to room
registry.multicast(
    filter={"current_room": "engineering"},
    message={"type": "chat", "from": "alice", "text": "Hello team!"}
)
```

#### Use Case 4: Agent Pool Management

```yaml
# Worker agent registry with task broadcast
pattern: multicast-registry
identity_schema:
  agent_id: string
  agent_type: enum[cpu, gpu, memory]
  available_resources: object
  current_tasks: int
  max_tasks: int
  tags: array[string]

operations:
  - register: Agent announces capacity
  - enumerate: Scheduler finds available agents
  - multicast: Broadcast cancel signal to all agents with specific tag
```

## Pattern Definition

### Core Concepts

The Multicast Registry pattern provides three primitive operations:

1. **Register**: Store identity with metadata
2. **Enumerate**: Query/list identities with optional filtering
3. **Multicast**: Publish message to all or filtered identities

**Identity**: Unique identifier (string) within the registry namespace

**Metadata**: Structured key-value data associated with identity (JSON-like)

**Filter**: Query expression for selecting identities based on metadata

**TTL (Time-To-Live)**: Optional expiration for identity registration (heartbeat pattern)

### Operations

#### Register Operation

```protobuf
message RegisterRequest {
  string identity = 1;                // Unique identity within namespace
  map<string, Value> metadata = 2;    // Metadata key-value pairs
  optional int64 ttl_seconds = 3;     // Optional TTL (0 = no expiration)
  bool replace = 4;                   // Replace if exists (default: false)
}

message RegisterResponse {
  bool success = 1;
  optional string error = 2;
  Timestamp registered_at = 3;
  optional Timestamp expires_at = 4;  // If TTL specified
}
```

**Semantics**:
- Identity must be unique within namespace
- Metadata can be arbitrary JSON-like structure
- TTL creates automatic expiration (requires heartbeat/re-registration)
- Replace flag controls conflict behavior

**Error Cases**:
- `ALREADY_EXISTS`: Identity already registered (if replace=false)
- `INVALID_METADATA`: Metadata doesn't match schema
- `QUOTA_EXCEEDED`: Namespace registration limit reached

#### Enumerate Operation

```protobuf
message EnumerateRequest {
  optional Filter filter = 1;         // Optional metadata filter
  optional Pagination pagination = 2; // Limit/offset for large registries
  bool include_metadata = 3;          // Return full metadata (default: true)
  repeated string sort_by = 4;        // Sort order (e.g., ["metadata.created_at desc"])
}

message EnumerateResponse {
  repeated Identity identities = 1;
  int64 total_count = 2;              // Total matching identities
  optional string next_cursor = 3;    // For pagination
}

message Identity {
  string identity = 1;
  map<string, Value> metadata = 2;    // If include_metadata=true
  Timestamp registered_at = 3;
  optional Timestamp expires_at = 4;
}
```

**Filter Syntax**:
```javascript
// Equality
{"service_name": "payment-service"}

// Comparison operators
{"battery_level.lt": 0.2}  // less than
{"version.gte": "2.0"}     // greater than or equal

// String matching
{"firmware_version.startswith": "1.0"}
{"endpoint.contains": "prod"}

// Logical operators
{"$and": [
  {"status": "healthy"},
  {"version.gte": "2.0"}
]}

{"$or": [
  {"device_type": "sensor"},
  {"device_type": "gateway"}
]}

// Array membership
{"capabilities.contains": "credit-card"}

// Existence
{"metadata.exists": "location"}
```

**Semantics**:
- Returns snapshot of current registrations
- Filter evaluated at query time (not subscription)
- Pagination for large result sets
- Sort by metadata fields

#### Multicast Operation

```protobuf
message MulticastRequest {
  optional Filter filter = 1;         // Target identities (null = all)
  bytes payload = 2;                  // Message payload
  optional string content_type = 3;   // MIME type (default: application/octet-stream)
  optional DeliverySemantics delivery = 4;  // At-most-once, at-least-once, exactly-once
  optional int64 timeout_ms = 5;      // Delivery timeout
}

message MulticastResponse {
  int64 target_count = 1;             // Number of identities matched by filter
  int64 delivered_count = 2;          // Number of messages delivered
  repeated DeliveryStatus statuses = 3; // Per-identity delivery status
}

message DeliveryStatus {
  string identity = 1;
  enum Status {
    DELIVERED = 0;
    PENDING = 1;
    FAILED = 2;
    TIMEOUT = 3;
  }
  Status status = 2;
  optional string error = 3;
}
```

**Semantics**:
- Filter applied at publish time (captures current registrations)
- Fan-out to all matching identities
- Delivery guarantees depend on backend
- Response includes per-identity delivery status (if durable backend)

**Delivery Semantics**:
- **At-most-once**: Fire-and-forget, no acknowledgment
- **At-least-once**: Retry until acknowledged (may duplicate)
- **Exactly-once**: Deduplication + acknowledgment (requires transactional backend)

### Optional: Unregister Operation

```protobuf
message UnregisterRequest {
  string identity = 1;
}

message UnregisterResponse {
  bool success = 1;
  optional string error = 2;
}
```

**Semantics**:
- Explicit removal from registry
- Alternative to waiting for TTL expiration
- Idempotent (unregister non-existent identity succeeds)

## Architecture: Pattern Composition

### Conceptual Model

The Multicast Registry pattern **composes** three data access primitives:

┌─────────────────────────────────────────────────────────┐
│          Multicast Registry Pattern                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │  KeyValue   │  │    PubSub    │  │    Queue      │ │
│  │  (Registry) │  │  (Broadcast) │  │  (Durable)    │ │
│  └─────────────┘  └──────────────┘  └───────────────┘ │
│        │                  │                  │         │
│        │                  │                  │         │
│        └──────────────────┴──────────────────┘         │
│                     Coordinated by Proxy               │
└─────────────────────────────────────────────────────────┘
```text

### Primitive Mapping

| Operation | Registry (KeyValue) | Pub/Sub | Queue/Durable |
|-----------|-------------------|---------|---------------|
| **Register** | `SET identity metadata` | Subscribe to identity's topic | Create queue for identity |
| **Enumerate** | `SCAN` with filter | (List subscriptions) | (List queues) |
| **Multicast** | `GET` identities by filter → fan-out | `PUBLISH` to each topic | `ENQUEUE` to each queue |
| **Unregister** | `DELETE identity` | Unsubscribe | Delete queue |
| **TTL** | `EXPIRE identity ttl` | (Auto-unsubscribe) | (Queue expiration) |

### Backend Slot Architecture

The pattern defines **three backend slots** that can be filled independently:

```
pattern: multicast-registry

backend_slots:
  registry:
    purpose: Store identity metadata
    operations: [set, get, scan, delete, expire]
    candidates: [redis, postgres, dynamodb, etcd]

  messaging:
    purpose: Deliver multicast messages
    operations: [publish, subscribe]
    candidates: [kafka, nats, redis-pubsub, rabbitmq]

  durability:
    purpose: Persist undelivered messages (optional)
    operations: [enqueue, dequeue, ack]
    candidates: [kafka, postgres, sqs, redis-stream]
```text

**Key Design Principle**: The same client API works with different backend combinations, allowing trade-offs between consistency, durability, and performance.

## Proxy Implementation

### Proxy Responsibilities

The Prism proxy coordinates the pattern by:

1. **Identity Lifecycle Management**:
   - Store identity + metadata in registry backend
   - Manage TTL/expiration (background cleanup)
   - Maintain subscriber mapping (identity → pub/sub topic/queue)

2. **Enumeration**:
   - Translate filter expressions to backend queries
   - Execute query against registry backend
   - Return matched identities with metadata

3. **Multicast Fan-out**:
   - Evaluate filter to find target identities
   - Fan out to messaging backend (pub/sub or queues)
   - Track delivery status (if durable backend)
   - Return aggregate delivery report

4. **Consistency Coordination**:
   - Ensure registry and messaging backend stay synchronized
   - Handle registration → subscription creation
   - Handle unregistration → cleanup

### Proxy State Machine

┌─────────────┐
│  Registering │
│   Identity   │
└──────┬───────┘
       │
       │ 1. Store metadata in registry backend
       ├───────────────────────────────────────┐
       │                                       │
       │ 2. Create subscriber mapping          │
       │    (identity → topic/queue)           │
       ├───────────────────────────────────────┤
       │                                       │
       │ 3. Subscribe to pub/sub or            │
       │    create queue (if durable)          │
       └───────────────────────────────────────┘
                      │
                      ▼
              ┌───────────────┐
              │   Registered  │
              │   (Active)    │
              └───────┬───────┘
                      │
        ┌─────────────┼─────────────┐
        │             │             │
   Multicast     Enumerate      TTL Expired
     Recv'd        Request        or Unreg
        │             │             │
        ▼             ▼             ▼
   ┌────────┐   ┌──────────┐  ┌───────────┐
   │ Publish│   │  Query   │  │  Cleanup  │
   │ to     │   │ Registry │  │  Unreg +  │
   │ Topic  │   │  Backend │  │  Unsub    │
   └────────┘   └──────────┘  └───────────┘
```

### Filter Evaluation Strategy

**Two evaluation strategies depending on backend**:

#### Strategy 1: Backend-Native Filtering

```rust
// PostgreSQL with JSONB
async fn enumerate_postgres(filter: &Filter) -> Vec<Identity> {
    let sql = translate_filter_to_sql(filter);
    // SELECT identity, metadata FROM registry WHERE metadata @> '{"status": "healthy"}'
    db.query(sql).await
}

// Redis with Lua script
async fn enumerate_redis(filter: &Filter) -> Vec<Identity> {
    let lua_script = translate_filter_to_lua(filter);
    redis.eval(lua_script).await
}
```

**Pros**: Fast, leverages backend indexing
**Cons**: Backend-specific query language

#### Strategy 2: Client-Side Filtering

```rust
// Fetch all identities, filter in proxy
async fn enumerate_generic(filter: &Filter) -> Vec<Identity> {
    let all_identities = registry_backend.scan_all().await;
    all_identities.into_iter()
        .filter(|id| filter.matches(&id.metadata))
        .collect()
}
```

**Pros**: Backend-agnostic, works with any registry
**Cons**: Inefficient for large registries

**Recommendation**: Use backend-native when available, fallback to client-side.

### Multicast Fan-out Algorithm

```rust
async fn multicast(
    registry: &RegistryBackend,
    messaging: &MessagingBackend,
    request: &MulticastRequest
) -> Result<MulticastResponse> {
    // 1. Evaluate filter to find target identities
    let targets = registry.enumerate(&request.filter).await?;

    // 2. Fan out to messaging backend
    let delivery_results = match messaging {
        MessagingBackend::PubSub(pubsub) => {
            // Parallel publish to each topic
            futures::future::join_all(
                targets.iter().map(|identity| {
                    pubsub.publish(&identity_topic(identity), &request.payload)
                })
            ).await
        }
        MessagingBackend::Queue(queue) => {
            // Enqueue to each queue
            futures::future::join_all(
                targets.iter().map(|identity| {
                    queue.enqueue(&identity_queue(identity), &request.payload)
                })
            ).await
        }
    };

    // 3. Aggregate delivery status
    Ok(MulticastResponse {
        target_count: targets.len() as i64,
        delivered_count: delivery_results.iter().filter(|r| r.is_ok()).count() as i64,
        statuses: delivery_results.into_iter()
            .zip(targets.iter())
            .map(|(result, identity)| DeliveryStatus {
                identity: identity.identity.clone(),
                status: match result {
                    Ok(_) => Status::Delivered,
                    Err(e) if e.is_timeout() => Status::Timeout,
                    Err(_) => Status::Failed,
                },
                error: result.err().map(|e| e.to_string()),
            })
            .collect(),
    })
}
```

## Backend Slot Requirements

### Slot 1: Registry Backend

**Purpose**: Store identity metadata with query/scan capabilities.

**Required Operations**:
- `set(identity, metadata, ttl)`: Store identity with metadata
- `get(identity)`: Retrieve metadata for identity
- `scan(filter)`: Query identities by metadata
- `delete(identity)`: Remove identity
- `expire(identity, ttl)`: Set TTL

**Backend Options**:

| Backend | Pros | Cons | Filter Support |
|---------|------|------|----------------|
| **Redis** | Fast, TTL built-in | No native JSON filter | Lua scripting |
| **PostgreSQL** | JSONB queries, indexes | Slower than Redis | Native JSON operators |
| **DynamoDB** | Scalable, TTL built-in | Limited query syntax | GSI + filter expressions |
| **etcd** | Consistent, watch API | Small value limit | Key prefix only |
| **MongoDB** | Flexible queries | Separate deployment | Native JSON queries |

**Recommendation**: **PostgreSQL** for rich filtering, **Redis** for speed/simplicity.

### Slot 2: Messaging Backend

**Purpose**: Deliver multicast messages to registered identities.

**Required Operations**:
- `publish(topic, payload)`: Publish message to topic
- `subscribe(topic)`: Subscribe to messages (consumer-side)

**Backend Options**:

| Backend | Pros | Cons | Delivery Guarantees |
|---------|------|------|---------------------|
| **NATS** | Lightweight, fast | At-most-once (core) | At-most-once (JetStream: at-least-once) |
| **Redis Pub/Sub** | Simple, low latency | No persistence | At-most-once |
| **Kafka** | Durable, high throughput | Complex setup | At-least-once |
| **RabbitMQ** | Mature, flexible | Operational overhead | At-least-once |

**Recommendation**: **NATS** for low-latency ephemeral, **Kafka** for durable multicast.

### Slot 3: Durability Backend (Optional)

**Purpose**: Persist undelivered messages for offline identities.

**Required Operations**:
- `enqueue(queue, payload)`: Add message to queue
- `dequeue(queue)`: Retrieve next message
- `ack(queue, message_id)`: Acknowledge delivery

**Backend Options**:

| Backend | Pros | Cons |
|---------|------|------|
| **Kafka** | High throughput, replayable | Heavy for simple queues |
| **PostgreSQL** | ACID transactions, simple | Lower throughput |
| **Redis Streams** | Fast, lightweight | Limited durability guarantees |
| **SQS** | Managed, scalable | AWS-only, cost |

**Recommendation**: Use **same as messaging backend** if possible (Kafka), else **PostgreSQL** for transactional guarantees.

## Configuration Examples

### Example 1: Redis Registry + NATS Pub/Sub (Low Latency)

```yaml
namespaces:
  - name: presence
    pattern: multicast-registry

    identity_schema:
      user_id: string
      display_name: string
      status: enum[online, away, busy, offline]
      current_room: string

    needs:
      latency: &lt;10ms
      consistency: eventual
      durability: ephemeral

    backend_slots:
      registry:
        type: redis
        host: localhost
        port: 6379
        ttl_default: 300  # 5 min heartbeat

      messaging:
        type: nats
        servers: ["nats://localhost:4222"]
        delivery: at-most-once
```

**Characteristics**:
- **Latency**: &lt;10ms for register, enumerate, multicast
- **Consistency**: Eventual (Redis async replication)
- **Durability**: Ephemeral (lost on server restart)
- **Use Cases**: Presence, real-time dashboards

### Example 2: PostgreSQL Registry + Kafka Pub/Sub (Durable)

```yaml
namespaces:
  - name: iot-devices
    pattern: multicast-registry

    identity_schema:
      device_id: string
      device_type: enum[sensor, actuator, gateway]
      location: geo_point
      firmware_version: string
      battery_level: float

    needs:
      consistency: strong
      durability: persistent
      audit: true

    backend_slots:
      registry:
        type: postgres
        connection: "postgres://localhost:5432/prism"
        schema: iot_registry
        indexes:
          - field: device_type
          - field: firmware_version
          - field: location
            type: gist  # GIS index

      messaging:
        type: kafka
        brokers: ["localhost:9092"]
        topic_prefix: "iot.commands."
        delivery: at-least-once
        retention: 7d

      durability:
        use_messaging: true  # Kafka provides persistence
```

**Characteristics**:
- **Latency**: ~50ms for multicast (Kafka write + fsync)
- **Consistency**: Strong (PostgreSQL ACID)
- **Durability**: Persistent (Kafka retention, Postgres WAL)
- **Audit**: All registrations and multicasts logged
- **Use Cases**: IoT device management, compliance-critical systems

### Example 3: DynamoDB Registry + SNS Fan-out (AWS-Native)

```yaml
namespaces:
  - name: microservices
    pattern: multicast-registry

    identity_schema:
      service_name: string
      instance_id: string
      version: string
      endpoint: string
      health_status: enum[healthy, degraded, unhealthy]

    needs:
      scale: 10000+ services
      region: multi-region
      cloud: aws

    backend_slots:
      registry:
        type: dynamodb
        table: prism-service-registry
        partition_key: service_name
        sort_key: instance_id
        ttl_attribute: expires_at
        gsi:
          - name: health-index
            keys: [health_status, service_name]

      messaging:
        type: sns
        topic_prefix: "prism-service-"
        delivery: at-most-once
```

**Characteristics**:
- **Scale**: 10,000+ services, auto-scaling
- **Latency**: ~20ms (DynamoDB), ~50ms (SNS)
- **Multi-region**: DynamoDB Global Tables
- **Use Cases**: Large-scale microservice mesh

### Example 4: Composed Pattern (PostgreSQL Outbox + Kafka)

```yaml
namespaces:
  - name: agents
    pattern: multicast-registry

    identity_schema:
      agent_id: string
      agent_type: string
      available_resources: object
      current_tasks: int

    needs:
      consistency: strong
      durability: persistent
      exactly_once: true

    backend_slots:
      registry:
        type: postgres
        connection: "postgres://localhost:5432/prism"

      messaging:
        type: kafka
        brokers: ["localhost:9092"]

      durability:
        use_messaging: true

      composition:
        pattern: outbox  # Transactional outbox pattern
        outbox_table: multicast_outbox
        poll_interval: 100ms
```

**Characteristics**:
- **Exactly-once semantics**: Transactional outbox ensures registry + multicast atomic
- **No dual-write problem**: Both write to Postgres, relay to Kafka
- **Use Cases**: Financial systems, critical coordination

## Client API Design

### Python Client API

```python
from prism import Client, Filter

client = Client(namespace="presence")

# Register identity
await client.registry.register(
    identity="user-alice-session-1",
    metadata={
        "user_id": "alice",
        "display_name": "Alice",
        "status": "online",
        "current_room": "engineering"
    },
    ttl=300  # 5 minutes
)

# Enumerate identities
identities = await client.registry.enumerate(
    filter=Filter(current_room="engineering", status="online")
)
print(f"Users in room: {[id.metadata['display_name'] for id in identities]}")

# Multicast to room
result = await client.registry.multicast(
    filter=Filter(current_room="engineering"),
    message={"type": "chat", "from": "alice", "text": "Hello!"}
)
print(f"Delivered to {result.delivered_count}/{result.target_count} users")

# Unregister
await client.registry.unregister(identity="user-alice-session-1")
```

### Go Client API

```go
import "github.com/prism/client-go"

func main() {
    client := prism.NewClient("presence")

    // Register
    err := client.Registry.Register(ctx, prism.RegisterRequest{
        Identity: "user-bob-session-2",
        Metadata: map[string]interface{}{
            "user_id":      "bob",
            "display_name": "Bob",
            "status":       "away",
            "current_room": "engineering",
        },
        TTL: 300,
    })

    // Enumerate
    identities, err := client.Registry.Enumerate(ctx, prism.EnumerateRequest{
        Filter: prism.Filter{"current_room": "engineering"},
    })

    // Multicast
    result, err := client.Registry.Multicast(ctx, prism.MulticastRequest{
        Filter: prism.Filter{"status": "online"},
        Payload: []byte(`{"type": "ping"}`),
    })
}
```

### Rust Client API

```rust
use prism::{Client, Filter};

#[tokio::main]
async fn main() -> Result<()> {
    let client = Client::new("microservices").await?;

    // Register
    client.registry.register(RegisterRequest {
        identity: "payment-service-1".into(),
        metadata: json!({
            "service_name": "payment-service",
            "version": "2.3.1",
            "health_status": "healthy",
        }),
        ttl: Some(30),
        ..Default::default()
    }).await?;

    // Enumerate
    let services = client.registry.enumerate(EnumerateRequest {
        filter: Some(Filter::new()
            .eq("service_name", "payment-service")
            .eq("health_status", "healthy")),
        ..Default::default()
    }).await?;

    // Multicast
    let result = client.registry.multicast(MulticastRequest {
        filter: Some(Filter::new().eq("service_name", "*")),
        payload: serde_json::to_vec(&json!({"type": "config_update"}))?,
        ..Default::default()
    }).await?;

    Ok(())
}
```

## Schema Definition

### Identity Schema

Namespaces using multicast-registry pattern MUST define an identity schema:

```yaml
identity_schema:
  # Required fields
  primary_key: string      # Identity field name

  # Metadata fields (JSON Schema)
  fields:
    user_id:
      type: string
      required: true

    display_name:
      type: string
      max_length: 100

    status:
      type: enum
      values: [online, away, busy, offline]
      default: offline

    current_room:
      type: string
      required: false
      index: true  # Backend should create index

    last_activity:
      type: timestamp
      auto: now  # Auto-set on register

    capabilities:
      type: array
      items: string
```

### Filter Schema

Filters follow MongoDB-like query syntax:

```yaml
filter_operators:
  # Equality
  - eq: Equal
  - ne: Not equal

  # Comparison
  - lt: Less than
  - lte: Less than or equal
  - gt: Greater than
  - gte: Greater than or equal

  # String
  - startswith: String prefix match
  - endswith: String suffix match
  - contains: Substring match
  - regex: Regular expression

  # Array
  - in: Value in array
  - contains: Array contains value

  # Logical
  - and: All conditions match
  - or: Any condition matches
  - not: Negate condition

  # Existence
  - exists: Field exists
  - type: Field type check
```

## Comparison to Alternatives

### vs. Pure Service Discovery (Consul, etcd)

| Feature | Multicast Registry | Service Discovery |
|---------|-------------------|-------------------|
| Identity Registration | ✅ First-class | ✅ Primary use case |
| Metadata Storage | ✅ Rich JSON | ✅ Key-value |
| Enumeration/Query | ✅ Flexible filtering | ⚠️ Key prefix only |
| Multicast Messaging | ✅ Built-in | ❌ Must integrate pub/sub |
| Consistency | ✅ Configurable | ✅ Strong (etcd), Eventual (Consul) |
| **Advantage** | Unified API for register+multicast | Battle-tested, wide adoption |

### vs. Pure Pub/Sub (Kafka, NATS)

| Feature | Multicast Registry | Pub/Sub |
|---------|-------------------|---------|
| Publish/Subscribe | ✅ Multicast operation | ✅ Core functionality |
| Identity Registry | ✅ Built-in | ❌ Application must maintain |
| Metadata Filtering | ✅ Query-based | ❌ Topic-based only |
| Dynamic Subscribers | ✅ Register/unregister | ⚠️ Topic creation |
| **Advantage** | Metadata-aware targeting | Simple, high throughput |

### vs. Actor Systems (Akka, Orleans)

| Feature | Multicast Registry | Actor Systems |
|---------|-------------------|---------------|
| Identity Management | ✅ Explicit register | ✅ Actor lifecycle |
| Multicast | ✅ Filter-based | ⚠️ Actor group broadcast |
| Language | ✅ Polyglot (gRPC) | ❌ JVM/.NET only |
| Learning Curve | ✅ Simple API | ⚠️ Actor model complexity |
| **Advantage** | No framework lock-in | Rich actor model features |

### vs. Message Queues (RabbitMQ, SQS)

| Feature | Multicast Registry | Message Queues |
|---------|-------------------|----------------|
| Queue Management | ✅ Auto-created per identity | ⚠️ Manual queue creation |
| Metadata Filtering | ✅ Dynamic queries | ❌ Static routing keys |
| Durability | ✅ Optional | ✅ Built-in |
| **Advantage** | Dynamic targeting | Mature queueing semantics |

## Implementation Phases

### Phase 1: Core Pattern (Week 1-2)

**Deliverables**:
- Protobuf definitions for Register/Enumerate/Multicast operations
- Proxy pattern handler with slot architecture
- Redis registry backend implementation
- NATS messaging backend implementation
- Python client library

**Demo**: Presence system with Redis+NATS

### Phase 2: Rich Backends (Week 3-4)

**Deliverables**:
- PostgreSQL registry backend with JSONB filtering
- Kafka messaging backend
- Filter expression parser and evaluator
- TTL/expiration background worker

**Demo**: IoT device registry with PostgreSQL+Kafka

### Phase 3: Durability & Outbox (Week 5-6)

**Deliverables**:
- Durability slot implementation
- Transactional outbox pattern
- Exactly-once delivery semantics
- Delivery status tracking

**Demo**: Agent pool management with exactly-once guarantees

### Phase 4: Advanced Features (Week 7-8)

**Deliverables**:
- DynamoDB registry backend
- SNS messaging backend
- Multi-region support
- Schema evolution and migration tools

**Demo**: Multi-region microservice registry

## Open Questions

1. **Filter Complexity Limits**: Should we limit filter complexity to prevent expensive queries?
   - **Proposal**: Max filter depth = 5, max clauses = 20
   - **Reasoning**: Prevent DoS via complex filters

2. **Multicast Ordering**: Do multicast messages need ordering guarantees?
   - **Proposal**: Best-effort ordering by default, optional strict ordering with Kafka
   - **Reasoning**: Ordering is expensive, not needed for many use cases

3. **Identity Namespace**: Should identities be globally unique or per-pattern?
   - **Proposal**: Namespace-scoped (same as other patterns)
   - **Reasoning**: Isolation, multi-tenancy

4. **Filter Subscription**: Should enumerate support watch/subscription for changes?
   - **Proposal**: Phase 2 feature - watch API for registry changes
   - **Reasoning**: Powerful but adds complexity

5. **Backpressure**: How to handle slow consumers during multicast?
   - **Proposal**: Async delivery with optional timeout
   - **Reasoning**: Don't block fast consumers on slow ones

## Security Considerations

1. **Identity Spoofing**: Prevent unauthorized identity registration
   - **Mitigation**: Require authentication, validate identity ownership

2. **Metadata Injection**: Malicious metadata could exploit filter queries
   - **Mitigation**: Schema validation, sanitize filter expressions

3. **Enumeration Privacy**: Prevent leaking sensitive identity metadata
   - **Mitigation**: Per-namespace ACLs, filter field permissions

4. **Multicast Abuse**: Prevent spam/DoS via unrestricted multicast
   - **Mitigation**: Rate limiting, quota per identity

5. **TTL Manipulation**: Prevent identities from lingering forever
   - **Mitigation**: Enforce max TTL, background cleanup

## Related Patterns and Documents

- [RFC-014: Layered Data Access Patterns](./RFC-014-layered-data-access-patterns.md) - Base client pattern catalog
- [RFC-008: Proxy Plugin Architecture](./RFC-008-proxy-plugin-architecture.md) - Plugin composition model
- [RFC-009: Distributed Reliability Patterns](./RFC-009-distributed-reliability-patterns.md) - Outbox pattern details
- [MEMO-004: Backend Plugin Implementation Guide](/memos/MEMO-004-backend-plugin-implementation-guide) - Backend selection criteria

## References

### Academic Papers
- ["The Actor Model"](https://www.info.ucl.ac.be/~pvr/Gul_Agha.pdf) - Carl Hewitt et al.
- ["Distributed Publish/Subscribe"](https://dl.acm.org/doi/10.1145/1809028.1806634) - ACM Computing Surveys

### Real-World Systems
- [Consul Service Mesh](https://www.consul.io/) - Service discovery with key-value store
- [etcd](https://etcd.io/) - Distributed key-value store with watch API
- [Akka Cluster](https://doc.akka.io/docs/akka/current/typed/cluster.html) - Actor-based clustering
- [Orleans Virtual Actors](https://learn.microsoft.com/en-us/dotnet/orleans/) - Microsoft's actor framework
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream) - Durable streaming layer

### Pattern Implementations
- [Netflix Eureka](https://github.com/Netflix/eureka) - Service registry with heartbeat
- [Kubernetes Service Discovery](https://kubernetes.io/docs/concepts/services-networking/service/) - Pod registry + DNS
- [AWS App Mesh](https://aws.amazon.com/app-mesh/) - Service mesh with discovery

## Revision History

- 2025-10-09: Initial draft covering pattern definition, backend slots, implementation plan

