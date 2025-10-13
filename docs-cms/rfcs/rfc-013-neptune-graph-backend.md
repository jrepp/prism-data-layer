---
author: Platform Team
created: 2025-10-09
doc_uuid: 8faa29ea-cf12-4b91-91f8-a586c79bd642
id: rfc-013
project_id: prism-data-layer
status: Draft
tags:
- backend
- graph
- neptune
- aws
- plugin
- implementation
title: Neptune Graph Backend Implementation
updated: 2025-10-09
---

# RFC-013: Neptune Graph Backend Implementation

> **Note**: This RFC provides implementation details for AWS Neptune as the first graph database backend. See [ADR-041: Graph Database Backend Support](/prism-data-layer/adr/adr-041-graph-database-backend) for the architectural decision and comparison of graph databases.

## Abstract

This RFC specifies the implementation details for the AWS Neptune graph backend plugin, including:
- Gremlin API integration
- IAM authentication
- Bulk import/export
- Performance optimization
- Cost considerations

Neptune was chosen as the first graph database implementation based on the comparison rubric in ADR-041.

## Context

Prism's graph database support (ADR-041) requires a concrete implementation. AWS Neptune was selected for the initial implementation due to:
- Fully managed service (zero operational burden)
- AWS ecosystem integration
- Multi-model support (Gremlin + SPARQL)
- Enterprise-grade reliability

This RFC focuses on applications that need to model and query highly connected data such as:
- **Social Networks**: User relationships, friend connections, followers
- **Knowledge Graphs**: Entity relationships, semantic networks
- **Recommendation Systems**: Item-item relationships, collaborative filtering
- **Fraud Detection**: Transaction networks, entity linkage
- **Dependency Graphs**: Service dependencies, package relationships

**AWS Neptune** is a managed graph database service that supports:
- **Property Graph Model**: Gremlin (Apache TinkerPop)
- **RDF Graph Model**: SPARQL
- **ACID Transactions**: Strong consistency guarantees
- **High Availability**: Multi-AZ deployments with automatic failover
- **Read Replicas**: Up to 15 read replicas for query scaling

## Decision

Implement a **Neptune Graph Backend Plugin** for Prism that provides:

1. **Graph Data Abstraction Layer**: Unified API for graph operations
2. **Gremlin Support**: Primary query interface (property graph model)
3. **SPARQL Support**: Optional for RDF/semantic web use cases
4. **Transaction Management**: ACID transactions for graph mutations
5. **Bulk Import/Export**: Efficient data loading and backup
6. **AWS Integration**: IAM authentication, VPC networking, CloudWatch metrics

## Rationale

### Why Neptune?

**Pros**:
- ✅ Fully managed (no operational burden)
- ✅ AWS native (easy integration with other AWS services)
- ✅ High performance (optimized for graph traversals)
- ✅ Multi-model (property graph + RDF)
- ✅ ACID transactions (strong consistency)
- ✅ Read replicas (horizontal scaling)
- ✅ Backup/restore (automated)

**Cons**:
- ❌ AWS vendor lock-in
- ❌ Higher cost than self-managed Neo4j
- ❌ Limited customization
- ❌ No embedded mode (cloud-only)

**Alternatives Considered**:

| Database | Pros | Cons | Verdict |
|----------|------|------|---------|
| **Neo4j** | Rich query language (Cypher), large community, self-hostable | Requires operational expertise, licensing costs for Enterprise | ❌ Rejected: Higher ops burden |
| **JanusGraph** | Open source, multi-backend, Gremlin-compatible | Complex to operate, slower than Neptune | ❌ Rejected: Operational complexity |
| **ArangoDB** | Multi-model (graph + document), open source | Smaller community, less mature graph features | ❌ Rejected: Less specialized |
| **DGraph** | GraphQL-native, open source, fast | Smaller ecosystem, less AWS integration | ❌ Rejected: Less mature |
| **Neptune** | Managed, AWS-native, Gremlin + SPARQL, ACID | AWS lock-in, cost | ✅ **Accepted**: Best for AWS deployments |

### When to Use Neptune Backend

