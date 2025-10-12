---
date: 2025-10-09
deciders: Platform Team
doc_uuid: 62421fcd-75b9-413a-ba59-a53387f52666
id: adr-043
project_id: prism-data-layer
status: Accepted
tags:
- architecture
- backend
- plugin
- protobuf
- api-design
title: 'ADR-043: Plugin Capability Discovery System'
---

## Context

Backend plugins have **varying capabilities** depending on the underlying data store:

**Example: Graph Operations**
- **Neptune**: Supports Gremlin, SPARQL, full ACID transactions, read replicas
- **Neo4j**: Supports Cypher, ACID transactions, no SPARQL
- **TinkerGraph (in-memory)**: Supports Gremlin, no persistence, no clustering
- **JanusGraph**: Supports Gremlin, eventual consistency, distributed

**Example: KeyValue Operations**
- **Redis**: Fast reads, limited transactions, no complex queries
- **PostgreSQL**: Full SQL, ACID transactions, complex queries, slower
- **DynamoDB**: Fast reads, limited transactions, eventual consistency option

**Current Problem**: Clients don't know what features a plugin supports until they try and fail. This leads to:
- Runtime errors for unsupported operations
- Poor error messages ("operation not supported")
- No way to select optimal plugin for use case
- No compile-time validation of plugin compatibility

## Decision

Implement a **Plugin Capability Discovery System** where:

1. **Plugins declare capabilities** in protobuf metadata
2. **Clients query capabilities** before invoking operations
3. **Prism validates** client requests against plugin capabilities
4. **Admin API exposes** capability matrix for operational visibility

### Capability Hierarchy

```protobuf
syntax = "proto3";

package prism.plugin.v1;

// Plugin capability declaration
message PluginCapabilities {
  // Plugin identity
  string plugin_name = 1;       // "postgres", "neptune", "redis"
  string plugin_version = 2;    // "1.2.0"
  repeated string backend_types = 3;  // ["postgres", "timescaledb"]

  // Supported data abstractions
  repeated DataAbstraction abstractions = 4;

  // Backend-specific features
  BackendFeatures features = 5;

  // Performance characteristics
  PerformanceProfile performance = 6;

  // Operational constraints
  OperationalConstraints constraints = 7;
}

enum DataAbstraction {
  DATA_ABSTRACTION_UNSPECIFIED = 0;
  DATA_ABSTRACTION_KEY_VALUE = 1;
  DATA_ABSTRACTION_TIME_SERIES = 2;
  DATA_ABSTRACTION_GRAPH = 3;
  DATA_ABSTRACTION_DOCUMENT = 4;
  DATA_ABSTRACTION_QUEUE = 5;
  DATA_ABSTRACTION_PUBSUB = 6;
}

message BackendFeatures {
  // Transaction support
  TransactionCapabilities transactions = 1;

  // Query capabilities
  QueryCapabilities queries = 2;

  // Consistency models
  repeated ConsistencyLevel consistency_levels = 3;

  // Persistence guarantees
  PersistenceFeatures persistence = 4;

  // Scaling capabilities
  ScalingFeatures scaling = 5;
}

message TransactionCapabilities {
  bool supports_transactions = 1;
  bool supports_acid = 2;
  bool supports_optimistic_locking = 3;
  bool supports_pessimistic_locking = 4;
  bool supports_distributed_transactions = 5;
  int64 max_transaction_duration_ms = 6;
}

message QueryCapabilities {
  // Graph-specific
  repeated string graph_query_languages = 1;  // ["gremlin", "cypher", "sparql"]
  bool supports_graph_algorithms = 2;
  repeated string supported_algorithms = 3;  // ["pagerank", "shortest_path"]

  // SQL-specific
  bool supports_sql = 4;
  repeated string sql_features = 5;  // ["joins", "window_functions", "cte"]

  // General
  bool supports_secondary_indexes = 6;
  bool supports_full_text_search = 7;
  bool supports_aggregations = 8;
}

enum ConsistencyLevel {
  CONSISTENCY_LEVEL_UNSPECIFIED = 0;
  CONSISTENCY_LEVEL_EVENTUAL = 1;
  CONSISTENCY_LEVEL_READ_AFTER_WRITE = 2;
  CONSISTENCY_LEVEL_STRONG = 3;
  CONSISTENCY_LEVEL_LINEARIZABLE = 4;
}

message PersistenceFeatures {
  bool supports_durable_writes = 1;
  bool supports_snapshots = 2;
  bool supports_point_in_time_recovery = 3;
  bool supports_continuous_backup = 4;
}

message ScalingFeatures {
  bool supports_read_replicas = 1;
  bool supports_horizontal_sharding = 2;
  bool supports_vertical_scaling = 3;
  int32 max_read_replicas = 4;
}

message PerformanceProfile {
  // Latency characteristics
  int64 typical_read_latency_p50_us = 1;
  int64 typical_write_latency_p50_us = 2;

  // Throughput
  int64 max_reads_per_second = 3;
  int64 max_writes_per_second = 4;

  // Batch sizes
  int32 max_batch_size = 5;
}

message OperationalConstraints {
  // Connection limits
  int32 max_connections_per_instance = 1;

  // Data limits
  int64 max_key_size_bytes = 2;
  int64 max_value_size_bytes = 3;
  int64 max_query_result_size_bytes = 4;

  // Deployment constraints
  repeated string required_cloud_providers = 5;  // ["aws", "gcp", "azure"]
  bool requires_vpc = 6;
}
```

