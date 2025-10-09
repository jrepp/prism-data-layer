---
title: "ADR-041: Graph Database Backend Support"
status: Accepted
date: 2025-10-09
deciders: Platform Team
tags: [backend, graph, database, plugin, architecture]
---

## Context

Prism requires graph database support for applications that model and query highly connected data such as:
- **Social Networks**: User relationships, friend connections, followers
- **Knowledge Graphs**: Entity relationships, semantic networks
- **Recommendation Systems**: Item-item relationships, collaborative filtering
- **Fraud Detection**: Transaction networks, entity linkage
- **Dependency Graphs**: Service dependencies, package relationships

Graph databases excel at traversing relationships and are fundamentally different from relational, document, or key-value stores.

## Decision

Add **graph database backend support** to Prism via the plugin architecture (ADR-005). Prism will provide a unified Graph Data Abstraction Layer that works across multiple graph database implementations.

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
```

## Graph Database Comparison Rubric

| Database | Model | Query Language | ACID | Managed | Cloud | Ops | Cost | Verdict |
|----------|-------|----------------|------|---------|-------|-----|------|---------|
| **AWS Neptune** | Property + RDF | Gremlin + SPARQL | ✅ Yes | ✅ Yes | AWS | ⭐⭐⭐⭐⭐ Easy | 💰💰💰 High | ✅ **AWS** |
| **Neo4j** | Property | Cypher | ✅ Yes | ⚠️ Aura (limited) | Multi | ⭐⭐⭐ Medium | 💰💰 Medium | ✅ **Self-Host** |
| **ArangoDB** | Multi-Model | AQL | ✅ Yes | ⚠️ Oasis | Multi | ⭐⭐⭐ Medium | 💰💰 Medium | ⚠️ Consider |
| **JanusGraph** | Property | Gremlin | ✅ Yes | ❌ No | - | ⭐⭐ Complex | 💰 Low | ❌ Too Complex |
| **DGraph** | Native GraphQL | GraphQL | ✅ Yes | ✅ Cloud | Multi | ⭐⭐⭐⭐ Easy | 💰💰 Medium | ⚠️ Consider |
| **TigerGraph** | Property | GSQL | ✅ Yes | ✅ Cloud | Multi | ⭐⭐⭐ Medium | 💰💰💰 High | ⚠️ Niche |

### Rubric Definitions

**Model**:
- Property: Property graph (vertices + edges with properties)
- RDF: Resource Description Framework (semantic web)
- Multi-Model: Graph + Document + Key-Value

**Managed**:
- ✅ Yes: Fully managed service available
- ⚠️ Limited: Managed but with restrictions
- ❌ No: Self-hosted only

**Cloud**:
- AWS/GCP/Azure/Multi: Cloud platform support
- Self: Self-hosted

**Ops Complexity** (1-5 stars):
- ⭐⭐⭐⭐⭐ Easy: Fully managed, minimal ops
- ⭐⭐⭐⭐ Easy: Managed with some tuning
- ⭐⭐⭐ Medium: Self-managed with tooling
- ⭐⭐ Complex: Requires graph DB expertise
- ⭐ Very Complex: Distributed system expertise

**Cost** (💰 = Low, 💰💰 = Medium, 💰💰💰 = High):
- Includes: Compute + Storage + Data Transfer + Licensing

### Detailed Comparison

#### AWS Neptune ✅ **Recommended for AWS Deployments**

**Pros**:
- ✅ Fully managed (no operational burden)
- ✅ AWS native (VPC, IAM, CloudWatch integration)
- ✅ Multi-model (Gremlin property graph + SPARQL RDF)
- ✅ ACID transactions
- ✅ Read replicas (up to 15)
- ✅ Automatic backups and point-in-time recovery

**Cons**:
- ❌ AWS vendor lock-in
- ❌ Higher cost than self-managed
- ❌ Gremlin query language (steeper learning curve than Cypher)

**When to Use**:
- Already on AWS
- Want zero ops burden
- Need multi-model (property + RDF)
- Willing to pay premium for managed service

**See**: [RFC-013: Neptune Graph Backend Implementation](../rfcs/RFC-013-neptune-graph-backend.md)

#### Neo4j ✅ **Recommended for Self-Hosted / Multi-Cloud**

**Pros**:
- ✅ Mature and widely adopted
- ✅ Cypher query language (most intuitive)
- ✅ Rich ecosystem (plugins, visualization, drivers)
- ✅ Self-hostable (Kubernetes, Docker, VMs)
- ✅ Community edition (free)
- ✅ Excellent documentation

**Cons**:
- ❌ Enterprise features require license ($$$)
- ❌ Operational complexity (clustering, backups)
- ❌ Aura managed service limited to certain clouds

**When to Use**:
- Multi-cloud or on-prem deployment
- Prefer Cypher over Gremlin
- Have Kubernetes/ops expertise
- Want rich visualization tools

**See**: Future RFC for Neo4j implementation

#### ArangoDB ⚠️ **Consider for Multi-Model Needs**

**Pros**:
- ✅ Multi-model (graph + document + key-value)
- ✅ AQL query language (SQL-like)
- ✅ Open source
- ✅ Good performance
- ✅ Managed Oasis offering

**Cons**:
- ⚠️ Smaller community than Neo4j
- ⚠️ Less mature graph features
- ⚠️ Oasis managed service newer

**When to Use**:
- Need multi-model (graph + document)
- Want SQL-like query language
- Comfortable with smaller ecosystem

#### JanusGraph ❌ **Not Recommended**

**Why Rejected**:
- Too complex to operate (requires Cassandra/HBase + Elasticsearch)
- Slower than Neptune/Neo4j
- Smaller community
- No managed offering

**Use Case**: Only if you already have Cassandra/HBase and need extreme scale.

#### DGraph ⚠️ **Consider for GraphQL-Native Apps**

**Pros**:
- ✅ GraphQL-native (no query language translation)
- ✅ Distributed by design
- ✅ Open source + Cloud offering
- ✅ Good performance

**Cons**:
- ⚠️ Smaller ecosystem
- ⚠️ Less mature than Neo4j/Neptune
- ⚠️ GraphQL-only (no Cypher/Gremlin)

**When to Use**:
- Building GraphQL API
- Want native GraphQL integration
- Comfortable with newer tech

#### TigerGraph ⚠️ **Niche Use Cases**

**Why Not Recommended**:
- Expensive
- Niche (analytics-focused)
- GSQL query language unique
- Overkill for most use cases

**Use Case**: Large-scale graph analytics (financial fraud, supply chain)

## Implementation Strategy

### Phase 1: AWS Neptune (Week 1-2)
- Implement Neptune plugin (Gremlin support)
- IAM authentication
- Basic CRUD operations
- See RFC-013 for details

### Phase 2: Neo4j (Week 3-4)
- Implement Neo4j plugin (Cypher support)
- Self-hosted deployment
- Kubernetes operator integration

### Phase 3: Multi-Plugin Support (Future)
- ArangoDB plugin (if demand exists)
- Query language abstraction layer
- Plugin selection based on requirements

## Decision Matrix

**Choose Neptune if**:
- ✅ Already on AWS
- ✅ Want fully managed
- ✅ Need RDF support
- ✅ Budget allows ($750+/month)

**Choose Neo4j if**:
- ✅ Multi-cloud or on-prem
- ✅ Want Cypher query language
- ✅ Have Kubernetes expertise
- ✅ Need community edition (free)

**Choose ArangoDB if**:
- ✅ Need multi-model (graph + document)
- ✅ Want SQL-like query language
- ✅ Comfortable with newer tech

**Choose something else if**:
- ❌ JanusGraph: Only if you already have Cassandra
- ❌ DGraph: Only if building GraphQL API
- ❌ TigerGraph: Only for large-scale analytics

## Plugin Interface

All graph database plugins must implement:

```go
type GraphBackendPlugin interface {
    // Vertex operations
    CreateVertex(ctx context.Context, req *CreateVertexRequest) (*CreateVertexResponse, error)
    GetVertex(ctx context.Context, req *GetVertexRequest) (*GetVertexResponse, error)
    UpdateVertex(ctx context.Context, req *UpdateVertexRequest) (*UpdateVertexResponse, error)
    DeleteVertex(ctx context.Context, req *DeleteVertexRequest) (*DeleteVertexResponse, error)

    // Edge operations
    CreateEdge(ctx context.Context, req *CreateEdgeRequest) (*CreateEdgeResponse, error)
    GetEdge(ctx context.Context, req *GetEdgeRequest) (*GetEdgeResponse, error)
    DeleteEdge(ctx context.Context, req *DeleteEdgeRequest) (*DeleteEdgeResponse, error)

    // Traversal operations
    Traverse(ctx context.Context, req *TraverseRequest) (*TraverseResponse, error)
    ShortestPath(ctx context.Context, req *ShortestPathRequest) (*ShortestPathResponse, error)

    // Query execution (plugin-specific language)
    ExecuteQuery(ctx context.Context, req *ExecuteQueryRequest) (*ExecuteQueryResponse, error)
}
```

## Consequences

### Positive
- ✅ Unified interface across graph databases
- ✅ Start with Neptune (managed), add Neo4j later (self-hosted)
- ✅ Flexible plugin architecture
- ✅ Clear decision rubric for users

### Negative
- ❌ Query language differences (Gremlin vs Cypher vs AQL)
- ❌ Different feature sets across plugins
- ❌ Abstraction layer may limit advanced features

### Neutral
- 🔄 Multiple plugins to maintain
- 🔄 Users must choose appropriate backend

## References

- [AWS Neptune Documentation](https://docs.aws.amazon.com/neptune/)
- [Neo4j Documentation](https://neo4j.com/docs/)
- [ArangoDB Documentation](https://www.arangodb.com/docs/)
- [Gremlin Query Language](https://tinkerpop.apache.org/gremlin.html)
- [Cypher Query Language](https://neo4j.com/docs/cypher-manual/)
- ADR-005: Backend Plugin Architecture
- ADR-025: Container Plugin Model
- RFC-013: Neptune Graph Backend Implementation

## Revision History

- 2025-10-09: Initial ADR for graph database support with comparison rubric
