# ADR-006: Namespace and Multi-Tenancy

**Status**: Accepted

**Date**: 2025-10-05

**Deciders**: Core Team

**Tags**: architecture, backend, operations

## Context

Multiple applications will use Prism, each with their own data. We need to:

1. **Isolate data** between applications (security, compliance)
2. **Prevent noisy neighbors** (one app's traffic shouldn't affect others)
3. **Enable self-service** (teams create their own datasets without platform team)
4. **Simplify operations** (consistent naming, easy to find data)

Netflix's Data Gateway uses **namespaces** as the abstraction layer between logical data models and physical storage.

**Problem**: How do we achieve multi-tenancy with isolation, performance, and operational simplicity?

## Decision

Use **namespaces** as the primary isolation boundary, with **sharded deployments** for fault isolation.

**Namespace**: Logical name for a dataset (e.g., `user-profiles`, `video-events`)

**Shard**: Physical deployment serving one or more namespaces

## Rationale

### Namespace Design

```
Namespace = Logical Dataset Name

Examples:
- user-profiles         (KeyValue, user data)
- video-view-events     (TimeSeries, analytics)
- social-graph          (Graph, relationships)
- payment-transactions  (KeyValue, financial data)
```

**Properties**:
- Globally unique within Prism
- Maps to backend-specific storage (table, topic, keyspace)
- Carries configuration (backend type, capacity, policies)
- Unit of access control

### Namespace Configuration

```yaml
namespace: user-profiles

# What abstraction?
abstraction: keyvalue

# Which backend?
backend: postgres

# Capacity estimates
capacity:
  estimated_read_rps: 5000
  estimated_write_rps: 500
  estimated_data_size_gb: 100

# Policies
policies:
  retention_days: null  # Keep forever
  consistency: strong
  cache_enabled: true
  cache_ttl_seconds: 300

# Access control
access:
  owners:
    - team: user-service-team
  consumers:
    - service: user-api (read-write)
    - service: analytics-pipeline (read-only)

# Backend-specific config
backend_config:
  postgres:
    connection_string: postgres://prod-postgres-1/prism
    pool_size: 20
    table_name: user_profiles
```

### Multi-Tenancy Strategies

Netflix uses **sharded deployments** (single-tenant architecture):

```
┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
│  Prism Shard 1  │      │  Prism Shard 2  │      │  Prism Shard 3  │
│                 │      │                 │      │                 │
│  Namespaces:    │      │  Namespaces:    │      │  Namespaces:    │
│  - user-profiles│      │  - video-events │      │  - social-graph │
│  - user-sessions│      │  - play-events  │      │  - friend-graph │
│                 │      │                 │      │                 │
│    Backend:     │      │    Backend:     │      │    Backend:     │
│    Postgres 1   │      │    Kafka 1      │      │    Neptune 1    │
└─────────────────┘      └─────────────────┘      └─────────────────┘
```

**Why sharding?**
- **Fault isolation**: Shard 1 crash doesn't affect Shard 2
- **Performance isolation**: Heavy load on Shard 2 doesn't slow Shard 1
- **Blast radius**: Security breach limited to one shard
- **Capacity**: Add shards independently

**Shard Assignment**:
```rust
// Deterministic shard selection
fn select_shard(namespace: &str, shards: &[Shard]) -> &Shard {
    let hash = hash_namespace(namespace);
    &shards[hash % shards.len()]
}
```

### Namespace to Backend Mapping

```
Namespace: user-profiles
    ↓
Backend: postgres
    ↓
Physical: prism_db.user_profiles table
```

```
Namespace: video-events
    ↓
Backend: kafka
    ↓
Physical: events-video topic (20 partitions)
```

```
Namespace: social-graph
    ↓
Backend: neptune
    ↓
Physical: social-graph-prod instance
```

### Alternatives Considered

1. **Shared Database, Schema-per-Tenant**
   - Pros: Simple, fewer resources
   - Cons: Noisy neighbors, blast radius issues
   - Rejected: Doesn't scale, risky

2. **Database-per-Namespace**
   - Pros: Complete isolation
   - Cons: Operational nightmare (1000s of databases)
   - Rejected: Too many moving parts

