---
title: "RFC-012: Prism Network Gateway (prism-netgw) - Multi-Region Control Plane"
status: Draft
author: Platform Team
created: 2025-10-09
updated: 2025-10-09
tags: [control-plane, multi-region, networking, orchestration, high-availability]
---

# RFC-012: Prism Network Gateway (prism-netgw) - Multi-Region Control Plane

## Abstract

This RFC proposes **prism-netgw**, a distributed control plane for managing collections of Prism data gateway clusters across multiple cloud providers, regions, and on-premises environments. prism-netgw handles cluster registration, configuration synchronization, health monitoring, and cross-region routing while tolerating high latency and network partitions.

## Motivation

### Problem Statement

Organizations deploying Prism at scale face several challenges:

1. **Multi-Region Deployments**: Prism gateways deployed across AWS, GCP, Azure, and on-prem
2. **Configuration Management**: Keeping namespace configs, backend definitions, and policies synchronized
3. **Cross-Region Discovery**: Applications need to discover nearest Prism gateway
4. **Health Monitoring**: Centralized visibility into all Prism instances
5. **High Latency Tolerance**: Cross-region communication experiences 100-500ms latency
6. **Network Partitions**: Cloud VPCs, on-prem networks may have intermittent connectivity

### Goals

- **Cluster Management**: Register, configure, and monitor Prism gateway clusters
- **Configuration Sync**: Distribute namespace and backend configs across regions
- **Service Discovery**: Enable clients to discover nearest healthy Prism gateway
- **Health Aggregation**: Collect health and metrics from all clusters
- **Latency Tolerance**: Operate correctly with 100-500ms cross-region latency
- **Partition Tolerance**: Handle network partitions gracefully
- **Multi-Cloud**: Support AWS, GCP, Azure, on-prem deployments

### Non-Goals

- **Not a data plane**: prism-netgw does NOT proxy data requests (Prism gateways handle that)
- **Not a service mesh**: Use dedicated service mesh (Istio, Linkerd) for data plane networking
- **Not a config database**: Uses etcd/Consul for distributed storage

## Architecture

### High-Level Design

┌─────────────────────────────────────────────────────────────────┐
│                      prism-netgw Control Plane                  │
│                    (Raft consensus, multi-region)               │
└─────────────────────────────────────────────────────────────────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
        ┌───────▼──────┐  ┌────▼─────┐  ┌──────▼──────┐
        │ AWS Region   │  │ GCP Zone │  │ On-Prem DC  │
        │ us-east-1    │  │ us-cent1 │  │ Seattle     │
        └──────────────┘  └──────────┘  └─────────────┘
                │               │               │
        ┌───────▼──────┐  ┌────▼─────┐  ┌──────▼──────┐
        │ Prism Cluster│  │  Prism   │  │   Prism     │
        │  (3 nodes)   │  │ Cluster  │  │  Cluster    │
        └──────────────┘  └──────────┘  └─────────────┘
                │               │               │
        ┌───────▼──────┐  ┌────▼─────┐  ┌──────▼──────┐
        │ Backends     │  │ Backends │  │  Backends   │
        │ (Postgres,   │  │ (Kafka,  │  │ (SQLite,    │
        │  Redis)      │  │  NATS)   │  │  Postgres)  │
        └──────────────┘  └──────────┘  └─────────────┘
```text

### Components

```mermaid
graph TB
    subgraph "prism-netgw Control Plane"
        API[Control Plane API<br/>:9980]
        Raft[Raft Consensus<br/>Multi-region]
        Store[Distributed Store<br/>etcd/Consul]
        Monitor[Health Monitor<br/>Polling]
        Sync[Config Sync<br/>Push/Pull]
        Discovery[Service Discovery<br/>DNS/gRPC]
    end

    subgraph "Prism Gateway Cluster (us-east-1)"
        Agent1[prism-agent<br/>:9981]
        Prism1[Prism Gateway 1]
        Prism2[Prism Gateway 2]
        Prism3[Prism Gateway 3]
    end

    subgraph "Prism Gateway Cluster (eu-west-1)"
        Agent2[prism-agent<br/>:9981]
        Prism4[Prism Gateway 4]
        Prism5[Prism Gateway 5]
    end

    API --> Raft
    Raft --> Store
    Monitor --> Agent1
    Monitor --> Agent2
    Sync --> Agent1
    Sync --> Agent2
    Agent1 --> Prism1
    Agent1 --> Prism2
    Agent1 --> Prism3
    Agent2 --> Prism4
    Agent2 --> Prism5
    Discovery -.->|Returns nearest| Prism1
    Discovery -.->|Returns nearest| Prism4
