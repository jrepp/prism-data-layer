---
title: "RFC-038: Admin Leader Election and High Availability with Raft"
status: Proposed
author: Engineering Team
created: 2025-10-17
updated: 2025-10-18
tags: [admin, ha, raft, consensus, leader-election, placement, single-region]
id: rfc-038
project_id: prism-data-access
doc_uuid: 7c2e4f8a-9b3d-4a1e-8d6f-5c9e7a2b4d3f
---

# RFC-038: Admin Leader Election and High Availability with Raft

## Summary

This RFC proposes implementing leader election and high availability for prism-admin using a mature Raft consensus library. The admin cluster will coordinate namespace assignments, pattern placements, and process scheduling across proxy and launcher instances, while ensuring prismctl commands remain highly available through automatic leader failover.

## Motivation

### Current State

Per ADR-055 and ADR-056, prism-admin currently operates as a single instance that:
- Accepts control plane connections from proxy and launcher instances
- Maintains namespace registry and pattern assignments
- Coordinates namespace-to-proxy distribution via partition hashing
- Tracks launcher health and pattern placement

**Problem**: Single admin instance creates operational challenges:
1. **Single Point of Failure**: If admin crashes, control plane is unavailable
2. **No Failover**: Proxy/launcher registration and heartbeats fail until admin restarts
3. **Lost State on Crash**: In-memory partition assignments lost on restart
4. **Manual Recovery**: Operators must manually restart admin and re-register components
5. **No Horizontal Scaling**: Cannot scale admin for high availability or load distribution

### Goals

**High Availability**:
- Multiple admin instances form a cluster with automatic leader election (single-region only)
- Leader handles all write operations (namespace creation, pattern assignment)
- All nodes can serve reads with configurable consistency levels (stale, lease-based, linearizable)
- Automatic failover when leader fails (&lt;500ms)
- No data loss on leader failure (consensus-based state replication)
- Clients connect to all nodes simultaneously for load-balanced reads

**Unified Coordination**:
- Leader coordinates namespace-to-proxy placement
- Leader schedules pattern-to-launcher placement
- Leader maintains global view of cluster resources
- Leader enforces capacity constraints and affinity rules

**prismctl High Availability**:
- prismctl connects to any admin instance (picks one from configured list)
- **Reads**: Served directly by connected node (stale reads OK for most operations)
- **Writes**: Follower automatically forwards to leader (single internal hop)
- Transparent failover on leader change or node failure

### Non-Goals

- **Distributed transactions across backends**: Raft only for admin state, not data plane
- **Multi-region admin clusters**: Explicit non-goal. Single-region only to avoid WAN latency/partitioning issues
- **Custom Raft implementation**: Use battle-tested library (Hashicorp/raft)
- **Leader-per-namespace**: Single leader for entire admin cluster (simpler operations)
- **Explicit leader discovery**: Clients connect to all nodes and automatically find leader

## Proposed Design

### Architecture Overview

```text
┌─────────────────────────────────────────────────────────────┐
│                    prismctl CLI                             │
│  • Connects to any admin instance from configured list      │
│  • Reads: served directly (stale OK)                        │
│  • Writes: forwarded to leader automatically                │
└──────────────────────────┬──────────────────────────────────┘
                           │ (connects to admin-02)
                           │
        ┌──────────┐ ┌─────▼─────┐ ┌───────────┐
        │ admin-01 │ │ admin-02  │ │ admin-03  │
        │ (Leader) │ │(Follower) │ │(Follower) │
        │          │ │           │ │           │
        │ Executes │◄┤ Forwards  │ │ Reads     │
        │ writes   │ │ writes    │ │ (local)   │
        └─────┬────┘ └─────┬─────┘ └─────┬─────┘
              │            │             │
              │ Raft Consensus Protocol   │
              │   (State Replication)     │
              └────────────┼───────────────┘
                           │
         ┌─────────────────┴─────────────────┐
         │                                   │
   ┌─────▼──────┐                    ┌───────▼──────┐
   │ proxy-01   │   ...              │ launcher-01  │
   │ (client)   │                    │ (client)     │
   └────────────┘                    └──────────────┘
```

**Key Components**:
1. **Raft Cluster**: 3 or 5 admin instances in single region running Raft consensus
2. **Leader Election**: Automatic leader election on cluster formation and leader failure (&lt;500ms)
3. **State Machine**: ALL admin state in Raft FSM (namespaces, proxies, launchers, patterns)
4. **Log Replication**: All state changes replicated via Raft log to quorum before ACK
5. **Read Consistency Levels**: Linearizable (leader + quorum), lease-based (leader), stale (any node)
6. **Follower Forwarding**: Writes automatically forwarded from follower to leader (clients connect to any node)
7. **Computed Partition Assignment**: Partition ranges derived from proxy set via consistent hashing (not stored in FSM)

### Raft Library Selection

**Recommendation: Hashicorp Raft** (github.com/hashicorp/raft)

**Rationale**:
- **Production-proven**: Used by Consul, Nomad, Vault (billions of requests/day)
- **Pure Go**: Native Go implementation, no CGo dependencies
- **Easy integration**: Clean API, well-documented
- **Batteries included**: Snapshot support, log compaction, leader forwarding
- **Testing support**: In-memory transport for local binary mode
- **Active maintenance**: Regular updates, security patches

