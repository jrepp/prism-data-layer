---
title: "MEMO-023: Multi-Tenancy Models and Isolation Strategies"
author: Jacob Repp
created: 2025-10-13
updated: 2025-10-13
tags: [tenancy, isolation, architecture, deployment, security]
id: memo-023
---

# Multi-Tenancy Models and Isolation Strategies

## Overview

Prism is designed to support multiple tenancy models with configurable component isolation at the proxy level. This flexibility allows organizations to choose the right balance between resource efficiency, performance isolation, security boundaries, and operational complexity based on their specific requirements.

This memo explores three primary tenancy models and three isolation levels, providing guidance on when to use each approach and how to configure Prism accordingly.

## Tenancy Models

### 1. Single Tenancy (Self-Managed)

**Architecture**: One proxy deployment per tenant, fully isolated infrastructure.

```
┌─────────────────────────────────────────────────────┐
│ Tenant A (Large Enterprise Application)            │
│                                                     │
│  ┌──────────────────────────────────────────┐     │
│  │ Prism Proxy Cluster (N-way deployment)   │     │
│  │                                           │     │
│  │  ┌────────┐  ┌────────┐  ┌────────┐     │     │
│  │  │Proxy-1 │  │Proxy-2 │  │Proxy-N │     │     │
│  │  └────────┘  └────────┘  └────────┘     │     │
│  └──────────────────────────────────────────┘     │
│           │           │           │                │
│           └───────────┴───────────┘                │
│                       │                            │
│  ┌──────────────────────────────────────────┐     │
│  │ Dedicated Backend Infrastructure         │     │
│  │  • Redis cluster                         │     │
│  │  • NATS cluster                          │     │
│  │  • PostgreSQL instance                   │     │
│  │  • Kafka cluster                         │     │
│  └──────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ Tenant B (Separate Infrastructure)                 │
│  [Similar isolated deployment]                     │
└─────────────────────────────────────────────────────┘
```

**Use Cases**:
- **Large enterprise applications** with high throughput requirements (>10K RPS)
- **Regulatory compliance** requirements mandating physical infrastructure separation (HIPAA, PCI-DSS, FedRAMP)
- **Noisy neighbor elimination** for mission-critical applications
- **Independent upgrade cycles** - tenant controls when to upgrade Prism versions
- **Custom performance tuning** - dedicated resources can be sized precisely for workload

**Characteristics**:
- **Complete isolation**: No shared infrastructure between tenants
- **Dedicated resources**: CPU, memory, network, storage all tenant-specific
- **Independent lifecycle**: Each tenant deployment can be upgraded, scaled, or maintained independently
- **Maximum performance**: No resource contention with other tenants
- **Higher cost**: Full infrastructure stack per tenant

**Configuration Example** (`single-tenant-config.yaml`):
```yaml
deployment:
  model: single_tenant
  tenant_id: enterprise-customer-a

proxy:
  replicas: 5  # N-way deployment
  resources:
    cpu: "4"
    memory: "8Gi"

  # Each tenant gets dedicated proxy cluster
  cluster:
    mode: dedicated
    load_balancer: haproxy  # or nginx, envoy

backends:
  # All backends are tenant-specific
  redis:
    endpoint: "redis.tenant-a.svc.cluster.local:6379"
    isolation: physical

  nats:
    endpoint: "nats.tenant-a.svc.cluster.local:4222"
    isolation: physical

  postgres:
    endpoint: "postgres.tenant-a.svc.cluster.local:5432"
    database: "tenant_a_db"
    isolation: physical

observability:
  # Dedicated observability stack
  metrics_endpoint: "prometheus.tenant-a.svc:9090"
  traces_endpoint: "tempo.tenant-a.svc:4317"
  logs_endpoint: "loki.tenant-a.svc:3100"
```

**Deployment Patterns**:

1. **Kubernetes Namespace per Tenant**:
   ```bash
   kubectl create namespace tenant-a
   helm install prism-proxy prism/proxy \
     --namespace tenant-a \
     --values tenant-a-values.yaml
   ```

2. **Bare Metal / VM Deployment**:
   ```bash
   # Each tenant gets dedicated servers
   ansible-playbook deploy-prism.yml \
     --extra-vars "tenant_id=tenant-a servers=proxy-a-[1:5]"
   ```

3. **Cloud Provider (AWS)**:
   ```terraform
   module "prism_tenant_a" {
     source = "./modules/prism-single-tenant"
     tenant_id = "tenant-a"
     vpc_id = aws_vpc.tenant_a.id
     proxy_count = 5
     instance_type = "c6i.2xlarge"
   }
   ```