```text

### Deployment Model

┌─────────────────────────────────────────────────────────────────┐
│                    Global Control Plane                         │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    │
│  │ netgw-leader │───▶│  netgw-node2 │◀──▶│  netgw-node3 │    │
│  │  (us-east-1) │    │  (eu-west-1) │    │ (ap-south-1) │    │
│  └──────────────┘    └──────────────┘    └──────────────┘    │
│         │                    │                    │            │
│         │ Raft consensus     │                    │            │
│         └────────────────────┴────────────────────┘            │
└─────────────────────────────────────────────────────────────────┘
```

**Deployment Options**:

1. **Multi-Region Active-Standby**: 1 leader, N followers in different regions
2. **Multi-Region Active-Active**: Raft quorum across regions (requires low latency between control plane nodes)
3. **Federated**: Independent control planes per region, manual config sync

## Core Concepts

### 1. Cluster Registration

Prism gateway clusters register with prism-netgw:

```protobuf
syntax = "proto3";

package prism.netgw.v1;

message RegisterClusterRequest {
  string cluster_id = 1;         // Unique cluster identifier (e.g., "aws-us-east-1-prod")
  string region = 2;              // Cloud region (e.g., "us-east-1")
  string cloud_provider = 3;      // "aws", "gcp", "azure", "on-prem"
  string vpc_id = 4;              // VPC or network identifier
  repeated string endpoints = 5;  // gRPC endpoints for Prism gateways
  map<string, string> labels = 6; // Arbitrary labels (e.g., "env": "prod")
}

message RegisterClusterResponse {
  string cluster_id = 1;
  int64 registration_version = 2;  // Version for optimistic concurrency
  google.protobuf.Timestamp expires_at = 3;  // TTL for heartbeat
}

service ControlPlaneService {
  rpc RegisterCluster(RegisterClusterRequest) returns (RegisterClusterResponse);
  rpc UnregisterCluster(UnregisterClusterRequest) returns (UnregisterClusterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
}
```

### 2. Configuration Synchronization

**Problem**: Namespace and backend configs must be consistent across all clusters.

**Solution**: Version-controlled config distribution with eventual consistency.

```protobuf
message SyncConfigRequest {
  string cluster_id = 1;
  int64 current_version = 2;  // Cluster's current config version
}

message SyncConfigResponse {
  int64 latest_version = 1;
  repeated NamespaceConfig namespaces = 2;
  repeated BackendConfig backends = 3;
  repeated Policy policies = 4;

  // Incremental updates if current_version is recent
  bool is_incremental = 10;
  repeated ConfigChange changes = 11;  // Only deltas since current_version
}

message ConfigChange {
  enum ChangeType {
    ADDED = 0;
    MODIFIED = 1;
    DELETED = 2;
  }

  ChangeType type = 1;
  string resource_type = 2;  // "namespace", "backend", "policy"
  string resource_id = 3;
  bytes resource_data = 4;   // Protobuf-encoded resource
}
```

**Push Model** (preferred):
prism-netgw   →  Watch(config_version)  →  prism-agent
              ←  ConfigUpdate stream     ←
```text

**Pull Model** (fallback for high latency):
prism-agent   →  SyncConfig(current_version)  →  prism-netgw
              ←  SyncConfigResponse            ←
```

### 3. Health Monitoring

**Hierarchical Health Model**:

Global Health (prism-netgw)
├── Cluster Health (per region)
│   ├── Gateway Health (per Prism instance)
│   │   ├── Process Health (alive, responsive)
│   │   ├── Backend Health (Postgres, Kafka, etc.)
│   │   └── Namespace Health (operational state)
│   └── Network Health (connectivity, latency)
└── Control Plane Health (netgw nodes)
```text

```protobuf
message ReportHealthRequest {
  string cluster_id = 1;
  google.protobuf.Timestamp timestamp = 2;

  repeated GatewayHealth gateways = 3;
  repeated BackendHealth backends = 4;
  repeated NamespaceHealth namespaces = 5;

  NetworkMetrics network = 6;  // Latency, packet loss, etc.
}

message GatewayHealth {
  string gateway_id = 1;
  HealthStatus status = 2;  // HEALTHY, DEGRADED, UNHEALTHY
  int64 active_sessions = 3;
  int64 requests_per_second = 4;
  double cpu_percent = 5;
  double memory_mb = 6;
}

message BackendHealth {
  string backend_type = 1;  // "postgres", "kafka", etc.
  HealthStatus status = 2;
  double latency_ms = 3;
  string error_message = 4;
}
```text

### 4. Service Discovery

**Goal**: Clients discover nearest healthy Prism gateway.

```protobuf
message DiscoverGatewaysRequest {
  string namespace = 1;        // Filter by namespace support
  string client_location = 2;   // "us-east-1", "eu-west-1", etc.
  int32 max_results = 3;        // Limit number of results
}

message DiscoverGatewaysResponse {
  repeated Gateway gateways = 1;
}

message Gateway {
  string gateway_id = 1;
  string cluster_id = 2;
  repeated string endpoints = 3;
  string region = 4;
  double latency_ms = 5;        // Estimated latency from client_location
  HealthStatus health = 6;
  int32 load_score = 7;         // 0-100 (lower is better)
}
```text

**DNS-based discovery** (alternative):
```bash
# Round-robin DNS for Prism gateways
dig prism.example.com
# → 10.0.1.10 (us-east-1)
# → 10.0.2.20 (eu-west-1)

# Geo-DNS for nearest gateway
dig prism.example.com
# → 10.0.1.10 (us-east-1) [if client in North America]
# → 10.0.2.20 (eu-west-1) [if client in Europe]
```text

### 5. Cross-Region Routing

**Use Case**: Application in `us-east-1` needs to access namespace hosted in `eu-west-1`.

**Options**:

1. **Direct Routing**: Client connects to remote gateway (simple, higher latency)
2. **Gateway-to-Gateway Forwarding**: Local gateway proxies to remote gateway (transparent)
3. **Data Replication**: Namespace replicated across regions (lowest latency, eventual consistency)

```protobuf
message RouteRequest {
  string namespace = 1;
  string client_region = 2;
}

message RouteResponse {
  enum RoutingStrategy {
    DIRECT = 0;          // Client connects directly to remote gateway
    PROXY = 1;           // Local gateway proxies to remote
    LOCAL_REPLICA = 2;   // Use local replica
  }

  RoutingStrategy strategy = 1;
  string target_gateway = 2;
  repeated string fallback_gateways = 3;
}
```text

## Latency and Partition Tolerance

### Handling High Latency (100-500ms)

**Strategies**:

1. **Async Configuration Push**: Don't block on config sync
   ```text
   prism-netgw: Config updated (version 123)
   → Async push to all clusters (fire-and-forget)
   → Eventually consistent (all clusters converge to version 123)
   ```text

2. **Heartbeat with Jitter**: Randomize heartbeat intervals to avoid thundering herd
   ```rust
   let heartbeat_interval = Duration::from_secs(30);
   let jitter = Duration::from_secs(rand::thread_rng().gen_range(0..10));
   sleep(heartbeat_interval + jitter).await;
   ```text

3. **Batch Updates**: Accumulate config changes and push in batches
   ```text
   Instead of: 10 individual namespace updates (10 round trips)
   Do: 1 batch with 10 namespace updates (1 round trip)
   ```text

4. **Caching**: Prism clusters cache config locally (survive netgw downtime)
   ```text
   prism-agent:
     - Fetches config from netgw periodically
     - Caches config on disk
     - Uses cached config if netgw unavailable
   ```text

### Handling Network Partitions

**CAP Theorem**: prism-netgw favors **Availability + Partition Tolerance** over **Consistency**.

**Scenario**: `eu-west-1` cluster loses connectivity to control plane.

**Behavior**:
1. **Local Operation**: Cluster continues serving requests using cached config
2. **Config Staleness**: Config may be stale (eventual consistency acceptable)
3. **Heartbeat Failure**: Cluster marked as "Unknown" in control plane
4. **Reconnection**: When partition heals, cluster syncs latest config

┌──────────────┐                ┌──────────────┐
│  prism-netgw │ ─── X ────────▶│ eu-west-1    │
│  (leader)    │                 │  (isolated)  │
└──────────────┘                 └──────────────┘
      │                                │
      │ Config version: 150            │ Config version: 147 (cached)
      │ Cluster status: UNKNOWN        │ Status: OPERATIONAL (degraded)
      │                                │
      │ ──────────────────────────────▶│ (partition heals)
      │ SyncConfig(current_version=147)│
      │ ◀──────────────────────────────│ Incremental updates: 148-150
```