**Use Neptune for**:
- Social graph queries (friends, followers, connections)
- Recommendation systems (item-item similarity)
- Knowledge graphs (entity relationships)
- Fraud detection (network analysis)
- Dependency resolution (package graphs, service graphs)

**Don't use Neptune for**:
- Simple key-value lookups (use Redis or DynamoDB)
- Time-series data (use ClickHouse or TimescaleDB)
- Document storage (use MongoDB or Postgres JSONB)
- Full-text search (use Elasticsearch)

## Graph Data Abstraction Layer

### Core Operations

```protobuf
syntax = "proto3";

package prism.graph.v1;

service GraphService {
  // Vertex operations
  rpc CreateVertex(CreateVertexRequest) returns (CreateVertexResponse);
  rpc GetVertex(GetVertexRequest) returns (GetVertexResponse);
  rpc UpdateVertex(UpdateVertexRequest) returns (UpdateVertexResponse);
  rpc DeleteVertex(DeleteVertexRequest) returns (DeleteVertexResponse);

  // Edge operations
  rpc CreateEdge(CreateEdgeRequest) returns (CreateEdgeResponse);
  rpc GetEdge(GetEdgeRequest) returns (GetEdgeResponse);
  rpc DeleteEdge(DeleteEdgeRequest) returns (DeleteEdgeResponse);

  // Traversal operations
  rpc Traverse(TraverseRequest) returns (TraverseResponse);
  rpc ShortestPath(ShortestPathRequest) returns (ShortestPathResponse);
  rpc PageRank(PageRankRequest) returns (PageRankResponse);

  // Bulk operations
  rpc BatchCreateVertices(BatchCreateVerticesRequest) returns (BatchCreateVerticesResponse);
  rpc BatchCreateEdges(BatchCreateEdgesRequest) returns (BatchCreateEdgesResponse);

  // Query operations
  rpc ExecuteGremlin(ExecuteGremlinRequest) returns (ExecuteGremlinResponse);
  rpc ExecuteSPARQL(ExecuteSPARQLRequest) returns (ExecuteSPARQLResponse);
}

message Vertex {
  string id = 1;
  string label = 2;  // Vertex type (e.g., "User", "Product")
  map<string, PropertyValue> properties = 3;
}

message Edge {
  string id = 1;
  string label = 2;  // Edge type (e.g., "FOLLOWS", "PURCHASED")
  string from_vertex_id = 3;
  string to_vertex_id = 4;
  map<string, PropertyValue> properties = 5;
}

message PropertyValue {
  oneof value {
    string string_value = 1;
    int64 int_value = 2;
    double double_value = 3;
    bool bool_value = 4;
    bytes bytes_value = 5;
  }
}

message TraverseRequest {
  string start_vertex_id = 1;
  repeated TraversalStep steps = 2;
  int32 max_depth = 3;
  int32 limit = 4;
}

message TraversalStep {
  enum Direction {
    OUT = 0;   // Outgoing edges
    IN = 1;    // Incoming edges
    BOTH = 2;  // Both directions
  }

  Direction direction = 1;
  repeated string edge_labels = 2;  // Filter by edge type
  map<string, PropertyValue> filters = 3;  // Property filters
}

message TraverseResponse {
  repeated Vertex vertices = 1;
  repeated Edge edges = 2;
  repeated Path paths = 3;
}

message Path {
  repeated string vertex_ids = 1;
  repeated string edge_ids = 2;
}
```

### Example: Social Graph Queries

**1. Find Friends of Friends**:

```gremlin
// Gremlin query
g.V('user:alice').out('FOLLOWS').out('FOLLOWS').dedup().limit(10)
```

```protobuf
// Prism API equivalent
TraverseRequest {
  start_vertex_id: "user:alice"
  steps: [
    TraversalStep { direction: OUT, edge_labels: ["FOLLOWS"] },
    TraversalStep { direction: OUT, edge_labels: ["FOLLOWS"] }
  ]
  max_depth: 2
  limit: 10
}
```

**2. Shortest Path**:

```gremlin
// Find shortest path from Alice to Bob
g.V('user:alice').repeat(out().simplePath()).until(hasId('user:bob')).path().limit(1)
```

