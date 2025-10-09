---
title: "MEMO-006: Backend Interface Decomposition and Schema Registry"
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [architecture, backend, patterns, schemas, registry, capabilities]
---

# MEMO-006: Backend Interface Decomposition and Schema Registry

## Purpose

Define how backend interfaces should be decomposed into thin, composable proto services, and establish a registry system for backend interfaces, pattern schemas, and slot schemas to enable straightforward configuration generation.

## Problem Statement

Current architecture treats backends as monolithic units (e.g., "Redis backend", "Postgres backend"). This creates several issues:

1. **Coarse granularity**: Redis supports 6+ distinct data models (KeyValue, PubSub, Streams, Lists, Sets, SortedSets), but we treat it as single unit
2. **Unclear capabilities**: No explicit mapping of which backends support which interfaces
3. **Pattern composition ambiguity**: Pattern executors need specific backend capabilities, but relationship isn't formally defined
4. **Configuration complexity**: Hard to generate configs that compose patterns with appropriate backends

## Solution: Three-Layer Schema Architecture

### Layer 1: Backend Interface Schemas

**Design Decision: Explicit Interface Flavors (Not Capability Flags)**

We use **thin, purpose-specific interfaces** rather than monolithic interfaces with capability flags because:

1. **Type Safety**: Compiler enforces contracts. If backend implements `KeyValueScanInterface`, scanning MUST work.
2. **Clear Contracts**: No runtime surprises. Interface presence guarantees functionality.
3. **Composability**: Backends compose multiple interfaces like traits/mixins.
4. **Proto-First**: All contracts in `.proto` files, not separate metadata.

**Pattern**: Each data model has multiple interface flavors:
- `<Model>Basic` - Core CRUD operations (required)
- `<Model>Scan` - Enumeration capability (optional)
- `<Model>TTL` - Time-to-live expiration (optional)
- `<Model>Transactional` - Multi-operation atomicity (optional)
- `<Model>Batch` - Bulk operations (optional)

**Examples**:

```protobuf
// proto/interfaces/keyvalue_basic.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

// Core key-value operations - ALL backends must implement
service KeyValueBasicInterface {
  rpc Set(SetRequest) returns (SetResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Exists(ExistsRequest) returns (ExistsResponse);
}
```

```protobuf
// proto/interfaces/keyvalue_scan.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

// Enumeration support - backends with efficient iteration
service KeyValueScanInterface {
  rpc Scan(ScanRequest) returns (stream ScanResponse);
  rpc ScanKeys(ScanKeysRequest) returns (stream KeyResponse);
  rpc Count(CountRequest) returns (CountResponse);
}

// Implemented by: Redis, PostgreSQL, etcd, DynamoDB
// NOT implemented by: MemStore (small only), S3 (expensive)
```

```protobuf
// proto/interfaces/keyvalue_ttl.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

// Time-to-live expiration
service KeyValueTTLInterface {
  rpc Expire(ExpireRequest) returns (ExpireResponse);
  rpc GetTTL(GetTTLRequest) returns (GetTTLResponse);
  rpc Persist(PersistRequest) returns (PersistResponse);  // Remove TTL
}

// Implemented by: Redis, DynamoDB, etcd, MemStore
// NOT implemented by: PostgreSQL (requires cron), S3 (lifecycle policies)
```

```protobuf
// proto/interfaces/keyvalue_transactional.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

// Multi-operation transactions
service KeyValueTransactionalInterface {
  rpc BeginTransaction(BeginTransactionRequest) returns (TransactionHandle);
  rpc SetInTransaction(TransactionSetRequest) returns (SetResponse);
  rpc GetInTransaction(TransactionGetRequest) returns (GetResponse);
  rpc Commit(CommitRequest) returns (CommitResponse);
  rpc Rollback(RollbackRequest) returns (RollbackResponse);
}

// Implemented by: Redis (MULTI/EXEC), PostgreSQL (ACID), DynamoDB (TransactWriteItems)
// NOT implemented by: MemStore, S3, etcd (single-key only)
```

