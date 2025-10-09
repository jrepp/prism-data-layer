---
title: "RFC-014: Layered Data Access Patterns"
status: Proposed
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [architecture, patterns, composition, data-access, layering]
---

## Abstract

This RFC specifies how Prism separates the **client API** (data access patterns like Queue, PubSub, Reader) from the **backend implementation** (composed strategies for satisfying those APIs). By layering reliability patterns (Claim Check, Outbox, CDC, Tiered Storage) beneath client-facing interfaces, Prism enables powerful data access capabilities without exposing complexity to applications.

## Motivation

Modern distributed applications require complex reliability patterns, but implementing them correctly is difficult:

**Problems**:
- Applications must implement reliability logic (claim check, outbox, tiered storage) themselves
- Backend-specific logic leaks into application code
- Switching backends requires rewriting application logic
- Patterns (e.g., Claim Check + Pub/Sub) must be composed manually
- Testing reliability patterns requires complex infrastructure

**Solution**: Prism provides a **layered architecture** that separates concerns:

```
┌──────────────────────────────────────────────────────────┐
│         Layer 3: Client API (What)                       │
│  Queue | PubSub | Reader | Transact | Cache             │
│  "I want to publish messages to a queue"                 │
└──────────────────────────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────┐
│         Layer 2: Pattern Composition (How)               │
│  Claim Check | Outbox | CDC | Tiered Storage | WAL      │
│  "Automatically store large payloads in S3"              │
└──────────────────────────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────┐
│         Layer 1: Backend Execution (Where)               │
│  Kafka | NATS | Postgres | Redis | S3 | ClickHouse      │
│  "Connect to and execute operations on backend"          │
└──────────────────────────────────────────────────────────┘
```

**Goals:**
- Define clear separation between client API and backend strategies
- Document how to compose multiple patterns for a single API
- Provide configuration-based pattern selection
- Enable testing patterns independently
- Support backend migration without client changes

**Non-Goals:**
- Runtime pattern switching (patterns chosen at namespace creation)
- Arbitrary pattern composition (only compatible patterns can layer)
- Application-level customization of patterns (Prism controls implementation)

## Client Pattern Catalog

This section shows **how application owners request high-level patterns** without needing to understand internal implementation. Each pattern maps to common application problems.

### Pattern 1: Durable Operation Log (WAL - Write-Ahead Log)

**Application Problem:**
You need **guaranteed durability** for operations - write operations must be persisted to disk before being acknowledged, and consumers must reliably process and acknowledge each operation. This is critical for financial transactions, order processing, audit logs, or any scenario where **operations cannot be lost**.

**What You Get:**
- **Durability guarantee**: Operations persisted to disk before acknowledgment
- **Reliable consumption**: Consumers must explicitly acknowledge each operation
- **Crash recovery**: Operations survive proxy/consumer crashes
- **Replay capability**: Re-process operations from any point in the log
- **Ordered processing**: Operations processed in write order

**Why WAL is Critical for Reliability:**

```mermaid
sequenceDiagram
    participant App as Application
    participant Proxy as Prism Proxy
    participant WAL as WAL (Disk)
    participant Consumer as Consumer
    participant Backend as Backend (Kafka)

    Note over App,Backend: Write Path (Durability)
    App->>Proxy: Publish operation
    Proxy->>WAL: 1. Append to WAL (fsync)
    WAL-->>Proxy: 2. Persisted to disk
    Proxy-->>App: 3. Acknowledge (operation durable)

    Note over Proxy,Backend: Async flush to backend
    Proxy->>Backend: 4. Flush WAL → Kafka
    Backend-->>Proxy: 5. Kafka acknowledged

    Note over Consumer,Backend: Read Path (Reliable Consumption)
    Consumer->>Backend: 6. Consume from Kafka
    Backend-->>Consumer: 7. Operation data
    Consumer->>Consumer: 8. Process operation
    Consumer->>Backend: 9. Acknowledge (committed offset)

    Note over App,Backend: Crash Recovery
    Proxy->>WAL: On restart: Read uncommitted WAL entries
    Proxy->>Backend: Replay to backend
```

**Client Configuration:**
```yaml
# Declare what you need - Prism handles the rest
namespaces:
  - name: order-processing
    pattern: durable-queue      # What pattern you need

    # Tell Prism what guarantees you need
    needs:
      durability: strong        # → Prism adds WAL with fsync
      replay: enabled           # → Prism keeps committed log for replay
      retention: 30days         # → Prism retains WAL for 30 days
      ordered: true             # → Prism guarantees order
```

**Client Code (Producer):**
```python
# Application publishes operation - Prism ensures durability
order = {"order_id": 12345, "amount": 99.99, "status": "pending"}

# Operation is persisted to WAL BEFORE acknowledgment
response = client.publish("orders", order)
# At this point:
#   ✓ Operation written to disk (survived crash)
#   ✓ Will be delivered to consumers
#   ✓ Can be replayed if needed

print("Order persisted:", response.offset)
```

**Client Code (Consumer):**
```python
# Consumer must explicitly acknowledge each operation
for operation in client.consume("orders"):
    try:
        # Process the operation
        result = process_order(operation.payload)

        # MUST acknowledge after successful processing
        operation.ack()  # Commits offset, operation won't be redelivered

    except Exception as e:
        # On failure: don't ack, operation will be retried
        logging.error(f"Failed to process order: {e}")
        operation.nack()  # Explicit negative ack (immediate retry)
```

**What Happens Internally:**

1. **Write Path (Producer)**:
   - **Layer 3**: Client calls `publish()`
   - **Layer 2**: WAL pattern writes to local append-only log on disk
   - **Layer 2**: Calls `fsync()` to ensure durability (survives crash)
   - **Layer 3**: Returns acknowledgment to client
   - **Background**: WAL flusher asynchronously sends to Kafka

2. **Read Path (Consumer)**:
   - **Layer 1**: Consumer reads from Kafka
   - **Layer 2**: WAL pattern tracks consumer position
   - **Consumer**: Processes operation
   - **Consumer**: Calls `operation.ack()` to commit progress
   - **Layer 2**: Updates consumer offset (operation marked consumed)

3. **Crash Recovery**:
   - **On proxy restart**: Read uncommitted WAL entries from disk
   - **Layer 2**: Replay uncommitted operations to Kafka
   - **Guarantee**: Zero lost operations