**Operational Considerations**:

- **Cost**: Highest per-tenant cost due to dedicated infrastructure
- **Maintenance**: Requires separate maintenance windows per tenant
- **Monitoring**: Dedicated observability stack per tenant increases operational overhead
- **Networking**: Requires careful network segmentation and firewall rules
- **Disaster Recovery**: Each tenant needs independent backup and recovery procedures

### 2. Multi-Tenant (Shared Proxy Pool)

**Architecture**: Control plane manages pool of proxies using `prism-bridge`, serving multiple tenants.

```
┌─────────────────────────────────────────────────────────────┐
│ Prism Control Plane (prism-bridge)                          │
│                                                              │
│  ┌──────────────────────────────────────────────────┐      │
│  │ Namespace Registry                                │      │
│  │  • tenant-a → proxy-pool-1                       │      │
│  │  • tenant-b → proxy-pool-1                       │      │
│  │  • tenant-c → proxy-pool-2 (premium tier)        │      │
│  └──────────────────────────────────────────────────┘      │
│                                                              │
│  ┌──────────────────────────────────────────────────┐      │
│  │ Load Balancer Integration                        │      │
│  │  • HAProxy config generation                     │      │
│  │  • DNS-based routing                             │      │
│  │  • Service mesh integration (Istio/Linkerd)      │      │
│  └──────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          │               │               │
┌─────────▼─────┐  ┌──────▼──────┐  ┌────▼──────────┐
│ Proxy Pool 1  │  │ Proxy Pool 2│  │ Proxy Pool N  │
│ (Standard)    │  │ (Premium)   │  │ (High-Perf)   │
│               │  │             │  │               │
│ ┌────┐ ┌────┐│  │ ┌────┐      │  │ ┌────┐        │
│ │Px-1│ │Px-2││  │ │Px-3│      │  │ │Px-N│        │
│ └────┘ └────┘│  │ └────┘      │  │ └────┘        │
└───────────────┘  └─────────────┘  └───────────────┘
        │                 │                 │
┌───────▼─────────────────▼─────────────────▼──────────┐
│ Shared Backend Infrastructure (with namespacing)     │
│  • Redis (multi-db or keyspace prefixes)             │
│  • NATS (subject hierarchies: tenant-a.*, tenant-b.*)│
│  • PostgreSQL (row-level security, schemas)          │
│  • Kafka (topic prefixes: tenant-a.*, tenant-b.*)    │
└──────────────────────────────────────────────────────┘
```

**Use Cases**:
- **SaaS platforms** serving hundreds or thousands of customers
- **Internal platform teams** providing data access as a service
- **Cost optimization** when complete isolation is not required
- **Rapid tenant onboarding** - add new tenants without provisioning infrastructure
- **Development/staging environments** where isolation requirements are lower

**Characteristics**:
- **Shared proxy infrastructure**: Multiple tenants route through same proxy pool
- **Logical isolation**: Namespacing and access control separate tenant data
- **Resource efficiency**: Higher utilization through multiplexing
- **Centralized management**: Single control plane manages all tenants
- **Noisy neighbor risk**: One tenant's traffic can impact others

**Configuration Example** (`multi-tenant-config.yaml`):
```yaml
deployment:
  model: multi_tenant
  control_plane: prism-bridge

prism_bridge:
  # Control plane for managing proxy pools
  listen_addr: "0.0.0.0:8980"

  # Namespace-to-pool mapping
  namespace_routing:
    - namespace: "tenant-a-*"
      pool: "standard"
      weight: 1

    - namespace: "tenant-b-*"
      pool: "standard"
      weight: 1

    - namespace: "premium-*"
      pool: "premium"
      weight: 1

  # Load balancer integration
  load_balancer:
    type: haproxy
    config_path: "/etc/haproxy/haproxy.cfg"
    reload_command: "systemctl reload haproxy"

  # Service discovery integration
  service_discovery:
    type: kubernetes
    label_selector: "app=prism-proxy"

proxy_pools:
  standard:
    replicas: 10
    resources:
      cpu: "2"
      memory: "4Gi"
    isolation_level: namespace  # See isolation section below

  premium:
    replicas: 5
    resources:
      cpu: "4"
      memory: "8Gi"
    isolation_level: session

backends:
  # Shared backends with logical isolation
  redis:
    endpoint: "redis-cluster.svc.cluster.local:6379"
    isolation:
      type: keyspace_prefix
      format: "tenant:{tenant_id}:{key}"

  nats:
    endpoint: "nats.svc.cluster.local:4222"
    isolation:
      type: subject_hierarchy
      format: "tenant.{tenant_id}.{subject}"

  postgres:
    endpoint: "postgres.svc.cluster.local:5432"
    isolation:
      type: row_level_security
      enable_rls: true
      tenant_column: "tenant_id"

  kafka:
    endpoint: "kafka.svc.cluster.local:9092"
    isolation:
      type: topic_prefix
      format: "{tenant_id}.{topic}"

observability:
  # Shared observability with tenant labels
  metrics_endpoint: "prometheus.svc:9090"
  traces_endpoint: "tempo.svc:4317"
  logs_endpoint: "loki.svc:3100"
  tenant_label: "prism_tenant_id"
```

