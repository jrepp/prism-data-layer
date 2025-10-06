# FR-001: Core Data Abstractions

**Status**: Draft

**Priority**: P0 (Critical)

**Owner**: Core Team

## Overview

Prism provides high-level data abstractions that hide backend complexity. Inspired by Netflix's KV DAL, we provide abstractions for the most common data access patterns.

## Stakeholders

- **Primary**: Application developers using Prism
- **Secondary**: Platform team operating backends

## User Stories

**As an application developer**, I want to store and retrieve key-value pairs without worrying about which database is used, so that I can focus on application logic.

**As an application developer**, I want to append time-series events and query by time range, so that I can build analytics features.

**As an application developer**, I want to traverse graph relationships efficiently, so that I can build social features.

**As a platform engineer**, I want to swap backend implementations transparently, so that I can optimize costs and performance.

## Data Abstractions

### 1. KeyValue Abstraction

**Model**: `HashMap<String, SortedMap<Bytes, Bytes>>`
A two-level map where each ID contains a sorted map of keys to values.

**Use Cases**:
- User profiles
- Application state
- Caching
- Session storage

**Operations**:

```protobuf
service KeyValueService {
  // Write operations
  rpc Put(PutRequest) returns (PutResponse);
  rpc PutBatch(PutBatchRequest) returns (PutBatchResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc DeleteBatch(DeleteBatchRequest) returns (DeleteBatchResponse);

  // Read operations
  rpc Get(GetRequest) returns (GetResponse);
  rpc GetBatch(GetBatchRequest) returns (GetBatchResponse);
  rpc Scan(ScanRequest) returns (stream ScanResponse);

  // Conditional operations
  rpc CompareAndSwap(CASRequest) returns (CASResponse);
}

message PutRequest {
  string namespace = 1;          // Logical dataset
  string id = 2;                  // Partition key
  repeated Item items = 3;        // Items to write
  int64 item_priority_token = 4; // For ordering
}

message Item {
  bytes key = 1;    // Sort key within partition
  bytes value = 2;  // Payload
  ItemMetadata metadata = 3;
}

message ItemMetadata {
  CompressionType compression = 1;
  EncryptionInfo encryption = 2;
  int32 ttl_seconds = 3;
}
```

**Backend Implementations**:
- PostgreSQL: Table with (id, key) composite primary key
- Cassandra: Natural fit (partition key, clustering key)
- SQLite: Same schema as Postgres, for local testing
- S3: For large blobs (chunking at application layer)

**Features**:
- **Chunking**: Automatic chunking for values > 1MB
- **Compression**: Client-side compression (LZ4, zstd)
- **Caching**: Optional look-aside cache (Redis/memcached)
- **Pagination**: Cursor-based for large scans

**Example**:

```rust
// Define data model
#[derive(prost::Message)]
pub struct UserProfile {
    #[prost(string, tag = "1")]
    pub user_id: String,
    #[prost(string, tag = "2")]
    pub email: String,
    #[prost(string, tag = "3")]
    pub name: String,
}

// Use KeyValue abstraction
let kv_client = prism::KeyValueClient::new("user-profiles");

// Write
let profile = UserProfile {
    user_id: "user123".to_string(),
    email: "alice@example.com".to_string(),
    name: "Alice".to_string(),
};

kv_client.put("user123", "profile", &profile).await?;

// Read
let profile: UserProfile = kv_client.get("user123", "profile").await?;
```

---

### 2. TimeSeries Abstraction

**Model**: Append-only log of events with timestamp-based querying

**Use Cases**:
- Application logs
- User activity tracking
- Metrics
- Event sourcing

**Operations**:

```protobuf
service TimeSeriesService {
  // Write
  rpc Append(AppendRequest) returns (AppendResponse);
  rpc AppendBatch(AppendBatchRequest) returns (AppendBatchResponse);

  // Read
  rpc Query(QueryRequest) returns (stream QueryResponse);
  rpc Tail(TailRequest) returns (stream TailResponse);  // Subscribe to new events

  // Management
  rpc CreateStream(CreateStreamRequest) returns (CreateStreamResponse);
  rpc DeleteStream(DeleteStreamRequest) returns (DeleteStreamResponse);
}

message AppendRequest {
  string stream = 1;        // Stream name
  int64 timestamp = 2;      // Event time (epoch millis)
  bytes payload = 3;        // Event data
  map<string, string> tags = 4;  // For filtering
}

message QueryRequest {
  string stream = 1;
  int64 start_time = 2;     // Inclusive
  int64 end_time = 3;       // Exclusive
  map<string, string> filter_tags = 4;
  int32 limit = 5;
  string cursor = 6;        // For pagination
}
```

**Backend Implementations**:
- Kafka: Natural fit for append-only log
- ClickHouse: For analytical queries
- PostgreSQL + TimescaleDB: Time-series extension
- NATS JetStream: Lightweight alternative to Kafka

**Features**:
- **Retention Policies**: Auto-delete old data
- **Compaction**: Optional for Kafka
- **Partitioning**: By time range for query efficiency
- **Replication**: Configurable durability

**Example**:

```rust
let ts_client = prism::TimeSeriesClient::new("user-events");

// Append event
ts_client.append("user:123:logins", Event {
    timestamp: Utc::now().timestamp_millis(),
    payload: serde_json::to_vec(&LoginEvent { ip: "1.2.3.4" })?,
    tags: hashmap! { "user_id" => "123", "event_type" => "login" },
}).await?;

// Query events
let events = ts_client.query("user:123:logins", QueryRequest {
    start_time: one_day_ago,
    end_time: now,
    filter_tags: hashmap! { "event_type" => "login" },
    limit: 100,
}).collect().await?;
```

---

### 3. Graph Abstraction

**Model**: Nodes and directed edges with properties

**Use Cases**:
- Social graphs
- Recommendation engines
- Knowledge graphs
- Dependency tracking

**Operations**:

```protobuf
service GraphService {
  // Node operations
  rpc CreateNode(CreateNodeRequest) returns (CreateNodeResponse);
  rpc GetNode(GetNodeRequest) returns (GetNodeResponse);
  rpc UpdateNode(UpdateNodeRequest) returns (UpdateNodeResponse);
  rpc DeleteNode(DeleteNodeRequest) returns (DeleteNodeResponse);

  // Edge operations
  rpc CreateEdge(CreateEdgeRequest) returns (CreateEdgeResponse);
  rpc GetEdges(GetEdgesRequest) returns (stream GetEdgesResponse);
  rpc DeleteEdge(DeleteEdgeRequest) returns (DeleteEdgeResponse);

  // Traversal
  rpc Traverse(TraverseRequest) returns (stream TraverseResponse);
  rpc ShortestPath(ShortestPathRequest) returns (ShortestPathResponse);
}

message Node {
  string id = 1;
  string type = 2;  // e.g., "user", "post", "product"
  map<string, bytes> properties = 3;
}

message Edge {
  string from_node_id = 1;
  string to_node_id = 2;
  string relationship = 3;  // e.g., "follows", "likes", "purchased"
  map<string, bytes> properties = 4;
}

message TraverseRequest {
  string start_node_id = 1;
  repeated string relationship_types = 2;  // Filter by relationship
  int32 max_depth = 3;
  int32 limit = 4;
}
```

**Backend Implementations**:
- AWS Neptune: Managed graph database
- Neo4j: Self-hosted option
- PostgreSQL: Recursive CTEs for simple graphs
- KeyValue + adjacency lists: For read-heavy graphs (like Netflix RDG)

**Features**:
- **BFS/DFS Traversal**: Breadth-first or depth-first
- **Path Finding**: Shortest path, all paths
- **Filtering**: By node/edge types
- **Pagination**: Limit results

**Example**:

```rust
let graph_client = prism::GraphClient::new("social-graph");

// Create nodes
graph_client.create_node(Node {
    id: "user:alice",
    type: "user",
    properties: hashmap! { "name" => b"Alice" },
}).await?;

graph_client.create_node(Node {
    id: "user:bob",
    type: "user",
    properties: hashmap! { "name" => b"Bob" },
}).await?;

// Create edge
graph_client.create_edge(Edge {
    from_node_id: "user:alice",
    to_node_id: "user:bob",
    relationship: "follows",
    properties: hashmap! {},
}).await?;

// Traverse
let followers = graph_client.traverse(TraverseRequest {
    start_node_id: "user:bob",
    relationship_types: vec!["follows"],
    max_depth: 1,
    limit: 100,
}).collect().await?;
```

---

## Acceptance Criteria

- [ ] All three abstractions have complete protobuf definitions
- [ ] Each abstraction has at least one backend implementation
- [ ] Integration tests cover all operations
- [ ] Load tests achieve performance targets (see NFR-001)
- [ ] Documentation includes examples for each abstraction
- [ ] Client libraries generated for Rust, Python, TypeScript

## Dependencies

- **ADR-003**: Protobuf as Single Source of Truth
- **ADR-004**: Local-First Testing
- **NFR-001**: Performance targets inform API design

## Implementation Notes

### Phased Rollout

**Phase 1**: KeyValue with PostgreSQL and SQLite
**Phase 2**: TimeSeries with Kafka
**Phase 3**: Graph with Neptune

### Common Features Across Abstractions

All abstractions support:
- **mTLS authentication**
- **Namespace isolation**
- **Metrics and tracing**
- **Rate limiting**
- **Automatic retries**
- **Circuit breaking**

### Protobuf Organization

```
proto/
├── prism/
│   ├── keyvalue/
│   │   └── v1/
│   │       ├── keyvalue.proto
│   │       └── types.proto
│   ├── timeseries/
│   │   └── v1/
│   │       ├── timeseries.proto
│   │       └── types.proto
│   └── graph/
│       └── v1/
│           ├── graph.proto
│           └── types.proto
```

## Open Questions

1. **Should we support transactions across abstractions?**
   - Probably not in v1; focus on single-abstraction transactions first

2. **How do we handle schema evolution?**
   - Protobuf handles backward compatibility
   - Need migration tooling for backend schema changes

3. **What's the story for full-text search?**
   - Separate abstraction? Or feature of KeyValue/Graph?

## References

- Netflix KV Data Abstraction Layer (docs/netflix/)
- Netflix Real-Time Distributed Graph (docs/netflix/video2.md)
- [AWS Neptune](https://aws.amazon.com/neptune/)
- [Kafka as a Storage System](https://kafka.apache.org/documentation/#design_storage)

## Revision History

- 2025-10-05: Initial draft
