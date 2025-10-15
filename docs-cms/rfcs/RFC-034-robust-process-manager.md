---
title: "RFC-034: Robust Process Manager Package Inspired by Kubelet"
status: Proposed
author: Claude Code
created: 2025-10-14
updated: 2025-10-14
tags: [process-management, concurrency, lifecycle, kubelet, reliability]
id: rfc-034
project_id: prism-data-access
doc_uuid: 824358fe-8a2d-4838-8976-ae447cd697b4
---

# RFC-034: Robust Process Manager Package Inspired by Kubelet

## Summary

This RFC proposes a robust process management package for Prism inspired by Kubernetes Kubelet's pod worker system. The package will manage 0 or more concurrent processes with proper lifecycle management, graceful termination, state tracking, and error recovery. While Kubelet manages container/pod lifecycles, our package will manage backend driver process lifecycles (plugins, adapters, workers) with similar guarantees around state transitions, termination handling, and resource cleanup.

## Motivation

Prism requires robust process management for:

1. **Backend Driver Processes**: Each backend driver (Redis, NATS, Kafka, PostgreSQL, MemStore) runs as a managed process with start, sync, terminating, and terminated phases
2. **Plugin Lifecycle**: Hot-reload capability requires graceful termination and restart of plugin processes
3. **Worker Pools**: Pattern implementations (multicast registry, consumer, producer) spawn worker goroutines that need coordination
4. **Concurrent Operations**: Multiple processes must run concurrently without interference, with proper isolation and state management
5. **Graceful Shutdown**: System shutdown must cleanly terminate all processes with timeout handling

**Current Gap**: We lack a unified process management system. Each component reinvents lifecycle management, leading to:
- Inconsistent termination handling (some processes block, others leak)
- Race conditions during startup/shutdown
- No visibility into process state (running? terminating? stuck?)
- Difficult debugging when processes hang
- No standard pattern for retries and backoff

## Kubelet's Process Management: Key Insights

### Architecture Overview

Kubelet's `pod_workers.go` (~1700 lines) implements a sophisticated state machine for managing pod lifecycles. Key components:

**1. Per-Process Goroutine with Channel Communication**
```go
type podWorkers struct {
    podUpdates      map[types.UID]chan struct{}       // One channel per process
    podSyncStatuses map[types.UID]*podSyncStatus      // State tracking
    podLock         sync.Mutex                         // Protects all state
    workQueue       queue.WorkQueue                    // Retry with backoff
}
```

**2. Four Lifecycle States**
```go
type PodWorkerState int

const (
    SyncPod        PodWorkerState = iota  // Starting and running
    TerminatingPod                        // Stopping containers
    TerminatedPod                         // Cleanup resources
)
```

**3. State Tracking Per Process**
```go
type podSyncStatus struct {
    ctx           context.Context
    cancelFn      context.CancelFunc
    working       bool
    pendingUpdate *UpdatePodOptions
    activeUpdate  *UpdatePodOptions

    // Lifecycle timestamps
    syncedAt      time.Time
    startedAt     time.Time
    terminatingAt time.Time
    terminatedAt  time.Time

    // Termination metadata
    gracePeriod   int64
    deleted       bool
    evicted       bool
    finished      bool
}
```

### Key Design Patterns

#### Pattern 1: Goroutine-Per-Process with Buffered Channels

Each process gets its own goroutine and update channel:

```go
// Spawn a worker goroutine
go func() {
    defer runtime.HandleCrash()
    defer klog.V(3).InfoS("Process worker has stopped", "procUID", uid)
    p.processWorkerLoop(uid, outCh)
}()
```

**Benefits**:
- Process isolation: one process failure doesn't affect others
- Non-blocking updates: buffered channel prevents publisher blocking
- Clean shutdown: close channel to signal termination

#### Pattern 2: State Machine with Immutable Transitions

State transitions are one-way and immutable:

```text
[SyncPod] → [TerminatingPod] → [TerminatedPod] → [Finished]
     ↑             ↓                   ↓
     └─────────────┴───────────────────┘
         (only via SyncKnownPods purge)
```

**Key Rules**:
1. Once terminating, cannot return to sync
2. Grace period can only decrease, never increase
3. Finished processes ignored until purged
4. State transitions hold lock briefly, sync operations do not

#### Pattern 3: Pending + Active Update Model

Two update slots prevent losing state:

```go
// Pending: queued update waiting for worker
pendingUpdate *UpdatePodOptions

// Active: currently processing update (visible to all)
activeUpdate  *UpdatePodOptions
```

**Flow**:
1. New update arrives → store in `pendingUpdate`
2. Worker goroutine wakes → move `pendingUpdate` to `activeUpdate`
3. Process sync → `activeUpdate` is source of truth
4. Another update arrives while processing → overwrites `pendingUpdate`

**Benefits**:
- Worker always processes latest state
- Intermediate updates can be skipped (optimization)
- Active update visible for health checks

#### Pattern 4: Context Cancellation for Interruption

Each process has a context for cancellation:

```go
// Initialize context for process
if status.ctx == nil || status.ctx.Err() == context.Canceled {
    status.ctx, status.cancelFn = context.WithCancel(context.Background())
}

// Cancel on termination request
if (becameTerminating || wasGracePeriodShortened) && status.cancelFn != nil {
    klog.V(3).InfoS("Cancelling current sync", "procUID", uid)
    status.cancelFn()
}
```

**Benefits**:
- Long-running sync operations can be interrupted
- Faster response to termination signals
- Graceful unwinding of nested operations

#### Pattern 5: Work Queue with Exponential Backoff

Failed syncs requeue with backoff:

```go
func (p *podWorkers) completeWork(podUID types.UID, phaseTransition bool, syncErr error) {
    switch {
    case phaseTransition:
        p.workQueue.Enqueue(podUID, 0)  // Immediate
    case syncErr == nil:
        p.workQueue.Enqueue(podUID, wait.Jitter(p.resyncInterval, 0.5))
    case isTransientError(syncErr):
        p.workQueue.Enqueue(podUID, wait.Jitter(1*time.Second, 0.5))
    default:
        p.workQueue.Enqueue(podUID, wait.Jitter(p.backOffPeriod, 0.5))
    }
}
```

**Benefits**:
- Transient errors retry quickly
- Persistent errors back off exponentially
- Phase transitions bypass queue for immediate action

#### Pattern 6: Graceful Termination with Grace Period

Termination is multi-phase with configurable grace period:

```go
// Phase 1: SyncTerminatingPod - stop containers
err = p.podSyncer.SyncTerminatingPod(ctx, pod, status, gracePeriod, statusFn)

// Phase 2: SyncTerminatedPod - cleanup resources
err = p.podSyncer.SyncTerminatedPod(ctx, pod, status)
```

**Grace Period Rules**:
1. Default from pod spec: `pod.Spec.TerminationGracePeriodSeconds`
2. Can be overridden (eviction, force delete)
3. Can only decrease, never increase
4. Minimum 1 second enforced

#### Pattern 7: Orphan Cleanup via SyncKnownPods

Periodic reconciliation removes finished processes:

```go
func (p *podWorkers) SyncKnownPods(desiredPods []*v1.Pod) map[types.UID]PodWorkerSync {
    for uid, status := range p.podSyncStatuses {
        _, knownPod := known[uid]
        orphan := !knownPod

        if status.restartRequested || orphan {
            if p.removeTerminatedWorker(uid, status, orphan) {
                continue  // Removed, don't return
            }
        }
    }
}
```

**Benefits**:
- Bounded memory: finished processes eventually purged
- Restart detection: same UID can be reused after purge
- Orphan termination: processes not in desired set are stopped

## Proposed Package: `pkg/procmgr`

### Core Types