**prism-bridge Architecture**:

`prism-bridge` is the control plane component for multi-tenant deployments:

1. **Namespace Registry**: Maps tenant namespaces to proxy pools
2. **Load Balancer Integration**: Generates HAProxy/nginx configs, updates DNS records
3. **Health Monitoring**: Tracks proxy health and removes unhealthy instances
4. **Dynamic Routing**: Routes tenant requests to appropriate proxy pool
5. **Orchestrator Integration**: Works with Kubernetes, Nomad, or other orchestrators

**Example prism-bridge API**:

```bash
# Register new tenant
curl -X POST http://prism-bridge:8980/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant-c",
    "namespace_pattern": "tenant-c-*",
    "pool": "standard",
    "isolation_level": "namespace"
  }'

# Get tenant routing info
curl http://prism-bridge:8980/api/v1/tenants/tenant-c

# List available proxy pools
curl http://prism-bridge:8980/api/v1/pools

# Update load balancer configuration
curl -X POST http://prism-bridge:8980/api/v1/loadbalancer/reload
```

**Orchestrator Integration**:

1. **Kubernetes (Controller Pattern)**:
   ```yaml
   apiVersion: prism.io/v1alpha1
   kind: PrismTenant
   metadata:
     name: tenant-c
   spec:
     pool: standard
     isolation_level: namespace
     namespaces:
       - tenant-c-production
       - tenant-c-staging
   ```

   prism-bridge watches for `PrismTenant` CRDs and updates routing accordingly.

2. **Nomad (Service Discovery)**:
   ```hcl
   job "prism-bridge" {
     group "control-plane" {
       task "bridge" {
         driver = "docker"
         config {
           image = "prism/bridge:latest"
         }
         service {
           name = "prism-bridge"
           port = "api"
           tags = ["control-plane"]
         }
       }
     }
   }
   ```

3. **Bare Metal (Address Lists)**:
   ```bash
   # prism-bridge maintains address list file
   prism-bridge get-addresses --pool standard > /etc/haproxy/backends-standard.lst
   ```

**Operational Considerations**:

- **Cost**: Significantly lower per-tenant cost (10-100x reduction)
- **Noisy Neighbors**: Requires careful resource limits and quality of service policies
- **Security**: Strong authentication and authorization critical (mTLS, namespace-based ACLs)
- **Monitoring**: Must track per-tenant metrics to identify noisy neighbors
- **Scalability**: Can scale to thousands of tenants on same infrastructure

### 3. Hybrid Tenancy (Tiered Service)

**Architecture**: Combination of single-tenant for premium customers and multi-tenant for standard customers.

```
┌──────────────────────────────────────────────────────┐
│ Prism Control Plane (prism-bridge)                   │
│                                                       │
│  Tenant Routing Rules:                               │
│    • enterprise-tier-* → dedicated proxy pools       │
│    • standard-tier-*   → shared proxy pools          │
└───────────────┬──────────────────┬───────────────────┘
                │                  │
        ┌───────▼────────┐   ┌─────▼──────────┐
        │ Enterprise     │   │ Standard       │
        │ Dedicated      │   │ Shared Pool    │
        │ Proxy Pool     │   │                │
        └────────────────┘   └────────────────┘
```

**Use Cases**:
- **Tiered SaaS pricing**: Enterprise customers get dedicated resources
- **Migration path**: Start multi-tenant, upgrade to single-tenant as customers grow
- **Compliance boundaries**: Some customers require dedicated infrastructure, others don't
- **Performance SLAs**: Different SLAs for different customer tiers