**Real-World Example:**
```
Problem: E-commerce order processing - cannot lose orders
Challenge: Proxy crashes between accepting order and publishing to Kafka

Before Prism (without WAL):
  1. Accept order HTTP request
  2. Publish to Kafka
  3. (CRASH HERE) → Order lost forever
  4. Return 200 OK to customer
  Result: Customer charged, order never processed

With Prism WAL:
  1. Accept order HTTP request
  2. Write to WAL on disk (fsync)
  3. Return 200 OK to customer (order persisted)
  4. (CRASH HERE) → WAL entry on disk
  5. On restart: Replay WAL → Kafka
  Result: Zero lost orders, customer trust maintained
```

**Performance Characteristics:**
- Write latency: **2-5ms** (includes fsync to disk)
- Throughput: **10k-50k operations/sec** per proxy instance
- Durability: **Survives crashes, power loss**
- Trade-off: Slightly higher latency vs in-memory queue, but **zero data loss**

### Pattern 2: Large Payload Pub/Sub (Claim Check)

**Application Problem:**
You need to publish large files (videos, ML models, datasets) but message queues have size limits (Kafka: 10MB, NATS: 8MB).

**What You Get:**
- Publish files up to 5GB through standard `publish()` API
- Automatic storage management (S3/Blob)
- Transparent retrieval for subscribers
- Automatic cleanup after consumption

**Client Configuration:**
```yaml
namespaces:
  - name: video-processing
    pattern: pubsub

    needs:
      max_message_size: 5GB        # → Prism adds Claim Check
      storage_backend: s3          # → Prism configures S3 storage
      cleanup: on_consume          # → Prism auto-deletes after read
```

**Client Code:**
```python
# Publisher: Send large video file (no special handling)
video_bytes = open("movie.mp4", "rb").read()  # 2.5GB
client.publish("videos", video_bytes)
# Prism: Detects size > threshold, stores in S3, publishes reference

# Subscriber: Receive full video (transparent retrieval)
event = client.subscribe("videos")
video_bytes = event.payload  # Full 2.5GB reconstructed
process_video(video_bytes)
# Prism: Fetches from S3, deletes S3 object after consumption
```

**What Happens Internally:**
1. **Layer 3**: Client calls `publish(2.5GB payload)`
2. **Layer 2**: Claim Check detects size > 1MB threshold
3. **Layer 2**: Stores payload in S3, generates claim check ID
4. **Layer 1**: Publishes lightweight message to Kafka (just metadata)

**Real-World Example:**
```
Problem: ML team publishes 50GB model weights per training run
Before Prism: Manual S3 upload + manual message with S3 key
With Prism: Standard publish() API, Prism handles everything
```

### Pattern 3: Transactional Messaging (Outbox)

**Application Problem:**
You need guaranteed message delivery when updating database - no lost messages, no duplicates, even if Kafka is down.

**What You Get:**
- Atomic database update + message publish
- Guaranteed delivery (survives crashes, Kafka outages)
- Exactly-once semantics
- No dual-write problems

**Client Configuration:**
```yaml
namespaces:
  - name: order-processing
    pattern: queue

    needs:
      consistency: strong          # → Prism adds Outbox pattern
      delivery_guarantee: exactly_once
```

**Client Code:**
```python
# Application code: Atomically update DB and publish event
with client.transaction() as tx:
    # 1. Update database
    tx.execute("UPDATE orders SET status='completed' WHERE id=$1", order_id)

    # 2. Publish event (atomic with DB update)
    tx.publish("order-events", {"order_id": order_id, "status": "completed"})

    # If commit succeeds, event WILL be published eventually
    # If commit fails, event is NOT published
    tx.commit()

# Prism handles background publishing from outbox table
```

**What Happens Internally:**
1. **Layer 3**: Client calls `tx.publish()`
2. **Layer 2**: Outbox pattern inserts into `outbox` table (same transaction)
3. **Layer 1**: Transaction commits to Postgres
4. **Background**: Outbox publisher polls table, publishes to Kafka, marks published

**Real-World Example:**
```
Problem: E-commerce order completion must trigger notification
Before Prism: Dual write bug caused missed notifications
With Prism: Outbox pattern guarantees delivery
```

### Pattern 4: Change Data Capture with Kafka (CDC + Outbox)

**Application Problem:**
You need to stream database changes to other systems (cache, search index, analytics) without dual writes.

**What You Get:**
- Automatic capture of database changes (INSERT/UPDATE/DELETE)
- Stream changes to Kafka for consumption
- No application code changes required
- Guaranteed ordering per key

**Client Configuration:**
```yaml
namespaces:
  - name: user-profiles
    pattern: reader  # Normal database reads/writes

    # Enable CDC streaming
    cdc:
      enabled: true
      source: postgres             # → Prism captures PostgreSQL WAL
      destination: kafka           # → Prism streams to Kafka
      topic: user-profile-changes

      # What to capture
      tables: [user_profiles]
      operations: [INSERT, UPDATE, DELETE]
```

**Client Code:**
```python
# Application: Normal database operations (no CDC code!)
client.update("user_profiles", user_id, {"email": "new@email.com"})

# Prism automatically publishes CDC event to Kafka:
# {
#   "operation": "UPDATE",
#   "table": "user_profiles",
#   "before": {"email": "old@email.com"},
#   "after": {"email": "new@email.com"},
#   "timestamp": "2025-10-09T10:30:00Z"
# }

# Other systems consume from Kafka:
def cache_invalidator():
    for change in kafka.consume("user-profile-changes"):
        if change.operation in ["UPDATE", "DELETE"]:
            redis.delete(f"user:{change.after.id}:profile")
```

**What Happens Internally:**
1. **Layer 3**: Client calls `update()`
2. **Layer 1**: Postgres executes UPDATE
3. **Layer 2**: CDC pattern captures WAL entry
4. **Layer 2**: Transforms WAL → CDC event
5. **Layer 1**: Publishes to Kafka topic

**Real-World Example:**
```
Problem: Keep Elasticsearch search index synced with PostgreSQL
Before Prism: Dual write (update DB, update ES) - race conditions
With Prism: CDC automatically streams changes, ES consumes
```

### Pattern 5: Transactional Large Payloads (Outbox + Claim Check)

**Application Problem:**
You need BOTH transactional guarantees (outbox) AND large payload support (claim check) - ML model releases, video uploads with metadata.

**What You Get:**
- Atomic transaction (if commit succeeds, event will be published)
- Large payload support (up to 5GB)
- No Kafka/NATS size limits
- Exactly-once delivery

**Client Configuration:**
```yaml
namespaces:
  - name: ml-model-releases
    pattern: pubsub

    needs:
      consistency: strong           # → Prism adds Outbox
      max_message_size: 5GB        # → Prism adds Claim Check
      delivery_guarantee: exactly_once
```