**Alternative considered: etcd/raft**:
- Pros: Also production-proven (etcd, Kubernetes control plane)
- Cons: More complex API, requires more boilerplate
- Decision: Hashicorp Raft simpler to integrate for admin use case

### State Machine Design

**Raft FSM (Finite State Machine)** manages admin cluster state:

```go
// AdminStateMachine implements raft.FSM
type AdminStateMachine struct {
    mu sync.RWMutex

    // All admin cluster state (versioned for schema evolution)
    state AdminState

    // Snapshot metadata
    lastAppliedIndex uint64
    lastAppliedTerm  uint64
}

// AdminState contains all replicated admin cluster state
type AdminState struct {
    Version int // Schema version for future evolution

    // Namespace registry
    Namespaces map[string]*Namespace

    // Proxy registry (partition assignment computed from this)
    Proxies map[string]*ProxyRegistration

    // Launcher registry
    Launchers map[string]*LauncherRegistration

    // Pattern → Launcher mapping (from ADR-056)
    Patterns map[string]*PatternAssignment
}

// NOTE: Partition → Proxy mapping NOT stored in FSM
// Computed on-demand via consistent hashing over proxy set
// This reduces Raft log entries and simplifies rebalancing

// Apply applies a Raft log entry to the FSM
func (fsm *AdminStateMachine) Apply(log *raft.Log) interface{} {
    var cmd Command
    if err := proto.Unmarshal(log.Data, &cmd); err != nil {
        return err
    }

    fsm.mu.Lock()
    defer fsm.mu.Unlock()

    switch cmd.Type {
    case CommandType_CREATE_NAMESPACE:
        return fsm.applyCreateNamespace(cmd.CreateNamespace)
    case CommandType_REGISTER_PROXY:
        return fsm.applyRegisterProxy(cmd.RegisterProxy)
    case CommandType_ASSIGN_PATTERN:
        return fsm.applyAssignPattern(cmd.AssignPattern)
    case CommandType_REGISTER_LAUNCHER:
        return fsm.applyRegisterLauncher(cmd.RegisterLauncher)
    default:
        return fmt.Errorf("unknown command type: %v", cmd.Type)
    }
}

// Snapshot returns FSM snapshot for log compaction
func (fsm *AdminStateMachine) Snapshot() (raft.FSMSnapshot, error) {
    fsm.mu.RLock()
    defer fsm.mu.RUnlock()

    // Atomic snapshot of entire state
    return &AdminSnapshot{
        state:            fsm.state,
        lastAppliedIndex: fsm.lastAppliedIndex,
        lastAppliedTerm:  fsm.lastAppliedTerm,
    }, nil
}

// Restore restores FSM from snapshot
func (fsm *AdminStateMachine) Restore(snapshot io.ReadCloser) error {
    defer snapshot.Close()

    var snap AdminSnapshot
    if err := gob.NewDecoder(snapshot).Decode(&snap); err != nil {
        return err
    }

    fsm.mu.Lock()
    defer fsm.mu.Unlock()

    // Atomic restore of entire state
    fsm.state = snap.state
    fsm.lastAppliedIndex = snap.lastAppliedIndex
    fsm.lastAppliedTerm = snap.lastAppliedTerm

    return nil
}
```

### Read Consistency Levels

**Single-region Raft enables flexible read consistency** based on operation requirements:

#### Consistency Levels

| Level | Where Served | Guarantee | Latency | Max Staleness | Use Case |
|-------|-------------|-----------|---------|---------------|----------|
| **Stale** | Any node (leader or follower) | Eventually consistent | &lt;1ms | 50-200ms | High-volume reads, displays |
| **Lease-based** | Leader only | Consistent within lease | 1-5ms | 0ms | Most admin operations |
| **Linearizable** | Leader + quorum check | Strongly consistent | 5-15ms | 0ms | Critical writes, read-after-write |

**Default mappings**:

```go
// pkg/admin/control_plane_service.go
const (
    ReadStale        = 0 // Read from local FSM (may be stale)
    ReadLeaseCheck   = 1 // Leader confirms lease before read
    ReadLinearizable = 2 // Leader + quorum check (slowest)
)

// Default consistency per operation
var defaultReadConsistency = map[string]int{
    "GetNamespace":       ReadStale,        // Display info
    "ListNamespaces":     ReadStale,        // Display info
    "GetClusterStatus":   ReadStale,        // Diagnostic info
    "ProxyHeartbeat":     ReadStale,        // High volume
    "RegisterProxy":      ReadLinearizable, // Must be exact
    "CreateNamespace":    ReadLinearizable, // Must not duplicate
    "AssignPattern":      ReadLinearizable, // Must not double-assign
}
```

#### Implementation Pattern

```go
// Read operations check requested consistency
func (s *ControlPlaneService) GetNamespace(
    ctx context.Context,
    req *pb.GetNamespaceRequest,
) (*pb.GetNamespaceResponse, error) {
    consistency := s.getReadConsistency(ctx)

    // Linearizable: must be leader + verify quorum
    if consistency == ReadLinearizable {
        if s.raft.State() != raft.Leader {
            return nil, status.Error(codes.FailedPrecondition, "not leader")
        }
        if err := s.raft.VerifyLeader().Error(); err != nil {
            return nil, status.Error(codes.Unavailable, "leader verification failed")
        }
    }

    // Lease-based: must be leader (assumes valid lease)
    if consistency == ReadLeaseCheck {
        if s.raft.State() != raft.Leader {
            return nil, status.Error(codes.FailedPrecondition, "not leader")
        }
    }

    // Stale reads: just read from local FSM (works on any node)
    ns, ok := s.fsm.GetNamespace(req.Namespace)
    if !ok {
        return nil, status.Error(codes.NotFound, "namespace not found")
    }

    return &pb.GetNamespaceResponse{
        Namespace: ns,
        ReadIndex: s.fsm.LastAppliedIndex(), // Client can see staleness
    }, nil
}
```