**Configuration Example**:
```yaml
deployment:
  model: hybrid

tenant_routing:
  rules:
    - match:
        tier: enterprise
        annual_revenue: ">100000"
      action:
        deployment: single_tenant
        pool: dedicated

    - match:
        tier: premium
      action:
        deployment: multi_tenant
        pool: premium
        isolation_level: session

    - match:
        tier: standard
      action:
        deployment: multi_tenant
        pool: standard
        isolation_level: namespace
```

## Isolation Levels

Isolation levels control how tenant workloads are separated **within** a shared proxy deployment. These can be configured independently of the tenancy model.

### 1. None (No Isolation)

**Description**: No enforced bulkhead between tenant data. All tenants share the same connection pools and backend resources.

```
┌─────────────────────────────────────┐
│ Prism Proxy                          │
│                                      │
│  ┌────────────────────────────┐    │
│  │ Shared Connection Pool     │    │
│  │  • 100 Redis connections   │    │
│  │  • 50 NATS connections     │    │
│  │  • 20 PostgreSQL conns     │    │
│  └────────────────────────────┘    │
│                                      │
│  All tenants use same connections   │
└─────────────────────────────────────┘
```

**When to Use**:
- **Development environments** where isolation is not a concern
- **Single logical application** with multiple "namespaces" that are actually just organizational units
- **Maximum performance** - connection pooling and multiplexing reduce latency
- **Trusted tenants** - all tenants are internal teams within same organization

**Configuration**:
```yaml
proxy:
  isolation_level: none

  connection_pools:
    redis:
      max_connections: 100
      shared: true

    nats:
      max_connections: 50
      shared: true
```

**Security Implications**:
- ⚠️ **No tenant isolation**: One tenant can exhaust connections for others
- ⚠️ **Cross-tenant visibility**: Application bugs could expose data across tenants
- ✅ **Use only in trusted environments**

**Performance Characteristics**:
- ✅ **Lowest latency**: No per-request connection setup
- ✅ **Highest throughput**: Maximum connection reuse
- ✅ **Lowest resource usage**: Minimal overhead

### 2. Namespace Isolation

**Description**: Each namespace has its own pool of pattern providers (backend connections). Namespaces are the primary isolation boundary.

```
┌─────────────────────────────────────────────────────┐
│ Prism Proxy                                          │
│                                                      │
│  ┌────────────────────────────────────────────┐    │
│  │ Namespace: tenant-a-production             │    │
│  │  ┌──────────────────────────────────┐     │    │
│  │  │ Pattern Provider Pool            │     │    │
│  │  │  • 20 Redis connections          │     │    │
│  │  │  • 10 NATS connections           │     │    │
│  │  └──────────────────────────────────┘     │    │
│  └────────────────────────────────────────────┘    │
│                                                      │
│  ┌────────────────────────────────────────────┐    │
│  │ Namespace: tenant-b-production             │    │
│  │  ┌──────────────────────────────────┐     │    │
│  │  │ Pattern Provider Pool            │     │    │
│  │  │  • 20 Redis connections          │     │    │
│  │  │  • 10 NATS connections           │     │    │
│  │  └──────────────────────────────────┘     │    │
│  └────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

**When to Use**:
- **Standard multi-tenant SaaS** where tenants share infrastructure but need resource guarantees
- **Internal platform teams** serving multiple product teams
- **Noisy neighbor mitigation** - one tenant's traffic spike doesn't affect others
- **Compliance requirements** needing logical separation (but not physical)

**Configuration**:
```yaml
proxy:
  isolation_level: namespace

  connection_pools:
    per_namespace: true

    redis:
      connections_per_namespace: 20
      max_total_connections: 500  # Hard limit across all namespaces

    nats:
      connections_per_namespace: 10
      max_total_connections: 250

  resource_limits:
    per_namespace:
      cpu: "1"           # 1 CPU core per namespace
      memory: "2Gi"      # 2GB RAM per namespace
      max_rps: 1000      # Max requests per second