**Client Code:**
```python
# Application: Publish large model with transactional guarantee
model_weights = load_model("model-v2.weights")  # 2GB

with client.transaction() as tx:
    # Update model registry
    tx.execute("""
        INSERT INTO model_registry (name, version, status)
        VALUES ($1, $2, 'published')
    """, "my-model", "v2")

    # Publish model (2GB payload, transactional)
    tx.publish("model-releases", {
        "model_name": "my-model",
        "version": "v2",
        "weights": model_weights  # 2GB
    })

    tx.commit()
# If commit succeeds: model will be published
# If commit fails: S3 object is cleaned up, no message sent
```

**What Happens Internally:**
1. **Layer 3**: Client calls `tx.publish(2GB)`
2. **Layer 2**: Claim Check stores 2GB in S3
3. **Layer 2**: Outbox inserts `{claim_check_id}` into outbox table
4. **Layer 1**: Transaction commits to Postgres
5. **Background**: Outbox publisher sends lightweight Kafka message

**Real-World Example:**
```
Problem: ML platform releases 50GB models, needs atomic model registry + notification
Before Prism: Manual S3 + outbox implementation, 500 LOC
With Prism: Standard transactional API, Prism composes patterns
```

### Pattern 6: Cached Reads with Auto-Invalidation (Cache + CDC)

**Application Problem:**
You need fast cached reads but cache must stay fresh when database changes.

**What You Get:**
- Lightning-fast reads (Redis cache)
- Automatic cache invalidation on updates
- No stale data
- No application cache management code

**Client Configuration:**
```yaml
namespaces:
  - name: product-catalog
    pattern: reader

    # Enable caching with CDC invalidation
    cache:
      enabled: true
      backend: redis
      ttl: 900  # 15 min fallback

    cdc:
      enabled: true
      destination: cache_invalidator
      operations: [UPDATE, DELETE]
```

**Client Code:**
```python
# Application: Just read - Prism handles caching
product = client.get("products", product_id)
# First read: Cache miss → Query Postgres → Populate cache
# Subsequent reads: Cache hit → Return from Redis (sub-ms)

# Another service updates product
other_service.update("products", product_id, {"price": 29.99})
# Prism CDC: Detects change, invalidates cache automatically

# Next read: Cache miss (invalidated) → Fresh data from Postgres
product = client.get("products", product_id)  # Gets updated price
```

**What Happens Internally:**
1. **Read Path**: Client → Check Redis → (miss) → Query Postgres → Cache in Redis
2. **Write Path**: Update Postgres → CDC captures change → Invalidate Redis key
3. **Next Read**: Cache miss → Fresh data

**Real-World Example:**
```
Problem: Product catalog with millions of reads/sec, frequent price updates
Before Prism: Manual cache + manual invalidation, stale data bugs
With Prism: Declare cache + CDC, Prism handles everything
```

### Pattern Selection Guide

| Use Case | Recommended Pattern | Configuration |
|----------|-------------------|---------------|
| **High-volume logging** | WAL + Tiered Storage | `write_rps: 100k+`, `retention: 90days` |
| **Large files (videos, models)** | Claim Check | `max_message_size: >1MB` |
| **Transactional events** | Outbox | `consistency: strong` |
| **Database change streaming** | CDC | `cdc.enabled: true` |
| **Large + transactional** | Outbox + Claim Check | Both requirements |
| **Fast cached reads** | Cache + CDC | `cache.enabled: true`, `cdc.enabled: true` |
| **Event sourcing** | WAL + Event Store | `audit: true`, `replay: enabled` |

### How Prism Selects Patterns

Application owners declare **requirements**, Prism selects **patterns**:

```yaml
# Application declares "what" they need
namespaces:
  - name: video-uploads
    needs:
      write_rps: 5000                    # High throughput
      max_message_size: 5GB              # Large payloads
      consistency: strong                # Transactional
      retention: 30days                  # Long-term storage

# Prism generates "how" to implement it
# Internally translates to:
#   patterns: [WAL, Outbox, Claim Check, Tiered Storage]
#   backend: [Kafka, S3, Postgres]
```

Application owners **never write pattern composition logic** - they declare needs, Prism handles the rest.

## Architecture Overview

### Proxy Internal Structure

The Prism proxy is structured to cleanly separate concerns across layers:

```mermaid
graph TB
    subgraph "External"
        Client[Client Application]
    end

    subgraph "Prism Proxy"
        subgraph "Frontend Layer"
            gRPC[gRPC Server<br/>:8980]
            Auth[Authentication<br/>Middleware]
            SessionMgr[Session<br/>Manager]
        end

        subgraph "API Layer (Layer 3)"
            QueueAPI[Queue API]
            PubSubAPI[PubSub API]
            ReaderAPI[Reader API]
            TransactAPI[Transact API]
        end

        subgraph "Pattern Layer (Layer 2)"
            direction LR
            PatternChain[Pattern Chain<br/>Executor]

            subgraph "Patterns"
                Outbox[Outbox<br/>Pattern]
                ClaimCheck[Claim Check<br/>Pattern]
                CDC[CDC<br/>Pattern]
                Tiered[Tiered Storage<br/>Pattern]
                WAL[WAL<br/>Pattern]
            end
        end

        subgraph "Backend Layer (Layer 1)"
            BackendRouter[Backend<br/>Router]

            subgraph "Backend Connectors"
                KafkaConn[Kafka<br/>Connector]
                PGConn[PostgreSQL<br/>Connector]
                S3Conn[S3<br/>Connector]
                RedisConn[Redis<br/>Connector]
            end
        end

        subgraph "Observability"
            Metrics[Prometheus<br/>Metrics]
            Traces[OpenTelemetry<br/>Traces]
            Logs[Structured<br/>Logs]
        end
    end

    subgraph "Backends"
        Kafka[(Kafka)]
        Postgres[(PostgreSQL)]
        S3[(S3)]
        Redis[(Redis)]
    end

    Client -->|mTLS/JWT| gRPC
    gRPC --> Auth
    Auth -->|Validate| SessionMgr
    SessionMgr --> QueueAPI
    SessionMgr --> PubSubAPI
    SessionMgr --> ReaderAPI
    SessionMgr --> TransactAPI

    QueueAPI --> PatternChain
    PubSubAPI --> PatternChain
    ReaderAPI --> PatternChain
    TransactAPI --> PatternChain

    PatternChain -->|Execute| Outbox
    PatternChain -->|Execute| ClaimCheck
    PatternChain -->|Execute| CDC
    PatternChain -->|Execute| Tiered
    PatternChain -->|Execute| WAL

    Outbox --> BackendRouter
    ClaimCheck --> BackendRouter
    CDC --> BackendRouter
    Tiered --> BackendRouter
    WAL --> BackendRouter

    BackendRouter -->|Route| KafkaConn
    BackendRouter -->|Route| PGConn
    BackendRouter -->|Route| S3Conn
    BackendRouter -->|Route| RedisConn

    KafkaConn --> Kafka
    PGConn --> Postgres
    S3Conn --> S3
    RedisConn --> Redis

    PatternChain -.->|Emit| Metrics
    PatternChain -.->|Emit| Traces
    PatternChain -.->|Emit| Logs
```

