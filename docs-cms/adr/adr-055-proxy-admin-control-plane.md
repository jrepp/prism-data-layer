---
date: 2025-10-15
deciders: Engineering Team
doc_uuid: 9a4e5b3c-7d6f-4e2a-b8c9-1f3e5d7a9b2c
id: adr-055
project_id: prism-data-layer
status: Accepted
tags:
- proxy
- admin
- control-plane
- grpc
- namespace
- partitioning
title: 'ADR-055: Proxy-Admin Control Plane Protocol'
---

## Context

Prism proxy instances currently operate independently without central coordination. This creates several operational challenges:

- **Namespace Management**: No central registry of which namespaces exist across proxy instances
- **Client Onboarding**: New clients must manually configure namespace settings in each proxy
- **Dynamic Configuration**: Namespace updates require proxy restarts or manual config reloads
- **Capacity Planning**: No visibility into which namespaces are active on which proxies
- **Partition Distribution**: Cannot distribute namespace traffic across multiple proxy instances

We need a control plane protocol that enables:
1. Proxy instances to register with prism-admin on startup
2. prism-admin to push namespace configurations to proxies
3. Client-initiated namespace creation flows through admin
4. Partition-based namespace distribution across proxy instances

## Decision

Implement bidirectional gRPC control plane protocol between prism-proxy and prism-admin:

**Proxy Startup**:
```bash
prism-proxy --admin-endpoint admin.prism.local:8981 --proxy-id proxy-01 --region us-west-2
```

**Control Plane Flows**:

1. **Proxy Registration** (proxy → admin):
   - Proxy connects on startup, sends ProxyRegistration with ID, address, region, capabilities
   - Admin records proxy in storage (proxies table from ADR-054)
   - Admin returns assigned namespaces for this proxy

2. **Namespace Assignment** (admin → proxy):
   - Admin pushes namespace configs to proxy via NamespaceAssignment message
   - Includes partition ID for distributed namespace routing
   - Proxy validates and activates namespace

3. **Client Namespace Creation** (client → proxy → admin → proxy):
   - Client sends CreateNamespace request to proxy
   - Proxy forwards to admin via control plane
   - Admin validates, persists, assigns partition
   - Admin sends NamespaceAssignment back to relevant proxies
   - Proxy acknowledges and becomes ready for client traffic

4. **Health & Heartbeat** (proxy ↔ admin):
   - Proxy sends heartbeat every 30s with namespace health stats
   - Admin tracks last_seen timestamp (ADR-054 proxies table)
   - Admin detects stale proxies and redistributes namespaces

**Partition Distribution**:

Namespaces include partition identifier for horizontal scaling:
- **Partition Key**: Hash of namespace name → partition ID (0-255)
- **Proxy Assignment**: Admin assigns namespace to proxy based on partition range
- **Consistent Hashing**: Partition → proxy mapping survives proxy additions/removals
- **Rebalancing**: Admin redistributes partitions when proxies join/leave

Example partition distribution:
```text
proxy-01: partitions [0-63]   → namespaces: ns-a (hash=12), ns-d (hash=55)
proxy-02: partitions [64-127] → namespaces: ns-b (hash=88), ns-e (hash=100)
proxy-03: partitions [128-191] → namespaces: ns-c (hash=145)
proxy-04: partitions [192-255] → namespaces: ns-f (hash=200)
```

**Protocol Messages** (protobuf):