```

**Implementation Details**:

1. **Connection Pool Isolation**:
   ```go
   type NamespaceConnectionPool struct {
       namespace   string
       redisConns  *pool.Pool  // Dedicated Redis connection pool
       natsConns   *pool.Pool  // Dedicated NATS connection pool
       kafkaConns  *pool.Pool  // Dedicated Kafka connection pool
   }

   // Proxy maintains map: namespace -> pool
   pools := map[string]*NamespaceConnectionPool{
       "tenant-a-prod": NewNamespaceConnectionPool("tenant-a-prod", config),
       "tenant-b-prod": NewNamespaceConnectionPool("tenant-b-prod", config),
   }
   ```

2. **Dynamic Pool Creation**:
   - First request from new namespace creates dedicated pool
   - Pools are lazily initialized (not created until first use)
   - Idle pools can be garbage collected after timeout

3. **Resource Accounting**:
   ```bash
   # Query per-namespace resource usage
   prismctl metrics namespace tenant-a-prod

   # Output:
   # Connections: Redis=18/20, NATS=7/10, Kafka=3/5
   # CPU: 0.7/1.0 cores
   # Memory: 1.2/2.0 GiB
   # RPS: 450/1000
   ```

**Security Implications**:
- ✅ **Resource isolation**: One tenant cannot exhaust another's connections
- ✅ **Failure isolation**: Connection pool exhaustion in one namespace doesn't affect others
- ⚠️ **Still shared process**: All namespaces run in same proxy process (memory limits apply to whole proxy)
- ✅ **Authentication required**: Namespace must be authenticated (mTLS, JWT, API key)

**Performance Characteristics**:
- ✅ **Good latency**: Connections are reused within namespace
- ✅ **Good throughput**: Each namespace has dedicated resources
- ⚠️ **Higher memory usage**: N namespaces × M connections per namespace

### 3. Session Isolation

**Description**: Shared pool of pattern providers (connections), but each session (client connection) gets dedicated connections that are **not reused** for other sessions. Connections are set up and torn down for each unique session.

```
┌─────────────────────────────────────────────────────┐
│ Prism Proxy                                          │
│                                                      │
│  Session 1 (client-a)                               │
│    ┌──────────────────────────────────┐            │
│    │ Dedicated Connections            │            │
│    │  Redis-conn-1, NATS-conn-1       │            │
│    └──────────────────────────────────┘            │
│          ↓ Torn down when session ends              │
│                                                      │
│  Session 2 (client-b)                               │
│    ┌──────────────────────────────────┐            │
│    │ Dedicated Connections            │            │
│    │  Redis-conn-2, NATS-conn-2       │            │
│    └──────────────────────────────────┘            │
│                                                      │
│  ⚠️ Connections NOT reused between sessions         │
└─────────────────────────────────────────────────────┘
```

**When to Use**:
- **Compliance requirements** mandating connection-level isolation (PCI-DSS, HIPAA)
- **Security-sensitive applications** where connection reuse is prohibited
- **Audit requirements** needing per-session connection lifecycle tracking
- **Premium tier customers** who pay for guaranteed dedicated connections
- **Short-lived sessions** where setup cost is acceptable

**Configuration**:
```yaml
proxy:
  isolation_level: session

  session_management:
    connection_reuse: false  # Key: disable connection reuse
    max_sessions: 1000       # Limit concurrent sessions
    session_timeout: 300s    # Idle session timeout

  connection_pools:
    shared: true             # Pool exists but connections are not reused

    redis:
      max_connections: 500   # Total pool size
      per_session_limit: 5   # Max connections per session

    nats:
      max_connections: 250
      per_session_limit: 3

  audit:
    log_connection_lifecycle: true
    log_session_lifecycle: true
