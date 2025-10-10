---
title: Prism Protobuf Data Model Plan
date: 2025-10-05
tags: [protobuf, planning, data-model]
---

This document outlines the complete protobuf structure for Prism, organized by module and abstraction.

**Version**: proto3 (latest stable)

## Directory Structure

```text
proto/
├── prism/
│   ├── options.proto              # Custom Prism options (tags)
│   ├── common/
│   │   ├── types.proto           # Common types (timestamps, UUIDs, etc.)
│   │   ├── errors.proto          # Error definitions
│   │   └── metadata.proto        # Item metadata (compression, encryption)
│   ├── keyvalue/
│   │   └── v1/
│   │       ├── keyvalue.proto    # KeyValue service
│   │       └── types.proto       # KeyValue-specific types
│   ├── timeseries/
│   │   └── v1/
│   │       ├── timeseries.proto  # TimeSeries service
│   │       └── types.proto       # TimeSeries-specific types
│   ├── graph/
│   │   └── v1/
│   │       ├── graph.proto       # Graph service
│   │       └── types.proto       # Graph-specific types
│   └── control/
│       └── v1/
│           ├── namespace.proto   # Namespace management
│           └── config.proto      # Configuration service
└── buf.yaml                       # Buf configuration
```

## Module 1: Custom Options (Prism Tags)

**File**: `proto/prism/options.proto`

```protobuf
syntax = "proto3";

package prism;

import "google/protobuf/descriptor.proto";

//
// Message-level options (for data models)
//

extend google.protobuf.MessageOptions {
  // Namespace: logical dataset name
  string namespace = 50000;

  // Access pattern hint for capacity planning
  // Values: "read_heavy" | "write_heavy" | "append_heavy" | "balanced"
  string access_pattern = 50001;

  // Capacity estimates
  int64 estimated_read_rps = 50002;
  int64 estimated_write_rps = 50003;
  int64 estimated_data_size_gb = 50004;

  // Backend selection
  // Values: "postgres" | "kafka" | "nats" | "sqlite" | "neptune" | "auto"
  string backend = 50005;

  // Consistency level
  // Values: "strong" | "eventual" | "causal"
  string consistency = 50006;

  // Retention policy
  int32 retention_days = 50007;  // 0 = keep forever

  // Caching
  bool enable_cache = 50008;
  int32 cache_ttl_seconds = 50009;
}

//
// Field-level options (for individual fields)
//

extend google.protobuf.FieldOptions {
  // Index type
  // Values: "primary" | "secondary" | "partition_key" | "clustering_key"
  string index = 50100;

  // PII classification
  // Values: "email" | "name" | "phone" | "ssn" | "address" | "ip_address" | "credit_card"
  string pii = 50101;

  // Encryption at rest
  bool encrypt_at_rest = 50102;

  // Masking in logs/traces
  bool mask_in_logs = 50103;

  // Audit all accesses to this field
  bool access_audit = 50104;

  // Validation rules
  string validation = 50105;  // "email" | "uuid" | "url" | "regex:..." | "min:N" | "max:N"

  // Max length for strings
  int32 max_length = 50106;

  // Required field (for backward compatibility tracking)
  bool required = 50107;
}

//
// Service-level options (for gRPC services)
//

extend google.protobuf.ServiceOptions {
  // Require authentication for all RPCs
  bool require_auth = 50200;

  // Service-wide rate limit (requests per second)
  int32 rate_limit_rps = 50201;

  // Service version
  string version = 50202;
}

//
// RPC-level options (for individual methods)
//

extend google.protobuf.MethodOptions {
  // Is this RPC idempotent? (safe to retry)
  bool idempotent = 50300;

  // RPC timeout in milliseconds
  int32 timeout_ms = 50301;

  // Cache RPC responses
  bool cacheable = 50302;
  int32 cache_ttl_seconds = 50303;

  // Required permissions
  // Values: "read" | "write" | "admin"
  repeated string required_permissions = 50304;
}
```

## Module 2: Common Types

**File**: `proto/prism/common/types.proto`