```protobuf
// proto/interfaces/keyvalue_batch.proto
syntax = "proto3";
package prism.interfaces.keyvalue;

// Bulk operations for efficiency
service KeyValueBatchInterface {
  rpc BatchSet(BatchSetRequest) returns (BatchSetResponse);
  rpc BatchGet(BatchGetRequest) returns (BatchGetResponse);
  rpc BatchDelete(BatchDeleteRequest) returns (BatchDeleteResponse);
}

// Implemented by: Redis (MGET/MSET), PostgreSQL (bulk INSERT), DynamoDB (BatchWriteItem)
```

```protobuf
// proto/interfaces/pubsub_basic.proto
syntax = "proto3";
package prism.interfaces.pubsub;

// Core publish/subscribe - ALL backends must implement
service PubSubBasicInterface {
  rpc Publish(PublishRequest) returns (PublishResponse);
  rpc Subscribe(SubscribeRequest) returns (stream Message);
  rpc Unsubscribe(UnsubscribeRequest) returns (UnsubscribeResponse);
}
```

```protobuf
// proto/interfaces/pubsub_wildcards.proto
syntax = "proto3";
package prism.interfaces.pubsub;

// Wildcard subscriptions (e.g., topic.*, topic.>)
service PubSubWildcardsInterface {
  rpc SubscribePattern(SubscribePatternRequest) returns (stream Message);
}

// Implemented by: NATS, Redis Pub/Sub, RabbitMQ
// NOT implemented by: Kafka (topics are explicit)
```

```protobuf
// proto/interfaces/pubsub_persistent.proto
syntax = "proto3";
package prism.interfaces.pubsub;

// Durable pub/sub with message persistence
service PubSubPersistentInterface {
  rpc PublishPersistent(PublishRequest) returns (PublishResponse);
  rpc SubscribeDurable(SubscribeDurableRequest) returns (stream Message);
  rpc GetLastMessageID(GetLastMessageIDRequest) returns (MessageIDResponse);
}

// Implemented by: Kafka, NATS JetStream, Redis Streams (as pub/sub)
// NOT implemented by: Redis Pub/Sub, NATS Core
```

```protobuf
// proto/interfaces/stream_basic.proto
syntax = "proto3";
package prism.interfaces.stream;

// Append-only log with offset-based reads
service StreamBasicInterface {
  rpc Append(AppendRequest) returns (AppendResponse);
  rpc Read(ReadRequest) returns (stream StreamRecord);
  rpc GetLatestOffset(GetLatestOffsetRequest) returns (OffsetResponse);
}
```

```protobuf
// proto/interfaces/stream_consumer_groups.proto
syntax = "proto3";
package prism.interfaces.stream;

// Consumer group coordination
service StreamConsumerGroupsInterface {
  rpc CreateConsumerGroup(CreateConsumerGroupRequest) returns (CreateConsumerGroupResponse);
  rpc JoinConsumerGroup(JoinConsumerGroupRequest) returns (stream StreamRecord);
  rpc Ack(AckRequest) returns (AckResponse);
  rpc GetConsumerGroupInfo(GetConsumerGroupInfoRequest) returns (ConsumerGroupInfo);
}

// Implemented by: Kafka, Redis Streams, NATS JetStream
// NOT implemented by: Kafka (different model), S3 (no coordination)
```

```protobuf
// proto/interfaces/stream_replay.proto
syntax = "proto3";
package prism.interfaces.stream;

// Read from arbitrary historical offset
service StreamReplayInterface {
  rpc SeekToOffset(SeekToOffsetRequest) returns (SeekResponse);
  rpc SeekToTimestamp(SeekToTimestampRequest) returns (SeekResponse);
  rpc ReplayRange(ReplayRangeRequest) returns (stream StreamRecord);
}

// Implemented by: Kafka, Redis Streams, NATS JetStream
```

**Complete Interface Catalog** (45 thin interfaces across 10 data models):