### Split-Brain Prevention

**Problem**: Network partition causes two control plane nodes to both claim leadership.

**Solution**: Raft consensus with quorum.

Cluster: 5 netgw nodes (us-east-1, us-west-2, eu-west-1, ap-south-1, ap-northeast-1)
Quorum: 3 nodes

Partition scenario:
  Group A: us-east-1, us-west-2, eu-west-1 (3 nodes, HAS QUORUM) → continues as leader
  Group B: ap-south-1, ap-northeast-1 (2 nodes, NO QUORUM) → becomes followers

Result: Only Group A can make config changes (split-brain prevented)
```text

## API Specification

### gRPC Service Definition

```protobuf
syntax = "proto3";

package prism.netgw.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";

service ControlPlaneService {
  // Cluster Management
  rpc RegisterCluster(RegisterClusterRequest) returns (RegisterClusterResponse);
  rpc UnregisterCluster(UnregisterClusterRequest) returns (UnregisterClusterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc ListClusters(ListClustersRequest) returns (ListClustersResponse);

  // Configuration Sync
  rpc SyncConfig(SyncConfigRequest) returns (SyncConfigResponse);
  rpc WatchConfig(WatchConfigRequest) returns (stream ConfigUpdate);  // Server streaming

  // Health Monitoring
  rpc ReportHealth(ReportHealthRequest) returns (ReportHealthResponse);
  rpc GetClusterHealth(GetClusterHealthRequest) returns (GetClusterHealthResponse);
  rpc GetGlobalHealth(GetGlobalHealthRequest) returns (GetGlobalHealthResponse);

  // Service Discovery
  rpc DiscoverGateways(DiscoverGatewaysRequest) returns (DiscoverGatewaysResponse);

  // Cross-Region Routing
  rpc RouteRequest(RouteRequest) returns (RouteResponse);

  // Metrics
  rpc GetMetrics(GetMetricsRequest) returns (GetMetricsResponse);
}
```text

## Deployment

### Kubernetes Deployment (Multi-Region)

```yaml
# Deploy netgw control plane in multiple regions
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: prism-netgw
  namespace: prism-system
spec:
  replicas: 3  # Raft quorum
  template:
    spec:
      containers:
      - name: netgw
        image: prism/netgw:latest
        ports:
        - containerPort: 9980
          name: grpc
        - containerPort: 9981
          name: raft
        env:
        - name: NETGW_REGION
          value: "us-east-1"
        - name: NETGW_PEERS
          value: "netgw-0.netgw.prism-system.svc.cluster.local:9981,netgw-1.netgw.prism-system.svc.cluster.local:9981,netgw-2.netgw.prism-system.svc.cluster.local:9981"
        - name: NETGW_CLUSTER_ID
          value: "global-control-plane"
        volumeMounts:
        - name: data
          mountPath: /var/lib/netgw
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```text

### Agent Deployment (Per Cluster)

```yaml
# Deploy prism-agent on each Prism gateway cluster
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: prism-agent
  namespace: prism
spec:
  template:
    spec:
      containers:
      - name: agent
        image: prism/agent:latest
        env:
        - name: NETGW_ENDPOINT
          value: "prism-netgw.prism-system.svc.cluster.local:9980"
        - name: CLUSTER_ID
          value: "aws-us-east-1-prod"
        - name: REGION
          value: "us-east-1"
        - name: CLOUD_PROVIDER
          value: "aws"
        volumeMounts:
        - name: config-cache
          mountPath: /var/cache/prism
      volumes:
      - name: config-cache
        emptyDir: {}
```text

## Security Considerations

### 1. Mutual TLS

All communication between netgw and agents uses mTLS:

```yaml
tls:
  server_cert: /etc/netgw/tls/server.crt
  server_key: /etc/netgw/tls/server.key
  client_ca: /etc/netgw/tls/ca.crt
  client_cert_required: true
```text

### 2. Authentication

Agents authenticate via client certificates:

CN=prism-agent,O=aws-us-east-1-prod,OU=prism-cluster
```

### 3. Authorization

RBAC policies for cluster operations:

```yaml
policies:
  - cluster_id: aws-us-east-1-prod
    allowed_operations:
      - RegisterCluster
      - Heartbeat
      - SyncConfig
      - ReportHealth
    forbidden_operations:
      - UnregisterCluster  # Only control plane admin
      - ListClusters       # Only control plane admin
```

### 4. Audit Logging

All control plane operations logged:

```json
{
  "timestamp": "2025-10-09T10:15:23Z",
  "operation": "RegisterCluster",
  "cluster_id": "aws-us-east-1-prod",
  "region": "us-east-1",
  "cloud_provider": "aws",
  "success": true,
  "latency_ms": 45
}
```

## Observability

### Metrics

# Cluster metrics
prism_netgw_clusters_total{region="us-east-1",cloud_provider="aws"} 5
prism_netgw_cluster_health{cluster_id="...",status="healthy"} 1

# Config sync metrics
prism_netgw_config_version{cluster_id="..."} 150
prism_netgw_config_sync_latency_ms{cluster_id="..."} 234

# Heartbeat metrics
prism_netgw_heartbeat_success_total{cluster_id="..."} 12345
prism_netgw_heartbeat_failure_total{cluster_id="..."} 3
prism_netgw_heartbeat_latency_ms{cluster_id="..."} 156
```text

### Distributed Tracing

Trace: RegisterCluster
├─ netgw: ValidateRequest (2ms)
├─ netgw: StoreCluster → etcd (45ms)
├─ netgw: PublishEvent → NATS (12ms)
└─ netgw: SendResponse (1ms)
Total: 60ms
```

## Migration Path

### Phase 1: Single-Region Deployment (Week 1)
- Deploy netgw control plane in one region
- Register Prism clusters in that region
- Basic config sync and health monitoring

### Phase 2: Multi-Region Expansion (Week 2-3)
- Deploy netgw nodes in 3 regions (Raft quorum)
- Enable cross-region config sync
- Implement service discovery

### Phase 3: Production Hardening (Week 4-5)
- Add latency tolerance mechanisms
- Implement partition handling
- Add comprehensive observability

### Phase 4: Advanced Features (Future)
- Gateway-to-gateway routing
- Data replication across regions
- Multi-cloud VPC peering

## Open Questions

1. **Control Plane Sizing**: How many netgw nodes for global deployment?
2. **Config Storage**: etcd vs Consul vs custom Raft?
3. **DNS vs gRPC Discovery**: Which is more reliable for clients?
4. **Cross-Region Bandwidth**: Cost implications of config sync?
5. **Failover Time**: Acceptable latency for cluster failover?

## References

- [Raft Consensus Algorithm](https://raft.github.io/)
- [etcd Architecture](https://etcd.io/docs/latest/learning/architecture/)
- [Consul Multi-Datacenter](https://www.consul.io/docs/architecture/multi-datacenter)
- [Kubernetes Federation](https://kubernetes.io/docs/concepts/cluster-administration/federation/)
- [Google Spanner](https://research.google/pubs/pub39966/) (global consistency)
- ADR-027: Admin API via gRPC
- RFC-010: Admin Protocol with OIDC

## Revision History

- 2025-10-09: Initial draft for prism-netgw multi-region control plane