```protobuf
syntax = "proto3";

package prism.common;

// Timestamp (epoch milliseconds for consistency with Kafka)
message Timestamp {
  int64 millis = 1;
}

// UUID (standard 128-bit UUID)
message UUID {
  string value = 1;  // UUID string format
}

// Cursor for pagination
message Cursor {
  bytes token = 1;  // Opaque pagination token
}

// Time range
message TimeRange {
  int64 start_millis = 1;  // Inclusive
  int64 end_millis = 2;     // Exclusive
}

// Tags for filtering/indexing
message Tags {
  map<string, string> tags = 1;
}

// Compression type
enum CompressionType {
  COMPRESSION_TYPE_UNSPECIFIED = 0;
  COMPRESSION_TYPE_NONE = 1;
  COMPRESSION_TYPE_LZ4 = 2;
  COMPRESSION_TYPE_ZSTD = 3;
  COMPRESSION_TYPE_GZIP = 4;
}

// Consistency level
enum ConsistencyLevel {
  CONSISTENCY_LEVEL_UNSPECIFIED = 0;
  CONSISTENCY_LEVEL_EVENTUAL = 1;
  CONSISTENCY_LEVEL_CAUSAL = 2;
  CONSISTENCY_LEVEL_STRONG = 3;
}
```

**File**: `proto/prism/common/metadata.proto`

```protobuf
syntax = "proto3";

package prism.common;

import "prism/common/types.proto";

// Metadata for stored items
message ItemMetadata {
  // Compression info
  CompressionType compression = 1;

  // Encryption info (if encrypted)
  EncryptionInfo encryption = 2;

  // TTL (time-to-live)
  int64 ttl_seconds = 3;

  // Chunking info (for large items)
  ChunkMetadata chunking = 4;

  // Created/updated timestamps
  int64 created_at_millis = 5;
  int64 updated_at_millis = 6;
}

message EncryptionInfo {
  string algorithm = 1;  // e.g., "AES-256-GCM"
  string key_id = 2;     // Key version for rotation
}

message ChunkMetadata {
  int32 total_chunks = 1;
  int32 chunk_size_bytes = 2;
  string hash_algorithm = 3;  // e.g., "SHA256"
  bytes hash = 4;             // Hash of complete data
}
```

**File**: `proto/prism/common/errors.proto`

```protobuf
syntax = "proto3";

package prism.common;

// Standard error response
message Error {
  ErrorCode code = 1;
  string message = 2;
  map<string, string> details = 3;
  string request_id = 4;
}

enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;

  // Client errors (4xx)
  ERROR_CODE_INVALID_REQUEST = 400;
  ERROR_CODE_UNAUTHORIZED = 401;
  ERROR_CODE_FORBIDDEN = 403;
  ERROR_CODE_NOT_FOUND = 404;
  ERROR_CODE_CONFLICT = 409;
  ERROR_CODE_PRECONDITION_FAILED = 412;
  ERROR_CODE_RATE_LIMITED = 429;

  // Server errors (5xx)
  ERROR_CODE_INTERNAL = 500;
  ERROR_CODE_BACKEND_UNAVAILABLE = 503;
  ERROR_CODE_TIMEOUT = 504;
}
```

## Module 3: KeyValue Abstraction

**File**: `proto/prism/keyvalue/v1/keyvalue.proto`

```protobuf
syntax = "proto3";

package prism.keyvalue.v1;

import "prism/options.proto";
import "prism/keyvalue/v1/types.proto";

option (prism.version) = "v1";

// KeyValue service: HashMap<String, SortedMap<Bytes, Bytes>>
service KeyValueService {
  option (prism.require_auth) = true;

  // Write operations
  rpc Put(PutRequest) returns (PutResponse) {
    option (prism.idempotent) = false;
    option (prism.timeout_ms) = 5000;
    option (prism.required_permissions) = "write";
  }

  rpc PutBatch(PutBatchRequest) returns (PutBatchResponse) {
    option (prism.idempotent) = false;
    option (prism.timeout_ms) = 10000;
    option (prism.required_permissions) = "write";
  }

  rpc Delete(DeleteRequest) returns (DeleteResponse) {
    option (prism.idempotent) = true;
    option (prism.timeout_ms) = 5000;
    option (prism.required_permissions) = "write";
  }

  // Read operations
  rpc Get(GetRequest) returns (GetResponse) {
    option (prism.idempotent) = true;
    option (prism.cacheable) = true;
    option (prism.cache_ttl_seconds) = 300;
    option (prism.timeout_ms) = 5000;
    option (prism.required_permissions) = "read";
  }

  rpc Scan(ScanRequest) returns (stream ScanResponse) {
    option (prism.idempotent) = true;
    option (prism.timeout_ms) = 30000;
    option (prism.required_permissions) = "read";
  }

  // Conditional operations
  rpc CompareAndSwap(CompareAndSwapRequest) returns (CompareAndSwapResponse) {
    option (prism.idempotent) = false;
    option (prism.timeout_ms) = 5000;
    option (prism.required_permissions) = "write";
  }
}
```