## Capability Discovery Flow

### 1. Plugin Registration

When a plugin starts, it registers its capabilities:

```go
// plugins/postgres/main.go
func (p *PostgresPlugin) GetCapabilities() *PluginCapabilities {
    return &PluginCapabilities{
        PluginName:    "postgres",
        PluginVersion: "1.2.0",
        BackendTypes:  []string{"postgres", "timescaledb"},
        Abstractions: []DataAbstraction{
            DataAbstraction_DATA_ABSTRACTION_KEY_VALUE,
            DataAbstraction_DATA_ABSTRACTION_TIME_SERIES,
        },
        Features: &BackendFeatures{
            Transactions: &TransactionCapabilities{
                SupportsTransactions:     true,
                SupportsAcid:             true,
                SupportsOptimisticLocking: true,
                MaxTransactionDurationMs: 30000,
            },
            Queries: &QueryCapabilities{
                SupportsSql: true,
                SqlFeatures: []string{"joins", "window_functions", "cte"},
                SupportsSecondaryIndexes: true,
                SupportsFullTextSearch:   true,
                SupportsAggregations:     true,
            },
            ConsistencyLevels: []ConsistencyLevel{
                ConsistencyLevel_CONSISTENCY_LEVEL_STRONG,
                ConsistencyLevel_CONSISTENCY_LEVEL_LINEARIZABLE,
            },
        },
        Performance: &PerformanceProfile{
            TypicalReadLatencyP50Us:  2000,   // 2ms
            TypicalWriteLatencyP50Us: 5000,   // 5ms
            MaxReadsPerSecond:        100000,
            MaxWritesPerSecond:       50000,
        },
    }
}
```

### 2. Client Capability Query

Clients query capabilities before selecting a backend:

```protobuf
service PluginDiscoveryService {
  // List all registered plugins
  rpc ListPlugins(ListPluginsRequest) returns (ListPluginsResponse);

  // Get capabilities for specific plugin
  rpc GetPluginCapabilities(GetPluginCapabilitiesRequest) returns (PluginCapabilities);

  // Find plugins matching requirements
  rpc FindPlugins(FindPluginsRequest) returns (FindPluginsResponse);
}

message FindPluginsRequest {
  // Required abstractions
  repeated DataAbstraction required_abstractions = 1;

  // Required features
  BackendFeatures required_features = 2;

  // Performance requirements
  PerformanceRequirements performance_requirements = 3;

  // Ranking preferences
  PluginRankingPreferences preferences = 4;
}

message PerformanceRequirements {
  int64 max_read_latency_p50_us = 1;
  int64 max_write_latency_p50_us = 2;
  int64 min_reads_per_second = 3;
  int64 min_writes_per_second = 4;
}

message PluginRankingPreferences {
  enum RankingStrategy {
    RANKING_STRATEGY_UNSPECIFIED = 0;
    RANKING_STRATEGY_LOWEST_LATENCY = 1;
    RANKING_STRATEGY_HIGHEST_THROUGHPUT = 2;
    RANKING_STRATEGY_STRONGEST_CONSISTENCY = 3;
    RANKING_STRATEGY_MOST_FEATURES = 4;
  }
  RankingStrategy strategy = 1;
}

message FindPluginsResponse {
  repeated PluginMatch matches = 1;
}

message PluginMatch {
  string plugin_name = 1;
  PluginCapabilities capabilities = 2;
  float compatibility_score = 3;  // 0.0 to 1.0
  repeated string missing_features = 4;
}
```

### 3. Runtime Validation

Prism validates operations against plugin capabilities:

```go
func (p *Proxy) ValidateOperation(
    pluginName string,
    operation string,
) error {
    caps, err := p.registry.GetCapabilities(pluginName)
    if err != nil {
        return fmt.Errorf("plugin not found: %w", err)
    }

    switch operation {
    case "BeginTransaction":
        if !caps.Features.Transactions.SupportsTransactions {
            return fmt.Errorf(
                "plugin %s does not support transactions",
                pluginName,
            )
        }
    case "ExecuteGremlinQuery":
        if !slices.Contains(
            caps.Features.Queries.GraphQueryLanguages,
            "gremlin",
        ) {
            return fmt.Errorf(
                "plugin %s does not support Gremlin queries",
                pluginName,
            )
        }
    }

    return nil
}
```