```go
package procmgr

import (
    "context"
    "sync"
    "time"
)

// ProcessState represents the lifecycle state of a managed process
type ProcessState int

const (
    // ProcessStateStarting - process is initializing
    ProcessStateStarting ProcessState = iota
    // ProcessStateSyncing - process is running and healthy
    ProcessStateSyncing
    // ProcessStateTerminating - process is shutting down
    ProcessStateTerminating
    // ProcessStateTerminated - process has stopped, awaiting cleanup
    ProcessStateTerminated
    // ProcessStateFinished - process fully cleaned up
    ProcessStateFinished
)

// ProcessID uniquely identifies a managed process
type ProcessID string

// ProcessUpdate contains state changes for a process
type ProcessUpdate struct {
    ID              ProcessID
    UpdateType      UpdateType
    StartTime       time.Time
    Config          interface{}  // Process-specific config
    TerminateOptions *TerminateOptions
}

// UpdateType specifies the kind of update
type UpdateType int

const (
    UpdateTypeCreate UpdateType = iota
    UpdateTypeUpdate
    UpdateTypeSync
    UpdateTypeTerminate
)

// TerminateOptions control process termination
type TerminateOptions struct {
    CompletedCh      chan<- struct{}
    Evict            bool
    GracePeriodSecs  *int64
    StatusFunc       ProcessStatusFunc
}

// ProcessStatusFunc is called to update process status on termination
type ProcessStatusFunc func(status *ProcessStatus)

// ProcessStatus tracks runtime state of a process
type ProcessStatus struct {
    State         ProcessState
    Healthy       bool
    LastSync      time.Time
    ErrorCount    int
    LastError     error
    RestartCount  int
}

// ProcessSyncer defines the interface for process lifecycle hooks
type ProcessSyncer interface {
    // SyncProcess starts/updates the process
    SyncProcess(ctx context.Context, updateType UpdateType, config interface{}) (terminal bool, error)

    // SyncTerminatingProcess stops the process
    SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn ProcessStatusFunc) error

    // SyncTerminatedProcess cleans up resources
    SyncTerminatedProcess(ctx context.Context, config interface{}) error
}

// ProcessManager manages 0 or more concurrent processes
type ProcessManager struct {
    mu sync.Mutex

    // Process tracking
    processUpdates  map[ProcessID]chan struct{}
    processStatuses map[ProcessID]*processStatus

    // Configuration
    syncer         ProcessSyncer
    resyncInterval time.Duration
    backOffPeriod  time.Duration
    workQueue      WorkQueue

    // Metrics
    metrics ProcessManagerMetrics
}

// Internal state tracking per process
type processStatus struct {
    ctx       context.Context
    cancelFn  context.CancelFunc

    working   bool
    pending   *ProcessUpdate
    active    *ProcessUpdate

    // Lifecycle timestamps
    syncedAt      time.Time
    startedAt     time.Time
    terminatingAt time.Time
    terminatedAt  time.Time
    finishedAt    time.Time

    // Termination metadata
    gracePeriod   int64
    evicted       bool
    finished      bool

    // Health tracking
    errorCount    int
    lastError     error
    restartCount  int
}
```

### Core API

```go
// NewProcessManager creates a new process manager
func NewProcessManager(opts ...Option) *ProcessManager

// UpdateProcess submits a process update
func (pm *ProcessManager) UpdateProcess(update ProcessUpdate)

// SyncKnownProcesses reconciles desired vs actual processes
func (pm *ProcessManager) SyncKnownProcesses(desiredIDs []ProcessID) map[ProcessID]ProcessStatus

// GetProcessStatus returns current status of a process
func (pm *ProcessManager) GetProcessStatus(id ProcessID) (*ProcessStatus, bool)

// IsProcessTerminated checks if process has terminated
func (pm *ProcessManager) IsProcessTerminated(id ProcessID) bool

// IsProcessFinished checks if process cleanup completed
func (pm *ProcessManager) IsProcessFinished(id ProcessID) bool

// Shutdown gracefully stops all processes
func (pm *ProcessManager) Shutdown(ctx context.Context) error
```