**File**: `proto/prism/keyvalue/v1/types.proto`

```protobuf
syntax = "proto3";

package prism.keyvalue.v1;

import "prism/common/metadata.proto";
import "prism/common/types.proto";

// Put request
message PutRequest {
  string namespace = 1;
  string id = 2;  // Partition key
  repeated Item items = 3;
  int64 item_priority_token = 4;  // For ordering within batch
}

message PutResponse {
  bool success = 1;
}

// Batch put
message PutBatchRequest {
  string namespace = 1;
  repeated PutRequest requests = 2;
}

message PutBatchResponse {
  repeated PutResponse responses = 1;
}

// Delete request
message DeleteRequest {
  string namespace = 1;
  string id = 2;
  repeated bytes keys = 3;
}

message DeleteResponse {
  bool success = 1;
  int32 deleted_count = 2;
}

// Get request
message GetRequest {
  string namespace = 1;
  string id = 2;
  KeyPredicate predicate = 3;
}

message GetResponse {
  repeated Item items = 1;
}

// Scan request
message ScanRequest {
  string namespace = 1;
  string id = 2;
  optional Cursor cursor = 3;
  int32 limit = 4;
}

message ScanResponse {
  repeated Item items = 1;
  optional Cursor next_cursor = 2;
}

// Compare and swap
message CompareAndSwapRequest {
  string namespace = 1;
  string id = 2;
  bytes key = 3;
  optional bytes old_value = 4;  // null = expect not exists
  bytes new_value = 5;
}

message CompareAndSwapResponse {
  bool success = 1;
  optional bytes actual_value = 2;  // Current value if CAS failed
}

// Item (key-value pair)
message Item {
  bytes key = 1;    // Sort key within partition
  bytes value = 2;  // Payload
  prism.common.ItemMetadata metadata = 3;
}

// Key predicate for filtering
message KeyPredicate {
  oneof predicate {
    MatchAll match_all = 1;          // All keys
    MatchKeys match_keys = 2;         // Specific keys
    MatchRange match_range = 3;       // Range of keys
  }
}

message MatchAll {}

message MatchKeys {
  repeated bytes keys = 1;
}

message MatchRange {
  optional bytes start_key = 1;  // Inclusive
  optional bytes end_key = 2;    // Exclusive
}
```

## Module 4: TimeSeries Abstraction

**File**: `proto/prism/timeseries/v1/timeseries.proto`

```protobuf
syntax = "proto3";

package prism.timeseries.v1;

import "prism/options.proto";
import "prism/timeseries/v1/types.proto";

// TimeSeries service: Append-only log with time-based queries
service TimeSeriesService {
  option (prism.require_auth) = true;

  // Write
  rpc Append(AppendRequest) returns (AppendResponse) {
    option (prism.idempotent) = false;
    option (prism.timeout_ms) = 5000;
    option (prism.required_permissions) = "write";
  }

  rpc AppendBatch(AppendBatchRequest) returns (AppendBatchResponse) {
    option (prism.idempotent) = false;
    option (prism.timeout_ms) = 10000;
    option (prism.required_permissions) = "write";
  }

  // Read
  rpc Query(QueryRequest) returns (stream QueryResponse) {
    option (prism.idempotent) = true;
    option (prism.timeout_ms) = 30000;
    option (prism.required_permissions) = "read";
  }

  // Subscribe to new events (tail)
  rpc Tail(TailRequest) returns (stream TailResponse) {
    option (prism.idempotent) = true;
    option (prism.required_permissions) = "read";
  }

  // Management
  rpc CreateStream(CreateStreamRequest) returns (CreateStreamResponse) {
    option (prism.idempotent) = true;
    option (prism.timeout_ms) = 10000;
    option (prism.required_permissions) = "admin";
  }
}
```