#### Consistency Guarantees

**Write semantics (all writes go to leader)**:
- At-least-once delivery (client retries on failure)
- Idempotency via natural keys (proxy_id, namespace, pattern_id)
- No duplicate writes (FSM checks existence before applying)

**Read semantics**:
- **Stale reads**: May lag up to 200ms behind leader (heartbeat interval)
- **Never read uncommitted data**: Raft safety property ensures FSM only applies committed entries
- **Eventually consistent**: All nodes eventually see all committed writes (Raft liveness)
- **Monotonic reads**: Reading from same node guarantees monotonic view (FSM only moves forward)

**Failure guarantees**:
- **Leader failure during write**: Client gets error, retries, write either commits or doesn't (no partial state)
- **Follower reads during election**: May see stale data (up to election timeout old), but never uncommitted
- **Network partition**: Minority partition cannot commit writes (no split-brain)

### Protobuf Commands

**Raft commands as protobuf messages** (proto/admin/commands.proto):

```protobuf
// Command wraps all admin state mutations
message Command {
  CommandType type = 1;
  oneof payload {
    CreateNamespaceCommand create_namespace = 2;
    RegisterProxyCommand register_proxy = 3;
    AssignPatternCommand assign_pattern = 4;
    RegisterLauncherCommand register_launcher = 5;
  }
}

enum CommandType {
  COMMAND_TYPE_UNSPECIFIED = 0;
  COMMAND_TYPE_CREATE_NAMESPACE = 1;
  COMMAND_TYPE_REGISTER_PROXY = 2;
  COMMAND_TYPE_ASSIGN_PATTERN = 3;
  COMMAND_TYPE_REGISTER_LAUNCHER = 4;
}

message CreateNamespaceCommand {
  string namespace = 1;
  int32 partition_id = 2;
  string assigned_proxy = 3;
  NamespaceConfig config = 4;
  string principal = 5;
  int64 timestamp = 6;
}

message RegisterProxyCommand {
  string proxy_id = 1;
  string address = 2;
  string region = 3;
  string version = 4;
  repeated string capabilities = 5;
  map<string, string> metadata = 6;
  int64 timestamp = 7;
}

message AssignPatternCommand {
  string pattern_id = 1;
  string pattern_type = 2;
  string launcher_id = 3;
  string namespace = 4;
  PatternConfig config = 5;
  int64 timestamp = 6;
}

message RegisterLauncherCommand {
  string launcher_id = 1;
  string address = 2;
  string region = 3;
  string version = 4;
  int32 max_patterns = 5;
  int64 timestamp = 6;
}
```

### Leader Election Flow

**Cluster Formation**:
1. Three admin instances start: admin-01, admin-02, admin-03
2. Each instance configured with peer list: ["admin-01:8990", "admin-02:8990", "admin-03:8990"]
3. Raft election timeout (150-300ms) expires on one instance
4. That instance transitions to candidate, requests votes
5. Quorum (2/3) vote for candidate → becomes leader
6. Leader sends heartbeats every 50ms to maintain leadership
7. Followers receive heartbeats, remain in follower state

**Leader Failure**:
1. Leader crashes or network partition
2. Followers stop receiving heartbeats (&gt;150ms timeout)
3. Follower transitions to candidate, increments term
4. Candidate requests votes with higher term
5. Quorum votes for candidate → new leader elected
6. New leader sends heartbeats, cluster operational
7. Old leader rejoins as follower (if recovered)

**Timeline**:
- Heartbeat interval: 50ms (default)
- Election timeout: 150-300ms (randomized)
- Leader failure detection: &lt;300ms
- New leader election: &lt;200ms
- **Total failover time: &lt;500ms**

### Control Plane Integration

**Proxy Registration** (extends ADR-055):

