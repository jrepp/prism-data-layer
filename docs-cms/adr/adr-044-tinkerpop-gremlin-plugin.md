---
date: 2025-10-09
deciders: Platform Team
doc_uuid: a8087fcc-a065-4bae-abe3-daf98be8c29b
id: adr-044
project_id: prism-data-layer
status: Accepted
tags:
- backend
- graph
- plugin
- gremlin
- tinkerpop
title: 'ADR-044: TinkerPop/Gremlin Generic Plugin'
---

## Context

**RFC-013** specifies Neptune-specific implementation, but **Gremlin is a standard query language** (Apache TinkerPop) supported by multiple graph databases:

| Database | Gremlin Support | Native Query Language |
|----------|-----------------|----------------------|
| **AWS Neptune** | ✅ Yes | Gremlin + SPARQL |
| **JanusGraph** | ✅ Yes (reference impl) | Gremlin only |
| **Azure Cosmos DB** | ✅ Yes (Gremlin API) | Gremlin + SQL |
| **Neo4j** | ⚠️ Via plugin | Cypher (native) |
| **ArangoDB** | ⚠️ Via adapter | AQL (native) |
| **TinkerGraph** | ✅ Yes (in-memory) | Gremlin only |

**Problem**: Neptune plugin is tightly coupled to AWS-specific features (IAM auth, VPC, CloudWatch). We want a **generic Gremlin plugin** that can connect to any TinkerPop-compatible backend.

## Decision

Create a **generic TinkerPop/Gremlin plugin** that:

1. **Connects to any Gremlin Server** (TinkerPop standard)
2. **Declares capabilities** based on backend (ADR-043)
3. **Provides Neptune plugin** as specialized subclass
4. **Enables community backends** (JanusGraph, Cosmos DB, etc.)

### Plugin Hierarchy

prism-graph-plugin (generic)
├── gremlin-core/          # Generic Gremlin client
│   ├── connection.go      # WebSocket connection pool
│   ├── query.go           # Gremlin query builder
│   └── capabilities.go    # Capability detection
├── plugins/
│   ├── neptune/           # AWS Neptune (specialized)
│   │   ├── iam_auth.go
│   │   ├── vpc_config.go
│   │   └── cloudwatch.go
│   ├── janusgraph/        # JanusGraph (generic)
│   ├── cosmos/            # Azure Cosmos DB Gremlin API
│   └── tinkergraph/       # In-memory (for testing)
└── proto/
    └── graph.proto        # Unified graph API
```text

## Generic Gremlin Plugin Architecture

### Configuration

```
# Generic Gremlin Server connection
graph_backend:
  type: gremlin
  config:
    host: gremlin-server.example.com
    port: 8182
    use_tls: true
    auth:
      method: basic  # or "iam", "none"
      username: admin
      password: ${GREMLIN_PASSWORD}
    connection_pool:
      min_connections: 2
      max_connections: 20
    capabilities:
      auto_detect: true  # Query server for capabilities
```text

### Neptune-Specific Configuration

```
# Neptune (inherits from gremlin, adds AWS-specific)
graph_backend:
  type: neptune
  config:
    cluster_endpoint: my-cluster.cluster-abc.us-east-1.neptune.amazonaws.com
    port: 8182
    region: us-east-1
    auth:
      method: iam
      role_arn: arn:aws:iam::123456789:role/NeptuneAccess
    vpc:
      security_groups: [sg-123456]
      subnets: [subnet-abc, subnet-def]
    cloudwatch:
      metrics_enabled: true
      log_group: /aws/neptune/my-cluster
```text

## Capability Detection

Generic plugin **auto-detects** backend capabilities:

```
// gremlin-core/capabilities.go
func (c *GremlinClient) DetectCapabilities() (*PluginCapabilities, error) {
    caps := &PluginCapabilities{
        PluginName:    "gremlin",
        PluginVersion: "1.0.0",
        Abstractions: []DataAbstraction{
            DataAbstraction_DATA_ABSTRACTION_GRAPH,
        },
    }

    // Query server features
    features, err := c.queryServerFeatures()
    if err != nil {
        return nil, err
    }

    // Gremlin is always supported (it's the native protocol)
    caps.Features = &BackendFeatures{
        Queries: &QueryCapabilities{
            GraphQueryLanguages: []string{"gremlin"},
        },
    }

    // Detect transaction support
    if features.SupportsTransactions {
        caps.Features.Transactions = &TransactionCapabilities{
            SupportsTransactions: true,
            SupportsAcid:         features.SupportsACID,
        }
    }

    // Detect consistency levels
    caps.Features.ConsistencyLevels = detectConsistencyLevels(features)

    // Detect graph algorithms
    if features.SupportsGraphAlgorithms {
        caps.Features.Queries.SupportsGraphAlgorithms = true
        caps.Features.Queries.SupportedAlgorithms = queryAvailableAlgorithms(c)
    }

    return caps, nil
}