### Configuration Options

```go
type Option func(*ProcessManager)

// WithResyncInterval sets periodic resync interval
func WithResyncInterval(d time.Duration) Option

// WithBackOffPeriod sets error backoff period
func WithBackOffPeriod(d time.Duration) Option

// WithMetricsCollector enables metrics
func WithMetricsCollector(mc MetricsCollector) Option

// WithLogger sets custom logger
func WithLogger(logger Logger) Option
```

### Work Queue with Backoff

```go
// WorkQueue manages process work items with backoff
type WorkQueue interface {
    Enqueue(id ProcessID, delay time.Duration)
    Dequeue() (ProcessID, bool)
    Len() int
}

// workQueue implements WorkQueue with priority queue
type workQueue struct {
    mu       sync.Mutex
    items    []*workItem
    notifyCh chan struct{}
}

type workItem struct {
    id       ProcessID
    readyAt  time.Time
    priority int
}
```

## Implementation Phases

### Phase 1: Core Process Manager (Week 1)

**Deliverables**:
1. Process manager struct with state tracking
2. Per-process goroutine with channel communication
3. State machine with immutable transitions
4. Pending + active update model
5. Context cancellation support

**Tests**:
- Create/update/terminate single process
- Concurrent operations on multiple processes
- State transition validation
- Context cancellation during sync

### Phase 2: Work Queue and Backoff (Week 2)

**Deliverables**:
1. Priority work queue implementation
2. Exponential backoff on errors
3. Jitter to prevent thundering herd
4. Immediate requeue on phase transitions

**Tests**:
- Backoff increases on repeated failures
- Transient vs persistent error handling
- Phase transitions bypass backoff
- Queue ordering correctness

### Phase 3: Graceful Termination (Week 3)

**Deliverables**:
1. Multi-phase termination (terminating → terminated)
2. Configurable grace period
3. Grace period decrease-only enforcement
4. Completion notification channels

**Tests**:
- Graceful shutdown within grace period
- Force kill after grace period expires
- Grace period cannot increase
- Completion channel closed on termination

### Phase 4: Orphan Cleanup and Metrics (Week 4)

**Deliverables**:
1. SyncKnownProcesses reconciliation
2. Orphan detection and cleanup
3. Finished process purging
4. Prometheus metrics integration

**Tests**:
- Orphaned processes terminated
- Finished processes purged after TTL
- Metrics exported correctly
- Memory doesn't grow unbounded

## Usage Examples

### Example 1: Backend Driver Management

```go
// Define driver lifecycle
type driverSyncer struct {
    drivers map[ProcessID]backend.Driver
}

func (ds *driverSyncer) SyncProcess(ctx context.Context, updateType UpdateType, config interface{}) (bool, error) {
    driverConfig := config.(*DriverConfig)
    driver, ok := ds.drivers[driverConfig.ID]

    if !ok {
        // Create new driver
        driver, err := backend.NewDriver(driverConfig)
        if err != nil {
            return false, fmt.Errorf("create driver: %w", err)
        }
        ds.drivers[driverConfig.ID] = driver
    }

    // Start driver
    if err := driver.Start(ctx); err != nil {
        return false, fmt.Errorf("start driver: %w", err)
    }

    // Check if driver reached terminal state
    if driver.State() == backend.StateFailed {
        return true, fmt.Errorf("driver failed")
    }

    return false, nil
}

func (ds *driverSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn ProcessStatusFunc) error {
    driverConfig := config.(*DriverConfig)
    driver := ds.drivers[driverConfig.ID]

    // Stop driver with grace period
    timeout := time.Duration(*gracePeriodSecs) * time.Second
    return driver.StopWithTimeout(ctx, timeout)
}

func (ds *driverSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
    driverConfig := config.(*DriverConfig)
    driver := ds.drivers[driverConfig.ID]

    // Cleanup resources
    if err := driver.Cleanup(); err != nil {
        return fmt.Errorf("cleanup: %w", err)
    }

    delete(ds.drivers, driverConfig.ID)
    return nil
}

// Usage
func main() {
    syncer := &driverSyncer{drivers: make(map[ProcessID]backend.Driver)}
    pm := procmgr.NewProcessManager(
        procmgr.WithSyncer(syncer),
        procmgr.WithResyncInterval(30*time.Second),
        procmgr.WithBackOffPeriod(5*time.Second),
    )

    // Start Redis driver
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "redis-driver",
        UpdateType: procmgr.UpdateTypeCreate,
        Config:     &DriverConfig{Type: "redis", DSN: "localhost:6379"},
    })

    // Start NATS driver
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "nats-driver",
        UpdateType: procmgr.UpdateTypeCreate,
        Config:     &DriverConfig{Type: "nats", DSN: "nats://localhost:4222"},
    })

    // Graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    pm.Shutdown(ctx)
}
```