**KeyValue (6 interfaces)**:
- `keyvalue_basic.proto` - Core CRUD (Set, Get, Delete, Exists)
- `keyvalue_scan.proto` - Enumeration (Scan, ScanKeys, Count)
- `keyvalue_ttl.proto` - Expiration (Expire, GetTTL, Persist)
- `keyvalue_transactional.proto` - Transactions (Begin, Commit, Rollback)
- `keyvalue_batch.proto` - Bulk operations (BatchSet, BatchGet, BatchDelete)
- `keyvalue_cas.proto` - Compare-and-swap (CompareAndSwap, CompareAndDelete)

**PubSub (5 interfaces)**:
- `pubsub_basic.proto` - Core pub/sub (Publish, Subscribe, Unsubscribe)
- `pubsub_wildcards.proto` - Pattern subscriptions (SubscribePattern)
- `pubsub_persistent.proto` - Durable messages (PublishPersistent, SubscribeDurable)
- `pubsub_filtering.proto` - Server-side filtering (PublishWithAttributes, SubscribeFiltered)
- `pubsub_ordering.proto` - Message ordering guarantees (PublishOrdered)

**Stream (5 interfaces)**:
- `stream_basic.proto` - Append-only log (Append, Read, GetLatestOffset)
- `stream_consumer_groups.proto` - Coordination (CreateGroup, Join, Ack)
- `stream_replay.proto` - Historical reads (SeekToOffset, SeekToTimestamp, ReplayRange)
- `stream_retention.proto` - Lifecycle management (SetRetention, GetRetention, Compact)
- `stream_partitioning.proto` - Parallel processing (GetPartitions, AppendToPartition)

**Queue (5 interfaces)**:
- `queue_basic.proto` - FIFO operations (Enqueue, Dequeue, Peek)
- `queue_visibility.proto` - Visibility timeout (DequeueWithTimeout, ExtendVisibility, Release)
- `queue_dead_letter.proto` - Failed message handling (ConfigureDeadLetter, GetDeadLetterQueue)
- `queue_priority.proto` - Priority queues (EnqueueWithPriority)
- `queue_delayed.proto` - Delayed delivery (EnqueueDelayed, GetScheduledCount)

**List (4 interfaces)**:
- `list_basic.proto` - Ordered operations (PushLeft, PushRight, PopLeft, PopRight)
- `list_indexing.proto` - Random access (Get, Set, Insert, Remove)
- `list_range.proto` - Bulk operations (GetRange, Trim)
- `list_blocking.proto` - Blocking pops (BlockingPopLeft, BlockingPopRight)

**Set (4 interfaces)**:
- `set_basic.proto` - Membership (Add, Remove, Contains, GetMembers)
- `set_operations.proto` - Set algebra (Union, Intersection, Difference)
- `set_cardinality.proto` - Size tracking (GetSize, IsMember)
- `set_random.proto` - Random sampling (GetRandomMember, PopRandomMember)

**SortedSet (5 interfaces)**:
- `sortedset_basic.proto` - Scored operations (Add, Remove, GetScore)
- `sortedset_range.proto` - Range queries (GetRange, GetRangeByScore)
- `sortedset_rank.proto` - Ranking (GetRank, GetReverseRank)
- `sortedset_operations.proto` - Set operations with scores (Union, Intersection)
- `sortedset_lex.proto` - Lexicographic range (GetRangeByLex)

**TimeSeries (4 interfaces)**:
- `timeseries_basic.proto` - Time-indexed writes (Insert, Query)
- `timeseries_aggregation.proto` - Downsampling (Aggregate, Rollup)
- `timeseries_retention.proto` - Data lifecycle (SetRetention, Compact)
- `timeseries_interpolation.proto` - Gap filling (InterpolateLinear, InterpolateStep)

**Graph (4 interfaces)**:
- `graph_basic.proto` - Node/edge CRUD (AddNode, AddEdge, DeleteNode, DeleteEdge)
- `graph_traversal.proto` - Graph walks (TraverseDepthFirst, TraverseBreadthFirst)
- `graph_query.proto` - Query languages (GremlinQuery, CypherQuery, SparqlQuery)
- `graph_analytics.proto` - Graph algorithms (ShortestPath, PageRank, ConnectedComponents)