```

**Implementation Details**:

1. **Session Identification**:
   ```go
   type Session struct {
       ID          string    // UUID
       ClientID    string    // Authenticated client identity
       Namespace   string    // Tenant namespace
       CreatedAt   time.Time
       LastActivity time.Time
       Connections  []*Connection  // Dedicated connections
   }
   ```

2. **Connection Lifecycle**:
   ```
   Client connects → Session created → Connections established
                                     ↓
   Client sends request → Use session's dedicated connections
                                     ↓
   Client disconnects → Session destroyed → Connections closed
   ```

3. **Connection Setup Cost**:
   - **Redis**: ~1-2ms per connection setup (TCP + AUTH)
   - **NATS**: ~2-5ms per connection setup (TCP + TLS + AUTH)
   - **PostgreSQL**: ~5-10ms per connection setup (TCP + TLS + AUTH + query cache warm-up)
   - **Kafka**: ~10-50ms per connection setup (TCP + SASL + metadata fetch)

4. **Audit Trail**:
   ```json
   {
     "event": "session_created",
     "session_id": "550e8400-e29b-41d4-a716-446655440000",
     "client_id": "user@example.com",
     "namespace": "tenant-a-prod",
     "timestamp": "2025-10-13T22:45:00Z"
   }
   {
     "event": "connection_established",
     "session_id": "550e8400-e29b-41d4-a716-446655440000",
     "backend": "redis",
     "connection_id": "redis-conn-12345",
     "timestamp": "2025-10-13T22:45:00.123Z"
   }
   {
     "event": "session_destroyed",
     "session_id": "550e8400-e29b-41d4-a716-446655440000",
     "duration_ms": 45000,
     "requests_handled": 120,
     "timestamp": "2025-10-13T22:45:45Z"
   }
   ```

**Security Implications**:
- ✅ **Maximum isolation**: No connection state shared between sessions
- ✅ **Audit compliance**: Full connection lifecycle tracking
- ✅ **Credential isolation**: Each session can use different backend credentials
- ⚠️ **DoS risk**: Malicious client can exhaust connection pool by creating many sessions

**Performance Characteristics**:
- ⚠️ **Higher latency**: Connection setup cost on every session (5-50ms depending on backend)
- ⚠️ **Lower throughput**: Cannot reuse connections, must establish fresh connections
- ⚠️ **Higher resource usage**: More total connections needed (no reuse)
- ✅ **Predictable latency**: No connection pool contention

## Comparison Matrix

| Aspect | Single Tenancy | Multi-Tenant | Isolation: None | Isolation: Namespace | Isolation: Session |
|--------|---------------|--------------|-----------------|---------------------|-------------------|
| **Resource Efficiency** | Low (dedicated infra) | High (shared) | Highest | Medium | Lower |
| **Noisy Neighbor Risk** | None | Medium-High | High | Low | Very Low |
| **Cost per Tenant** | $1000-10000/month | $1-100/month | Lowest | Medium | Higher |
| **Compliance** | Easiest (physical isolation) | Harder (logical isolation) | Not suitable | GDPR, SOC2 | HIPAA, PCI-DSS |
| **Scalability** | 10-100 tenants | 1000-100000 tenants | Unlimited (within proxy capacity) | 100-1000 namespaces | 100-1000 concurrent sessions |
| **Performance** | Highest (no contention) | Good (with limits) | Highest (connection reuse) | Good | Lower (setup cost) |
| **Setup Latency** | N/A (pre-provisioned) | Instant (shared pool) | ~1ms | ~1ms | 5-50ms |
| **Operational Complexity** | High (many deployments) | Medium (single control plane) | Lowest | Medium (pool management) | Higher (session tracking) |
| **Blast Radius** | One tenant only | All tenants in pool | All tenants | One namespace | One session |

## Decision Framework

### Choose **Single Tenancy** if:
- [ ] Regulatory compliance requires physical isolation (HIPAA, FedRAMP, PCI-DSS Level 1)
- [ ] Customer pays >$10K/month and demands dedicated resources
- [ ] Throughput requirements exceed 10K RPS per tenant
- [ ] Independent upgrade cycles are required
- [ ] Maximum performance is critical (trading cost for speed)

### Choose **Multi-Tenant** if:
- [ ] Serving 100+ customers on SaaS platform
- [ ] Cost optimization is priority (shared infrastructure is 10-100x cheaper)
- [ ] Rapid tenant onboarding is needed (minutes, not days)
- [ ] Centralized management is preferred
- [ ] Noisy neighbor risk is acceptable with proper limits

### Choose **Hybrid** if:
- [ ] You have both enterprise and standard tier customers
- [ ] Some customers need dedicated resources, others don't
- [ ] You want migration path from multi-tenant to single-tenant
- [ ] Different SLAs for different customer tiers

### Choose **Isolation: None** if:
- [ ] Development/staging environment only
- [ ] All "tenants" are internal teams (trusted)
- [ ] Absolute maximum performance is required
- [ ] No compliance requirements

### Choose **Isolation: Namespace** if:
- [ ] Production multi-tenant SaaS
- [ ] Noisy neighbor mitigation is required
- [ ] Compliance requires logical separation (GDPR, SOC2)
- [ ] Resource guarantees needed per tenant

### Choose **Isolation: Session** if:
- [ ] Compliance mandates connection-level isolation (PCI-DSS, HIPAA)
- [ ] Audit requirements need per-session connection tracking
- [ ] Premium customers pay for guaranteed dedicated connections
- [ ] Sessions are short-lived (<5 minutes)
- [ ] Connection setup cost (5-50ms) is acceptable

## Configuration Examples

### Example 1: SaaS Startup (Multi-Tenant + Namespace Isolation)

```yaml
deployment:
  model: multi_tenant
  control_plane: prism-bridge

