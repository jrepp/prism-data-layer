---
title: "MEMO-033: Process Isolation and the Bulkhead Pattern"
author: "Prism Team"
created: "2025-10-15"
updated: "2025-10-15"
tags: [isolation, bulkhead, procmgr, patterns, reliability, multi-tenancy, fault-isolation]
id: memo-033
project_id: "prism"
doc_uuid: "a9c3f05a-04d7-43c4-a949-dbdff8fb2ce0"
---

# MEMO-033: Process Isolation and the Bulkhead Pattern

## Executive Summary

This memo documents the implementation of process isolation capabilities in Prism, using the bulkhead pattern to prevent failures in one namespace or session from affecting others. The `isolation` package builds on `procmgr` (RFC-034) to provide three isolation levels: None, Namespace, and Session.

**Key Achievement**: Pattern tests can now run with configurable isolation, ensuring that requests for different namespaces or sessions are routed to separate processes, preventing cascading failures and improving multi-tenant reliability.

## Background

### Problem Statement

When running acceptance tests or production workloads with multiple tenants or sessions, we need to prevent:

1. **Failure propagation**: A crash in one tenant's process affecting other tenants
2. **Resource contention**: One tenant consuming all available resources
3. **Security leaks**: Memory sharing or state leakage between tenants
4. **Debugging complexity**: Difficulty isolating which tenant caused a failure

### Prior Art: Bulkhead Pattern

The bulkhead pattern (named after ship compartments that prevent sinking if one compartment floods) isolates system components so that failures in one component don't cascade to others.

**Key Principle**: Segment resources into isolated pools so that exhaustion or failure of one pool doesn't affect others.

## Architecture

### Components

```text
┌─────────────────────────────────────────────────────────┐
│                  Pattern Runner Framework                │
│  (tests/acceptance/framework/)                           │
└───────────────────┬─────────────────────────────────────┘
                    │
                    │ uses
                    ▼
┌─────────────────────────────────────────────────────────┐
│               Isolation Package                          │
│  (pkg/isolation/)                                        │
│                                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │         IsolationManager                         │   │
│  │  - GetOrCreateProcess()                          │   │
│  │  - TerminateProcess()                            │   │
│  │  - ListProcesses()                               │   │
│  │  - Health()                                      │   │
│  └─────────────────┬───────────────────────────────┘   │
│                    │                                     │
│                    │ manages                             │
│                    ▼                                     │
│  ┌─────────────────────────────────────────────────┐   │
│  │   IsolationKey → ProcessID Mapping               │   │
│  │                                                   │   │
│  │   Level: None       → "shared"                   │   │
│  │   Level: Namespace  → "ns:<namespace>"           │   │
│  │   Level: Session    → "session:<session>"        │   │
│  └─────────────────┬───────────────────────────────┘   │
└────────────────────┼───────────────────────────────────┘
                     │
                     │ uses
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Process Manager (procmgr)                   │
│  (pkg/procmgr/ - RFC-034)                                │
│                                                          │
│  - Robust lifecycle management                           │
│  - Work queue with exponential backoff                   │
│  - Health monitoring and metrics                         │
│  - Graceful termination                                  │
└─────────────────────────────────────────────────────────┘
```

### Three Isolation Levels

#### 1. IsolationNone (Shared)

All requests share a single process. Useful for:
- Local development
- Simple test scenarios
- Maximum resource efficiency

```text
Request 1 (ns:A, session:1) ──┐
Request 2 (ns:B, session:2) ──┼─→ [Process: shared]
Request 3 (ns:A, session:3) ──┘
```

**Process ID**: `"shared"`

**Characteristics**:
- Lowest overhead (one process for all requests)
- No isolation (failures affect all requests)
- Fastest (no process creation/switching)

#### 2. IsolationNamespace (Tenant Isolation)

Each namespace gets its own process. Useful for:
- Multi-tenant systems
- SaaS applications
- Isolating customer workloads

```text
Request 1 (ns:A, session:1) ──┐
Request 2 (ns:A, session:2) ──┼─→ [Process: ns:A]
Request 3 (ns:B, session:1) ────→ [Process: ns:B]
```

**Process ID**: `"ns:<namespace>"`

**Characteristics**:
- One process per namespace/tenant
- Failures in namespace A don't affect namespace B
- Resource limits can be applied per tenant
- Ideal for multi-tenant production deployments

#### 3. IsolationSession (Session Isolation)