**Document (3 interfaces)**:
- `document_basic.proto` - JSON/BSON storage (Insert, Update, Delete, Get)
- `document_query.proto` - Document queries (Find, FindOne, Aggregate)
- `document_indexing.proto` - Secondary indexes (CreateIndex, DropIndex, ListIndexes)

### Layer 2: Backend Implementation Matrix

**Definition**: Mapping of which backends implement which interfaces.

**Example Matrix** (stored as `registry/backends/redis.yaml`):

```yaml
backend: redis
description: "In-memory data structure store"
plugin: prism-redis:v1.2.0
connection_string_format: "redis://host:port/db"

# Redis implements 16 interfaces across 6 data models
implements:
  # KeyValue (5 of 6) - Missing only CAS
  - keyvalue_basic          # Set, Get, Delete, Exists
  - keyvalue_scan           # Scan, ScanKeys, Count
  - keyvalue_ttl            # Expire, GetTTL, Persist
  - keyvalue_transactional  # MULTI/EXEC transactions
  - keyvalue_batch          # MGET, MSET, MDEL

  # PubSub (2 of 5) - Fire-and-forget messaging
  - pubsub_basic            # Publish, Subscribe, Unsubscribe
  - pubsub_wildcards        # Pattern subscriptions (*)

  # Stream (4 of 5) - Redis Streams
  - stream_basic            # XADD, XREAD, XINFO
  - stream_consumer_groups  # XGROUP, XREADGROUP, XACK
  - stream_replay           # XREAD with offset
  - stream_retention        # MAXLEN, XTRIM

  # List (4 of 4) - Complete list support
  - list_basic              # LPUSH, RPUSH, LPOP, RPOP
  - list_indexing           # LINDEX, LSET, LINSERT, LREM
  - list_range              # LRANGE, LTRIM
  - list_blocking           # BLPOP, BRPOP

  # Set (4 of 4) - Complete set support
  - set_basic               # SADD, SREM, SISMEMBER, SMEMBERS
  - set_operations          # SUNION, SINTER, SDIFF
  - set_cardinality         # SCARD
  - set_random              # SRANDMEMBER, SPOP

  # SortedSet (5 of 5) - Complete sorted set support
  - sortedset_basic         # ZADD, ZREM, ZSCORE
  - sortedset_range         # ZRANGE, ZRANGEBYSCORE
  - sortedset_rank          # ZRANK, ZREVRANK
  - sortedset_operations    # ZUNION, ZINTER
  - sortedset_lex           # ZRANGEBYLEX

# 16 interfaces total
```

**Postgres Backend** (stored as `registry/backends/postgres.yaml`):

```yaml
backend: postgres
description: "Relational database with strong consistency"
plugin: prism-postgres:v1.5.0
connection_string_format: "postgresql://user:pass@host:port/db"

# Postgres implements 9 interfaces across 5 data models
implements:
  # KeyValue (4 of 6) - No TTL (requires cron), no CAS
  - keyvalue_basic          # INSERT, SELECT, DELETE via KV table
  - keyvalue_scan           # SELECT * FROM kv WHERE key LIKE ...
  - keyvalue_transactional  # ACID transactions
  - keyvalue_batch          # Bulk INSERT, SELECT IN (...)

  # Queue (4 of 5) - Using queue table with SKIP LOCKED
  - queue_basic             # INSERT INTO queue, SELECT FOR UPDATE SKIP LOCKED
  - queue_visibility        # Visibility timeout via timestamp
  - queue_dead_letter       # Failed messages to DLQ table
  - queue_delayed           # Scheduled delivery via timestamp

  # TimeSeries (3 of 4) - With TimescaleDB extension
  - timeseries_basic        # Hypertables for time-series data
  - timeseries_aggregation  # Continuous aggregates
  - timeseries_retention    # Retention policies

  # Document (3 of 3) - JSONB support
  - document_basic          # INSERT, UPDATE, DELETE with JSONB column
  - document_query          # WHERE jsonb_column @> '{"key": "value"}'
  - document_indexing       # GIN indexes on JSONB

  # Graph (2 of 4) - Limited graph support
  - graph_basic             # Nodes and edges tables
  - graph_traversal         # Recursive CTEs (WITH RECURSIVE)

# 16 interfaces total (different mix than Redis)
```