### Authentication and Authorization Flow

```mermaid
sequenceDiagram
    participant Client
    participant gRPC as gRPC Server
    participant Auth as Auth Middleware
    participant JWT as JWT Validator
    participant RBAC as RBAC Policy Engine
    participant Session as Session Manager
    participant API as API Layer

    Client->>gRPC: Request + JWT Token
    gRPC->>Auth: Intercept Request

    Auth->>JWT: Validate Token
    JWT->>JWT: Check signature<br/>Check expiry<br/>Extract claims

    alt Token Valid
        JWT-->>Auth: Claims {user, groups, scopes}

        Auth->>RBAC: Authorize Operation
        RBAC->>RBAC: Check namespace access<br/>Check operation permission<br/>Check rate limits

        alt Authorized
            RBAC-->>Auth: Allow
            Auth->>Session: Get/Create Session
            Session-->>Auth: Session Context
            Auth->>API: Forward Request + Context
            API-->>Client: Response
        else Not Authorized
            RBAC-->>Auth: Deny
            Auth-->>Client: PermissionDenied (7)
        end
    else Token Invalid
        JWT-->>Auth: Error
        Auth-->>Client: Unauthenticated (16)
    end
```

### Pattern Layer Execution Flow

```mermaid
sequenceDiagram
    participant API as API Layer (Layer 3)
    participant Chain as Pattern Chain
    participant P1 as Pattern 1<br/>(Outbox)
    participant P2 as Pattern 2<br/>(Claim Check)
    participant Backend as Backend Layer (Layer 1)
    participant Obs as Observability

    Note over API,Obs: Publish Flow

    API->>Chain: Publish(topic, payload, metadata)
    Chain->>Obs: Start Trace "publish"

    Chain->>P1: process_publish(ctx)
    P1->>Obs: Span "outbox-pattern"
    P1->>P1: BEGIN TRANSACTION
    P1->>P1: ctx = wrap in outbox
    P1->>Obs: Metric: outbox_inserted++

    P1->>P2: Continue with modified ctx
    P2->>Obs: Span "claim-check-pattern"

    alt Payload > Threshold
        P2->>Backend: Store payload in S3
        Backend-->>P2: S3 URL
        P2->>P2: Replace payload with<br/>claim_check_id
        P2->>Obs: Metric: claim_check_stored++
    end

    P2->>P1: Return modified ctx
    P1->>Backend: INSERT INTO outbox
    P1->>P1: COMMIT TRANSACTION

    P1->>Chain: Success
    Chain->>Obs: End Trace (duration: 52ms)
    Chain->>API: PublishResponse

    Note over API,Obs: Background: Outbox Publisher

    loop Every 100ms
        P1->>Backend: SELECT unpublished FROM outbox
        P1->>Backend: Publish to Kafka
        P1->>Backend: UPDATE outbox published_at
        P1->>Obs: Metric: outbox_published++
    end
```

### Pattern Routing and Backend Execution

```mermaid
graph LR
    subgraph "Pattern Layer"
        Input[Pattern Input<br/>Context]

        subgraph "Pattern Decision Tree"
            CheckOutbox{Outbox<br/>Enabled?}
            CheckClaim{Claim Check<br/>Enabled?}
            CheckSize{Payload > 1MB?}
            CheckCDC{CDC<br/>Enabled?}
        end

        Output[Pattern Output<br/>Context]
    end

    subgraph "Backend Router"
        Route[Route by<br/>Backend Type]

        subgraph "Execution Strategies"
            Direct[Direct Execute]
            Transact[Transactional<br/>Execute]
            Stream[Streaming<br/>Execute]
            Batch[Batch<br/>Execute]
        end
    end

    subgraph "Backend Connectors"
        KafkaOps[Kafka Operations]
        PGOps[Postgres Operations]
        S3Ops[S3 Operations]
        RedisOps[Redis Operations]
    end

    Input --> CheckOutbox

    CheckOutbox -->|Yes| Transact
    CheckOutbox -->|No| CheckClaim

    CheckClaim -->|Yes| CheckSize
    CheckClaim -->|No| CheckCDC

    CheckSize -->|Yes| S3Ops
    CheckSize -->|No| Output

    CheckCDC -->|Yes| KafkaOps
    CheckCDC -->|No| Output

    Output --> Route

    Route -->|Queue/PubSub| Direct
    Route -->|Transact| Transact
    Route -->|Reader| Stream
    Route -->|Bulk Insert| Batch

    Direct --> KafkaOps
    Transact --> PGOps
    Stream --> PGOps
    Batch --> RedisOps
```

### Three-Layer Model

#### Layer 3: Client API (Abstraction)

The **What** layer - defines the interface applications use:

```protobuf
// Example: PubSub Service
service PubSubService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Event);
}

message PublishRequest {
  string topic = 1;
  bytes payload = 2;  // Application doesn't know about Claim Check
  map<string, string> metadata = 3;
}
```

**Key Characteristics:**
- Backend-agnostic (no Kafka/NATS specific details)
- Pattern-agnostic (no Claim Check/Outbox details)
- Stable API (evolves slowly)
- Type-safe via protobuf

#### Layer 2: Pattern Composition (Strategy)

The **How** layer - implements reliability patterns transparently:

```yaml
# Namespace configuration
namespaces:
  - name: video-processing
    # Layer 3: Client sees PubSub API
    client_api: pubsub

    # Layer 2: Composed patterns (order matters!)
    patterns:
      - type: claim-check        # Pattern 1: Handle large payloads
        threshold: 1MB
        storage: s3
        bucket: video-processing

      - type: outbox             # Pattern 2: Transactional guarantees
        table: video_outbox
        database: postgres

    # Layer 1: Backend execution
    backend:
      queue: kafka
      topic_prefix: video
```