func (c *GremlinClient) queryServerFeatures() (*ServerFeatures, error) {
    // TinkerPop doesn't have a standard capabilities API,
    // so we probe with test queries
    features := &ServerFeatures{}

    // Test transaction support
    _, err := c.Submit("g.tx().open()")
    features.SupportsTransactions = (err == nil)

    // Test SPARQL (Neptune-specific)
    _, err = c.Submit("SELECT ?s ?p ?o WHERE { ?s ?p ?o } LIMIT 1")
    features.SupportsSPARQL = (err == nil)

    // Test graph algorithms (JanusGraph, Neptune)
    _, err = c.Submit("g.V().pageRank()")
    features.SupportsGraphAlgorithms = (err == nil)

    return features, nil
}
```text

### Backend-Specific Specialization

Neptune plugin **extends** generic plugin with AWS features:

```
// plugins/neptune/plugin.go
type NeptunePlugin struct {
    *gremlin.GenericGremlinPlugin  // Embed generic plugin
    iamAuth    *IAMAuth
    cloudWatch *CloudWatchClient
}

func (p *NeptunePlugin) GetCapabilities() (*PluginCapabilities, error) {
    // Start with generic Gremlin capabilities
    caps, err := p.GenericGremlinPlugin.GetCapabilities()
    if err != nil {
        return nil, err
    }

    // Add Neptune-specific features
    caps.PluginName = "neptune"
    caps.BackendTypes = []string{"neptune"}

    // Neptune always supports SPARQL
    caps.Features.Queries.GraphQueryLanguages = append(
        caps.Features.Queries.GraphQueryLanguages,
        "sparql",
    )

    // Neptune always has read replicas
    caps.Features.Scaling = &ScalingFeatures{
        SupportsReadReplicas: true,
        MaxReadReplicas:      15,
    }

    // Neptune-specific performance profile
    caps.Performance = &PerformanceProfile{
        TypicalReadLatencyP50Us:  3000,  // 3ms
        TypicalWriteLatencyP50Us: 8000,  // 8ms
        MaxReadsPerSecond:        50000,
        MaxWritesPerSecond:       25000,
    }

    return caps, nil
}
```text

## Example: Multi-Backend Support

Application uses **same Gremlin API** across different backends:

### Development: TinkerGraph (in-memory)

```
namespace: user-graph-dev
backend:
  type: tinkergraph
  config:
    auto_detect: true
```text

**Detected Capabilities**:
- Gremlin: ✅
- Transactions: ❌ (in-memory only)
- ACID: ❌
- Persistence: ❌
- Read Replicas: ❌

### Staging: JanusGraph (self-hosted)

```
namespace: user-graph-staging
backend:
  type: janusgraph
  config:
    host: janusgraph.staging.internal
    port: 8182
    auth:
      method: basic
      username: prism
      password: ${JANUS_PASSWORD}
```text

**Detected Capabilities**:
- Gremlin: ✅
- Transactions: ✅
- ACID: ⚠️ Eventual consistency (Cassandra backend)
- Persistence: ✅
- Read Replicas: ✅ (via Cassandra replication)

### Production: Neptune (AWS)

```
namespace: user-graph-prod
backend:
  type: neptune
  config:
    cluster_endpoint: prod-cluster.cluster-xyz.us-east-1.neptune.amazonaws.com
    region: us-east-1
    auth:
      method: iam
```text

**Detected Capabilities**:
- Gremlin: ✅
- SPARQL: ✅ (Neptune-specific)
- Transactions: ✅
- ACID: ✅
- Persistence: ✅
- Read Replicas: ✅ (up to 15)

## Client Code (Backend-Agnostic)

Application code is **identical** across all backends:

```
// Same code works with TinkerGraph, JanusGraph, Neptune
client := prism.NewGraphClient(namespace)

// Create vertices
alice := client.AddVertex("User", map[string]interface{}{
    "name":  "Alice",
    "email": "alice@example.com",
})

bob := client.AddVertex("User", map[string]interface{}{
    "name":  "Bob",
    "email": "bob@example.com",
})

// Create edge
client.AddEdge("FOLLOWS", alice.ID, bob.ID, map[string]interface{}{
    "since": "2025-01-01",
})

// Query: Find friends of friends
result, err := client.Gremlin(
    "g.V('user:alice').out('FOLLOWS').out('FOLLOWS').dedup().limit(10)",
)
```text

**Backend selection** is configuration-driven, not code-driven.

## Capability-Based Query Validation

Prism **validates queries** against backend capabilities:

```
func (p *Proxy) ExecuteGremlinQuery(
    namespace string,
    query string,
) (*GraphResult, error) {
    // Get plugin for namespace
    plugin, err := p.getPlugin(namespace)
    if err != nil {
        return nil, err
    }

    // Get capabilities
    caps, err := plugin.GetCapabilities()
    if err != nil {
        return nil, err
    }

    // Validate: Does backend support Gremlin?
    if !slices.Contains(caps.Features.Queries.GraphQueryLanguages, "gremlin") {
        return nil, fmt.Errorf(
            "backend %s does not support Gremlin queries",
            plugin.Name(),
        )
    }

    // Check for unsupported features in query
    if err := validateQueryFeatures(query, caps); err != nil {
        return nil, fmt.Errorf("unsupported query feature: %w", err)
    }

    // Execute query
    return plugin.ExecuteGremlin(query)
}

