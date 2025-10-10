---
title: "ADR-034: Product/Feature Sharding Strategy"
status: Proposed
date: 2025-10-08
deciders: [System]
tags: ['architecture', 'deployment', 'reliability', 'operations', 'performance']
---

## Context

As Prism scales to support multiple products and features, we need a strategy for isolating workloads to prevent:

1. **Noisy Neighbor Problems**: High-traffic feature affecting low-latency feature
2. **Blast Radius**: Incidents in one product affecting others
3. **Resource Contention**: Shared connection pools, memory, CPU causing unpredictable performance
4. **Deployment Risk**: Changes to support one feature breaking another

Netflix's Data Gateway experience shows that **shared infrastructure leads to operational complexity at scale**:
- One team's traffic spike affects unrelated services
- Debugging performance issues requires analyzing all tenants
- Rolling out backend changes requires coordinating with all affected teams
- Capacity planning becomes combinatorially complex

From Netflix's scale metrics:
- **8 million QPS** across key-value abstraction
- **3,500+ use cases** sharing infrastructure
- **10 million writes/sec** for time-series data

At this scale, **sharding becomes essential** for operational sanity and performance isolation.

## Decision

**Implement multi-level sharding strategy** based on product/feature boundaries:

### 1. **Namespace-Level Isolation** (Already Exists)
Each namespace gets isolated:
- Backend connections
- Authentication context
- Rate limits
- Metrics

### 2. **Proxy Instance Sharding** (New)
Deploy separate Prism proxy instances for:
- **Product Sharding**: Different products (recommendation, playback, search)
- **Feature Sharding**: Different features within a product (experimental vs stable)
- **SLA Tiers**: Different latency/availability requirements

### 3. **Backend Cluster Sharding** (New)
Dedicated backend clusters per shard:
- Prevents cross-product resource contention
- Enables independent scaling
- Allows backend version divergence

### Sharding Taxonomy

┌─────────────────────────────────────────────────────────────┐
│                    Organization                              │
│  ┌──────────────────────┐  ┌───────────────────────────┐   │
│  │  Product: Playback   │  │  Product: Recommendation  │   │
│  │  ┌────────────────┐  │  │  ┌─────────────────────┐  │   │
│  │  │ Feature: Live  │  │  │  │  Feature: Trending  │  │   │
│  │  │ SLA: P99<10ms  │  │  │  │  SLA: P99<50ms      │  │   │
│  │  │                │  │  │  │                     │  │   │
│  │  │ Prism Instance │  │  │  │  Prism Instance     │  │   │
│  │  │   prism-play   │  │  │  │    prism-rec        │  │   │
│  │  │       ↓        │  │  │  │        ↓            │  │   │
│  │  │  Redis Cluster │  │  │  │  Postgres Cluster   │  │   │
│  │  │   redis-live   │  │  │  │   pg-trending       │  │   │
│  │  └────────────────┘  │  │  └─────────────────────┘  │   │
│  └──────────────────────┘  └───────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```text

## Rationale

### Why Shard by Product/Feature?

**Netflix's Lessons**:
- **Fault Isolation**: Cassandra cluster failure affects only one product, not all
- **Performance Predictability**: Each product has dedicated resources, no surprise degradation
- **Independent Evolution**: Upgrade Kafka version for analytics without affecting playback
- **Clear Ownership**: Each product team owns their Prism shard and backend

**Specific Examples from Netflix**:
- Search traffic reduced by 75% with client-side compression → didn't affect other products
- Maestro workflow engine "100x faster" redesign → isolated deployment, no cross-product risk
- Time-series data (10M writes/sec) → separate clusters from key-value (8M QPS)

### Sharding Dimensions

| Dimension | Rationale | Example |
|-----------|-----------|---------|
| **Product** | Different products have different scale/SLAs | Playback (low latency) vs Analytics (high throughput) |
| **Feature** | Experimental features shouldn't affect stable | Canary testing new cache strategy |
| **SLA Tier** | Different availability/latency requirements | P99 &lt;10ms vs P99 &lt;100ms |
| **Region** | Regulatory/latency requirements | US-West vs EU (GDPR) |
| **Environment** | Dev/staging/prod isolation | Prevents test traffic affecting prod |

### When to Shard

**Shard proactively when**:
- Traffic exceeds 10K RPS for a single namespace
- P99 latency SLA is &lt;50ms (needs dedicated resources)
- Product has distinct backend requirements (different databases)
- Regulatory isolation required (GDPR, HIPAA)

**Delay sharding when**:
- Total traffic &lt;1K RPS across all namespaces
- Products have similar SLAs and resource profiles
- Operational overhead of managing multiple instances outweighs benefits

## Alternatives Considered

### 1. Single Shared Prism Instance for All

- **Pros**: Simple, minimal operational overhead, efficient resource utilization
- **Cons**: Noisy neighbor, blast radius, complex capacity planning
- **Rejected because**: Doesn't scale operationally beyond ~10 products

### 2. One Prism Instance Per Namespace

- **Pros**: Maximum isolation
- **Cons**: Massive operational overhead (1000s of instances), resource waste
- **Rejected because**: Operationally infeasible at scale

### 3. Dynamic Auto-Sharding (Like Database Sharding)

- **Pros**: Automatic, adapts to load
- **Cons**: Complex routing, hard to debug, unclear ownership
- **Rejected because**: Too complex for initial version, unclear operational model

## Consequences

### Positive

- **Fault Isolation**: Product A's outage doesn't affect Product B
- **Performance Predictability**: Dedicated resources mean stable latency
- **Independent Deployment**: Upgrade Prism for one product without risk to others
- **Clear Ownership**: Each product team owns their shard
- **Simplified Capacity Planning**: Plan per-product instead of combinatorially
- **Regulatory Compliance**: Easy to isolate GDPR/HIPAA data

### Negative

- **Operational Overhead**: More instances to deploy, monitor, maintain
- **Resource Efficiency**: May underutilize resources if shards are too granular
- **Cross-Product Features**: Harder to implement features that span products
- **Configuration Management**: Need tooling to manage multiple instances

### Neutral

- **Sharding Decisions**: Need clear criteria for when to create new shard
- **Routing Layer**: May need service mesh or load balancer to route to shards
- **Cost**: More instances = higher cost, but may be offset by better resource utilization per shard

## Implementation Notes

### Deployment Topology

**Shared Namespace Proxy (Small Scale)**:
```yaml
# Single Prism instance, multiple namespaces
services:
  prism-shared:
    image: prism/proxy:latest
    replicas: 3
    namespaces:
      - user-profiles
      - session-cache
      - recommendations