**MemStore Backend** (stored as `registry/backends/memstore.yaml`):

```yaml
backend: memstore
description: "In-memory Go map for local testing"
plugin: built-in
connection_string_format: "mem://local"

# MemStore implements 2 interfaces - minimal viable keyvalue
implements:
  - keyvalue_basic  # sync.Map operations
  - keyvalue_ttl    # TTL with time.AfterFunc cleanup

# 2 interfaces total - intentionally minimal for fast local testing
```

**Kafka Backend** (stored as `registry/backends/kafka.yaml`):

```yaml
backend: kafka
description: "Distributed event streaming platform"
plugin: prism-kafka:v2.0.0
connection_string_format: "kafka://broker1:9092,broker2:9092"

# Kafka implements 5 stream interfaces + pub/sub
implements:
  # Stream (5 of 5) - Complete streaming platform
  - stream_basic                # Produce, Consume
  - stream_consumer_groups      # Consumer group coordination
  - stream_replay               # Seek to offset/timestamp
  - stream_retention            # Retention policies
  - stream_partitioning         # Topic partitions

  # PubSub (2 of 5) - Topics as pub/sub channels
  - pubsub_basic                # Publish to topic, subscribe to topic
  - pubsub_persistent           # Durable messages with offsets

# 7 interfaces total - focused on streaming
```

**Backend Interface Count Comparison**:

| Backend | Interfaces | Data Models | Notes |
|---------|------------|-------------|-------|
| Redis | 16 | KeyValue, PubSub, Stream, List, Set, SortedSet | Most versatile general-purpose backend |
| Postgres | 16 | KeyValue, Queue, TimeSeries, Document, Graph | Different mix, strong consistency |
| Kafka | 7 | Stream, PubSub | Specialized for event streaming |
| NATS | 8 | PubSub, Stream, Queue | Lightweight messaging |
| DynamoDB | 9 | KeyValue, Document, Set | AWS managed NoSQL |
| ClickHouse | 3 | TimeSeries, Stream | Specialized for analytics |
| Neptune | 4 | Graph | Specialized for graph queries |
| MemStore | 2 | KeyValue | Minimal for local testing |

**Key Insights**:
1. **Redis & Postgres are workhorses**: Both implement 16 interfaces but different mixes
2. **Specialized backends focus**: Kafka (streaming), Neptune (graph), ClickHouse (analytics)
3. **Test backends minimal**: MemStore implements just enough for local development
4. **No backend implements all 45 interfaces**: Backends specialize in what they're good at
```

### Layer 3: Pattern Schemas with Slots

**Definition**: High-level patterns that compose multiple backend interfaces.

**Pattern Schema Example** (stored as `registry/patterns/multicast_registry.yaml`):

```yaml
pattern: multicast-registry
version: v1
description: "Register identities with metadata and multicast messages to filtered subsets"
executor: prism-pattern-multicast-registry:v1.0.0

# Pattern requires two slots (+ one optional) to be filled
slots:
  registry:
    description: "Stores identity → metadata mappings"
    required_interfaces:
      - keyvalue_basic  # MUST implement basic KV operations
      - keyvalue_scan   # MUST support enumeration
    optional_interfaces:
      - keyvalue_ttl    # Nice to have: auto-expire offline identities
    recommended_backends:
      - redis           # Has all 3 interfaces
      - postgres        # Has basic + scan (no TTL)
      - dynamodb        # Has all 3 interfaces
      - etcd            # Has all 3 interfaces

  messaging:
    description: "Delivers multicast messages to identities"
    required_interfaces:
      - pubsub_basic    # MUST implement basic pub/sub
    optional_interfaces:
      - pubsub_persistent  # Nice to have: durable delivery
    recommended_backends:
      - nats            # Has basic (+ wildcards if needed)
      - redis           # Has basic + wildcards
      - kafka           # Has basic + persistent

  # Optional third slot for durability
  durability:
    description: "Persists undelivered messages for offline identities"
    required_interfaces:
      - queue_basic           # MUST implement basic queue ops
      - queue_visibility      # MUST support visibility timeout
      - queue_dead_letter     # MUST handle failed deliveries
    recommended_backends:
      - postgres        # Has all 3 queue interfaces
      - sqs             # Has all 3 queue interfaces (AWS)
      - rabbitmq        # Has all 3 queue interfaces
    optional: true