## Example: Selecting Graph Plugin

Client wants to run Gremlin queries with ACID transactions:

```go
// Client code
req := &FindPluginsRequest{
    RequiredAbstractions: []DataAbstraction{
        DataAbstraction_DATA_ABSTRACTION_GRAPH,
    },
    RequiredFeatures: &BackendFeatures{
        Transactions: &TransactionCapabilities{
            SupportsTransactions: true,
            SupportsAcid:         true,
        },
        Queries: &QueryCapabilities{
            GraphQueryLanguages: []string{"gremlin"},
        },
    },
    Preferences: &PluginRankingPreferences{
        Strategy: RankingStrategy_RANKING_STRATEGY_LOWEST_LATENCY,
    },
}

resp, err := discoveryClient.FindPlugins(ctx, req)
if err != nil {
    log.Fatal(err)
}

if len(resp.Matches) == 0 {
    log.Fatal("No plugins match requirements")
}

// Best match
bestMatch := resp.Matches[0]
fmt.Printf("Selected plugin: %s (score: %.2f)\n",
    bestMatch.PluginName,
    bestMatch.CompatibilityScore,
)

// Matches: neptune (score: 0.95), neo4j (score: 0.90)
```

## Capability Inheritance and Composition

Some plugins support **multiple abstractions** with different capabilities:

```protobuf
message PluginCapabilities {
  // ... base fields ...

  // Abstraction-specific capabilities
  map<string, AbstractionCapabilities> abstraction_capabilities = 10;
}

message AbstractionCapabilities {
  DataAbstraction abstraction = 1;
  BackendFeatures features = 2;
  PerformanceProfile performance = 3;
}
```

**Example: Postgres plugin**:
```go
capabilities := &PluginCapabilities{
    PluginName: "postgres",
    AbstractionCapabilities: map[string]*AbstractionCapabilities{
        "keyvalue": {
            Abstraction: DataAbstraction_DATA_ABSTRACTION_KEY_VALUE,
            Features: &BackendFeatures{
                Transactions: &TransactionCapabilities{
                    SupportsAcid: true,
                },
            },
            Performance: &PerformanceProfile{
                TypicalReadLatencyP50Us: 2000,
            },
        },
        "timeseries": {
            Abstraction: DataAbstraction_DATA_ABSTRACTION_TIME_SERIES,
            Features: &BackendFeatures{
                Queries: &QueryCapabilities{
                    SupportsAggregations: true,
                },
            },
            Performance: &PerformanceProfile{
                TypicalReadLatencyP50Us: 5000,  // Slower for aggregations
            },
        },
    },
}
```

## Admin UI: Capability Matrix

Admin UI displays plugin capabilities in a comparison matrix:

```bash
prismctl plugin capabilities postgres neptune redis

┏━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━┳━━━━━━━━━┳━━━━━━━┓
┃ Feature          ┃ Postgres ┃ Neptune ┃ Redis ┃
┡━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━╇━━━━━━━━━╇━━━━━━━┩
│ Transactions     │ ✓ ACID   │ ✓ ACID  │ ✗     │
│ Graph (Gremlin)  │ ✗        │ ✓       │ ✗     │
│ Graph (Cypher)   │ ✗        │ ✗       │ ✗     │
│ SQL              │ ✓        │ ✗       │ ✗     │
│ Read Replicas    │ ✓ (15)   │ ✓ (15)  │ ✓ (5) │
│ P50 Read Latency │ 2ms      │ 3ms     │ 0.3ms │
│ Max Throughput   │ 100K/s   │ 50K/s   │ 1M/s  │
└──────────────────┴──────────┴─────────┴───────┘
```

## Consequences

### Positive

- ✅ **Clients know upfront** what plugins support
- ✅ **Better error messages**: "Neptune doesn't support Cypher, use Gremlin"
- ✅ **Automated plugin selection** based on requirements
- ✅ **Documentation auto-generated** from capability metadata
- ✅ **Testing simplified**: validate capabilities, not behavior
- ✅ **Operational visibility**: understand what backends can do

### Negative

- ❌ **Complexity**: More protobuf definitions to maintain
- ❌ **Version skew**: Plugin capabilities may change across versions
- ❌ **False advertising**: Plugins might claim unsupported features

### Neutral

- 🔄 **Capability evolution**: Must version capability schema carefully
- 🔄 **Partial support**: Some features may be "best effort"

## References

- ADR-005: Backend Plugin Architecture
- ADR-025: Container Plugin Model
- ADR-041: Graph Database Backend Support
- ADR-044: TinkerPop/Gremlin Generic Plugin (proposed)
- [Apache TinkerPop: Provider Requirements](https://tinkerpop.apache.org/)

## Revision History

- 2025-10-09: Initial ADR for plugin capability discovery