**Pattern Execution Order:**

```mermaid
sequenceDiagram
    participant App as Application
    participant API as Layer 3: PubSub API
    participant Pat1 as Layer 2: Claim Check
    participant Pat2 as Layer 2: Outbox
    participant Backend as Layer 1: Kafka

    App->>API: Publish(topic, 50MB payload)
    API->>Pat1: Process (50MB > 1MB threshold)
    Pat1->>Pat1: Store payload in S3
    Pat1->>Pat1: Replace payload with claim_check_id
    Pat1->>Pat2: Continue ({topic, claim_check_id})

    Pat2->>Pat2: INSERT INTO video_outbox<br/>(topic, claim_check_id)
    Pat2->>Pat2: COMMIT transaction

    Pat2->>Backend: Publish to Kafka<br/>(lightweight message)
    Backend-->>Pat2: Acknowledged
    Pat2-->>API: Success
    API-->>App: PublishResponse
```

#### Layer 1: Backend Execution (Implementation)

The **Where** layer - connects to and executes on specific backends:

```rust
// Backend-specific implementation
impl KafkaBackend {
    async fn publish(&self, topic: &str, payload: &[u8]) -> Result<Offset> {
        self.producer
            .send(topic, payload, None)
            .await
            .map_err(|e| Error::Backend(e))
    }
}
```

**Key Characteristics:**
- Backend-specific logic encapsulated
- Connection pooling and retries
- Performance optimization per backend
- Pluggable (new backends without API changes)

## Pattern Composition

### Compatible Pattern Combinations

Not all patterns can be layered together. Compatibility depends on:
- **Ordering**: Some patterns must come before others
- **Data Flow**: Patterns must pass compatible data structures
- **Semantics**: Patterns can't contradict (e.g., eventual + strong consistency)

#### Composition Matrix

| Base API | Compatible Patterns (In Order) | Example Use Case |
|----------|-------------------------------|------------------|
| **PubSub** | Claim Check → Kafka/NATS | Large payload pub/sub |
| **PubSub** | Outbox → Claim Check → Kafka | Transactional large payloads |
| **Queue** | Claim Check → Kafka | Large message queue |
| **Queue** | WAL → Tiered Storage | Fast writes + archival |
| **Reader** | Cache (Look-Aside) → Postgres | Frequent reads |
| **Reader** | CDC → Cache Invalidation | Fresh cached reads |
| **Transact** | Outbox → Queue Publisher | Transactional messaging |
| **Transact** | Event Sourcing → Materialized Views | Audit + performance |

### Publisher with Claim Check Pattern

**Scenario**: Application needs to publish large video files (50MB-5GB) to a pub/sub system, but Kafka/NATS have 1-10MB message limits.

#### Without Layering (Application Code)

```python
# Application must implement Claim Check manually
def publish_video(video_id, video_bytes):
    if len(video_bytes) > 1_000_000:  # > 1MB
        # Upload to S3
        s3_key = f"videos/{video_id}"
        s3.put_object(Bucket="videos", Key=s3_key, Body=video_bytes)

        # Publish reference
        kafka.produce("videos", {
            "video_id": video_id,
            "s3_reference": s3_key,
            "size": len(video_bytes)
        })
    else:
        # Publish inline
        kafka.produce("videos", {
            "video_id": video_id,
            "payload": video_bytes
        })

# Consumer must implement Claim Check retrieval
def consume_video():
    msg = kafka.consume("videos")
    if "s3_reference" in msg:
        # Download from S3
        video_bytes = s3.get_object(
            Bucket="videos",
            Key=msg["s3_reference"]
        )["Body"].read()
    else:
        video_bytes = msg["payload"]

    process_video(video_bytes)
```

**Problems**:
- 20+ lines of boilerplate per producer/consumer
- Must handle S3 credentials, retries, errors
- No automatic cleanup of claim check objects
- Different logic for small vs large payloads

#### With Prism Layering (Zero Application Code)

**Configuration**:
```yaml
namespaces:
  - name: video-processing
    client_api: pubsub

    patterns:
      - type: claim-check
        threshold: 1MB
        storage:
          backend: s3
          bucket: prism-claim-checks
          prefix: videos/
        cleanup:
          strategy: on_consume
          ttl: 604800  # 7 days fallback

    backend:
      type: kafka
      brokers: [kafka-1:9092, kafka-2:9092]
      topic: videos
```

**Application Code**:
```python
# Producer: Prism handles Claim Check automatically
client.publish("videos", video_bytes)
# Prism:
# 1. Detects size > 1MB
# 2. Uploads to S3: s3://prism-claim-checks/videos/{uuid}
# 3. Publishes Kafka: {claim_check_id, size, metadata}

# Consumer: Prism reconstructs full payload
event = client.subscribe("videos")
video_bytes = event.payload  # Prism fetched from S3 automatically
process_video(video_bytes)
```

**Benefits**:
- 2 lines of application code (vs 20+)
- Automatic threshold detection
- Transparent S3 upload/download
- Automatic cleanup after consumption
- Same API for small and large payloads

### Outbox + Claim Check Layering

**Scenario**: Application needs **transactional guarantees** (outbox) AND **large payload handling** (claim check).

#### Pattern Layering

```mermaid
sequenceDiagram
    participant App as Application
    participant Prism as Prism Proxy
    participant DB as PostgreSQL
    participant S3 as S3 Storage
    participant Kafka as Kafka

    App->>Prism: Publish(topic, 100MB model weights)

    Note over Prism: Layer 2: Claim Check Pattern
    Prism->>Prism: Detect 100MB > 1MB threshold
    Prism->>S3: PUT ml-models/model-v2.bin
    S3-->>Prism: Success, S3 URL

    Note over Prism: Layer 2: Outbox Pattern
    Prism->>DB: BEGIN TRANSACTION
    Prism->>DB: INSERT INTO outbox<br/>(event_type, claim_check_id,<br/> metadata, payload_size)
    Prism->>DB: COMMIT TRANSACTION
    DB-->>Prism: Transaction committed

    Prism-->>App: PublishResponse (success)

    Note over Prism: Background: Outbox Publisher
    loop Every 100ms
        Prism->>DB: SELECT * FROM outbox<br/>WHERE published_at IS NULL
        DB-->>Prism: Unpublished events

        Prism->>Kafka: Publish lightweight message<br/>{claim_check_id, metadata}
        Kafka-->>Prism: Acknowledged

        Prism->>DB: UPDATE outbox<br/>SET published_at = NOW()
    end
```