proxy:
  replicas: 5
  isolation_level: namespace

  connection_pools:
    per_namespace: true
    redis:
      connections_per_namespace: 10
      max_total_connections: 500

  resource_limits:
    per_namespace:
      cpu: "0.5"
      memory: "1Gi"
      max_rps: 500

backends:
  redis:
    endpoint: "redis-cluster.svc:6379"
    isolation:
      type: keyspace_prefix
      format: "tenant:{tenant_id}:{key}"

observability:
  tenant_label: "prism_tenant_id"
```

### Example 2: Enterprise Healthcare (Single Tenant + Session Isolation)

```yaml
deployment:
  model: single_tenant
  tenant_id: healthcare-customer-a

proxy:
  replicas: 10
  isolation_level: session

  session_management:
    connection_reuse: false
    max_sessions: 5000
    session_timeout: 300s

  audit:
    log_connection_lifecycle: true
    log_session_lifecycle: true
    audit_backend: s3://audit-logs/healthcare-a/

backends:
  postgres:
    endpoint: "postgres.healthcare-a.internal:5432"
    ssl_mode: require
    client_cert_auth: true

compliance:
  frameworks: ["HIPAA", "SOC2"]
  pii_encryption: true
  audit_retention_days: 2555  # 7 years
```

### Example 3: Hybrid Platform (Mix of Enterprise and Standard)

```yaml
deployment:
  model: hybrid

tenant_routing:
  rules:
    - match:
        tier: enterprise
        contracts: ["healthcare-a", "finance-b"]
      action:
        deployment: single_tenant
        isolation_level: session

    - match:
        tier: premium
        annual_revenue: ">10000"
      action:
        deployment: multi_tenant
        pool: premium
        isolation_level: namespace
        connection_pools:
          redis:
            connections_per_namespace: 20

    - match:
        tier: standard
      action:
        deployment: multi_tenant
        pool: standard
        isolation_level: namespace
        connection_pools:
          redis:
            connections_per_namespace: 5
```

## Implementation Roadmap

### Phase 1: Multi-Tenant with Namespace Isolation (POC)
**Timeline**: 4-6 weeks

- [ ] Implement namespace-aware connection pooling
- [ ] Add per-namespace resource accounting
- [ ] Implement prism-bridge basic routing
- [ ] Add namespace-based authentication (mTLS, JWT)
- [ ] Implement Redis keyspace prefix isolation
- [ ] Implement NATS subject hierarchy isolation
- [ ] Add per-namespace metrics and logging

### Phase 2: Session Isolation and Audit
**Timeline**: 3-4 weeks

- [ ] Implement session lifecycle management
- [ ] Add connection-level audit logging
- [ ] Implement connection setup/teardown per session
- [ ] Add session timeout and cleanup
- [ ] Implement audit trail export (S3, CloudWatch Logs)

### Phase 3: Single-Tenant Deployment Automation
**Timeline**: 4-6 weeks

- [ ] Create Terraform modules for single-tenant deployment
- [ ] Add Kubernetes Helm charts with namespace-per-tenant pattern
- [ ] Implement tenant-specific observability stacks
- [ ] Add automated backup/restore per tenant
- [ ] Create cost tracking per tenant

### Phase 4: prism-bridge Advanced Features
**Timeline**: 6-8 weeks

- [ ] Implement load balancer integration (HAProxy, nginx, Envoy)
- [ ] Add service mesh integration (Istio, Linkerd)
- [ ] Implement dynamic pool scaling based on load
- [ ] Add tenant migration tools (move tenant between pools)
- [ ] Implement advanced routing (geo, latency-based, weighted)

### Phase 5: Compliance and Security Hardening
**Timeline**: 4-6 weeks

- [ ] HIPAA compliance validation and documentation
- [ ] PCI-DSS compliance validation
- [ ] SOC2 audit preparation
- [ ] Implement field-level encryption for PII
- [ ] Add credential rotation automation
- [ ] Implement anomaly detection for tenant behavior

## Security Considerations

### Authentication and Authorization

1. **Namespace Authentication**:
   - Every request must include authenticated namespace identity
   - mTLS (client certificates) or JWT tokens
   - API keys for less sensitive environments

2. **Backend Authorization**:
   - Proxy enforces namespace-to-backend access control
   - Row-level security (PostgreSQL)
   - Keyspace prefixes (Redis)
   - Subject hierarchies (NATS)
   - Topic ACLs (Kafka)

### Data Isolation

1. **At-Rest Encryption**:
   - All backend data encrypted (backend responsibility)
   - Separate encryption keys per tenant (single-tenant)
   - Key rotation automation

2. **In-Transit Encryption**:
   - TLS for all client → proxy connections
   - TLS for all proxy → backend connections
   - Certificate validation and pinning

3. **Cross-Tenant Protection**:
   - Mandatory namespace prefix validation
   - SQL injection protection (parameterized queries)
   - Subject/topic ACL enforcement

### Audit and Compliance

1. **Audit Logging**:
   ```json
   {
     "timestamp": "2025-10-13T22:45:00Z",
     "tenant_id": "healthcare-a",
     "namespace": "healthcare-a-prod",
     "session_id": "550e8400-e29b-41d4-a716-446655440000",
     "user_id": "doctor@hospital.com",
     "action": "query",
     "resource": "patient_records",
     "result": "success",
     "rows_accessed": 5,
     "pii_accessed": true
   }
   ```

2. **Compliance Reports**:
   - HIPAA audit trail (7-year retention)
   - GDPR data access logs (right to know)
   - SOC2 access control reports
   - PCI-DSS data flow diagrams

## Operational Considerations

### Monitoring

Per-tenant metrics to track:
```promql
# Request rate per tenant
rate(prism_requests_total{tenant_id="tenant-a"}[5m])