```text

**Product-Sharded Deployment (Medium Scale)**:
```yaml
# Separate Prism instances per product
services:
  prism-playback:
    image: prism/proxy:latest
    replicas: 5
    namespaces:
      - playback-events
      - playback-state

  prism-search:
    image: prism/proxy:latest
    replicas: 3
    namespaces:
      - search-index
      - search-cache
```text

**Feature + SLA Sharded (Large Scale)**:
```yaml
# Sharded by product, feature, and SLA tier
services:
  prism-playback-live:  # Low latency tier
    image: prism/proxy:latest
    replicas: 10
    sla: p99_10ms
    backends:
      - redis-live-cluster

  prism-playback-vod:  # Standard latency tier
    image: prism/proxy:latest
    replicas: 5
    sla: p99_50ms
    backends:
      - redis-vod-cluster
```text

### Routing to Shards

**Service Mesh Approach** (Recommended):
```yaml
# Istio VirtualService routes clients to correct shard
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: prism-routing
spec:
  hosts:
    - prism.example.com
  http:
    - match:
        - headers:
            x-product:
              exact: playback
      route:
        - destination:
            host: prism-playback
    - match:
        - headers:
            x-product:
              exact: search
      route:
        - destination:
            host: prism-search
```text

**Client-Side Routing**:
```rust
// Client selects shard based on product
let prism_endpoint = match product {
    Product::Playback => "prism-playback.example.com:50051",
    Product::Search => "prism-search.example.com:50051",
    Product::Recommendations => "prism-rec.example.com:50051",
};

let client = PrismClient::connect(prism_endpoint).await?;
```text

### Configuration Management

Use Kubernetes ConfigMaps or CRDs to define shards:

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismShard
metadata:
  name: playback-live
spec:
  product: playback
  feature: live
  slaTier: p99_10ms
  replicas: 10
  backends:
    - name: redis-live
      type: redis
      cluster: redis-live-01
  namespaces:
    - playback-events
    - playback-state
  resources:
    requests:
      cpu: "4"
      memory: "8Gi"
```text

(See ADR-037 for Kubernetes Operator details)

### Migration Path

**Phase 1**: Single shared instance (current state)
**Phase 2**: Shard by product (2-3 products initially)
**Phase 3**: Shard by product + SLA tier (as traffic grows)
**Phase 4**: Full product/feature/region sharding (Netflix scale)

## References

- [Netflix Data Gateway Scale](/prism-data-layer/netflix/scale) - 8M QPS, 3500+ use cases
- [Netflix Multi-Region Deployment](/prism-data-layer/netflix/key-use-cases)
- ADR-033: Capability API (shard discovery)
- ADR-037: Kubernetes Operator (shard management automation)
- RFC-008: Proxy Plugin Architecture (backend isolation per shard)

## Revision History

- 2025-10-08: Initial draft based on Netflix's sharding experience
