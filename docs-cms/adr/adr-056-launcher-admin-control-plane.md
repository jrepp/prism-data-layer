---
date: 2025-10-15
deciders: Engineering Team
doc_uuid: 2f8a9c4e-1b3d-4a5f-9e7c-6d2f8a4b9c3e
id: adr-056
project_id: prism-data-layer
status: Accepted
tags:
- launcher
- admin
- control-plane
- grpc
- patterns
- lifecycle
title: 'ADR-056: Launcher-Admin Control Plane Protocol'
---

## Context

The pattern-launcher (prism-launcher) currently operates independently without admin coordination. This creates operational challenges:

- **Pattern Registry**: No central view of which patterns are running on which launchers
- **Pattern Distribution**: Cannot distribute patterns across launcher instances
- **Dynamic Pattern Provisioning**: Pattern deployments require manual launcher configuration
- **Health Monitoring**: No centralized view of pattern health across launchers
- **Namespace Coordination**: Launchers don't coordinate with admin on namespace assignments

Per ADR-055, prism-proxy now connects to prism-admin via control plane protocol. We need the same bidirectional gRPC control plane between prism-launcher and prism-admin for:

1. Launcher instances register with admin on startup
2. Admin tracks running patterns per launcher
3. Admin can provision/deprovision patterns dynamically
4. Launchers report pattern health via heartbeat
5. Namespace-pattern mapping coordinated by admin

## Decision

Extend the ControlPlane gRPC service (from ADR-055) to support launcher registration and pattern lifecycle management:

**Launcher Startup**:
```bash
pattern-launcher --admin-endpoint admin.prism.local:8981 --launcher-id launcher-01 --listen :7070
```

**Extended Control Plane Flows**:

1. **Launcher Registration** (launcher → admin):
   - Launcher connects on startup, sends LauncherRegistration with ID, address, capabilities
   - Admin records launcher in storage (launchers table)
   - Admin returns assigned patterns for this launcher

2. **Pattern Assignment** (admin → launcher):
   - Admin pushes PatternAssignment to launcher with pattern config
   - Includes namespace and backend slot configuration
   - Launcher validates, provisions pattern process, and activates

3. **Pattern Provisioning** (client → admin → launcher):
   - Client requests pattern deployment (e.g., via prismctl)
   - Admin selects launcher based on capacity/region
   - Admin sends PatternAssignment to launcher
   - Launcher provisions pattern, acknowledges when ready

4. **Pattern Health Heartbeat** (launcher ↔ admin):
   - Launcher sends heartbeat every 30s with pattern health
   - Reports: pattern status, memory usage, restart count, error count
   - Admin updates pattern registry with health data

5. **Pattern Deprovisioning** (admin → launcher):
   - Admin sends RevokePattern message
   - Launcher gracefully shuts down pattern (30s timeout)
   - Launcher acknowledges pattern stopped

**Protobuf Extensions** (add to ControlPlane service):