Each session gets its own process. Useful for:
- User session isolation
- Per-connection isolation
- Maximum security requirements

```text
Request 1 (ns:A, session:1) ──┐
Request 2 (ns:B, session:1) ──┼─→ [Process: session:1]
Request 3 (ns:A, session:2) ────→ [Process: session:2]
```

**Process ID**: `"session:<session>"`

**Characteristics**:
- One process per session
- Highest isolation (even same namespace gets different processes)
- Higher overhead (more processes)
- Ideal for high-security scenarios

## Implementation Details

### Core Types

```go
// IsolationLevel defines how requests are isolated
type IsolationLevel int

const (
    IsolationNone      IsolationLevel = iota
    IsolationNamespace
    IsolationSession
)

// IsolationKey identifies the namespace and session
type IsolationKey struct {
    Namespace string
    Session   string
}

// ProcessConfig holds configuration for a managed process
type ProcessConfig struct {
    Key            IsolationKey
    BackendConfig  interface{}
    GracePeriodSec int64
}

// IsolationManager manages process isolation
type IsolationManager struct {
    level  IsolationLevel
    pm     *procmgr.ProcessManager
    syncer ProcessSyncer
    // ... internal state
}
```

### Process ID Generation

The `IsolationKey.ProcessID()` method generates process IDs based on the isolation level:

```go
func (key IsolationKey) ProcessID(level IsolationLevel) procmgr.ProcessID {
    switch level {
    case IsolationNone:
        return "shared"
    case IsolationNamespace:
        return procmgr.ProcessID("ns:" + key.Namespace)
    case IsolationSession:
        return procmgr.ProcessID("session:" + key.Session)
    default:
        return "shared"
    }
}
```

This ensures requests are routed to the correct isolated process.

### Process Lifecycle

```text
1. Request arrives with (namespace, session)
   ↓
2. Generate ProcessID from IsolationKey
   ↓
3. Check if process already exists
   ↓
   ├─ Yes → Return existing ProcessHandle
   │         (reuse process)
   │
   └─ No  → Create new process
             ├─ Call ProcessSyncer.SyncProcess()
             ├─ Wait for process to start
             └─ Return ProcessHandle
```

### Integration with procmgr

The `isolation` package delegates to `procmgr` for:

1. **Lifecycle management**: Starting, stopping, restarting processes
2. **Health monitoring**: Tracking process health and errors
3. **Work queue**: Retry scheduling with exponential backoff
4. **Metrics**: Prometheus metrics collection
5. **Graceful shutdown**: Orderly termination with grace periods

This reuses all the robustness guarantees from RFC-034.

## Pattern Runner Integration

### Framework Enhancement

The pattern runner framework now supports isolated test execution via `RunIsolatedPatternTests()`:

```go
// Configure namespace isolation
opts := IsolatedTestOptions{
    IsolationConfig: IsolationConfig{
        Level:          isolation.IsolationNamespace,
        Namespace:      "default",
        GracePeriodSec: 10,
    },
    // Generate unique namespace per test
    NamespaceGenerator: func(backendName, testName string) string {
        return fmt.Sprintf("%s-%s", backendName, testName)
    },
}

// Run tests with isolation
RunIsolatedPatternTests(t, PatternKeyValueBasic, tests, syncer, opts)
```

### IsolatedBackend Wrapper

The `IsolatedBackend` type wraps a standard `Backend` with isolation management:

```go
type IsolatedBackend struct {
    Backend
    isolationMgr *isolation.IsolationManager
    config       IsolationConfig
}

// SetupIsolated creates an isolated process for the test
func (ib *IsolatedBackend) SetupIsolated(t *testing.T, ctx context.Context, key isolation.IsolationKey) (driver interface{}, cleanup func())
```

This allows existing tests to gain isolation with minimal code changes.

## Usage Examples

### Example 1: No Isolation (Development)

```go
config := IsolationConfig{
    Level:          isolation.IsolationNone,
    Namespace:      "dev",
    Session:        "local",
    GracePeriodSec: 5,
}

im := isolation.NewIsolationManager(config.Level, syncer)

// All requests use process ID "shared"
handle1, _ := im.GetOrCreateProcess(ctx, key1, config1)
handle2, _ := im.GetOrCreateProcess(ctx, key2, config2)

assert.Equal(t, "shared", handle1.ID)
assert.Equal(t, "shared", handle2.ID)
```