**File**: `proto/prism/timeseries/v1/types.proto`

```protobuf
syntax = "proto3";

package prism.timeseries.v1;

import "prism/common/types.proto";

// Append event
message AppendRequest {
  string stream = 1;
  Event event = 2;
}

message AppendResponse {
  int64 offset = 1;  // Position in stream
  int64 timestamp_millis = 2;
}

message AppendBatchRequest {
  string stream = 1;
  repeated Event events = 2;
}

message AppendBatchResponse {
  repeated int64 offsets = 1;
}

// Event
message Event {
  int64 timestamp_millis = 1;  // Event time
  bytes payload = 2;           // Event data (serialized proto, JSON, etc.)
  map<string, string> tags = 3; // For filtering
}

// Query
message QueryRequest {
  string stream = 1;
  int64 start_time_millis = 2;  // Inclusive
  int64 end_time_millis = 3;    // Exclusive
  map<string, string> filter_tags = 4;
  int32 limit = 5;
  optional Cursor cursor = 6;
}

message QueryResponse {
  repeated Event events = 1;
  optional Cursor next_cursor = 2;
}

// Tail (subscribe)
message TailRequest {
  string stream = 1;
  int64 from_timestamp_millis = 2;  // Start from this time
  map<string, string> filter_tags = 3;
}

message TailResponse {
  Event event = 1;
}

// Stream management
message CreateStreamRequest {
  string stream = 1;
  StreamConfig config = 2;
}

message CreateStreamResponse {
  bool success = 1;
}

message StreamConfig {
  int32 retention_days = 1;
  int32 partitions = 2;  // For Kafka backend
  int32 replication_factor = 3;
}
```

## Module 5: Graph Abstraction

**File**: `proto/prism/graph/v1/graph.proto`

```protobuf
syntax = "proto3";

package prism.graph.v1;

import "prism/options.proto";
import "prism/graph/v1/types.proto";

// Graph service: Nodes and edges
service GraphService {
  option (prism.require_auth) = true;

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
```

**File**: `proto/prism/graph/v1/types.proto`

```protobuf
syntax = "proto3";

package prism.graph.v1;

// Node
message Node {
  string id = 1;
  string type = 2;  // e.g., "user", "post", "product"
  map<string, bytes> properties = 3;
  int64 created_at_millis = 4;
}

message CreateNodeRequest {
  string graph = 1;  // Graph namespace
  Node node = 2;
}

message CreateNodeResponse {
  bool success = 1;
}

message GetNodeRequest {
  string graph = 1;
  string node_id = 2;
}

message GetNodeResponse {
  optional Node node = 1;
}

// Edge
message Edge {
  string from_node_id = 1;
  string to_node_id = 2;
  string relationship = 3;  // e.g., "follows", "likes", "purchased"
  map<string, bytes> properties = 4;
  int64 created_at_millis = 5;
}

message CreateEdgeRequest {
  string graph = 1;
  Edge edge = 2;
}

message CreateEdgeResponse {
  bool success = 1;
}

message GetEdgesRequest {
  string graph = 1;
  string node_id = 2;
  Direction direction = 3;
  repeated string relationship_types = 4;  // Filter
}

message GetEdgesResponse {
  repeated Edge edges = 1;
}

enum Direction {
  DIRECTION_UNSPECIFIED = 0;
  DIRECTION_OUTGOING = 1;  // Edges from this node
  DIRECTION_INCOMING = 2;  // Edges to this node
  DIRECTION_BOTH = 3;
}

// Traversal
message TraverseRequest {
  string graph = 1;
  string start_node_id = 2;
  repeated string relationship_types = 3;
  int32 max_depth = 4;
  int32 limit = 5;
  TraversalAlgorithm algorithm = 6;
}

message TraverseResponse {
  repeated TraversalResult results = 1;
}

message TraversalResult {
  Node node = 1;
  int32 depth = 2;  // Distance from start node
  repeated Edge path = 3;  // Path to this node
}

enum TraversalAlgorithm {
  TRAVERSAL_ALGORITHM_UNSPECIFIED = 0;
  TRAVERSAL_ALGORITHM_BFS = 1;  // Breadth-first
  TRAVERSAL_ALGORITHM_DFS = 2;  // Depth-first
}

message ShortestPathRequest {
  string graph = 1;
  string from_node_id = 2;
  string to_node_id = 3;
  repeated string relationship_types = 4;
  int32 max_depth = 5;
}

message ShortestPathResponse {
  repeated Edge path = 1;
  int32 distance = 2;
}
```