# Pattern-level API (different from backend interfaces)
api:
  proto_file: "proto/patterns/multicast_registry.proto"
  service: MulticastRegistryService
  methods:
    - Register(RegisterRequest) returns (RegisterResponse)
    - Enumerate(EnumerateRequest) returns (EnumerateResponse)
    - Multicast(MulticastRequest) returns (MulticastResponse)
    - Deregister(DeregisterRequest) returns (DeregisterResponse)

# How pattern executor uses slots
implementation:
  register_flow:
    - slot: registry
      operation: Set(identity, metadata)
    - slot: messaging
      operation: Subscribe(identity)  # Pre-subscribe for receiving

  enumerate_flow:
    - slot: registry
      operation: Scan(filter)

  multicast_flow:
    - slot: registry
      operation: Scan(filter)  # Get matching identities
    - slot: messaging
      operation: Publish(identity, message)  # Fan-out
    - slot: durability  # If configured
      operation: Enqueue(identity, message)  # For offline identities
```

**Example Configuration** (using the pattern):

```yaml
namespaces:
  - name: iot-devices
    pattern: multicast-registry
    pattern_version: v1

    # Fill the required slots with backends that implement required interfaces
    slots:
      registry:
        backend: redis
        # Redis implements: keyvalue_basic, keyvalue_scan, keyvalue_ttl ✓
        interfaces:
          - keyvalue_basic
          - keyvalue_scan
          - keyvalue_ttl  # Optional but Redis has it
        config:
          connection: "redis://localhost:6379/0"
          key_prefix: "iot:"
          ttl_seconds: 3600

      messaging:
        backend: nats
        # NATS implements: pubsub_basic, pubsub_wildcards ✓
        interfaces:
          - pubsub_basic
        config:
          connection: "nats://localhost:4222"
          subject_prefix: "iot.devices."

      # Optional durability slot
      durability:
        backend: postgres
        # Postgres implements: queue_basic, queue_visibility, queue_dead_letter ✓
        interfaces:
          - queue_basic
          - queue_visibility
          - queue_dead_letter
        config:
          connection: "postgresql://localhost:5432/prism"
          table: "iot_message_queue"
          visibility_timeout: 30