### Example 2: Namespace Isolation (Multi-Tenant)

```go
config := IsolationConfig{
    Level:          isolation.IsolationNamespace,
    GracePeriodSec: 10,
}

im := isolation.NewIsolationManager(config.Level, syncer)

key1 := isolation.IsolationKey{Namespace: "tenant-a", Session: "s1"}
key2 := isolation.IsolationKey{Namespace: "tenant-b", Session: "s2"}

handle1, _ := im.GetOrCreateProcess(ctx, key1, config1)
handle2, _ := im.GetOrCreateProcess(ctx, key2, config2)

// Different namespaces get different processes
assert.Equal(t, "ns:tenant-a", handle1.ID)
assert.Equal(t, "ns:tenant-b", handle2.ID)
assert.NotEqual(t, handle1.ID, handle2.ID)
```

### Example 3: Session Isolation (High Security)

```go
config := IsolationConfig{
    Level:          isolation.IsolationSession,
    GracePeriodSec: 10,
}

im := isolation.NewIsolationManager(config.Level, syncer)

key1 := isolation.IsolationKey{Namespace: "tenant-a", Session: "session-1"}
key2 := isolation.IsolationKey{Namespace: "tenant-a", Session: "session-2"}

handle1, _ := im.GetOrCreateProcess(ctx, key1, config1)
handle2, _ := im.GetOrCreateProcess(ctx, key2, config2)

// Different sessions get different processes (even same namespace)
assert.Equal(t, "session:session-1", handle1.ID)
assert.Equal(t, "session:session-2", handle2.ID)
assert.NotEqual(t, handle1.ID, handle2.ID)
```

## Testing

### Test Coverage

**Package**: `pkg/isolation/`

**Tests**: 10 tests, all passing (1.155s runtime)

```text
✅ TestIsolationLevel_String - String representation of levels
✅ TestIsolationKey_ProcessID - ProcessID generation
✅ TestIsolationManager_None - Shared process behavior
✅ TestIsolationManager_Namespace - Namespace isolation
✅ TestIsolationManager_Session - Session isolation
✅ TestIsolationManager_GetProcess - Process retrieval
✅ TestIsolationManager_TerminateProcess - Graceful termination
✅ TestIsolationManager_ListProcesses - Listing all processes
✅ TestIsolationManager_Health - Health reporting
✅ TestIsolationManager_ConcurrentAccess - 10 concurrent goroutines
```

### Framework Integration Tests

**File**: `tests/acceptance/framework/isolation_example_test.go`

Demonstrates:
1. No isolation (all tests share one process)
2. Namespace isolation (one process per namespace)
3. Session isolation (one process per session)
4. Health monitoring and reporting

## Performance Characteristics

### Process Creation Overhead

| Isolation Level | Processes Created | Overhead         | Best For          |
|-----------------|-------------------|------------------|-------------------|
| None            | 1                 | Minimal (&lt;1ms)    | Development       |
| Namespace       | N (tenants)       | Low (~10-50ms)   | Multi-tenant SaaS |
| Session         | M (sessions)      | Medium (~10-50ms)| High security     |

**Measured**: Process creation takes ~10ms (includes SyncProcess call)

### Memory Footprint

- **None**: Single process memory footprint
- **Namespace**: N × (process memory + backend connection pool)
- **Session**: M × (process memory + backend connection pool)

**Recommendation**: Use Namespace isolation for most production scenarios (balances isolation and resource usage)

### Failure Isolation

| Scenario                        | None | Namespace | Session |
|---------------------------------|------|-----------|---------|
| Tenant A crashes                | ❌   | ✅        | ✅      |
| Session 1 OOMs                  | ❌   | ❌        | ✅      |
| Tenant A exhausts connections   | ❌   | ✅        | ✅      |
| Memory leak in shared code      | ❌   | ❌        | ❌      |

## Health Monitoring

### Health Metrics

The `IsolationManager.Health()` method returns:

```go
type HealthCheck struct {
    TotalProcesses       int
    RunningProcesses     int
    TerminatingProcesses int
    FailedProcesses      int
    WorkQueueDepth       int
    Processes            map[ProcessID]ProcessHealth
}

type ProcessHealth struct {
    State        ProcessState
    Healthy      bool
    Uptime       time.Duration
    LastSync     time.Time
    ErrorCount   int
    RestartCount int
}
```

### Health Reporter

The `IsolationHealthReporter` aggregates health across multiple backends:

```go
reporter := NewIsolationHealthReporter()
reporter.Register("Redis", isolatedRedis)
reporter.Register("NATS", isolatedNATS)

health := reporter.GetHealth()
report := reporter.Report() // Human-readable report
```

### Example Health Report

```text
=== Isolation Health Report ===

Backend: Redis
  Total Processes: 3
  Running: 3
  Terminating: 0
  Failed: 0
  Work Queue Depth: 0

  Process ns:tenant-a:
    State: Syncing
    Healthy: true
    Uptime: 5m23s
    Last Sync: 2025-10-15 08:30:00
    Errors: 0
    Restarts: 0

  Process ns:tenant-b:
    State: Syncing
    Healthy: true
    Uptime: 3m12s
    Last Sync: 2025-10-15 08:28:00
    Errors: 0
    Restarts: 0
```

## Prometheus Metrics Integration

Since `isolation` builds on `procmgr`, all procmgr Prometheus metrics are available:

### Metrics Available

1. **procmgr_process_state_transitions_total** - Process state changes (labels: process_id, from_state, to_state)
2. **procmgr_process_sync_duration_seconds** - Sync duration histogram
3. **procmgr_process_termination_duration_seconds** - Termination duration histogram
4. **procmgr_process_errors_total** - Error counter (labels: process_id, error_type)
5. **procmgr_process_restarts_total** - Restart counter
6. **procmgr_work_queue_depth** - Current queue depth gauge
7. **procmgr_work_queue_adds_total** - Items added to queue
8. **procmgr_work_queue_retries_total** - Retry attempts
9. **procmgr_work_queue_backoff_duration_seconds** - Backoff duration histogram

### Example PromQL Queries

```promql
# Number of isolated processes per namespace
count by (process_id) (procmgr_process_state_transitions_total{process_id=~"ns:.*"})

# Average sync duration per namespace
rate(procmgr_process_sync_duration_seconds_sum{process_id=~"ns:.*"}[5m])
/ rate(procmgr_process_sync_duration_seconds_count{process_id=~"ns:.*"}[5m])

# Error rate by namespace
rate(procmgr_process_errors_total{process_id=~"ns:.*"}[5m])

# Processes in unhealthy state
procmgr_process_state_transitions_total{to_state!="Syncing"}
```

## Production Deployment Considerations

### When to Use Each Isolation Level

#### IsolationNone

**Use Cases**:
- Single-tenant deployments
- Development environments
- Non-critical workloads
- Maximum performance requirements

**Risks**:
- No fault isolation
- Resource contention
- Security boundary only at application level

#### IsolationNamespace (Recommended)

**Use Cases**:
- Multi-tenant SaaS applications
- Enterprise deployments with multiple customers
- Compliance requirements for tenant isolation
- Predictable tenant workloads

**Benefits**:
- Strong fault isolation per tenant
- Resource limits per tenant
- Clear billing boundaries
- Reasonable overhead

**Recommended Configuration**:
```go
IsolationConfig{
    Level:          isolation.IsolationNamespace,
    GracePeriodSec: 30,
    ProcessOptions: []procmgr.Option{
        procmgr.WithResyncInterval(60 * time.Second),
        procmgr.WithBackOffPeriod(10 * time.Second),
        procmgr.WithMetricsCollector(prometheusMetrics),
    },
}
```

#### IsolationSession

**Use Cases**:
- High-security requirements (PCI, HIPAA)
- Untrusted user input
- Per-user resource limits
- Short-lived sessions

**Considerations**:
- Higher resource overhead
- Process creation latency
- More complex lifecycle management
- Best with session pooling/reuse

### Resource Limits

Consider setting process-level resource limits using cgroups or systemd:

```go
// In ProcessSyncer.SyncProcess(), configure limits
cmd := exec.CommandContext(ctx, "systemd-run", "--scope",
    "--property=MemoryMax=512M",
    "--property=CPUQuota=50%",
    "./backend-process")
```

### Monitoring Recommendations

1. **Alert on high process counts** (indicates scaling issues)
2. **Alert on failed processes** (indicates backend instability)
3. **Track process uptime distribution** (identify restart patterns)
4. **Monitor work queue depth** (indicates scheduling bottlenecks)

## Future Enhancements

### 1. Dynamic Isolation Level Switching

Allow changing isolation level at runtime based on load:

```go
// Promote from None to Namespace under high load
if currentLoad > threshold {
    im.SetLevel(isolation.IsolationNamespace)
}
```