```go
// ControlPlaneService now uses Raft for coordination
func (s *ControlPlaneService) RegisterProxy(
    ctx context.Context,
    req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
    // If not leader, forward to leader internally
    // This keeps client simple (connects to any admin node)
    if s.raft.State() != raft.Leader {
        return s.forwardToLeader(ctx, req)
    }

    // Build Raft command with idempotency key
    cmd := &pb.Command{
        Type: pb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
        RegisterProxy: &pb.RegisterProxyCommand{
            ProxyId:      req.ProxyId, // Natural idempotency key
            Address:      req.Address,
            Region:       req.Region,
            Version:      req.Version,
            Capabilities: req.Capabilities,
            Metadata:     req.Metadata,
            Timestamp:    time.Now().Unix(),
        },
    }

    // Serialize command
    data, err := proto.Marshal(cmd)
    if err != nil {
        return nil, err
    }

    // Apply via Raft (blocks until replicated to quorum)
    // Reduced timeout: should complete in <200ms for single-region
    future := s.raft.Apply(data, 3*time.Second)
    if err := future.Error(); err != nil {
        return nil, fmt.Errorf("raft apply failed: %w", err)
    }

    // Get result from FSM (handles idempotent retries)
    result := future.Response()
    if err, ok := result.(error); ok {
        return nil, err
    }

    // Compute partition ranges (not stored in FSM - derived from proxy set)
    ranges := s.computePartitionRangesForProxy(req.ProxyId)

    // Get initial namespace assignments (read from FSM)
    namespaces := s.fsm.GetNamespacesForRanges(ranges)

    return &pb.ProxyRegistrationAck{
        Success:           true,
        Message:           "Proxy registered successfully",
        InitialNamespaces: namespaces,
        PartitionRanges:   ranges,
    }, nil
}

// forwardToLeader internally forwards write to leader (single gRPC hop)
func (s *ControlPlaneService) forwardToLeader(
    ctx context.Context,
    req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
    leaderAddr, _ := s.raft.LeaderWithID()
    if leaderAddr == "" {
        return nil, status.Error(codes.Unavailable, "no leader elected")
    }

    // Reuse connection pool to leader (don't create new conn each time)
    conn := s.leaderConnPool.Get(string(leaderAddr))
    client := pb.NewControlPlaneClient(conn)

    // Forward to leader with same context (preserves timeout, metadata)
    return client.RegisterProxy(ctx, req)
}

// computePartitionRangesForProxy uses consistent hashing (not stored in Raft)
func (s *ControlPlaneService) computePartitionRangesForProxy(
    proxyID string,
) []*pb.PartitionRange {
    // Get current proxy set from FSM
    proxies := s.fsm.GetAllProxies()

    // Compute ranges via consistent hashing (deterministic)
    return s.consistentHash.ComputeRanges(proxyID, proxies)
}
```

**Pattern Assignment** (extends ADR-056):

```go
func (s *ControlPlaneService) AssignPattern(
    ctx context.Context,
    req *pb.PatternAssignment,
) (*pb.PatternAssignmentAck, error) {
    // Leader-only operation
    if s.raft.State() != raft.Leader {
        return s.forwardToLeader(ctx, "AssignPattern", req)
    }

    // Select launcher based on capacity
    launcherID, err := s.selectLauncher(req.Namespace, req.PatternType)
    if err != nil {
        return nil, err
    }

    // Build Raft command
    cmd := &pb.Command{
        Type: pb.CommandType_COMMAND_TYPE_ASSIGN_PATTERN,
        AssignPattern: &pb.AssignPatternCommand{
            PatternId:   req.PatternId,
            PatternType: req.PatternType,
            LauncherId:  launcherID,
            Namespace:   req.Namespace,
            Config:      req.Config,
            Timestamp:   time.Now().Unix(),
        },
    }

    // Apply via Raft
    data, err := proto.Marshal(cmd)
    if err != nil {
        return nil, err
    }

    future := s.raft.Apply(data, 10*time.Second)
    if err := future.Error(); err != nil {
        return nil, fmt.Errorf("raft apply failed: %w", err)
    }

    // Push assignment to launcher
    if err := s.pushAssignmentToLauncher(launcherID, req); err != nil {
        log.Error().Err(err).Msg("Failed to push assignment to launcher")
        // Note: Assignment recorded in Raft, will retry on launcher heartbeat
    }

    return &pb.PatternAssignmentAck{
        Success: true,
        Message: "Pattern assigned to launcher " + launcherID,
    }, nil
}

// selectLauncher chooses launcher for pattern placement
func (s *ControlPlaneService) selectLauncher(
    namespace string,
    patternType string,
) (string, error) {
    launchers := s.fsm.GetHealthyLaunchers()
    if len(launchers) == 0 {
        return "", fmt.Errorf("no healthy launchers available")
    }

    // Placement strategy: least-loaded launcher
    var selected string
    minPatternCount := int32(1000000)

    for _, launcher := range launchers {
        patternCount := s.fsm.GetPatternCountForLauncher(launcher.LauncherId)
        if patternCount < minPatternCount &&
           patternCount < launcher.MaxPatterns {
            selected = launcher.LauncherId
            minPatternCount = patternCount
        }
    }

    if selected == "" {
        return "", fmt.Errorf("no launcher with available capacity")
    }

    return selected, nil
}
```

### prismctl High Availability

**Simple client connection strategy**: Connect to any admin node, server handles forwarding

```go
// pkg/client/admin_client.go
type AdminClient struct {
    adminAddrs []string
    conn       *grpc.ClientConn
    client     pb.AdminServiceClient
}

func NewAdminClient(adminAddrs []string) (*AdminClient, error) {
    // Try each admin instance until one succeeds
    // Don't need to find leader - server forwards writes automatically
    for _, addr := range adminAddrs {
        conn, err := grpc.Dial(addr,
            grpc.WithTransportCredentials(insecure.NewCredentials()),
            grpc.WithKeepaliveParams(keepalive.ClientParameters{
                Time:    30 * time.Second,
                Timeout: 10 * time.Second,
            }))
        if err != nil {
            continue
        }

        // Test connectivity with lightweight status check
        client := pb.NewAdminServiceClient(conn)
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()

        if _, err = client.GetClusterStatus(ctx, &pb.GetClusterStatusRequest{}); err == nil {
            // Success - connected to healthy admin instance
            return &AdminClient{
                adminAddrs: adminAddrs,
                conn:       conn,
                client:     client,
            }, nil
        }

        conn.Close()
    }

    return nil, fmt.Errorf("no admin instances available")
}

// Reads served directly by connected node (stale reads OK)
func (c *AdminClient) GetNamespace(
    ctx context.Context,
    namespace string,
) (*pb.Namespace, error) {
    resp, err := c.client.GetNamespace(ctx, &pb.GetNamespaceRequest{
        Namespace: namespace,
    })
    if err != nil {
        return nil, err
    }
    return resp.Namespace, nil
}

// Writes forwarded to leader automatically by server
func (c *AdminClient) CreateNamespace(
    ctx context.Context,
    namespace string,
    config *pb.NamespaceConfig,
) error {
    req := &pb.CreateNamespaceRequest{
        Namespace: namespace,
        Config:    config,
    }

    // Single call - server forwards to leader if needed
    _, err := c.client.CreateNamespace(ctx, req)
    if err != nil {
        // Only retry on specific transient errors
        if status.Code(err) == codes.Unavailable {
            // No leader elected - retry with backoff
            return fmt.Errorf("no leader elected: %w", err)
        }
        return err
    }

    return nil
}

// Client reconnects on connection failure (not shown: reconnect logic)
// For multi-node setup, try next admin address on connection failure
```