**Configuration**:
```yaml
namespaces:
  - name: ml-model-releases
    client_api: pubsub

    patterns:
      # Order matters: Outbox runs first (wraps everything in transaction)
      - type: outbox
        database: postgres
        table: ml_model_outbox
        publisher:
          interval: 100ms
          batch_size: 100

      # Claim Check runs second (inside outbox transaction)
      - type: claim-check
        threshold: 10MB
        storage:
          backend: s3
          bucket: ml-models
          prefix: releases/
        metadata_field: claim_check_id  # Store S3 reference in outbox

    backend:
      type: kafka
      topic: model-releases
```

**Guarantees**:
- ✅ If transaction commits, event WILL be published (outbox)
- ✅ If transaction fails, S3 object can be garbage collected
- ✅ Large models don't block database (claim check)
- ✅ Kafka receives lightweight messages (&lt;1KB)

### CDC + Cache Invalidation Layering

**Scenario**: Keep cache synchronized with database changes using CDC.

#### Pattern Composition

```yaml
namespaces:
  - name: user-profiles
    client_api: reader

    patterns:
      # Pattern 1: Look-Aside Cache (fast reads)
      - type: cache
        strategy: look-aside
        backend: redis
        ttl: 900  # 15 minutes
        key_pattern: "user:{id}:profile"

      # Pattern 2: CDC for cache invalidation
      - type: cdc
        source:
          backend: postgres
          database: users_db
          table: user_profiles
        sink:
          backend: kafka
          topic: user-profile-changes

        # Consumers: Cache invalidator
        consumers:
          - name: cache-invalidator
            type: cache_invalidator
            backend: redis
            operations: [UPDATE, DELETE]
            key_extractor: "user:{after.id}:profile"

    backend:
      type: postgres
      database: users_db
```

**Data Flow**:

```mermaid
graph LR
    App[Application]
    Prism[Prism Proxy]
    Cache[Redis Cache]
    DB[(PostgreSQL)]
    CDC[CDC Connector]
    Kafka[Kafka]
    Invalidator[Cache Invalidator]

    App -->|Read user profile| Prism

    Prism -->|1. Check cache| Cache
    Cache -.->|Cache Hit| Prism

    Prism -->|2. Query DB<br/>(on miss)| DB
    DB -.->|User data| Prism
    Prism -.->|3. Populate cache| Cache

    App2[Another App] -->|Update profile| DB
    DB -->|WAL stream| CDC
    CDC -->|Parse changes| Kafka
    Kafka -->|Subscribe| Invalidator
    Invalidator -->|DEL user:123:profile| Cache
```

**Benefits**:
- Applications always read from cache (fast)
- Cache stays synchronized with database
- No dual writes or race conditions
- Automatic invalidation on UPDATE/DELETE

## Separation of Concerns

### Client API vs Backend Strategy

#### Example: Queue Service

**Client API (Layer 3)** - Stable interface:
```protobuf
service QueueService {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Message);
}
```

**Backend Strategy (Layer 2 + 1)** - Implementation details:

| Strategy Combination | Backends | Use Case |
|---------------------|----------|----------|
| Queue (simple) | Kafka | Standard message queue |
| WAL + Queue | Kafka WAL → Postgres | Durable + queryable queue |
| Claim Check + Queue | S3 + Kafka | Large message queue |
| Outbox + Queue | Postgres outbox → Kafka | Transactional queue |
| Tiered Queue | Redis (hot) → Postgres (warm) → S3 (cold) | Multi-tier retention |

**Application doesn't know which strategy** - same API for all:
```python
# Works with ANY backend strategy
client.publish("events", payload)
messages = client.subscribe("events")
```

### Pattern Configuration Encapsulation

Applications declare requirements; Prism selects patterns:

```yaml
# Application-facing configuration
namespaces:
  - name: video-processing
    needs:
      message_size: 5GB              # Prism adds Claim Check
      consistency: strong            # Prism adds Outbox
      retention: 30 days             # Prism adds Tiered Storage
      throughput: 10k msgs/s         # Prism sizes Kafka partitions

# Prism generates internal config:
namespaces:
  - name: video-processing
    client_api: pubsub
    patterns:
      - type: outbox              # For consistency
      - type: claim-check         # For message_size
      - type: tiered-storage      # For retention
    backend:
      type: kafka
      partitions: 20              # For throughput
```

## Stitching Patterns Together

### Pattern Interfaces

Each pattern implements standard interfaces for composability:

```rust
/// Pattern trait for composing reliability patterns
#[async_trait]
pub trait Pattern: Send + Sync {
    /// Process outgoing data (before backend)
    async fn process_publish(
        &self,
        ctx: &mut PublishContext,
    ) -> Result<()>;

    /// Process incoming data (after backend)
    async fn process_consume(
        &self,
        ctx: &mut ConsumeContext,
    ) -> Result<()>;

    /// Pattern metadata
    fn metadata(&self) -> PatternMetadata;
}

pub struct PublishContext {
    pub topic: String,
    pub payload: Vec<u8>,
    pub metadata: HashMap<String, String>,
    pub backend: BackendType,
}

pub struct ConsumeContext {
    pub message_id: String,
    pub payload: Vec<u8>,
    pub metadata: HashMap<String, String>,
}
```

### Example: Claim Check Pattern Implementation

```rust
pub struct ClaimCheckPattern {
    threshold: usize,
    storage: Arc<dyn ObjectStorage>,
}

#[async_trait]
impl Pattern for ClaimCheckPattern {
    async fn process_publish(&self, ctx: &mut PublishContext) -> Result<()> {
        // Check threshold
        if ctx.payload.len() > self.threshold {
            // Store in object storage
            let claim_check_id = Uuid::new_v4().to_string();
            let key = format!("claim-checks/{}", claim_check_id);

            self.storage.put(&key, &ctx.payload).await?;

            // Replace payload with reference
            ctx.metadata.insert("claim_check_id".to_string(), claim_check_id);
            ctx.metadata.insert("payload_size".to_string(), ctx.payload.len().to_string());
            ctx.payload = vec![];  // Empty payload, reference in metadata
        }

        Ok(())
    }

    async fn process_consume(&self, ctx: &mut ConsumeContext) -> Result<()> {
        // Check for claim check reference
        if let Some(claim_check_id) = ctx.metadata.get("claim_check_id") {
            let key = format!("claim-checks/{}", claim_check_id);

            // Fetch from object storage
            ctx.payload = self.storage.get(&key).await?;

            // Optional: Delete claim check after consumption
            // self.storage.delete(&key).await?;
        }

        Ok(())
    }

    fn metadata(&self) -> PatternMetadata {
        PatternMetadata {
            name: "claim-check".to_string(),
            version: "1.0.0".to_string(),
            compatible_backends: vec![BackendType::Kafka, BackendType::Nats],
        }
    }
}
```