```protobuf
service ControlPlane {
  // Proxy → Admin: Register proxy on startup
  rpc RegisterProxy(ProxyRegistration) returns (ProxyRegistrationAck);

  // Admin → Proxy: Push namespace configuration
  rpc AssignNamespace(NamespaceAssignment) returns (NamespaceAssignmentAck);

  // Proxy → Admin: Request namespace creation (client-initiated)
  rpc CreateNamespace(CreateNamespaceRequest) returns (CreateNamespaceResponse);

  // Proxy → Admin: Heartbeat with namespace health
  rpc Heartbeat(ProxyHeartbeat) returns (HeartbeatAck);

  // Admin → Proxy: Revoke namespace assignment
  rpc RevokeNamespace(NamespaceRevocation) returns (NamespaceRevocationAck);
}

message ProxyRegistration {
  string proxy_id = 1;          // Unique proxy identifier (proxy-01)
  string address = 2;           // Proxy gRPC address (proxy-01.prism.local:8980)
  string region = 3;            // Deployment region (us-west-2)
  string version = 4;           // Proxy version (0.1.0)
  repeated string capabilities = 5; // Supported patterns (keyvalue, pubsub)
  map<string, string> metadata = 6; // Custom labels
}

message ProxyRegistrationAck {
  bool success = 1;
  string message = 2;
  repeated NamespaceAssignment initial_namespaces = 3; // Pre-assigned namespaces
  repeated PartitionRange partition_ranges = 4;        // Assigned partition ranges
}

message NamespaceAssignment {
  string namespace = 1;
  int32 partition_id = 2;       // Partition ID (0-255)
  NamespaceConfig config = 3;   // Full namespace configuration
  int64 version = 4;            // Config version for idempotency
}

message NamespaceConfig {
  map<string, BackendConfig> backends = 1;
  map<string, PatternConfig> patterns = 2;
  AuthConfig auth = 3;
  map<string, string> metadata = 4;
}

message CreateNamespaceRequest {
  string namespace = 1;
  string requesting_proxy = 2;  // Proxy ID handling client request
  NamespaceConfig config = 3;
  string principal = 4;         // Authenticated user creating namespace
}

message CreateNamespaceResponse {
  bool success = 1;
  string message = 2;
  int32 assigned_partition = 3;
  string assigned_proxy = 4;    // Proxy that will handle this namespace
}

message ProxyHeartbeat {
  string proxy_id = 1;
  map<string, NamespaceHealth> namespace_health = 2;
  ResourceUsage resources = 3;
  int64 timestamp = 4;
}

message NamespaceHealth {
  int32 active_sessions = 1;
  int64 requests_per_second = 2;
  string status = 3;            // healthy, degraded, unhealthy
}

message PartitionRange {
  int32 start = 1;              // Inclusive
  int32 end = 2;                // Inclusive
}
```

## Rationale

**Why Control Plane Protocol:**
- Centralized namespace management enables operational visibility
- Dynamic configuration without proxy restarts
- Foundation for multi-proxy namespace distribution
- Client onboarding without direct admin access

**Why Partition-Based Distribution:**
- Consistent hashing enables predictable namespace → proxy routing
- Horizontal scaling by adding proxies (redistribute partitions)
- Namespace isolation (each namespace maps to one proxy per partition)
- Load balancing via partition rebalancing

**Why gRPC Bidirectional:**
- Admin can push configs to proxies (admin → proxy)
- Proxies can request namespace creation (proxy → admin)
- Efficient binary protocol with streaming support
- Type-safe protobuf contracts

**Why Heartbeat Every 30s:**
- Reasonable balance between admin load and stale proxy detection
- Fast enough for operational alerting (&lt;1min to detect failure)
- Includes namespace health stats for capacity planning

### Alternatives Considered

1. **Config File Only (No Control Plane)**
   - Pros: Simple, no runtime dependencies
   - Cons: Manual namespace distribution, no dynamic updates, no visibility
   - Rejected because: Operational burden scales with proxy count

2. **HTTP/REST Control Plane**
   - Pros: Familiar, curl-friendly
   - Cons: Verbose JSON payloads, no streaming, no bidirectional
   - Rejected because: gRPC provides better performance and type safety

3. **Kafka-Based Event Bus**
   - Pros: Decoupled, events persisted
   - Cons: Requires Kafka dependency, eventual consistency, complex
   - Rejected because: gRPC request-response fits control plane semantics

4. **Service Mesh (Istio/Linkerd)**
   - Pros: Industry standard, rich features
   - Cons: Heavy infrastructure, learning curve, overkill for simple control plane
   - Rejected because: Application-level control plane is simpler

## Consequences

### Positive

- **Centralized Visibility**: Admin has complete view of all proxies and namespaces
- **Dynamic Configuration**: Namespace changes propagate immediately without restarts
- **Client Onboarding**: Clients create namespaces via proxy, admin handles distribution
- **Horizontal Scaling**: Add proxies, admin redistributes partitions automatically
- **Operational Metrics**: Heartbeat provides namespace health across proxies
- **Partition Isolation**: Namespace traffic isolated to assigned proxy
- **Graceful Degradation**: Proxy operates with local config if admin unavailable