### Configuration

**Admin cluster config** (config/admin-cluster.yaml):

```yaml
cluster:
  # Node ID (unique per admin instance)
  node_id: "admin-01"

  # Bind address for Raft transport
  bind_addr: "0.0.0.0:8990"

  # Advertised address for peer communication
  advertise_addr: "admin-01.prism.local:8990"

  # Initial cluster members (for bootstrap)
  peers:
    - "admin-01.prism.local:8990"
    - "admin-02.prism.local:8990"
    - "admin-03.prism.local:8990"

  # Raft configuration
  raft:
    heartbeat_timeout: "50ms"
    election_timeout: "150ms"
    commit_timeout: "50ms"
    snapshot_interval: "2m"
    snapshot_threshold: 8192      # Log entries before snapshot
    snapshot_size_mb: 50          # Max snapshot size before compaction
    trailing_logs: 10240          # Keep this many logs after snapshot

    # Read consistency
    enable_follower_reads: true   # Allow stale reads from followers
    max_staleness: "200ms"        # Acceptable staleness for reads
    lease_duration: "10s"         # Leader lease for lease-based reads

  # Storage
  data_dir: "/var/lib/prism-admin/raft"
  log_retention: "7d"

# Control plane server (for proxy/launcher connections)
control_plane:
  listen: "0.0.0.0:8981"

  # Default read consistency per operation
  read_consistency:
    proxy_heartbeat: "stale"          # High volume, can be stale
    proxy_registration: "linearizable" # Must be exact
    namespace_get: "stale"            # Display info
    namespace_list: "stale"           # Display info
    namespace_create: "linearizable"  # Must not duplicate
    pattern_assignment: "linearizable" # Must not double-assign

# Admin API (for prismctl)
admin_api:
  listen: "0.0.0.0:8980"

  # Default consistency for CLI operations
  default_read_consistency: "stale"   # Most CLI reads can tolerate staleness
```

**Local binary mode** (single admin, in-memory Raft):

```yaml
cluster:
  # Single-node mode for development
  node_id: "admin-local"
  bind_addr: "127.0.0.1:8990"
  advertise_addr: "127.0.0.1:8990"
  peers: []  # Empty peers = single-node mode

  # In-memory transport for local testing
  raft:
    transport: "inmem"
    heartbeat_timeout: "50ms"
    election_timeout: "150ms"

control_plane:
  listen: "127.0.0.1:8981"

admin_api:
  listen: "127.0.0.1:8980"
```

### Deployment Modes

#### Mode 1: Local Binary (Development)

```bash
# Single admin instance with in-memory Raft
prism-admin --config config/admin-local.yaml
```

**Characteristics**:
- Single node, no replication
- In-memory transport (no network)
- Fast startup (&lt;100ms)
- State lost on restart
- Perfect for local development and testing

#### Mode 2: Docker Compose (Integration Testing)

```yaml
services:
  admin-01:
    image: prism/admin:latest
    environment:
      PRISM_NODE_ID: "admin-01"
      PRISM_BIND_ADDR: "0.0.0.0:8990"
      PRISM_ADVERTISE_ADDR: "admin-01:8990"
      PRISM_PEERS: "admin-01:8990,admin-02:8990,admin-03:8990"
    ports:
      - "8980:8980"  # Admin API
      - "8981:8981"  # Control plane
      - "8990:8990"  # Raft
    volumes:
      - admin-01-data:/var/lib/prism-admin

  admin-02:
    image: prism/admin:latest
    environment:
      PRISM_NODE_ID: "admin-02"
      PRISM_BIND_ADDR: "0.0.0.0:8990"
      PRISM_ADVERTISE_ADDR: "admin-02:8990"
      PRISM_PEERS: "admin-01:8990,admin-02:8990,admin-03:8990"
    ports:
      - "8982:8980"
      - "8983:8981"
      - "8991:8990"
    volumes:
      - admin-02-data:/var/lib/prism-admin

  admin-03:
    image: prism/admin:latest
    environment:
      PRISM_NODE_ID: "admin-03"
      PRISM_BIND_ADDR: "0.0.0.0:8990"
      PRISM_ADVERTISE_ADDR: "admin-03:8990"
      PRISM_PEERS: "admin-01:8990,admin-02:8990,admin-03:8990"
    ports:
      - "8984:8980"
      - "8985:8981"
      - "8992:8990"
    volumes:
      - admin-03-data:/var/lib/prism-admin

volumes:
  admin-01-data:
  admin-02-data:
  admin-03-data:
```