### Example 2: Worker Pool Management

```go
// Worker pool with dynamic scaling
type workerPoolSyncer struct {
    pools map[ProcessID]*WorkerPool
}

func (wps *workerPoolSyncer) SyncProcess(ctx context.Context, updateType UpdateType, config interface{}) (bool, error) {
    poolConfig := config.(*PoolConfig)
    pool, ok := wps.pools[poolConfig.ID]

    if !ok {
        // Create new pool
        pool = NewWorkerPool(poolConfig.NumWorkers)
        wps.pools[poolConfig.ID] = pool
    } else {
        // Scale pool
        pool.Scale(poolConfig.NumWorkers)
    }

    // Start workers
    return false, pool.Start(ctx)
}

func (wps *workerPoolSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn ProcessStatusFunc) error {
    poolConfig := config.(*PoolConfig)
    pool := wps.pools[poolConfig.ID]

    // Drain work queue
    pool.Drain()

    // Stop workers with timeout
    timeout := time.Duration(*gracePeriodSecs) * time.Second
    return pool.StopWithTimeout(ctx, timeout)
}

func (wps *workerPoolSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
    poolConfig := config.(*PoolConfig)
    delete(wps.pools, poolConfig.ID)
    return nil
}
```

### Example 3: Plugin Hot Reload

```go
// Hot reload plugin without downtime
func reloadPlugin(pm *ProcessManager, pluginID ProcessID, newConfig *PluginConfig) error {
    // Step 1: Check current state
    status, ok := pm.GetProcessStatus(pluginID)
    if !ok {
        return fmt.Errorf("plugin %s not found", pluginID)
    }

    // Step 2: Graceful termination with callback
    completedCh := make(chan struct{})
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         pluginID,
        UpdateType: procmgr.UpdateTypeTerminate,
        TerminateOptions: &procmgr.TerminateOptions{
            CompletedCh:     completedCh,
            GracePeriodSecs: ptr.Int64(10),
        },
    })

    // Step 3: Wait for termination
    select {
    case <-completedCh:
        // Old plugin terminated
    case <-time.After(15 * time.Second):
        return fmt.Errorf("plugin termination timeout")
    }

    // Step 4: Wait for cleanup
    for {
        if pm.IsProcessFinished(pluginID) {
            break
        }
        time.Sleep(100 * time.Millisecond)
    }

    // Step 5: Start new version
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         pluginID,
        UpdateType: procmgr.UpdateTypeCreate,
        Config:     newConfig,
    })

    return nil
}
```

## Metrics and Observability

### Prometheus Metrics

```go
// Process lifecycle metrics
process_state_total{id, state} counter
process_sync_duration_seconds{id, type} histogram
process_termination_duration_seconds{id} histogram
process_error_total{id, type} counter
process_restart_total{id} counter

// Queue metrics
work_queue_depth gauge
work_queue_add_total counter
work_queue_retry_total counter
work_queue_backoff_duration_seconds histogram
```