# Error rate per tenant
rate(prism_errors_total{tenant_id="tenant-a"}[5m]) / rate(prism_requests_total{tenant_id="tenant-a"}[5m])

# Latency percentiles per tenant
histogram_quantile(0.99, prism_request_duration_seconds{tenant_id="tenant-a"})

# Connection pool usage per tenant
prism_connection_pool_active{tenant_id="tenant-a", backend="redis"}
prism_connection_pool_max{tenant_id="tenant-a", backend="redis"}

# Resource usage per namespace
prism_namespace_cpu_usage{namespace="tenant-a-prod"}
prism_namespace_memory_usage{namespace="tenant-a-prod"}
```

### Alerting

Critical alerts:
```yaml
alerts:
  - name: TenantConnectionPoolExhausted
    condition: prism_connection_pool_active / prism_connection_pool_max > 0.9
    severity: warning
    message: "Tenant {{$labels.tenant_id}} using >90% of connection pool"

  - name: TenantHighErrorRate
    condition: rate(prism_errors_total[5m]) / rate(prism_requests_total[5m]) > 0.05
    severity: critical
    message: "Tenant {{$labels.tenant_id}} error rate >5%"

  - name: TenantHighLatency
    condition: histogram_quantile(0.99, prism_request_duration_seconds) > 1.0
    severity: warning
    message: "Tenant {{$labels.tenant_id}} p99 latency >1s"
```

### Capacity Planning

Resource scaling guidelines:

| Tenant Count | Proxy Replicas | Total Connections | Memory per Proxy | CPU per Proxy |
|--------------|----------------|-------------------|------------------|---------------|
| 10 | 2 | 200 | 4GB | 2 cores |
| 50 | 5 | 500 | 4GB | 2 cores |
| 100 | 10 | 1000 | 4GB | 2 cores |
| 500 | 20 | 2000 | 8GB | 4 cores |
| 1000 | 40 | 4000 | 8GB | 4 cores |

## Conclusion

Prism's flexible tenancy and isolation model allows organizations to choose the right balance between cost, performance, security, and operational complexity. Key takeaways:

1. **Single-tenant** for enterprise customers with regulatory requirements or >10K RPS
2. **Multi-tenant** for SaaS platforms serving hundreds/thousands of customers
3. **Namespace isolation** for production multi-tenant deployments (standard choice)
4. **Session isolation** only when compliance mandates connection-level isolation
5. **No isolation** only for development/trusted environments

The recommended starting point for most SaaS platforms is **multi-tenant with namespace isolation**, as it provides good balance of cost efficiency, noisy neighbor protection, and operational simplicity. Upgrade to single-tenant or session isolation only when specific requirements dictate.

## References

- ADR-042: Multi-Tenancy Architecture
- ADR-043: Namespace-Based Access Control
- RFC-025: prism-bridge Control Plane Design
- RFC-026: Session Lifecycle Management
- RFC-030: Schema Evolution and Pub/Sub Validation (Consumer Metadata)
- MEMO-009: Topaz Local Authorizer Configuration (Authorization)
- MEMO-016: Observability Lifecycle Implementation (Per-Tenant Metrics)