#### Mode 3: Kubernetes (Production)

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: prism-admin
spec:
  serviceName: prism-admin
  replicas: 3
  selector:
    matchLabels:
      app: prism-admin
  template:
    metadata:
      labels:
        app: prism-admin
    spec:
      containers:
      - name: admin
        image: prism/admin:v0.1.0
        env:
        - name: PRISM_NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: PRISM_BIND_ADDR
          value: "0.0.0.0:8990"
        - name: PRISM_ADVERTISE_ADDR
          value: "$(PRISM_NODE_ID).prism-admin:8990"
        - name: PRISM_PEERS
          value: "prism-admin-0.prism-admin:8990,prism-admin-1.prism-admin:8990,prism-admin-2.prism-admin:8990"
        ports:
        - name: admin-api
          containerPort: 8980
        - name: control-plane
          containerPort: 8981
        - name: raft
          containerPort: 8990
        volumeMounts:
        - name: data
          mountPath: /var/lib/prism-admin
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

### Observability

**Metrics** (Prometheus):

```text
# Raft metrics
prism_admin_raft_state{node_id="admin-01"} 1  # 1=leader, 2=follower, 3=candidate
prism_admin_raft_term{node_id="admin-01"} 42
prism_admin_raft_last_log_index{node_id="admin-01"} 1234
prism_admin_raft_last_applied_index{node_id="admin-01"} 1234
prism_admin_raft_commit_time_seconds{node_id="admin-01",quantile="0.99"} 0.025

# Cluster health
prism_admin_cluster_size{node_id="admin-01"} 3
prism_admin_leader_changes_total{node_id="admin-01"} 2

# FSM metrics
prism_admin_fsm_namespaces_total{node_id="admin-01"} 45
prism_admin_fsm_proxies_total{node_id="admin-01"} 10
prism_admin_fsm_launchers_total{node_id="admin-01"} 5
prism_admin_fsm_patterns_total{node_id="admin-01"} 123

# Control plane metrics
prism_admin_control_plane_requests_total{method="RegisterProxy",status="success"} 10
prism_admin_control_plane_request_duration_seconds{method="RegisterProxy",quantile="0.99"} 0.015
```

**Logging**:

```json
{
  "level": "info",
  "msg": "Raft state transition",
  "node_id": "admin-01",
  "from": "follower",
  "to": "leader",
  "term": 42,
  "timestamp": "2025-10-17T10:23:45Z"
}

{
  "level": "info",
  "msg": "Applied Raft command",
  "node_id": "admin-01",
  "command_type": "CREATE_NAMESPACE",
  "namespace": "prod-orders",
  "index": 1234,
  "duration_ms": 15.3,
  "timestamp": "2025-10-17T10:23:46Z"
}

{
  "level": "warn",
  "msg": "Heartbeat timeout, becoming candidate",
  "node_id": "admin-02",
  "term": 42,
  "elapsed_ms": 175,
  "timestamp": "2025-10-17T10:24:00Z"
}
```

## Implementation Plan

### Phase 1: Raft Foundation (Week 1-2)

**Deliverables**:
- Integrate Hashicorp Raft library
- Implement AdminStateMachine (FSM)
- Configure Raft with appropriate timeouts
- Snapshot and restore implementation
- Single-node mode for local development

**Tests**:
- FSM apply commands correctly
- Snapshot/restore preserves state
- Single-node cluster starts successfully

### Phase 2: Cluster Formation (Week 2-3)

**Deliverables**:
- Multi-node cluster configuration
- Peer discovery and joining
- Leader election
- Heartbeat and log replication
- Docker Compose 3-node cluster

**Tests**:
- 3-node cluster forms successfully
- Leader elected within 500ms
- Commands replicated to followers
- Leader failover &lt;1 second

### Phase 3: Control Plane Integration (Week 3-4)

**Deliverables**:
- Proxy registration via Raft
- Namespace creation via Raft
- Pattern assignment via Raft
- Launcher registration via Raft
- Leader forwarding for followers

**Tests**:
- Proxy registers successfully
- Namespace created and replicated
- Pattern assigned and tracked
- Followers forward to leader

### Phase 4: prismctl HA Support (Week 4)

**Deliverables**:
- prismctl connects to any admin instance
- Auto-discovery of leader
- Retry on leader failover
- Read-from-follower optimization

**Tests**:
- prismctl connects to follower, succeeds
- Leader fails, prismctl auto-retries
- Reads succeed from followers

### Phase 5: Observability (Week 5)

**Deliverables**:
- Prometheus metrics export
- Structured logging
- Leader change alerts
- FSM state metrics

**Tests**:
- Metrics exported correctly
- Logs parseable as JSON
- Alerts fire on leader change

## Testing Strategy

### Unit Tests