```protobuf
service ControlPlane {
  // ... existing proxy RPCs ...

  // Launcher → Admin: Register launcher on startup
  rpc RegisterLauncher(LauncherRegistration) returns (LauncherRegistrationAck);

  // Admin → Launcher: Assign pattern to launcher
  rpc AssignPattern(PatternAssignment) returns (PatternAssignmentAck);

  // Launcher → Admin: Report pattern health
  rpc LauncherHeartbeat(LauncherHeartbeat) returns (HeartbeatAck);

  // Admin → Launcher: Deprovision pattern
  rpc RevokePattern(PatternRevocation) returns (PatternRevocationAck);
}

message LauncherRegistration {
  string launcher_id = 1;           // Unique launcher identifier (launcher-01)
  string address = 2;               // Launcher gRPC address (launcher-01.prism.local:7070)
  string region = 3;                // Deployment region (us-west-2)
  string version = 4;               // Launcher version (0.1.0)
  repeated string capabilities = 5; // Supported isolation levels (none, namespace, session)
  int32 max_patterns = 6;           // Maximum concurrent patterns
  map<string, string> metadata = 7; // Custom labels
}

message LauncherRegistrationAck {
  bool success = 1;
  string message = 2;
  repeated PatternAssignment initial_patterns = 3; // Pre-assigned patterns
  int32 assigned_capacity = 4;                     // Number of pattern slots assigned
}

message PatternAssignment {
  string pattern_id = 1;            // Unique pattern identifier
  string pattern_type = 2;          // Pattern type (keyvalue, pubsub, multicast_registry)
  string namespace = 3;             // Target namespace
  string isolation_level = 4;       // Isolation level (none, namespace, session)
  PatternConfig config = 5;         // Pattern-specific configuration
  map<string, BackendConfig> backends = 6; // Backend slot configurations
  int64 version = 7;                // Config version for idempotency
}

message PatternConfig {
  map<string, string> settings = 1; // Pattern-specific settings
  int32 port = 2;                   // gRPC port for pattern
  int32 health_check_port = 3;      // HTTP health check port
  string log_level = 4;             // Logging verbosity
}

message LauncherHeartbeat {
  string launcher_id = 1;
  map<string, PatternHealth> pattern_health = 2;
  LauncherResourceUsage resources = 3;
  int64 timestamp = 4;
}

message PatternHealth {
  string status = 1;                // running, starting, stopping, failed, stopped
  int32 pid = 2;                    // Process ID
  int32 restart_count = 3;          // Number of restarts
  int32 error_count = 4;            // Cumulative error count
  int64 memory_mb = 5;              // Memory usage in MB
  int64 uptime_seconds = 6;         // Seconds since pattern started
  string last_error = 7;            // Last error message (if any)
}

message LauncherResourceUsage {
  int32 pattern_count = 1;          // Current pattern count
  int32 max_patterns = 2;           // Maximum capacity
  int64 total_memory_mb = 3;        // Total memory used by all patterns
  float cpu_percent = 4;            // CPU utilization percentage
}

message PatternRevocation {
  string launcher_id = 1;
  string pattern_id = 2;
  int32 graceful_timeout_seconds = 3; // Timeout before force kill (default 30s)
}

message PatternRevocationAck {
  bool success = 1;
  string message = 2;
  int64 stopped_at = 3;             // Unix timestamp when pattern stopped
}
```

## Rationale

**Why Extend ControlPlane Service (not separate service):**
- Single gRPC connection from launcher to admin (reuses ADR-055 infrastructure)
- Proxy and launcher share control plane concepts (registration, heartbeat, health)
- Unified admin control surface for all components
- Simpler authentication/authorization (same mTLS certs)

**Why Pattern Assignment vs Self-Provisioning:**
- Admin has global view of launcher capacity
- Admin can balance patterns across launchers
- Admin enforces namespace → launcher affinity
- Client requests go through admin (centralized policy)

**Why 30s Heartbeat Interval:**
- Matches proxy heartbeat interval (ADR-055)
- Sufficient for pattern health monitoring
- Detects failed launchers within 1 minute
- Low overhead (~33 heartbeats/hour per launcher)

**Why Graceful Timeout on Deprovision:**
- Patterns may have in-flight requests to drain
- Backend connections need graceful close
- Prevents data loss during shutdown
- Default 30s matches Kubernetes terminationGracePeriodSeconds

### Alternatives Considered

1. **Separate LauncherControl Service**
   - Pros: Clean separation, launcher-specific API
   - Cons: Two gRPC connections per launcher, more complex mTLS setup
   - Rejected because: Single control plane service is simpler

2. **Launcher Polls Admin for Assignments**
   - Pros: No admin → launcher push required
   - Cons: Higher latency, more network traffic, admin can't push urgent changes
   - Rejected because: Bidirectional gRPC enables instant pattern provisioning