### Logging

```text
INFO  Process starting                            id=redis-driver config=<redacted>
INFO  Process synced successfully                  id=redis-driver duration=125ms
WARN  Process sync failed (transient error)       id=redis-driver error="connection refused" retry_in=1s
ERROR Process sync failed (persistent error)      id=redis-driver error="auth failed" backoff=5s
INFO  Process termination requested               id=redis-driver grace_period=10s
INFO  Process terminating                         id=redis-driver containers_stopped=2
INFO  Process terminated successfully             id=redis-driver duration=3.2s
INFO  Process cleanup completed                   id=redis-driver
```

### Health Checks

```go
// Health check endpoint
type HealthCheck struct {
    TotalProcesses    int
    RunningProcesses  int
    TerminatingProcesses int
    FailedProcesses   int
    Processes         map[ProcessID]ProcessHealth
}

type ProcessHealth struct {
    State        ProcessState
    Healthy      bool
    Uptime       time.Duration
    LastSync     time.Time
    ErrorCount   int
    RestartCount int
}

func (pm *ProcessManager) Health() HealthCheck {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    health := HealthCheck{
        Processes: make(map[ProcessID]ProcessHealth),
    }

    for id, status := range pm.processStatuses {
        health.TotalProcesses++

        if status.State() == ProcessStateSyncing {
            health.RunningProcesses++
        } else if status.State() == ProcessStateTerminating {
            health.TerminatingProcesses++
        }

        if status.errorCount > 5 {
            health.FailedProcesses++
        }

        health.Processes[id] = ProcessHealth{
            State:        status.State(),
            Healthy:      status.errorCount < 5,
            Uptime:       time.Since(status.startedAt),
            LastSync:     status.active.StartTime,
            ErrorCount:   status.errorCount,
            RestartCount: status.restartCount,
        }
    }

    return health
}
```

## Testing Strategy

### Unit Tests

```go
func TestProcessManager_CreateProcess(t *testing.T) {
    syncer := &mockSyncer{}
    pm := procmgr.NewProcessManager(procmgr.WithSyncer(syncer))

    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "test-1",
        UpdateType: procmgr.UpdateTypeCreate,
        Config:     &TestConfig{},
    })

    // Wait for sync
    time.Sleep(100 * time.Millisecond)

    status, ok := pm.GetProcessStatus("test-1")
    require.True(t, ok)
    assert.Equal(t, procmgr.ProcessStateSyncing, status.State)
    assert.Equal(t, 1, syncer.syncCalled)
}

func TestProcessManager_GracefulTermination(t *testing.T) {
    syncer := &mockSyncer{syncDuration: 5 * time.Second}
    pm := procmgr.NewProcessManager(procmgr.WithSyncer(syncer))

    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "test-1",
        UpdateType: procmgr.UpdateTypeCreate,
        Config:     &TestConfig{},
    })

    // Terminate with grace period
    completedCh := make(chan struct{})
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "test-1",
        UpdateType: procmgr.UpdateTypeTerminate,
        TerminateOptions: &procmgr.TerminateOptions{
            CompletedCh:     completedCh,
            GracePeriodSecs: ptr.Int64(10),
        },
    })

    // Should complete within grace period
    select {
    case <-completedCh:
        // Success
    case <-time.After(15 * time.Second):
        t.Fatal("termination timeout")
    }

    assert.True(t, pm.IsProcessTerminated("test-1"))
}

func TestProcessManager_ConcurrentProcesses(t *testing.T) {
    syncer := &mockSyncer{}
    pm := procmgr.NewProcessManager(procmgr.WithSyncer(syncer))

    // Create 100 processes concurrently
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            pm.UpdateProcess(procmgr.ProcessUpdate{
                ID:         ProcessID(fmt.Sprintf("test-%d", id)),
                UpdateType: procmgr.UpdateTypeCreate,
                Config:     &TestConfig{},
            })
        }(i)
    }
    wg.Wait()

    // All should be created
    time.Sleep(1 * time.Second)
    for i := 0; i < 100; i++ {
        status, ok := pm.GetProcessStatus(ProcessID(fmt.Sprintf("test-%d", i)))
        assert.True(t, ok)
        assert.Equal(t, procmgr.ProcessStateSyncing, status.State)
    }
}
```