```

## Capabilities Expressed Through Interfaces

**Design Note**: We do NOT use separate capability flags. Instead, **capabilities are expressed through interface presence**.

**Examples**:

| Capability | How It's Expressed |
|------------|-------------------|
| TTL Support | Backend implements `keyvalue_ttl` interface |
| Scan Support | Backend implements `keyvalue_scan` interface |
| Transactions | Backend implements `keyvalue_transactional` interface |
| Wildcards in Pub/Sub | Backend implements `pubsub_wildcards` interface |
| Consumer Groups | Backend implements `stream_consumer_groups` interface |
| Replay | Backend implements `stream_replay` interface |
| Persistence | Backend implements `pubsub_persistent` interface |
| Visibility Timeout | Backend implements `queue_visibility` interface |
| Dead Letter Queue | Backend implements `queue_dead_letter` interface |
| Priority Queues | Backend implements `queue_priority` interface |
| Graph Traversal | Backend implements `graph_traversal` interface |
| Document Indexing | Backend implements `document_indexing` interface |

**Why This is Better**:
1. **Type-safe**: Compiler checks backend has required interfaces
2. **Self-documenting**: Look at implemented interfaces to know capabilities
3. **No runtime surprises**: If interface is present, capability MUST work
4. **Proto-first**: Everything in `.proto` files, not separate YAML metadata

## Schema Registry Filesystem Layout

```
registry/
├── interfaces/              # Layer 1: Backend interface schemas
│   ├── keyvalue.yaml       # Interface definition + capabilities
│   ├── pubsub.yaml
│   ├── stream.yaml
│   ├── list.yaml
│   ├── set.yaml
│   ├── sortedset.yaml
│   ├── timeseries.yaml
│   ├── graph.yaml
│   ├── document.yaml
│   └── queue.yaml
│
├── backends/                # Layer 2: Backend implementation matrix
│   ├── redis.yaml          # Which interfaces Redis implements
│   ├── postgres.yaml
│   ├── nats.yaml
│   ├── kafka.yaml
│   ├── dynamodb.yaml
│   ├── clickhouse.yaml
│   ├── neptune.yaml
│   └── memstore.yaml
│
├── patterns/                # Layer 3: Pattern schemas with slots
│   ├── multicast-registry.yaml
│   ├── saga.yaml
│   ├── event-sourcing.yaml
│   ├── cache-aside.yaml
│   └── work-queue.yaml
│
├── capabilities.yaml        # Cross-cutting capabilities definitions
├── matrix.yaml              # Complete backend × interface matrix
└── README.md                # Registry documentation

proto/
├── interfaces/              # Protobuf definitions for interfaces
│   ├── keyvalue.proto
│   ├── pubsub.proto
│   ├── stream.proto
│   └── ...
│
└── patterns/                # Protobuf definitions for patterns
    ├── multicast_registry.proto
    ├── saga.proto
    └── ...
```

## Benefits

### 1. **Explicit Capability Mapping**

```yaml
# Before (ambiguous)
backend: redis

# After (explicit)
slots:
  registry:
    backend: redis
    interface: keyvalue  # Clear which Redis interface
    required_capabilities:
      scan_support: true
      ttl_support: true
```

### 2. **Straightforward Configuration Generation**

```python
# Generate config from requirements
requirements = {
    "pattern": "multicast-registry",
    "needs_ttl": True,
    "needs_persistence": False
}

# Find backends that satisfy requirements
registry_backends = find_backends(
    interface="keyvalue",
    capabilities={"scan_support": True, "ttl_support": True}
)
# → [redis, dynamodb, etcd]

messaging_backends = find_backends(
    interface="pubsub",
    capabilities={"persistence": False}
)
# → [nats, redis]

# Generate config
config = generate_namespace_config(
    pattern="multicast-registry",
    registry_backend="redis",
    messaging_backend="nats"
)
```

### 3. **Backend Substitutability**

```yaml
# Development (fast local testing)
slots:
  registry:
    backend: memstore
    interface: keyvalue
  messaging:
    backend: nats
    interface: pubsub

# Production (durable)
slots:
  registry:
    backend: redis
    interface: keyvalue
  messaging:
    backend: kafka
    interface: pubsub
```

### 4. **Pattern Portability**

Same pattern works with different backend combinations:

```yaml
# Combination 1: Redis + NATS
slots: {registry: redis, messaging: nats}

# Combination 2: Postgres + Kafka
slots: {registry: postgres, messaging: kafka}