3. **Pattern Assignment via Message Queue**
   - Pros: Decoupled, queue-based workflow
   - Cons: Requires Kafka/NATS dependency, eventual consistency
   - Rejected because: gRPC request-response is sufficient for control plane

4. **Static Pattern-Launcher Mapping**
   - Pros: Simple, no runtime coordination
   - Cons: Cannot rebalance patterns, no dynamic provisioning
   - Rejected because: Dynamic assignment enables horizontal scaling

## Consequences

### Positive

- **Centralized Pattern Registry**: Admin has complete view of patterns across all launchers
- **Dynamic Pattern Provisioning**: Patterns deployed via admin without launcher restarts
- **Load Balancing**: Admin distributes patterns based on launcher capacity
- **Health Monitoring**: Pattern health visible in admin UI/API
- **Namespace Coordination**: Admin ensures namespace-pattern consistency
- **Graceful Degradation**: Launcher operates independently if admin unavailable
- **Horizontal Scaling**: Add launchers, admin distributes patterns automatically

### Negative

- **Control Plane Dependency**: Launchers require admin for pattern provisioning
- **Admin as SPOF**: If admin down, cannot provision new patterns (existing continue)
- **Pattern Handoff Complexity**: Moving patterns between launchers requires coordination
- **Connection Overhead**: Each launcher maintains persistent gRPC connection
- **State Synchronization**: Admin and launcher must agree on pattern assignments

### Neutral

- Launchers can optionally run without admin (local patterns directory mode)
- Admin stores launcher state in SQLite/PostgreSQL (ADR-054 storage)
- Pattern capacity (max_patterns) configurable per launcher
- Control plane protocol versioned independently from pattern protocols

## Implementation Notes

### Launcher-Side Admin Client

Go implementation in `pkg/launcher/admin_client.go`:

```go
type LauncherAdminClient struct {
    client    pb.ControlPlaneClient
    conn      *grpc.ClientConn
    launcherID string
    address    string
    region     string
    maxPatterns int32
}

func NewLauncherAdminClient(
    adminEndpoint string,
    launcherID string,
    address string,
    region string,
    maxPatterns int32,
) (*LauncherAdminClient, error) {
    conn, err := grpc.Dial(
        adminEndpoint,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to dial admin: %w", err)
    }

    return &LauncherAdminClient{
        client:      pb.NewControlPlaneClient(conn),
        conn:        conn,
        launcherID:  launcherID,
        address:     address,
        region:      region,
        maxPatterns: maxPatterns,
    }, nil
}

func (c *LauncherAdminClient) Register(ctx context.Context) (*pb.LauncherRegistrationAck, error) {
    req := &pb.LauncherRegistration{
        LauncherId:   c.launcherID,
        Address:      c.address,
        Region:       c.region,
        Version:      version.Version,
        Capabilities: []string{"none", "namespace", "session"},
        MaxPatterns:  c.maxPatterns,
        Metadata:     map[string]string{},
    }

    return c.client.RegisterLauncher(ctx, req)
}

func (c *LauncherAdminClient) StartHeartbeatLoop(
    ctx context.Context,
    manager *procmgr.ProcessManager,
) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            c.sendHeartbeat(ctx, manager)
        }
    }
}

func (c *LauncherAdminClient) sendHeartbeat(
    ctx context.Context,
    manager *procmgr.ProcessManager,
) error {
    patternHealth := c.collectPatternHealth(manager)
    resources := c.collectResourceUsage(manager)

    req := &pb.LauncherHeartbeat{
        LauncherId:    c.launcherID,
        PatternHealth: patternHealth,
        Resources:     resources,
        Timestamp:     time.Now().Unix(),
    }

    _, err := c.client.LauncherHeartbeat(ctx, req)
    if err != nil {
        log.Warn().Err(err).Msg("Heartbeat failed")
    }
    return err
}

func (c *LauncherAdminClient) collectPatternHealth(
    manager *procmgr.ProcessManager,
) map[string]*pb.PatternHealth {
    patterns := manager.ListProcesses()
    health := make(map[string]*pb.PatternHealth)

    for _, p := range patterns {
        health[p.ID] = &pb.PatternHealth{
            Status:        string(p.State),
            Pid:           int32(p.PID),
            RestartCount:  int32(p.RestartCount),
            ErrorCount:    int32(p.ErrorCount),
            MemoryMb:      p.MemoryMB,
            UptimeSeconds: int64(time.Since(p.StartTime).Seconds()),
            LastError:     p.LastError,
        }
    }

    return health
}

func (c *LauncherAdminClient) collectResourceUsage(
    manager *procmgr.ProcessManager,
) *pb.LauncherResourceUsage {
    patterns := manager.ListProcesses()
    var totalMemory int64
    for _, p := range patterns {
        totalMemory += p.MemoryMB
    }

    return &pb.LauncherResourceUsage{
        PatternCount:  int32(len(patterns)),
        MaxPatterns:   c.maxPatterns,
        TotalMemoryMb: totalMemory,
        CpuPercent:    getCPUUsage(), // Platform-specific
    }
}
```

