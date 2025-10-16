---
date: 2025-10-15
deciders: Engineering Team
doc_uuid: 7e2b9f5a-3c8d-4e6f-a9b1-5f7c8e3d2a4b
id: adr-057
project_id: prism-data-layer
status: Accepted
tags:
- launcher
- refactoring
- control-plane
- architecture
- procmgr
title: 'ADR-057: Refactor pattern-launcher to prism-launcher as General Control Plane Launcher'
---

## Context

The current `pattern-launcher` is narrowly focused on pattern process lifecycle management (RFC-035). However, the control plane architecture (ADR-055, ADR-056) reveals the need for a more general launcher capable of managing multiple types of Prism components:

**Current Limitations**:
- Name "pattern-launcher" implies it only launches patterns
- Architecture assumes all managed processes are pattern implementations
- Process management logic tightly coupled to pattern-specific concepts
- Cannot easily launch other Prism components (proxies, backends, utilities)
- prismctl local command manually launches each component separately

**Emerging Requirements**:
- Launch prism-proxy instances dynamically
- Launch backend drivers as separate processes (not just patterns)
- Launch auxiliary services (monitoring agents, log collectors)
- Unified process lifecycle for all Prism components
- Control plane coordination for all managed processes (not just patterns)

**Control Plane Evolution**:
- ADR-055: Proxies register with admin via control plane
- ADR-056: Pattern-launcher registers with admin via control plane
- Need: General launcher that can register ANY managed process type with admin
- Goal: Single launcher binary managing entire Prism stack

## Decision

Refactor `pattern-launcher` to `prism-launcher` as a general-purpose control plane launcher capable of managing any Prism component:

**Naming Changes**:
- Binary: `pattern-launcher` → `prism-launcher`
- Package: `pkg/launcher` (existing) → `pkg/launcher` (generalized)
- Process types: `Pattern` → `ManagedProcess` with type field

**Architecture Changes**:

1. **Process Type Abstraction**:
```go
type ProcessType string

const (
    ProcessTypePattern  ProcessType = "pattern"
    ProcessTypeProxy    ProcessType = "proxy"
    ProcessTypeBackend  ProcessType = "backend"
    ProcessTypeUtility  ProcessType = "utility"
)

type ManagedProcess struct {
    ID             string
    Type           ProcessType
    Binary         string
    Args           []string
    Env            map[string]string
    Config         interface{}  // Type-specific config
    IsolationLevel IsolationLevel
    HealthCheck    HealthCheckConfig
    RestartPolicy  RestartPolicy
}
```

2. **Unified Control Plane Registration**:
```go
// Register launcher with admin (not just pattern-launcher)
type LauncherRegistration struct {
    LauncherID      string
    Address         string
    Region          string
    Capabilities    []string  // ["pattern", "proxy", "backend"]
    MaxProcesses    int32     // Not just max_patterns
    ProcessTypes    []string  // Types this launcher can manage
}
```

3. **Process Assignment Protocol**:
```go
// Admin assigns any process type, not just patterns
type ProcessAssignment struct {
    ProcessID   string
    ProcessType ProcessType  // pattern, proxy, backend, utility
    Namespace   string
    Config      ProcessConfig  // Type-specific configuration
    Slots       map[string]BackendConfig  // Only for patterns
}

type ProcessConfig struct {
    // Common fields
    Binary      string
    Args        []string
    Env         map[string]string
    Port        int32
    HealthPort  int32

    // Type-specific payloads
    PatternConfig  *PatternConfig   // Non-nil if ProcessType=pattern
    ProxyConfig    *ProxyConfig     // Non-nil if ProcessType=proxy
    BackendConfig  *BackendConfig   // Non-nil if ProcessType=backend
}
```

4. **Process Manager Generalization**:
```go
// pkg/procmgr stays mostly the same but concepts generalize
type ProcessManager struct {
    processes      map[string]*ManagedProcess  // Not just patterns
    isolationMgr   *isolation.IsolationManager
    healthChecker  *HealthChecker
}

func (pm *ProcessManager) Launch(proc *ManagedProcess) error {
    // Works for any process type
    // Pattern-specific logic only fires if proc.Type == ProcessTypePattern
}
```

5. **Launcher Command Structure**:
```bash
prism-launcher \
  --admin-endpoint admin.prism.local:8981 \
  --launcher-id launcher-01 \
  --listen :7070 \
  --max-processes 50 \
  --capabilities pattern,proxy,backend \
  --region us-west-2
```

**Process Type Capabilities**:

| Process Type | Description | Examples |
|--------------|-------------|----------|
| **pattern** | Pattern implementations | keyvalue-runner, pubsub-runner, multicast-registry |
| **proxy** | Prism proxy instances | prism-proxy (control + data plane) |
| **backend** | Backend driver processes | redis-driver, kafka-driver, nats-driver |
| **utility** | Auxiliary services | log-collector, metrics-exporter, health-monitor |