### Negative

- **Control Plane Dependency**: Proxies require admin connectivity for namespace operations
- **Admin as SPOF**: If admin down, cannot create namespaces (but existing work)
- **Partition Rebalancing**: Moving partitions requires namespace handoff coordination
- **Connection Overhead**: Each proxy maintains persistent gRPC connection to admin
- **State Synchronization**: Admin and proxy must agree on namespace assignments

### Neutral

- Proxies can optionally run without admin (local config file mode)
- Admin stores proxy state in SQLite/PostgreSQL (ADR-054)
- Partition count (256) fixed for now, can increase in future versions
- Control plane protocol versioned independently from data plane

## Implementation Notes

### Proxy-Side Admin Client

Rust implementation in `prism-proxy/src/admin_client.rs`:

```rust
use tonic::transport::Channel;
use tokio::time::{interval, Duration};

pub struct AdminClient {
    client: ControlPlaneClient<Channel>,
    proxy_id: String,
    address: String,
    region: String,
}

impl AdminClient {
    pub async fn new(
        admin_endpoint: &str,
        proxy_id: String,
        address: String,
        region: String,
    ) -> Result<Self> {
        let channel = Channel::from_static(admin_endpoint)
            .connect()
            .await?;

        let client = ControlPlaneClient::new(channel);

        Ok(Self { client, proxy_id, address, region })
    }

    pub async fn register(&mut self) -> Result<ProxyRegistrationAck> {
        let request = ProxyRegistration {
            proxy_id: self.proxy_id.clone(),
            address: self.address.clone(),
            region: self.region.clone(),
            version: env!("CARGO_PKG_VERSION").to_string(),
            capabilities: vec!["keyvalue".to_string(), "pubsub".to_string()],
            metadata: HashMap::new(),
        };

        let response = self.client.register_proxy(request).await?;
        Ok(response.into_inner())
    }

    pub async fn start_heartbeat_loop(&mut self) {
        let mut ticker = interval(Duration::from_secs(30));

        loop {
            ticker.tick().await;

            let heartbeat = ProxyHeartbeat {
                proxy_id: self.proxy_id.clone(),
                namespace_health: self.collect_namespace_health(),
                resources: self.collect_resource_usage(),
                timestamp: SystemTime::now().duration_since(UNIX_EPOCH)
                    .unwrap().as_secs() as i64,
            };

            if let Err(e) = self.client.heartbeat(heartbeat).await {
                warn!("Heartbeat failed: {}", e);
            }
        }
    }

    pub async fn create_namespace(
        &mut self,
        namespace: &str,
        config: NamespaceConfig,
        principal: &str,
    ) -> Result<CreateNamespaceResponse> {
        let request = CreateNamespaceRequest {
            namespace: namespace.to_string(),
            requesting_proxy: self.proxy_id.clone(),
            config: Some(config),
            principal: principal.to_string(),
        };

        let response = self.client.create_namespace(request).await?;
        Ok(response.into_inner())
    }
}
```

### Admin-Side Control Plane Service

Go implementation in `cmd/prism-admin/control_plane.go`:

```go
type ControlPlaneService struct {
    storage *Storage
    partitions *PartitionManager
}

func (s *ControlPlaneService) RegisterProxy(
    ctx context.Context,
    req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
    // Record proxy in storage
    proxy := &Proxy{
        ProxyID: req.ProxyId,
        Address: req.Address,
        Version: req.Version,
        Status: "healthy",
        LastSeen: time.Now(),
        Metadata: req.Metadata,
    }

    if err := s.storage.UpsertProxy(ctx, proxy); err != nil {
        return nil, err
    }

    // Assign partition ranges
    ranges := s.partitions.AssignRanges(req.ProxyId)

    // Get initial namespace assignments
    namespaces := s.partitions.GetNamespacesForRanges(ranges)

    return &pb.ProxyRegistrationAck{
        Success: true,
        Message: "Proxy registered successfully",
        InitialNamespaces: namespaces,
        PartitionRanges: ranges,
    }, nil
}

func (s *ControlPlaneService) CreateNamespace(
    ctx context.Context,
    req *pb.CreateNamespaceRequest,
) (*pb.CreateNamespaceResponse, error) {
    // Calculate partition ID
    partitionID := s.partitions.HashNamespace(req.Namespace)

    // Find proxy for partition
    proxyID, err := s.partitions.GetProxyForPartition(partitionID)
    if err != nil {
        return nil, err
    }

    // Persist namespace
    ns := &Namespace{
        Name: req.Namespace,
        Description: "Created via " + req.RequestingProxy,
        Metadata: req.Config.Metadata,
    }

    if err := s.storage.CreateNamespace(ctx, ns); err != nil {
        return nil, err
    }

    // Send assignment to proxy
    assignment := &pb.NamespaceAssignment{
        Namespace: req.Namespace,
        PartitionId: partitionID,
        Config: req.Config,
        Version: 1,
    }

    if err := s.sendAssignmentToProxy(proxyID, assignment); err != nil {
        return nil, err
    }

    return &pb.CreateNamespaceResponse{
        Success: true,
        Message: "Namespace created and assigned",
        AssignedPartition: partitionID,
        AssignedProxy: proxyID,
    }, nil
}
```