### Integration Tests

```go
func TestProcessManager_RealBackendDriver(t *testing.T) {
    // Start real Redis container
    redis := testcontainers.RunRedis(t)
    defer redis.Stop()

    // Create process manager with real driver
    syncer := &driverSyncer{drivers: make(map[ProcessID]backend.Driver)}
    pm := procmgr.NewProcessManager(procmgr.WithSyncer(syncer))

    // Start Redis driver
    pm.UpdateProcess(procmgr.ProcessUpdate{
        ID:         "redis-driver",
        UpdateType: procmgr.UpdateTypeCreate,
        Config: &DriverConfig{
            Type: "redis",
            DSN:  redis.ConnectionString(),
        },
    })

    // Wait for driver to be healthy
    require.Eventually(t, func() bool {
        status, ok := pm.GetProcessStatus("redis-driver")
        return ok && status.State == procmgr.ProcessStateSyncing && status.Healthy
    }, 5*time.Second, 100*time.Millisecond)

    // Use driver
    driver := syncer.drivers["redis-driver"]
    err := driver.Set("key", []byte("value"))
    require.NoError(t, err)

    // Graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    err = pm.Shutdown(ctx)
    require.NoError(t, err)
}
```

### Load Tests

```go
func TestProcessManager_HighChurn(t *testing.T) {
    syncer := &mockSyncer{}
    pm := procmgr.NewProcessManager(procmgr.WithSyncer(syncer))

    // Churn: create and destroy processes rapidly
    for i := 0; i < 1000; i++ {
        id := ProcessID(fmt.Sprintf("test-%d", i))

        // Create
        pm.UpdateProcess(procmgr.ProcessUpdate{
            ID:         id,
            UpdateType: procmgr.UpdateTypeCreate,
            Config:     &TestConfig{},
        })

        // Terminate after 10ms
        time.Sleep(10 * time.Millisecond)
        pm.UpdateProcess(procmgr.ProcessUpdate{
            ID:         id,
            UpdateType: procmgr.UpdateTypeTerminate,
        })
    }

    // All should eventually finish
    require.Eventually(t, func() bool {
        synced := pm.SyncKnownProcesses([]ProcessID{})
        return len(synced) == 0
    }, 30*time.Second, 100*time.Millisecond)
}
```

## Security Considerations

1. **Resource Limits**: Process manager should enforce CPU/memory limits per process
2. **Privilege Separation**: Processes run with minimal privileges
3. **Signal Handling**: Proper SIGTERM/SIGKILL handling for Unix processes
4. **Audit Logging**: All process lifecycle events logged for security audits
5. **Deadlock Detection**: Timeout enforcement prevents hung processes

## Performance Considerations

1. **Lock Contention**: State lock held briefly, sync operations run without lock
2. **Channel Buffering**: Buffered channels (size 1) prevent publisher blocking
3. **Work Queue Priority**: Phase transitions bypass queue for immediate execution
4. **Jitter**: Random jitter prevents thundering herd on backoff retry
5. **Memory Bounds**: Finished processes purged after TTL to prevent unbounded growth

## Alternatives Considered

### Alternative 1: errgroup.Group

**Pros**:
- Built-in concurrency management
- Automatic error propagation
- Simple API

**Cons**:
- No state tracking (starting, terminating, terminated)
- No graceful termination with grace period
- No retry/backoff on failure
- No process-level isolation (one failure stops all)