**Backward Compatibility**:
- Existing pattern-launcher configs continue to work
- ProcessType defaults to "pattern" if not specified
- Pattern-specific fields (slots, isolation) only apply when type=pattern
- Admin can gradually migrate to new ProcessAssignment messages

## Rationale

**Why Generalize Beyond Patterns:**
- prismctl local needs unified launcher for entire stack (admin, proxy, patterns)
- Launching proxy instances dynamically enables horizontal scaling
- Backend drivers may run as separate processes (not in-proxy)
- Monitoring/utility processes need same lifecycle management
- Single launcher binary simplifies deployment

**Why Keep pkg/procmgr Intact:**
- Process manager is already general-purpose (manages any process)
- Isolation levels work for any process type (not just patterns)
- Health checks, restarts, circuit breakers apply universally
- Only process *assignment* logic needs generalization

**Why Type-Specific Config Payloads:**
- Patterns need slot configurations (backends for pattern slots)
- Proxies need admin-endpoint, control-port, data-port
- Backends need connection strings, credentials
- Type-safe configs prevent mismatched assignments

**Why Single Binary (not multiple launchers):**
- Simplifies deployment (one launcher, many process types)
- Unified control plane protocol (not pattern-specific)
- Easier operational reasoning (one launcher process to monitor)
- Enables mixed workloads (patterns + proxies + backends on same launcher)

### Alternatives Considered

1. **Separate Launchers per Process Type**
   - pattern-launcher for patterns
   - proxy-launcher for proxies
   - backend-launcher for backends
   - Pros: Clean separation, type-specific code
   - Cons: 3+ binaries, 3+ control plane connections, operational complexity
   - Rejected because: Single launcher is simpler