### 2. Process Pooling

Pre-warm process pools for faster request handling:

```go
// Pre-create processes for known tenants
pool := isolation.NewProcessPool(im, []string{"tenant-a", "tenant-b", "tenant-c"})
```

### 3. Adaptive Isolation

Automatically isolate misbehaving tenants:

```go
// If tenant-a has >10 errors, isolate it
if errorRate["tenant-a"] > 10 {
    im.IsolateTenant("tenant-a", isolation.IsolationSession)
}
```

### 4. Cross-Region Isolation

Extend isolation to route requests to region-specific processes:

```go
type IsolationKey struct {
    Namespace string
    Session   string
    Region    string  // NEW
}
```

## Comparison with Other Approaches

### vs. Separate Deployments

| Aspect            | Process Isolation       | Separate Deployments     |
|-------------------|-------------------------|--------------------------|
| Overhead          | Low (shared host)       | High (separate hosts)    |
| Deployment time   | Instant                 | Minutes                  |
| Cost              | Shared infrastructure   | Per-deployment cost      |
| Isolation         | Process-level           | VM/container-level       |
| Best for          | Many small tenants      | Few large tenants        |

### vs. Thread Pools

| Aspect            | Process Isolation       | Thread Pools             |
|-------------------|-------------------------|--------------------------|
| Crash isolation   | ✅ Full                 | ❌ Shared process space  |
| Memory isolation  | ✅ Separate address space| ❌ Shared heap           |
| Resource limits   | ✅ OS-level cgroups     | ❌ Application-level     |
| Overhead          | Medium                  | Low                      |
| Best for          | Untrusted workloads     | Trusted internal services|

### vs. Kubernetes Namespaces

| Aspect            | Process Isolation       | K8s Namespaces           |
|-------------------|-------------------------|--------------------------|
| Granularity       | Per request             | Per deployment           |
| Startup time      | &lt;50ms                    | Seconds to minutes       |
| Resource overhead | Single binary           | Full pod + sidecar       |
| Orchestration     | Built-in                | Requires K8s             |
| Best for          | Dynamic isolation       | Static tenant boundaries |

## Conclusion

The `isolation` package provides a flexible, production-ready solution for process isolation in Prism. By building on `procmgr`, it inherits robust lifecycle management, health monitoring, and metrics collection while adding multi-tenant isolation capabilities.

**Key Takeaways**:

1. ✅ **Three isolation levels** (None, Namespace, Session) cover different use cases
2. ✅ **Bulkhead pattern** prevents cascading failures across tenants/sessions
3. ✅ **Built on RFC-034** (procmgr) for robust process management
4. ✅ **Pattern runner integration** enables isolated acceptance testing
5. ✅ **Comprehensive testing** (10 tests, all passing)
6. ✅ **Production-ready** with health monitoring and Prometheus metrics

**Recommended Default**: `IsolationNamespace` for multi-tenant production deployments (balances isolation and resource efficiency)

## References

1. [RFC-034: Robust Process Manager Package](/rfc/rfc-034)
2. [pkg/procmgr/README.md](https://github.com/jrepp/prism-data-layer/tree/main/pkg/procmgr/README.md)
3. [pkg/isolation/README.md](https://github.com/jrepp/prism-data-layer/tree/main/pkg/isolation/README.md)
4. [Bulkhead Pattern - Microsoft Azure](https://docs.microsoft.com/en-us/azure/architecture/patterns/bulkhead)
5. [Release It! - Michael Nygard](https://pragprog.com/titles/mnee2/release-it-second-edition/) (Bulkhead pattern origin)

## Appendix A: Complete Code Example

See `tests/acceptance/framework/isolation_example_test.go` for complete runnable examples demonstrating all three isolation levels.

## Appendix B: Performance Benchmark Results

```text
BenchmarkIsolationManager_GetOrCreateProcess_None-8       100000    10523 ns/op
BenchmarkIsolationManager_GetOrCreateProcess_Namespace-8   50000    28742 ns/op
BenchmarkIsolationManager_GetOrCreateProcess_Session-8     50000    29183 ns/op
BenchmarkIsolationManager_TerminateProcess-8               30000    45891 ns/op
```

**Interpretation**:
- Process creation: ~10-30ms depending on isolation level
- Termination: ~45ms (includes graceful shutdown)
- Acceptable for test frameworks and production request routing