```go
func TestAdminStateMachine_ApplyCreateNamespace(t *testing.T) {
    fsm := NewAdminStateMachine()

    cmd := &pb.Command{
        Type: pb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
        CreateNamespace: &pb.CreateNamespaceCommand{
            Namespace:     "test-ns",
            PartitionId:   42,
            AssignedProxy: "proxy-01",
            Config:        &pb.NamespaceConfig{},
            Principal:     "admin",
            Timestamp:     time.Now().Unix(),
        },
    }

    data, err := proto.Marshal(cmd)
    require.NoError(t, err)

    log := &raft.Log{Data: data}
    result := fsm.Apply(log)
    require.Nil(t, result)

    // Verify namespace created
    ns, ok := fsm.GetNamespace("test-ns")
    require.True(t, ok)
    assert.Equal(t, int32(42), ns.PartitionId)
}

func TestAdminStateMachine_Snapshot(t *testing.T) {
    fsm := NewAdminStateMachine()

    // Create state
    fsm.applyCreateNamespace(&pb.CreateNamespaceCommand{
        Namespace:   "ns1",
        PartitionId: 10,
    })

    // Take snapshot
    snapshot, err := fsm.Snapshot()
    require.NoError(t, err)

    // Persist snapshot
    var buf bytes.Buffer
    sink := &mockSnapshotSink{buf: &buf}
    err = snapshot.Persist(sink)
    require.NoError(t, err)

    // Restore to new FSM
    fsm2 := NewAdminStateMachine()
    err = fsm2.Restore(io.NopCloser(&buf))
    require.NoError(t, err)

    // Verify state restored
    ns, ok := fsm2.GetNamespace("ns1")
    require.True(t, ok)
    assert.Equal(t, int32(10), ns.PartitionId)
}
```

### Integration Tests

```go
func TestRaftCluster_LeaderElection(t *testing.T) {
    // Start 3-node cluster
    cluster := NewTestCluster(t, 3)
    defer cluster.Shutdown()

    // Wait for leader election
    leader := cluster.WaitForLeader(5 * time.Second)
    require.NotNil(t, leader)

    // Verify followers acknowledge leader
    for _, node := range cluster.Nodes() {
        if node != leader {
            assert.Equal(t, raft.Follower, node.State())
        }
    }
}

func TestRaftCluster_LeaderFailover(t *testing.T) {
    cluster := NewTestCluster(t, 3)
    defer cluster.Shutdown()

    // Get initial leader
    leader1 := cluster.WaitForLeader(5 * time.Second)
    require.NotNil(t, leader1)

    // Apply command
    cmd := &pb.Command{
        Type: pb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
        CreateNamespace: &pb.CreateNamespaceCommand{
            Namespace: "test-ns",
        },
    }
    err := leader1.ApplyCommand(cmd, 5*time.Second)
    require.NoError(t, err)

    // Kill leader
    cluster.Shutdown(leader1.NodeID())

    // Wait for new leader
    leader2 := cluster.WaitForLeader(2 * time.Second)
    require.NotNil(t, leader2)
    assert.NotEqual(t, leader1.NodeID(), leader2.NodeID())

    // Verify state preserved
    ns, ok := leader2.FSM().GetNamespace("test-ns")
    require.True(t, ok)
    assert.Equal(t, "test-ns", ns.Namespace)
}

func TestControlPlane_ProxyRegistrationWithRaft(t *testing.T) {
    cluster := NewTestCluster(t, 3)
    defer cluster.Shutdown()

    leader := cluster.WaitForLeader(5 * time.Second)

    // Register proxy via leader
    req := &pb.ProxyRegistration{
        ProxyId: "proxy-01",
        Address: "proxy-01:8980",
        Region:  "us-west-2",
    }

    ack, err := leader.ControlPlane().RegisterProxy(context.Background(), req)
    require.NoError(t, err)
    assert.True(t, ack.Success)

    // Verify replicated to followers
    time.Sleep(100 * time.Millisecond)
    for _, node := range cluster.Nodes() {
        proxy, ok := node.FSM().GetProxy("proxy-01")
        require.True(t, ok)
        assert.Equal(t, "proxy-01:8980", proxy.Address)
    }
}
```

### End-to-End Tests

```bash
#!/bin/bash
# Test leader failover with real processes

# Start 3-node admin cluster
docker-compose -f docker-compose-admin-cluster.yaml up -d
sleep 5

# Get leader
LEADER=$(prismctl admin cluster status --format json | jq -r '.leader_id')
echo "Initial leader: $LEADER"

# Create namespace
prismctl admin namespace create test-ns

# Verify namespace exists
prismctl admin namespace list | grep test-ns

# Kill leader
docker-compose -f docker-compose-admin-cluster.yaml stop admin-$LEADER
sleep 2

# Verify new leader elected
NEW_LEADER=$(prismctl admin cluster status --format json | jq -r '.leader_id')
echo "New leader: $NEW_LEADER"

# Verify namespace still exists
prismctl admin namespace list | grep test-ns

# Create another namespace with new leader
prismctl admin namespace create test-ns-2

# Verify second namespace exists
prismctl admin namespace list | grep test-ns-2

echo "Leader failover test PASSED"
```

## Operational Considerations

### Cluster Sizing

**Development**: 1 node (single-node mode)
- In-memory Raft transport
- No replication
- Fast startup

**Testing**: 3 nodes
- Tolerates 1 failure (quorum = 2/3)
- Replication for durability testing
- Failover testing

**Production**: 3 or 5 nodes
- **3 nodes**: Tolerates 1 failure (quorum = 2/3)
- **5 nodes**: Tolerates 2 failures (quorum = 3/5)
- **Recommendation**: Start with 3 nodes, scale to 5 if higher availability needed

### Disaster Recovery

**Backup strategy**:
1. Periodic Raft snapshot backups (every 2 minutes)
2. Snapshot stored in persistent volume
3. Optional: Export snapshot to S3/GCS