# Combination 3: DynamoDB + SNS
slots: {registry: dynamodb, messaging: sns}
```

## Implementation Phases

### Phase 1: Define Interface Schemas (Week 1)
1. Create `registry/interfaces/` with 10 core interface definitions
2. Document capabilities per interface in `registry/capabilities.yaml`
3. Generate protobuf files in `proto/interfaces/`

### Phase 2: Backend Implementation Matrix (Week 2)
1. Create `registry/backends/` with 8 backend definitions
2. Map which interfaces each backend implements
3. Document backend-specific capabilities
4. Generate `registry/matrix.yaml` (complete backend × interface matrix)

### Phase 3: Pattern Schemas (Week 3)
1. Create `registry/patterns/` with 5 initial pattern definitions
2. Define slots for each pattern
3. Specify required vs optional capabilities per slot
4. Generate pattern protobuf files in `proto/patterns/`

### Phase 4: Configuration Generator (Week 4)
1. Build `prismctl generate config` command
2. Input: Pattern requirements + constraints
3. Output: Valid namespace configuration
4. Validation: Check backend implements required interfaces/capabilities

### Phase 5: Registry Validation Tool (Week 5)
1. Validate all YAML schemas
2. Check backend × interface matrix consistency
3. Verify capability references are valid
4. Ensure pattern slot requirements are satisfiable

## Example: Full Configuration Generation Flow

```bash
# 1. List available patterns
$ prismctl registry patterns list
multicast-registry    v1    Register identities and multicast to subsets
event-sourcing        v1    Append-only event log with replay
saga                  v1    Distributed transaction coordinator
work-queue            v1    Background job processing with retries

# 2. Show pattern requirements
$ prismctl registry patterns describe multicast-registry
Pattern: multicast-registry v1
Slots:
  registry (required):
    Interface: keyvalue
    Required capabilities: scan_support
    Optional capabilities: ttl_support
    Recommended backends: redis, postgres, dynamodb, etcd

  messaging (required):
    Interface: pubsub
    Recommended backends: nats, redis, kafka

# 3. Find backends that satisfy requirements
$ prismctl registry backends find --interface=keyvalue --capability=scan_support
redis       ✓ keyvalue with scan_support, ttl_support
postgres    ✓ keyvalue with scan_support
dynamodb    ✓ keyvalue with scan_support, ttl_support
etcd        ✓ keyvalue with scan_support, ttl_support

# 4. Generate configuration
$ prismctl generate config \
    --pattern=multicast-registry \
    --slot=registry:redis \
    --slot=messaging:nats \
    --namespace=iot-devices

Generated config:
namespaces:
  - name: iot-devices
    pattern: multicast-registry
    pattern_version: v1
    slots:
      registry:
        backend: redis
        interface: keyvalue
        config:
          connection: "redis://localhost:6379/0"
      messaging:
        backend: nats
        interface: pubsub
        config:
          connection: "nats://localhost:4222"
```

## Validation Rules

### Backend Validation
1. Backend must declare which interfaces it implements (list of interface names)
2. Backend plugin must exist and match version
3. All listed interfaces must have corresponding `.proto` definitions

### Pattern Validation
1. Pattern must declare all required slots
2. Each slot must specify required interfaces (list of interface names)
3. Optional interfaces must be marked as such
4. Pattern executor plugin must exist and match version
5. Pattern API proto file must exist

### Configuration Validation
1. All required slots must be filled
2. Each slot's backend must implement ALL required interfaces for that slot
3. Backend is validated at config-load time: `prismctl validate config.yaml`
4. Connection strings must match backend's expected format

**Example Validation**:
```bash
$ prismctl validate namespace-config.yaml

Validating namespace: iot-devices
Pattern: multicast-registry v1

Slot: registry
  Backend: redis
  Required interfaces:
    ✓ keyvalue_basic       (redis implements)
    ✓ keyvalue_scan        (redis implements)
  Optional interfaces:
    ✓ keyvalue_ttl         (redis implements)

Slot: messaging
  Backend: nats
  Required interfaces:
    ✓ pubsub_basic         (nats implements)

✅ Configuration valid
```

## Related Documents

- [RFC-014: Layered Data Access Patterns](/rfc/RFC-014-layered-data-access-patterns) - Layer 1 primitives
- [RFC-017: Multicast Registry Pattern](/rfc/RFC-017-multicast-registry-pattern) - Example pattern with slots
- [MEMO-005: Client Protocol Design Philosophy](/memos/MEMO-005-client-protocol-design-philosophy) - Layered API architecture
- [RFC-008: Proxy Plugin Architecture](/rfc/RFC-008-proxy-plugin-architecture) - Plugin system

## Revision History

- 2025-10-09: Initial draft defining three-layer schema architecture (interfaces, backends, patterns)