2. **Keep pattern-launcher Name, Generalize Internally**
   - Pros: No renaming required
   - Cons: Misleading name (doesn't launch only patterns), confusing documentation
   - Rejected because: Name should reflect capability

3. **Launcher Plugins (Launcher launches launchers)**
   - Pros: Extensible, type-specific launch logic pluggable
   - Cons: Over-engineered, unnecessary indirection
   - Rejected because: Process types are finite and known

4. **Admin Directly Launches Processes (No Launcher)**
   - Pros: Simpler control plane (no launcher)
   - Cons: Admin needs SSH/exec access to hosts, security risk, no local process management
   - Rejected because: Launcher provides local process lifecycle management

## Consequences

### Positive

- **Unified Process Management**: Single launcher for all Prism components
- **Simplified Deployment**: One binary instead of multiple launchers
- **Flexible Workloads**: Mix patterns, proxies, backends on same launcher
- **Control Plane Simplicity**: One registration protocol for all process types
- **prismctl local Integration**: Single launcher manages entire local stack
- **Horizontal Scaling**: Admin can launch proxy instances dynamically
- **Backend Process Support**: Backend drivers can run as managed processes
- **Operational Visibility**: All processes visible in admin UI/API

### Negative

- **Increased Complexity**: ProcessConfig becomes type-discriminated union
- **Backward Compatibility**: Must maintain pattern-launcher compatibility
- **Testing Surface**: Must test all process types (patterns, proxies, backends)
- **Type Safety**: Config type mismatches possible (pattern config sent to proxy)
- **Documentation**: Must document all supported process types

### Neutral

- Binary renamed from pattern-launcher → prism-launcher
- ProcessType enum extensible (add new types in future)
- Admin must validate ProcessType before assignment
- Launcher capabilities advertised in registration (not all launchers support all types)

## Implementation Notes

### Phase 1: Rename and Generalize Types (Week 1)

1. Rename binary:
```bash
# Makefile
build/binaries/prism-launcher: pkg/launcher/*.go cmd/prism-launcher/*.go
	go build -o $@ ./cmd/prism-launcher
```

2. Introduce ProcessType enum:
```go
// pkg/launcher/types.go
type ProcessType string

const (
    ProcessTypePattern  ProcessType = "pattern"
    ProcessTypeProxy    ProcessType = "proxy"
    ProcessTypeBackend  ProcessType = "backend"
    ProcessTypeUtility  ProcessType = "utility"
)
```

3. Rename Process → ManagedProcess:
```go
// pkg/launcher/process.go
type ManagedProcess struct {
    ID             string
    Type           ProcessType  // NEW
    Binary         string
    Args           []string
    Config         ProcessConfig  // Generalized
    // ... rest stays same
}
```

### Phase 2: Generalize Control Plane Protocol (Week 2)

Update ADR-056 protobuf messages:

```protobuf
message LauncherRegistration {
  string launcher_id = 1;
  string address = 2;
  repeated string capabilities = 3;  // ["pattern", "proxy", "backend"]
  int32 max_processes = 4;           // Renamed from max_patterns
  repeated string process_types = 5; // Process types this launcher supports
}

message ProcessAssignment {
  string process_id = 1;
  string process_type = 2;           // "pattern", "proxy", "backend", "utility"
  string namespace = 3;
  ProcessConfig config = 4;
}

message ProcessConfig {
  // Common
  string binary = 1;
  repeated string args = 2;
  map<string, string> env = 3;
  int32 port = 4;
  int32 health_port = 5;

  // Type-specific configs
  PatternConfig pattern = 10;
  ProxyConfig proxy = 11;
  BackendConfig backend = 12;
  UtilityConfig utility = 13;
}

message PatternConfig {
  string pattern_type = 1;           // keyvalue, pubsub, etc.
  string isolation_level = 2;        // none, namespace, session
  map<string, BackendConfig> slots = 3;
}

message ProxyConfig {
  string admin_endpoint = 1;
  int32 control_port = 2;
  int32 data_port = 3;
  string proxy_id = 4;
}

message BackendConfig {
  string backend_type = 1;           // redis, kafka, nats, postgres
  string connection_string = 2;
  map<string, string> credentials = 3;
}

message UtilityConfig {
  string utility_type = 1;           // log-collector, metrics-exporter
  map<string, string> settings = 2;
}
```

### Phase 3: Update Process Manager (Week 2)

Minimal changes required:

```go
// pkg/procmgr/process_manager.go

func (pm *ProcessManager) Launch(proc *ManagedProcess) error {
    // Validate process type
    if !isValidProcessType(proc.Type) {
        return fmt.Errorf("unsupported process type: %s", proc.Type)
    }

    // Type-specific validation
    switch proc.Type {
    case ProcessTypePattern:
        if err := validatePatternConfig(proc.Config.PatternConfig); err != nil {
            return err
        }
    case ProcessTypeProxy:
        if err := validateProxyConfig(proc.Config.ProxyConfig); err != nil {
            return err
        }
    // ... other types
    }

    // Existing launch logic works for all types
    return pm.launchProcess(proc)
}
```

### Phase 4: Update prismctl local (Week 3)

Simplify to use single prism-launcher:

```go
// cmd/prismctl/cmd/local.go
components := []struct {
    name    string
    binary  string
    args    []string
}{
    {
        name:   "prism-admin",
        binary: filepath.Join(binDir, "prism-admin"),
        args:   []string{"serve", "--port=8980"},
    },
    {
        name:   "prism-launcher",
        binary: filepath.Join(binDir, "prism-launcher"),
        args:   []string{
            "--admin-endpoint=localhost:8980",
            "--launcher-id=launcher-01",
            "--listen=:7070",
            "--max-processes=50",
            "--capabilities=pattern,proxy,backend",
        },
    },
}

// After launcher starts, admin can dynamically provision:
// - 2 proxy instances (proxy-01, proxy-02)
// - keyvalue pattern with memstore backend
// - Any other components
```

### Phase 5: Admin-Side Assignment Logic (Week 3)

```go
// cmd/prism-admin/process_provisioner.go

func (s *ControlPlaneService) AssignProcess(
    ctx context.Context,
    req *pb.ProcessAssignment,
) (*pb.ProcessAssignmentAck, error) {
    // Select launcher based on capabilities
    launchers, err := s.storage.ListLaunchersByCapability(ctx, req.ProcessType)
    if err != nil {
        return nil, err
    }

    if len(launchers) == 0 {
        return nil, fmt.Errorf("no launchers support process type: %s", req.ProcessType)
    }

    // Choose launcher with most available capacity
    launcher := selectLauncherByCapacity(launchers)

    // Send assignment to launcher
    if err := s.sendProcessAssignment(launcher.LauncherID, req); err != nil {
        return nil, err
    }

    return &pb.ProcessAssignmentAck{
        Success:     true,
        LauncherId:  launcher.LauncherID,
        ProcessId:   req.ProcessId,
    }, nil
}
```

### Phase 6: Documentation Updates (Week 4)

- Update RFC-035 to reflect generalized launcher
- Update MEMO-034 quick start guide
- Create new MEMO for prism-launcher usage patterns
- Update prismctl local documentation
- Migration guide from pattern-launcher → prism-launcher

## Migration Strategy

**Backward Compatibility**:
1. Keep `pattern-launcher` as symlink to `prism-launcher` for 1-2 releases
2. Default ProcessType to "pattern" if not specified
3. Admin recognizes both old PatternAssignment and new ProcessAssignment
4. Gradual migration: existing deployments continue working

**Migration Steps**:
1. Release prism-launcher with backward compatibility
2. Update admin to support both protocols
3. Documentation shows prism-launcher (note pattern-launcher deprecated)
4. After 2 releases, remove pattern-launcher symlink

## References

- [ADR-055: Proxy-Admin Control Plane Protocol](/adr/adr-055) - Proxy registration
- [ADR-056: Launcher-Admin Control Plane Protocol](/adr/adr-056) - Pattern-launcher registration
- [RFC-035: Pattern Process Launcher](/rfc/rfc-035) - Original pattern-launcher design
- [MEMO-034: Pattern Process Launcher Quick Start](/memos/memo-034) - Usage guide
- [pkg/procmgr](https://github.com/jrepp/prism-data-layer/tree/main/pkg/procmgr) - Process manager implementation

## Revision History

- 2025-10-15: Initial draft - Refactoring pattern-launcher to prism-launcher