**Recovery scenarios**:
1. **Single node failure**: Cluster continues with quorum, failed node rejoins
2. **Quorum loss (2/3 nodes fail)**: Restore from snapshot, manual intervention
3. **Complete cluster loss**: Bootstrap new cluster from latest snapshot

**Restore procedure**:
```bash
# Stop cluster
docker-compose -f docker-compose-admin-cluster.yaml down

# Restore snapshot to all nodes
cp snapshots/admin-snapshot-2025-10-17.db admin-01-data/raft/snapshots/
cp snapshots/admin-snapshot-2025-10-17.db admin-02-data/raft/snapshots/
cp snapshots/admin-snapshot-2025-10-17.db admin-03-data/raft/snapshots/

# Start cluster (will restore from snapshot)
docker-compose -f docker-compose-admin-cluster.yaml up -d
```

### Monitoring and Alerts

**Critical alerts**:
- `AdminNoLeader`: No leader elected for &gt;5 seconds
- `AdminLeaderFlapping`: &gt;3 leader changes in 5 minutes
- `AdminQuorumLoss`: &lt;quorum nodes healthy
- `AdminSnapshotFailure`: Snapshot creation failed

**Warning alerts**:
- `AdminHighCommitLatency`: P99 commit latency &gt;100ms
- `AdminLogGrowth`: Raft log &gt;100k entries (snapshot overdue)
- `AdminFollowerLag`: Follower &gt;1000 entries behind leader

## Alternatives Considered

### Alternative 1: Zookeeper for Coordination

**Pros**:
- Mature, battle-tested (Kafka, HBase use it)
- Rich coordination primitives (watches, locks)
- Java ecosystem support

**Cons**:
- Java dependency (JVM overhead)
- Complex operational model (ZAB protocol)
- Overkill for admin coordination (we only need leader election + replicated state)

**Verdict**: Rejected, Raft simpler and sufficient

### Alternative 2: etcd for State Storage

**Pros**:
- Production-proven (Kubernetes control plane)
- gRPC API built-in
- Supports both leader election and KV storage

**Cons**:
- External dependency (another process)
- Admin becomes stateless (all state in etcd)
- More operational complexity (monitor admin + etcd)

**Verdict**: Rejected, embedded Raft simpler for admin use case

### Alternative 3: Active-Passive Failover (Keepalived)

**Pros**:
- Simple failover model (one active, rest standby)
- No consensus protocol needed

**Cons**:
- Split-brain risk (two active admins)
- No state replication (must use shared storage)
- Slower failover (health checks + VIP migration)

**Verdict**: Rejected, Raft provides stronger consistency guarantees

### Alternative 4: No HA (Single Admin)

**Pros**:
- Simplest implementation
- No coordination overhead

**Cons**:
- Single point of failure
- Downtime during admin restarts
- Manual recovery required

**Verdict**: Rejected, high availability is a core requirement

## Resolved Design Decisions

1. **Multi-region admin clusters**: Explicit non-goal. Single-region only to avoid WAN latency and partitioning complexity. Future multi-region support would use regional clusters with eventual consistency sync, not WAN Raft.

2. **Read consistency**: Followers serve stale reads (max 200ms staleness). Three levels: stale (any node), lease-based (leader), linearizable (leader + quorum). Default per-operation consistency configured in control plane.

3. **Partition assignment storage**: NOT stored in Raft FSM. Computed on-demand via consistent hashing over proxy set. Reduces Raft log entries by ~50% and simplifies rebalancing.

4. **Client-server communication**: Clients connect to any admin node. Reads served locally, writes automatically forwarded to leader by server. Client doesn't need leader discovery logic.

## Open Questions

1. **Snapshot frequency**: 2 minutes + 8192 entries + 50MB size limit sufficient? Will tune based on production write patterns.

2. **Partition count**: 256 partitions sufficient for scale? Will re-evaluate at 1000+ proxies (currently supports ~100 proxies comfortably).

3. **SQLite integration (ADR-054)**: How does persistent storage integrate with Raft? Recommendation: Raft log drives SQLite writes, snapshots are SQLite database files (rqlite model).

## References

- [Hashicorp Raft Library](https://github.com/hashicorp/raft)
- [Raft Consensus Algorithm](https://raft.github.io/)
- [In Search of an Understandable Consensus Algorithm (Raft Paper)](https://raft.github.io/raft.pdf)
- [ADR-055: Proxy-Admin Control Plane Protocol](/adr/adr-055)
- [ADR-056: Launcher-Admin Control Plane Protocol](/adr/adr-056)
- [ADR-054: SQLite Storage for prism-admin](/adr/adr-054)
- [RFC-034: Robust Process Manager Package](/rfc/rfc-034)
- [RFC-003: Admin Interface for Prism](/rfc/rfc-003)

## Revision History

- 2025-10-18: **Major revision** - Simplified to single-region HA with read consistency levels. Key changes:
  - Explicit single-region scope (no WAN Raft complexity)
  - Added three read consistency levels (stale, lease-based, linearizable)
  - Removed partition map from FSM (computed via consistent hashing)
  - Simplified client: connect to any node, server forwards writes to leader
  - Added per-operation consistency defaults
  - Clarified follower forwarding pattern (writes only)
  - Unified AdminState struct for atomic snapshots

- 2025-10-17: Initial draft - Admin leader election with Hashicorp Raft