```protobuf
// Prism API equivalent
ShortestPathRequest {
  start_vertex_id: "user:alice"
  end_vertex_id: "user:bob"
  max_hops: 6  // Six degrees of separation
}
```

**3. PageRank for Recommendations**:

```gremlin
// Compute PageRank to find influential users
g.V().pageRank().by('pagerank').order().by('pagerank', desc).limit(10)
```

```protobuf
// Prism API equivalent
PageRankRequest {
  vertex_label: "User"
  iterations: 20
  damping_factor: 0.85
  limit: 10
}
```

## Implementation

### Plugin Architecture

```go
// plugins/backends/neptune/plugin.go
package neptune

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/neptune"
    "github.com/apache/tinkerpop/gremlin-go/driver"
)

type NeptunePlugin struct {
    config    *NeptuneConfig
    client    *neptune.Client
    gremlin   *driver.DriverRemoteConnection
    namespace string
}

type NeptuneConfig struct {
    ClusterEndpoint string // e.g., "my-cluster.cluster-abc.us-east-1.neptune.amazonaws.com"
    Port            int    // 8182 for Gremlin, 8181 for SPARQL
    IAMAuth         bool   // Use IAM database authentication
    Region          string
}

func (p *NeptunePlugin) CreateVertex(ctx context.Context, req *CreateVertexRequest) (*CreateVertexResponse, error) {
    // Build Gremlin query
    query := fmt.Sprintf("g.addV('%s').property(id, '%s')", req.Label, req.Id)
    for key, value := range req.Properties {
        query += fmt.Sprintf(".property('%s', %v)", key, value)
    }

    // Execute via Gremlin driver
    result, err := p.gremlin.SubmitWithBindings(query, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create vertex: %w", err)
    }

    return &CreateVertexResponse{Vertex: parseVertex(result)}, nil
}

func (p *NeptunePlugin) CreateEdge(ctx context.Context, req *CreateEdgeRequest) (*CreateEdgeResponse, error) {
    // Gremlin query: g.V('from').addE('label').to(V('to'))
    query := fmt.Sprintf(
        "g.V('%s').addE('%s').to(g.V('%s')).property(id, '%s')",
        req.FromVertexId, req.Label, req.ToVertexId, req.Id,
    )

    for key, value := range req.Properties {
        query += fmt.Sprintf(".property('%s', %v)", key, value)
    }

    result, err := p.gremlin.SubmitWithBindings(query, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create edge: %w", err)
    }

    return &CreateEdgeResponse{Edge: parseEdge(result)}, nil
}

func (p *NeptunePlugin) Traverse(ctx context.Context, req *TraverseRequest) (*TraverseResponse, error) {
    // Build Gremlin traversal
    query := fmt.Sprintf("g.V('%s')", req.StartVertexId)

    for _, step := range req.Steps {
        switch step.Direction {
        case Direction_OUT:
            query += ".out()"
        case Direction_IN:
            query += ".in()"
        case Direction_BOTH:
            query += ".both()"
        }

        if len(step.EdgeLabels) > 0 {
            labels := strings.Join(step.EdgeLabels, "', '")
            query += fmt.Sprintf("('%s')", labels)
        }
    }

    query += fmt.Sprintf(".dedup().limit(%d)", req.Limit)

    result, err := p.gremlin.SubmitWithBindings(query, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to traverse: %w", err)
    }

    return parseTraversalResult(result), nil
}
```

### IAM Authentication

```go
func (p *NeptunePlugin) authenticateWithIAM(ctx context.Context) error {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.config.Region))
    if err != nil {
        return fmt.Errorf("failed to load AWS config: %w", err)
    }

    // Generate pre-signed URL for IAM auth
    credentials, err := cfg.Credentials.Retrieve(ctx)
    if err != nil {
        return fmt.Errorf("failed to retrieve credentials: %w", err)
    }

    // Connect to Neptune with IAM signature
    p.gremlin, err = driver.NewDriverRemoteConnection(
        p.config.ClusterEndpoint+":"+strconv.Itoa(p.config.Port),
        func(settings *driver.Settings) {
            settings.AuthInfo = &driver.AuthInfo{
                AccessKey: credentials.AccessKeyID,
                SecretKey: credentials.SecretAccessKey,
                SessionToken: credentials.SessionToken,
            }
        },
    )

    return err
}
```