### Partition Manager

```go
type PartitionManager struct {
    mu sync.RWMutex
    proxies map[string][]PartitionRange // proxy_id → partition ranges
    partitionMap map[int32]string       // partition_id → proxy_id
}

func (pm *PartitionManager) HashNamespace(namespace string) int32 {
    hash := crc32.ChecksumIEEE([]byte(namespace))
    return int32(hash % 256) // 256 partitions
}

func (pm *PartitionManager) AssignRanges(proxyID string) []PartitionRange {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    // Simple round-robin distribution
    proxyCount := len(pm.proxies) + 1 // +1 for new proxy
    rangeSize := 256 / proxyCount

    proxyIndex := len(pm.proxies)
    start := proxyIndex * rangeSize
    end := start + rangeSize - 1

    if end > 255 {
        end = 255
    }

    ranges := []PartitionRange{{Start: start, End: end}}
    pm.proxies[proxyID] = ranges

    // Update partition map
    for i := start; i <= end; i++ {
        pm.partitionMap[int32(i)] = proxyID
    }

    return ranges
}

func (pm *PartitionManager) GetProxyForPartition(partitionID int32) (string, error) {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    proxyID, ok := pm.partitionMap[partitionID]
    if !ok {
        return "", fmt.Errorf("no proxy assigned to partition %d", partitionID)
    }

    return proxyID, nil
}
```

### Proxy Configuration

Add admin endpoint to proxy config:

```yaml
admin:
  endpoint: "admin.prism.local:8981"
  proxy_id: "proxy-01"
  region: "us-west-2"
  heartbeat_interval: "30s"
  reconnect_backoff: "5s"
```

### Graceful Fallback

If admin unavailable, proxy operates with local config:

```rust
async fn start_proxy(config: ProxyConfig) -> Result<()> {
    // Try connecting to admin
    match AdminClient::new(&config.admin.endpoint, ...).await {
        Ok(mut admin_client) => {
            info!("Connected to admin, registering proxy");

            match admin_client.register().await {
                Ok(ack) => {
                    info!("Registered with admin, received {} namespaces",
                          ack.initial_namespaces.len());

                    // Apply admin-provided namespaces
                    for ns in ack.initial_namespaces {
                        apply_namespace(ns).await?;
                    }

                    // Start heartbeat loop in background
                    tokio::spawn(async move {
                        admin_client.start_heartbeat_loop().await;
                    });
                }
                Err(e) => {
                    warn!("Registration failed: {}, using local config", e);
                    load_local_config().await?;
                }
            }
        }
        Err(e) => {
            warn!("Admin connection failed: {}, using local config", e);
            load_local_config().await?;
        }
    }

    // Start data plane regardless of admin connectivity
    start_data_plane().await
}
```

## References

- [ADR-027: Admin API gRPC](/adr/adr-027) - Admin API definition
- [ADR-040: Go Binary Admin CLI](/adr/adr-040) - Admin CLI architecture
- ADR-054: SQLite Storage for prism-admin (planned) - Storage for proxy registry
- [RFC-003: Protobuf Single Source of Truth](/rfc/rfc-003) - Protobuf code generation
- [Consistent Hashing](https://en.wikipedia.org/wiki/Consistent_hashing)

## Revision History

- 2025-10-15: Initial draft - Proxy-admin control plane with partition distribution