## Module 6: Control Plane (Namespace Management)

**File**: `proto/prism/control/v1/namespace.proto`

```protobuf
syntax = "proto3";

package prism.control.v1;

import "prism/options.proto";

// Namespace management service
service NamespaceService {
  option (prism.require_auth) = true;

  rpc CreateNamespace(CreateNamespaceRequest) returns (CreateNamespaceResponse);
  rpc GetNamespace(GetNamespaceRequest) returns (GetNamespaceResponse);
  rpc ListNamespaces(ListNamespacesRequest) returns (ListNamespacesResponse);
  rpc UpdateNamespace(UpdateNamespaceRequest) returns (UpdateNamespaceResponse);
  rpc DeleteNamespace(DeleteNamespaceRequest) returns (DeleteNamespaceResponse);

  // Watch for namespace changes (long-poll or stream)
  rpc WatchNamespaces(WatchNamespacesRequest) returns (stream WatchNamespacesResponse);
}

message Namespace {
  string name = 1;
  AbstractionType abstraction = 2;
  string backend = 3;
  CapacitySpec capacity = 4;
  NamespacePolicies policies = 5;
  AccessControl access_control = 6;
  NamespaceStatus status = 7;
  int64 created_at_millis = 8;
}

enum AbstractionType {
  ABSTRACTION_TYPE_UNSPECIFIED = 0;
  ABSTRACTION_TYPE_KEYVALUE = 1;
  ABSTRACTION_TYPE_TIMESERIES = 2;
  ABSTRACTION_TYPE_GRAPH = 3;
}

message CapacitySpec {
  int64 estimated_read_rps = 1;
  int64 estimated_write_rps = 2;
  int64 estimated_data_size_gb = 3;
}

message NamespacePolicies {
  int32 retention_days = 1;
  string consistency = 2;
  bool cache_enabled = 3;
  int32 cache_ttl_seconds = 4;
}

message AccessControl {
  repeated Owner owners = 1;
  repeated Consumer consumers = 2;
}

message Owner {
  string team = 1;
  string role = 2;
}

message Consumer {
  string service_pattern = 1;  // e.g., "user-api.prod.*"
  repeated string permissions = 2;  // "read", "write", "admin"
}

enum NamespaceStatus {
  NAMESPACE_STATUS_UNSPECIFIED = 0;
  NAMESPACE_STATUS_PROVISIONING = 1;
  NAMESPACE_STATUS_ACTIVE = 2;
  NAMESPACE_STATUS_DEGRADED = 3;
  NAMESPACE_STATUS_DELETING = 4;
  NAMESPACE_STATUS_DELETED = 5;
}
```

## Buf Configuration

**File**: `buf.yaml`

```yaml
version: v1

name: buf.build/prism/prism  # If using Buf Schema Registry
deps:
  - buf.build/googleapis/googleapis

breaking:
  use:
    - FILE  # Prevent breaking changes

lint:
  use:
    - DEFAULT
  enum_zero_value_suffix: _UNSPECIFIED
  rpc_request_standard_name: true
  rpc_response_standard_name: true
  service_suffix: Service

build:
  excludes:
    - proto/vendor
```

## Summary

This protobuf plan provides:

✅ **Complete type system** across 6 modules
✅ **Custom Prism tags** for capacity planning, PII, caching, etc.
✅ **Three core abstractions**: KeyValue, TimeSeries, Graph
✅ **Control plane** for namespace management
✅ **Versioned APIs** (v1) for future evolution
✅ **Rich metadata** for compression, encryption, chunking
✅ **Consistent patterns** across all services

**Next Steps**:
1. Implement `proto/prism/options.proto` first (foundation)
2. Add `common/` types
3. Implement KeyValue abstraction (simplest)
4. Generate Rust code with `prost`
5. Build SQLite backend to validate design