### Pattern Chain Execution

Prism executes patterns in order:

```rust
pub struct PatternChain {
    patterns: Vec<Box<dyn Pattern>>,
}

impl PatternChain {
    pub async fn process_publish(&self, mut ctx: PublishContext) -> Result<PublishContext> {
        // Execute patterns in order
        for pattern in &self.patterns {
            pattern.process_publish(&mut ctx).await?;
        }
        Ok(ctx)
    }

    pub async fn process_consume(&self, mut ctx: ConsumeContext) -> Result<ConsumeContext> {
        // Execute patterns in reverse order for consume
        for pattern in self.patterns.iter().rev() {
            pattern.process_consume(&mut ctx).await?;
        }
        Ok(ctx)
    }
}
```

**Example execution with Outbox + Claim Check**:

```
Publish Flow:
  App → [Layer 3: PubSub API]
    → [Layer 2: Outbox Pattern] (begin transaction)
      → [Layer 2: Claim Check Pattern] (store large payload)
        → [Layer 1: Kafka Backend] (publish lightweight message)
      ← (commit transaction)
    ← (return success)
  ← PublishResponse

Consume Flow:
  [Layer 1: Kafka Backend] (receive message)
    → [Layer 2: Claim Check Pattern] (fetch from S3)
      → [Layer 2: Outbox Pattern] (no-op for consume)
        → [Layer 3: PubSub API]
          → App (full payload reconstructed)
```

## Building on Existing RFCs

This RFC builds on and connects:

### RFC-001: Prism Architecture
- **Layer 3 Client API** maps to RFC-001 "Interface Layers" (Queue, PubSub, Reader, Transact)
- **Layer 1 Backend Execution** maps to RFC-001 "Container Plugin Model"
- **Layer 2 Pattern Composition** is NEW - enables powerful combinations

### RFC-002: Data Layer Interface
- Client-facing protobuf APIs defined in RFC-002
- Applications use stable APIs from RFC-002
- Patterns (Layer 2) transparently implement RFC-002 interfaces

### RFC-007: Cache Strategies
- Look-Aside and Write-Through caching are **patterns** (Layer 2)
- Can compose with other patterns (e.g., CDC + Cache Invalidation)
- Cache configuration moves from application to namespace config

### RFC-009: Distributed Reliability Patterns
- All 7 patterns (Claim Check, Outbox, WAL, CDC, Tiered Storage, Event Sourcing, CQRS) live in Layer 2
- Can be composed as shown in RFC-009 "Pattern Composition and Layering" section
- This RFC formalizes the layering architecture

### RFC-010: Admin Protocol with OIDC
- Admin API operates at Layer 3 (control plane, not data plane)
- Patterns configured via admin API
- Observability of pattern health exposed via admin API

### RFC-011: Data Proxy Authentication
- Authentication happens at Layer 3 (before patterns)
- Patterns inherit session context
- Backend credentials managed at Layer 1

## Configuration Schema

### Namespace Pattern Configuration

```yaml
namespaces:
  - name: video-processing

    # Layer 3: Client API
    client_api: pubsub

    # Layer 2: Pattern composition (ordered!)
    patterns:
      - type: outbox
        enabled: true
        config:
          database: postgres
          table: video_outbox
          publisher:
            interval: 100ms
            batch_size: 100

      - type: claim-check
        enabled: true
        config:
          threshold: 1MB
          storage:
            backend: s3
            bucket: prism-claim-checks
            prefix: videos/
          compression: gzip
          cleanup:
            strategy: on_consume
            ttl: 604800

      - type: tiered-storage
        enabled: false  # Optional pattern

    # Layer 1: Backend configuration
    backend:
      type: kafka
      brokers: [kafka-1:9092, kafka-2:9092]
      topic: videos
      partitions: 10
      replication: 3

    # Observability
    observability:
      trace_patterns: true  # Trace each pattern execution
      pattern_metrics: true
```

### Pattern Validation

Prism validates pattern compatibility at namespace creation:

```rust
pub fn validate_pattern_chain(
    api: ClientApi,
    patterns: &[PatternConfig],
    backend: &BackendConfig,
) -> Result<()> {
    // Check API compatibility
    for pattern in patterns {
        if !pattern.supports_api(&api) {
            return Err(Error::IncompatiblePattern {
                pattern: pattern.type_name(),
                api: api.name(),
            });
        }
    }

    // Check pattern ordering
    for window in patterns.windows(2) {
        if !window[1].compatible_with(&window[0]) {
            return Err(Error::IncompatiblePatternOrder {
                first: window[0].type_name(),
                second: window[1].type_name(),
            });
        }
    }

    // Check backend compatibility
    if !backend.supports_patterns(patterns) {
        return Err(Error::BackendIncompatibleWithPatterns);
    }

    Ok(())
}
```

## Testing Strategy

### Unit Tests: Individual Patterns

```rust
#[tokio::test]
async fn test_claim_check_pattern_threshold() {
    let storage = Arc::new(MockObjectStorage::new());
    let pattern = ClaimCheckPattern {
        threshold: 1_000_000,  // 1MB
        storage: storage.clone(),
    };

    // Small payload - should not trigger claim check
    let mut ctx = PublishContext {
        topic: "test".to_string(),
        payload: vec![0u8; 500_000],  // 500KB
        ..Default::default()
    };

    pattern.process_publish(&mut ctx).await.unwrap();
    assert!(!ctx.metadata.contains_key("claim_check_id"));
    assert_eq!(ctx.payload.len(), 500_000);

    // Large payload - should trigger claim check
    let mut ctx = PublishContext {
        topic: "test".to_string(),
        payload: vec![0u8; 2_000_000],  // 2MB
        ..Default::default()
    };

    pattern.process_publish(&mut ctx).await.unwrap();
    assert!(ctx.metadata.contains_key("claim_check_id"));
    assert_eq!(ctx.payload.len(), 0);  // Payload replaced
    assert_eq!(storage.object_count(), 1);
}
```

### Integration Tests: Pattern Chains