### Bulk Import

**Neptune Bulk Loader** for large datasets:

```go
func (p *NeptunePlugin) BulkImport(ctx context.Context, s3Path string) error {
    // Use Neptune Bulk Loader API
    input := &neptune.StartLoaderJobInput{
        ClusterIdentifier: &p.config.ClusterIdentifier,
        Source:            aws.String(s3Path),  // s3://bucket/data.csv
        Format:            aws.String("csv"),   // or "gremlinJson", "ntriples", "rdfxml"
        IAMRoleArn:        aws.String(p.config.LoaderRoleARN),
        ParallelismLevel:  aws.Int32(4),  // Parallel load streams
    }

    result, err := p.client.StartLoaderJob(ctx, input)
    if err != nil {
        return fmt.Errorf("failed to start bulk load: %w", err)
    }

    // Poll for completion
    return p.waitForLoaderJob(ctx, *result.LoadId)
}
```

**CSV Format** for bulk load:
```csv
~id,~label,name:String,age:Int
user:1,User,Alice,30
user:2,User,Bob,25

~id,~label,~from,~to,since:Date
follows:1,FOLLOWS,user:1,user:2,2023-01-15
```

## Performance Considerations

### Read Replicas

```yaml
# Use read replicas for query-heavy workloads
neptune_config:
  cluster_endpoint: my-cluster.cluster-abc.us-east-1.neptune.amazonaws.com  # Writer
  reader_endpoint: my-cluster.cluster-ro-abc.us-east-1.neptune.amazonaws.com  # Readers

  # Route read-only queries to replicas
  routing:
    write_operations: [CreateVertex, CreateEdge, UpdateVertex, DeleteVertex, DeleteEdge]
    read_operations: [GetVertex, GetEdge, Traverse, ShortestPath, PageRank]
```

### Query Optimization

**1. Use indexes for frequent lookups**:
```gremlin
// Bad: Full scan
g.V().has('email', 'alice@example.com')

// Good: Use vertex ID
g.V('user:alice@example.com')
```

**2. Limit traversal depth**:
```gremlin
// Bad: Unbounded traversal
g.V('user:alice').repeat(out('FOLLOWS')).until(has('name', 'target'))

// Good: Limit depth
g.V('user:alice').repeat(out('FOLLOWS')).times(3).has('name', 'target')
```

**3. Use projection to reduce data transfer**:
```gremlin
// Bad: Return full vertex
g.V().hasLabel('User')

// Good: Project only needed fields
g.V().hasLabel('User').valueMap('name', 'email')
```

## Cost Optimization

**Neptune Pricing** (us-east-1, as of 2025):
- **Instances**: $0.348/hr for db.r5.large (2 vCPUs, 16 GB RAM)
- **Storage**: $0.10/GB-month
- **I/O**: $0.20 per 1M requests
- **Backup**: $0.021/GB-month

**Optimization Strategies**:
1. **Use read replicas** instead of scaling up writer instance
2. **Enable caching** in Prism proxy to reduce Neptune queries
3. **Batch writes** to reduce I/O charges
4. **Use bulk loader** for large imports (faster + cheaper)
5. **Right-size instances** based on workload

**Example Cost**:
- **Writer**: db.r5.large × 1 = $250/month
- **Readers**: db.r5.large × 2 = $500/month
- **Storage**: 100 GB × $0.10 = $10/month
- **I/O**: 10M requests × $0.20 = $2/month
- **Total**: ~$762/month for 3-node cluster with 100 GB data

## Monitoring

### CloudWatch Metrics

```yaml
metrics:
  - neptune_cluster_cpu_utilization        # CPU usage
  - neptune_cluster_storage_used           # Storage consumption
  - neptune_cluster_main_request_latency   # Query latency
  - neptune_cluster_engine_uptime          # Uptime
  - neptune_cluster_backup_retention_period # Backup age

alerts:
  - metric: neptune_cluster_cpu_utilization
    threshold: 80
    action: scale_up_instance

  - metric: neptune_cluster_storage_used
    threshold: 90
    action: notify_ops_team
```