3. **Multi-Tenant Prism with Row-Level Security**
   - Pros: Efficient resource usage
   - Cons: One bug = all data leaked
   - Rejected: Security risk too high

4. **Kubernetes Namespaces**
   - Pros: Leverages K8s multi-tenancy
   - Cons: We're not using K8s (see ADR-001)
   - Rejected: Doesn't apply

## Consequences

### Positive

- **Strong Isolation**: Each shard is independent
- **Predictable Performance**: No noisy neighbors
- **Operational Clarity**: Easy to reason about deployments
- **Security**: Blast radius limited to shard
- **Scalability**: Add shards as needed

### Negative

- **Resource Usage**: More instances than multi-tenant approach
  - *Mitigation*: Right-size instances; co-locate small namespaces
- **Complexity**: More deployments to manage
  - *Mitigation*: Automation, declarative config

### Neutral

- **Shard Rebalancing**: Moving namespaces between shards is hard
  - Use shadow traffic (ADR-009) for migrations

## Implementation Notes

### Namespace Lifecycle

1. **Creation**:
   ```bash
   # Via protobuf definition
   message UserProfile {
     option (prism.namespace) = "user-profiles";
     option (prism.backend) = "postgres";
     // ...
   }

   # Or via API
   prism-cli create-namespace \
     --name user-profiles \
     --abstraction keyvalue \
     --backend postgres \
     --capacity-estimate-rps 5000
   ```

2. **Provisioning**:
   - Capacity planner calculates requirements
   - Backend resources created (tables, topics, etc.)
   - Namespace registered in control plane
   - Monitoring and alerts configured

3. **Access Control**:
   ```rust
   // Check if service can access namespace
   if !authz.can_access(service_id, namespace, AccessLevel::ReadWrite) {
       return Err(Error::Forbidden);
   }
   ```

4. **Deletion**:
   - Mark namespace as deleted
   - Stop accepting new requests
   - Drain existing requests
   - Delete backend resources
   - Archive audit logs

### Namespace Metadata Store

```rust
pub struct NamespaceMetadata {
    pub name: String,
    pub abstraction: AbstractionType,
    pub backend: String,
    pub shard_id: String,
    pub capacity: CapacitySpec,
    pub policies: NamespacePolicies,
    pub access_control: AccessControl,
    pub backend_config: serde_json::Value,
    pub created_at: Timestamp,
    pub status: NamespaceStatus,
}

pub enum NamespaceStatus {
    Provisioning,
    Active,
    Degraded,
    Deleting,
    Deleted,
}
```

Stored in:
- **Control plane database** (Postgres)
- **In-memory cache** in each shard (fast lookups)
- **Watch for updates** (long-polling or pub/sub)

### Namespace Discovery

```rust
// Client discovers which shard serves a namespace
pub struct DiscoveryClient {
    control_plane_url: String,
}

impl DiscoveryClient {
    pub async fn resolve(&self, namespace: &str) -> Result<ShardInfo> {
        let response = self.http_client
            .get(&format!("{}/namespaces/{}", self.control_plane_url, namespace))
            .send()
            .await?;

        let metadata: NamespaceMetadata = response.json().await?;
        Ok(ShardInfo {
            endpoints: metadata.shard_endpoints(),
            backend: metadata.backend,
        })
    }
}
```

### Co-Location Strategy

Small namespaces can share a shard:

```yaml
shard: prod-shard-1
namespaces:
  - user-profiles       (5000 RPS)
  - user-preferences    (500 RPS)   # Co-located
  - user-settings       (200 RPS)   # Co-located
```

Large namespaces get dedicated shards:

```yaml
shard: prod-shard-video-events
namespaces:
  - video-events        (200,000 RPS)  # Dedicated shard
```

## References

- Netflix Data Gateway: Namespace Abstraction
- [AWS Multi-Tenancy Strategies](https://aws.amazon.com/blogs/architecture/multi-tenant-saas-architecture/)
- ADR-002: Client-Originated Configuration
- ADR-005: Backend Plugin Architecture
- ADR-007: Authentication and Authorization

## Revision History

- 2025-10-05: Initial draft and acceptance