**Verdict**: Too simplistic for our needs.

### Alternative 2: golang.org/x/sync/semaphore

**Pros**:
- Resource limiting (max concurrent processes)
- Lightweight

**Cons**:
- No lifecycle management
- No state machine
- No termination handling

**Verdict**: Complementary tool, not a replacement.

### Alternative 3: github.com/oklog/run

**Pros**:
- Actor-based concurrency
- Graceful shutdown support

**Cons**:
- No state tracking
- No retry/backoff
- No per-process isolation
- All actors share one context

**Verdict**: Good for simple cases, insufficient for complex lifecycle management.

## Open Questions

1. **Process Dependencies**: Should process manager support dependency graphs (process A must start before process B)?
2. **Health Checks**: Should health checks be built-in or delegated to the syncer?
3. **Resource Limits**: Should cgroups/ulimits be enforced by process manager?
4. **Checkpointing**: Should process state be persisted for restart recovery?
5. **Dynamic Configuration**: Should processes support hot config reload without restart?

## References

1. [Kubernetes Kubelet pod_workers.go](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/pod_workers.go)
2. [Kubernetes Pod Lifecycle Documentation](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)
3. [Go Context Cancellation Patterns](https://go.dev/blog/context)
4. [Graceful Shutdown Patterns in Go](https://medium.com/@pinkudebnath/graceful-shutdown-of-golang-servers-using-context-and-os-signals-cc1fa2c55e97)
5. [Exponential Backoff and Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)

## Appendix A: Kubelet Architecture Diagram

```text
                              ┌─────────────────┐
                              │  UpdatePod()    │
                              │  (Public API)   │
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │  podSyncStatus  │
                              │  (State Store)  │
                              │  - pending      │
                              │  - active       │
                              │  - timestamps   │
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │  podUpdates     │
                              │  chan struct{}  │
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │ podWorkerLoop() │
                              │  (Goroutine)    │
                              └────────┬────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    │                  │                  │
          ┌─────────▼────────┐ ┌──────▼──────┐ ┌────────▼────────┐
          │    SyncPod       │ │ SyncTermina-│ │ SyncTerminated  │
          │  (Start/Update)  │ │  tingPod    │ │      Pod        │
          │                  │ │  (Stop)     │ │   (Cleanup)     │
          └──────────────────┘ └─────────────┘ └─────────────────┘
                    │                  │                  │
                    └──────────────────┼──────────────────┘
                                       │
                              ┌────────▼────────┐
                              │  completeWork() │
                              │  (Requeue)      │
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │   workQueue     │
                              │  (Backoff)      │
                              └─────────────────┘
```

## Appendix B: State Transition Diagram

```text
                         ┌──────────────────────┐
                         │  Not Exists          │
                         └──────┬───────────────┘
                                │ UpdatePod(Create)
                                ▼
                         ┌──────────────────────┐
                         │  SyncPod             │
                         │  (Starting/Running)  │
                         └──────┬───────────────┘
                                │ Termination Requested
                                │ (Delete, Evict, Failed)
                                ▼
                         ┌──────────────────────┐
                         │  TerminatingPod      │
                         │  (Stopping)          │
                         └──────┬───────────────┘
                                │ Containers Stopped
                                ▼
                         ┌──────────────────────┐
                         │  TerminatedPod       │
                         │  (Cleanup)           │
                         └──────┬───────────────┘
                                │ Cleanup Complete
                                ▼
                         ┌──────────────────────┐
                         │  Finished            │
                         │  (Awaiting Purge)    │
                         └──────┬───────────────┘
                                │ SyncKnownPods()
                                │ (Orphan or Restart)
                                ▼
                         ┌──────────────────────┐
                         │  Not Exists          │
                         └──────────────────────┘
```

---

**Status**: Proposed
**Next Steps**:
1. Review RFC with team
2. Prototype core types and state machine
3. Implement Phase 1 deliverables
4. Integration with existing backend drivers