```rust
#[tokio::test]
async fn test_outbox_claim_check_chain() {
    let db = setup_test_db().await;
    let s3 = setup_test_s3().await;
    let kafka = setup_test_kafka().await;

    let chain = PatternChain::new(vec![
        Box::new(OutboxPattern::new(db.clone())),
        Box::new(ClaimCheckPattern::new(1_000_000, s3.clone())),
    ]);

    // Publish large payload
    let ctx = PublishContext {
        topic: "test".to_string(),
        payload: vec![0u8; 5_000_000],  // 5MB
        ..Default::default()
    };

    let ctx = chain.process_publish(ctx).await.unwrap();

    // Verify outbox entry created
    let outbox_entries = db.query("SELECT * FROM outbox WHERE published_at IS NULL").await.unwrap();
    assert_eq!(outbox_entries.len(), 1);

    // Verify S3 object stored
    assert_eq!(s3.object_count(), 1);

    // Verify Kafka message is lightweight
    assert!(ctx.metadata.contains_key("claim_check_id"));
    assert_eq!(ctx.payload.len(), 0);
}
```

### End-to-End Tests

```python
def test_e2e_large_payload_pubsub():
    # Setup Prism with Outbox + Claim Check
    prism = PrismTestServer(config={
        "namespace": "test",
        "client_api": "pubsub",
        "patterns": [
            {"type": "outbox", "database": "postgres"},
            {"type": "claim-check", "threshold": "1MB", "storage": "s3"}
        ],
        "backend": {"type": "kafka"}
    })

    client = prism.client()

    # Publish 10MB payload
    large_payload = b"x" * 10_000_000
    response = client.publish("test-topic", large_payload)
    assert response.success

    # Consume and verify full payload reconstructed
    subscriber = client.subscribe("test-topic")
    event = next(subscriber)
    assert event.payload == large_payload

    # Verify S3 object cleaned up after consumption
    assert prism.s3.object_count() == 0
```

## Performance Characteristics

### Pattern Overhead

| Pattern | Latency Added | Memory Overhead | Use When |
|---------|---------------|-----------------|----------|
| **None** | 0ms | 0MB | Simple use cases |
| **Claim Check** | +10-50ms (S3 upload) | ~10MB (buffer) | Payload > 1MB |
| **Outbox** | +5-10ms (DB write) | ~1MB (buffer) | Need transactions |
| **CDC** | Background | ~5MB (replication) | Keep systems synced |
| **Tiered Storage** | Variable | ~10MB (tier metadata) | Hot/warm/cold data |
| **WAL** | +2-5ms (log append) | ~50MB (WAL buffer) | High write throughput |

### Example: Outbox + Claim Check Performance

**Baseline (no patterns)**: 10ms P99 latency, 10k RPS

**With Outbox + Claim Check**:
- Small payloads (&lt;1MB): 15ms P99 (+5ms for outbox), 8k RPS
- Large payloads (&gt;1MB): 60ms P99 (+50ms for S3 upload), 1k RPS

**Trade-off**: Slightly higher latency for **strong guarantees** (transactional + large payload support).

## Migration Strategy

### Phase 1: Single Pattern (Low Risk)

Start with one pattern per namespace:

```yaml
# Before: Direct Kafka
backend: kafka

# After: Add Claim Check only
patterns:
  - type: claim-check
    threshold: 1MB
backend: kafka
```

### Phase 2: Compose Two Patterns (Medium Risk)

Add second compatible pattern:

```yaml
patterns:
  - type: outbox         # Transactional guarantees
  - type: claim-check    # Large payload handling
backend: kafka
```

### Phase 3: Complex Composition (Higher Risk)

Layer 3+ patterns for advanced use cases:

```yaml
patterns:
  - type: outbox
  - type: claim-check
  - type: tiered-storage  # Archive old messages to S3
  - type: cdc             # Replicate to analytics DB
backend: kafka
```

## Observability

### Pattern Metrics

```
# Claim Check
prism_pattern_claim_check_stored_total{namespace="videos"} 1234
prism_pattern_claim_check_retrieved_total{namespace="videos"} 1230
prism_pattern_claim_check_storage_bytes{namespace="videos"} 5.2e9
prism_pattern_claim_check_cleanup_success{namespace="videos"} 1230

# Outbox
prism_pattern_outbox_inserted_total{namespace="videos"} 1234
prism_pattern_outbox_published_total{namespace="videos"} 1234
prism_pattern_outbox_lag_seconds{namespace="videos"} 0.15
prism_pattern_outbox_pending_count{namespace="videos"} 5

# Pattern Chain
prism_pattern_chain_duration_seconds{namespace="videos", pattern="claim-check"} 0.042
prism_pattern_chain_duration_seconds{namespace="videos", pattern="outbox"} 0.008
```

### Distributed Tracing

```
Trace: Publish large video
├─ Span: PubSubService.Publish [12ms]
│  ├─ Span: OutboxPattern.process_publish [8ms]
│  │  ├─ Span: db.begin_transaction [1ms]
│  │  ├─ Span: ClaimCheckPattern.process_publish [45ms]
│  │  │  ├─ Span: s3.put_object [42ms]
│  │  │  └─ Span: generate_claim_check_id [0.1ms]
│  │  ├─ Span: db.insert_outbox [2ms]
│  │  └─ Span: db.commit_transaction [1ms]
│  └─ Span: kafka.produce [3ms]
```

## References

### Related RFCs
- RFC-001: Prism Architecture - Defines interface layers
- RFC-002: Data Layer Interface - Client API specifications
- RFC-007: Cache Strategies - Cache as a pattern
- RFC-009: Distributed Reliability Patterns - Individual patterns
- RFC-010: Admin Protocol with OIDC - Pattern configuration
- RFC-011: Data Proxy Authentication - Authentication layer

### External References
- [Enterprise Integration Patterns](https://www.enterpriseintegrationpatterns.com/)
- [Claim Check Pattern](https://www.enterpriseintegrationpatterns.com/patterns/messaging/StoreInLibrary.html)
- [Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html)
- [Decorator Pattern](https://en.wikipedia.org/wiki/Decorator_pattern) - Inspiration for pattern composition

### ADRs
- ADR-024: Layered Interface Hierarchy
- ADR-025: Container Plugin Model
- ADR-034: Product/Feature Sharding Strategy

## Open Questions

1. **Pattern Hot-Reload**: Can patterns be added/removed without restarting proxy?
2. **Pattern Configuration Evolution**: How to update pattern config for existing namespaces?
3. **Pattern Performance Profiling**: Which patterns add most latency in production?
4. **Custom Patterns**: Can users define custom patterns via plugins?
5. **Pattern Versioning**: How to version patterns independently of proxy?

## Revision History

- 2025-10-09: Initial draft defining layered data access patterns, pattern composition, and separation of client API from backend strategies