### Admin-Side Launcher Control

Go implementation in `cmd/prism-admin/launcher_control.go`:

```go
func (s *ControlPlaneService) RegisterLauncher(
    ctx context.Context,
    req *pb.LauncherRegistration,
) (*pb.LauncherRegistrationAck, error) {
    // Record launcher in storage
    launcher := &Launcher{
        LauncherID:  req.LauncherId,
        Address:     req.Address,
        Version:     req.Version,
        Status:      "healthy",
        MaxPatterns: req.MaxPatterns,
        LastSeen:    time.Now(),
        Metadata:    req.Metadata,
    }

    if err := s.storage.UpsertLauncher(ctx, launcher); err != nil {
        return nil, err
    }

    // Get patterns assigned to this launcher
    patterns, err := s.storage.ListPatternsByLauncher(ctx, req.LauncherId)
    if err != nil {
        return nil, err
    }

    // Convert to PatternAssignment messages
    assignments := make([]*pb.PatternAssignment, len(patterns))
    for i, p := range patterns {
        assignments[i] = &pb.PatternAssignment{
            PatternId:      p.PatternID,
            PatternType:    p.PatternType,
            Namespace:      p.Namespace,
            IsolationLevel: p.IsolationLevel,
            Config:         p.Config,
            Backends:       p.Backends,
            Version:        p.Version,
        }
    }

    return &pb.LauncherRegistrationAck{
        Success:          true,
        Message:          "Launcher registered successfully",
        InitialPatterns:  assignments,
        AssignedCapacity: int32(len(patterns)),
    }, nil
}

func (s *ControlPlaneService) AssignPattern(
    ctx context.Context,
    req *pb.PatternAssignment,
) (*pb.PatternAssignmentAck, error) {
    // Persist pattern assignment
    pattern := &Pattern{
        PatternID:      req.PatternId,
        PatternType:    req.PatternType,
        Namespace:      req.Namespace,
        IsolationLevel: req.IsolationLevel,
        Config:         req.Config,
        Backends:       req.Backends,
        Status:         "provisioning",
    }

    if err := s.storage.CreatePattern(ctx, pattern); err != nil {
        return nil, err
    }

    return &pb.PatternAssignmentAck{
        Success: true,
        Message: "Pattern assigned successfully",
    }, nil
}

func (s *ControlPlaneService) LauncherHeartbeat(
    ctx context.Context,
    req *pb.LauncherHeartbeat,
) (*pb.HeartbeatAck, error) {
    // Update launcher last_seen timestamp
    if err := s.storage.TouchLauncher(ctx, req.LauncherId); err != nil {
        log.Error().Err(err).Msg("Failed to update launcher timestamp")
    }

    // Update pattern health in storage
    for patternID, health := range req.PatternHealth {
        if err := s.storage.UpdatePatternHealth(ctx, patternID, health); err != nil {
            log.Error().Err(err).Str("pattern_id", patternID).Msg("Failed to update pattern health")
        }
    }

    return &pb.HeartbeatAck{
        Success: true,
    }, nil
}
```