func validateQueryFeatures(query string, caps *PluginCapabilities) error {
    // Example: Check for graph algorithms
    if strings.Contains(query, ".pageRank()") {
        if !caps.Features.Queries.SupportsGraphAlgorithms {
            return fmt.Errorf(
                "backend does not support graph algorithms like pageRank()",
            )
        }
    }

    // Example: Check for SPARQL (Neptune-specific)
    if strings.HasPrefix(query, "SELECT") {
        if !slices.Contains(caps.Features.Queries.GraphQueryLanguages, "sparql") {
            return fmt.Errorf(
                "backend does not support SPARQL queries",
            )
        }
    }

    return nil
}
```text

## Benefits of Generic Plugin

### 1. **Development Flexibility**

Start with in-memory TinkerGraph, move to production Neptune:

```
# Development (local)
prismctl namespace create user-graph-dev --backend tinkergraph

# Staging (self-hosted)
prismctl namespace create user-graph-staging --backend janusgraph

# Production (AWS)
prismctl namespace create user-graph-prod --backend neptune
```text

### 2. **Cost Optimization**

Use cheaper backends for non-critical workloads:

```
# Expensive: Neptune (ACID, replicas, managed)
production_graph:
  backend: neptune
  cost: ~$750/month

# Moderate: JanusGraph (self-hosted, Cassandra)
staging_graph:
  backend: janusgraph
  cost: ~$200/month (EC2 + Cassandra)

# Cheap: TinkerGraph (in-memory, ephemeral)
dev_graph:
  backend: tinkergraph
  cost: $0 (local)
```text

### 3. **Vendor Independence**

Not locked into AWS:

- **AWS**: Neptune
- **Azure**: Cosmos DB Gremlin API
- **GCP**: Use JanusGraph on GKE
- **On-Prem**: JanusGraph or Neo4j (via adapter)

### 4. **Testing Simplified**

Integration tests use TinkerGraph (no external dependencies):

```
func TestGraphTraversal(t *testing.T) {
    // Fast, deterministic, no setup required
    plugin := NewTinkerGraphPlugin()

    // Create test graph
    plugin.AddVertex("A", "User", nil)
    plugin.AddVertex("B", "User", nil)
    plugin.AddEdge("follows-1", "FOLLOWS", "A", "B", nil)

    // Test traversal
    result, err := plugin.Gremlin("g.V('A').out('FOLLOWS')")
    require.NoError(t, err)
    assert.Len(t, result.Vertices, 1)
    assert.Equal(t, "B", result.Vertices[0].Id)
}
```text

## Community Ecosystem

Generic plugin enables **community-contributed backends**:

prism-plugins/
├── official/
│   ├── neptune/           # AWS Neptune (official)
│   ├── janusgraph/        # JanusGraph (official)
│   └── tinkergraph/       # In-memory testing (official)
├── community/
│   ├── cosmosdb/          # Azure Cosmos DB (community)
│   ├── neo4j-gremlin/     # Neo4j via Gremlin plugin (community)
│   └── arangodb-gremlin/  # ArangoDB via adapter (community)
```

## Consequences

### Positive

- ✅ **Gremlin works across backends** (Neptune, JanusGraph, Cosmos DB)
- ✅ **Development → Production** transition seamless
- ✅ **Cost-optimized** backend selection per environment
- ✅ **Vendor independence** (not locked to AWS)
- ✅ **Community ecosystem** for niche backends
- ✅ **Testing simplified** with in-memory TinkerGraph

### Negative

- ❌ **Capability detection** not standardized (must probe)
- ❌ **Feature parity** varies across backends
- ❌ **Backend-specific optimizations** harder to leverage

### Neutral

- 🔄 **Abstraction overhead**: Generic plugin is slightly slower
- 🔄 **Capability evolution**: Must update detection logic

## References

- [Apache TinkerPop](https://tinkerpop.apache.org/)
- [Gremlin Query Language](https://tinkerpop.apache.org/docs/current/reference/#graph-traversal-steps)
- [JanusGraph](https://janusgraph.org/)
- ADR-041: Graph Database Backend Support
- ADR-043: Plugin Capability Discovery System
- RFC-013: Neptune Graph Backend Implementation

## Revision History

- 2025-10-09: Initial ADR for generic TinkerPop/Gremlin plugin