### Query Profiling

```gremlin
// Enable profiling for slow queries
g.V().has('email', 'alice@example.com').profile()
```

**Example output**:
Step                                    Count  Traversers  Time (ms)
=====================================================
NeptuneGraphStep(vertex,[email.eq(alice)])  1      1           2.345
```text

## Testing Strategy

### Unit Tests

```
func TestCreateVertex(t *testing.T) {
    plugin := setupNeptunePlugin(t)

    req := &CreateVertexRequest{
        Id:    "user:test1",
        Label: "User",
        Properties: map[string]*PropertyValue{
            "name": {Value: &PropertyValue_StringValue{StringValue: "Test User"}},
        },
    }

    resp, err := plugin.CreateVertex(context.Background(), req)
    require.NoError(t, err)
    assert.Equal(t, "user:test1", resp.Vertex.Id)
}
```text

### Integration Tests

```
func TestGraphTraversal(t *testing.T) {
    plugin := setupRealNeptune(t)  // Connect to test Neptune cluster

    // Create test graph: A -> B -> C
    createVertex(plugin, "A", "User")
    createVertex(plugin, "B", "User")
    createVertex(plugin, "C", "User")
    createEdge(plugin, "A", "B", "FOLLOWS")
    createEdge(plugin, "B", "C", "FOLLOWS")

    // Traverse: A -> FOLLOWS -> FOLLOWS -> C
    req := &TraverseRequest{
        StartVertexId: "A",
        Steps: []*TraversalStep{
            {Direction: Direction_OUT, EdgeLabels: []string{"FOLLOWS"}},
            {Direction: Direction_OUT, EdgeLabels: []string{"FOLLOWS"}},
        },
        Limit: 10,
    }

    resp, err := plugin.Traverse(context.Background(), req)
    require.NoError(t, err)
    assert.Contains(t, resp.Vertices, vertexWithId("C"))
}
```text

## Migration Path

### Phase 1: Basic Operations (Week 1)
- Implement CreateVertex, GetVertex, CreateEdge
- IAM authentication
- Basic Gremlin query execution

### Phase 2: Traversals (Week 2)
- Implement Traverse, ShortestPath
- Add query optimization
- Read replica support

### Phase 3: Bulk Operations (Week 3)
- Bulk import/export
- Batch creates
- Backup/restore integration

### Phase 4: Advanced (Future)
- SPARQL support
- Graph algorithms (PageRank, community detection)
- Custom indexes

## Security Considerations

### 1. IAM Authentication
- Use IAM database authentication (no passwords in config)
- Rotate credentials automatically via AWS credentials provider

### 2. VPC Isolation
- Deploy Neptune in private subnet
- Only Prism proxy can access (no public endpoint)

### 3. Encryption
- Enable encryption at rest (KMS)
- Enable encryption in transit (TLS)

### 4. Audit Logging
- Enable Neptune audit logs to CloudWatch
- Log all mutations (create, update, delete)

## Consequences

### Positive
- ✅ Fully managed (no operational burden)
- ✅ High performance for graph queries
- ✅ ACID transactions for data integrity
- ✅ Read replicas for scalability
- ✅ AWS ecosystem integration

### Negative
- ❌ AWS vendor lock-in
- ❌ Higher cost than self-hosted solutions
- ❌ Limited to AWS regions
- ❌ Gremlin learning curve for developers

### Neutral
- 🔄 Two query languages (Gremlin + SPARQL) adds complexity but flexibility
- 🔄 Requires graph data modeling (different from relational/document stores)

## References

- [AWS Neptune Documentation](https://docs.aws.amazon.com/neptune/)
- [Apache TinkerPop (Gremlin)](https://tinkerpop.apache.org/)
- [Gremlin Query Language](https://tinkerpop.apache.org/docs/current/reference/#traversal)
- [Neptune Bulk Loader](https://docs.aws.amazon.com/neptune/latest/userguide/bulk-load.html)
- ADR-005: Backend Plugin Architecture
- ADR-025: Container Plugin Model

## Revision History

- 2025-10-09: Initial proposal for Neptune graph backend plugin

```