### Storage Schema Extensions

Add launchers table to ADR-054 schema:

```sql
-- Launchers table
CREATE TABLE IF NOT EXISTS launchers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    launcher_id TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    version TEXT,
    status TEXT CHECK(status IN ('healthy', 'unhealthy', 'unknown')) NOT NULL DEFAULT 'unknown',
    max_patterns INTEGER NOT NULL DEFAULT 10,
    last_seen TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata TEXT -- JSON
);

-- Add launcher_id foreign key to patterns table
ALTER TABLE patterns ADD COLUMN launcher_id TEXT;
ALTER TABLE patterns ADD FOREIGN KEY (launcher_id) REFERENCES launchers(launcher_id) ON DELETE SET NULL;

-- Indexes
CREATE INDEX idx_launchers_status ON launchers(status, last_seen);
CREATE INDEX idx_patterns_launcher ON patterns(launcher_id);
```

### Launcher Configuration

Add admin endpoint to launcher config:

```yaml
admin:
  endpoint: "admin.prism.local:8981"
  launcher_id: "launcher-01"
  region: "us-west-2"
  max_patterns: 20
  heartbeat_interval: "30s"
  reconnect_backoff: "5s"

launcher:
  listen: ":7070"
  patterns_dir: "./patterns"
  log_level: "info"
```

### Graceful Fallback

If admin unavailable, launcher operates with local patterns directory:

```go
func Start(cfg *Config) error {
    // Try connecting to admin
    adminClient, err := NewLauncherAdminClient(
        cfg.Admin.Endpoint,
        cfg.Admin.LauncherID,
        cfg.Launcher.Listen,
        cfg.Admin.Region,
        cfg.Admin.MaxPatterns,
    )
    if err != nil {
        log.Warn().Err(err).Msg("Admin connection failed, using local patterns directory")
        return startWithLocalPatterns(cfg)
    }

    // Register with admin
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    ack, err := adminClient.Register(ctx)
    if err != nil {
        log.Warn().Err(err).Msg("Registration failed, using local patterns directory")
        return startWithLocalPatterns(cfg)
    }

    log.Info().
        Int("initial_patterns", len(ack.InitialPatterns)).
        Msg("Registered with admin")

    // Apply admin-provided patterns
    for _, assignment := range ack.InitialPatterns {
        if err := provisionPattern(assignment); err != nil {
            log.Error().Err(err).Str("pattern_id", assignment.PatternId).Msg("Failed to provision pattern")
        }
    }

    // Start heartbeat loop
    go adminClient.StartHeartbeatLoop(context.Background(), processManager)

    // Start launcher gRPC server
    return startGRPCServer(cfg)
}
```

### prismctl Local Integration

Update `cmd/prismctl/cmd/local.go` to use admin-connected launcher:

```go
{
    name:    "pattern-launcher",
    binary:  filepath.Join(binDir, "pattern-launcher"),
    args:    []string{
        "--admin-endpoint=localhost:8980",  // Control plane port
        "--launcher-id=launcher-01",
        "--listen=:7070",
        "--max-patterns=10",
    },
    logFile: filepath.Join(logsDir, "launcher.log"),
    delay:   2 * time.Second,
},
```

## References

- [ADR-055: Proxy-Admin Control Plane Protocol](/adr/adr-055) - Proxy registration and namespace distribution
- [ADR-054: SQLite Storage for prism-admin](/adr/adr-054) - Storage for launcher registry
- [RFC-035: Pattern Process Launcher](/rfc/rfc-035) - Pattern lifecycle management
- [MEMO-034: Pattern Process Launcher Quick Start](/memos/memo-034) - Launcher usage guide

## Revision History

- 2025-10-15: Initial draft - Launcher-admin control plane with pattern assignment
