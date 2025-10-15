# ProcessManager Integration Example

This example demonstrates how to use the `procmgr` package to manage multiple concurrent backend plugin instances with proper lifecycle management.

## Overview

The example manages 3 MemStore plugin instances using the ProcessManager:
- Automatic initialization and health checks
- Periodic resyncs every 10 seconds
- Graceful termination with configurable grace periods
- Automatic retry with exponential backoff on errors
- Health monitoring and status reporting

## What the Example Demonstrates

### 1. Process Lifecycle Management
- **Starting**: Creates and initializes 3 MemStore instances with different configurations
- **Syncing**: Periodic health checks ensure processes stay healthy
- **Updating**: Demonstrates runtime configuration updates
- **Terminating**: Graceful shutdown with 5-second grace period
- **Cleanup**: Automatic resource cleanup after termination

### 2. Error Handling
- Automatic retries on transient failures
- Exponential backoff (1s, 2s, 4s, ..., max 5s)
- Work queue for delayed retries
- Health-based failure detection

### 3. Observability
- Health API shows process states and statistics
- Work queue depth monitoring
- Per-process uptime tracking
- Error count tracking

## Running the Example

```bash
# From the example directory
cd pkg/procmgr/example

# Initialize Go module dependencies
go mod tidy

# Run the example
go run memstore_example.go
```

## Expected Output

```
=== MemStore ProcessManager Example ===

--- Starting 3 MemStore processes ---
Creating new MemStore instance: memstore-1
Initializing MemStore memstore-1 with config: ...
Starting MemStore memstore-1
Checking health of MemStore memstore-1
MemStore memstore-1 health: healthy - healthy, 0 keys stored
...

--- Checking process health ---
Total processes: 3
Running processes: 3
Failed processes: 0
Work queue depth: 3
  Process memstore-1: state=Syncing, healthy=true, uptime=2s, errors=0
  Process memstore-2: state=Syncing, healthy=true, uptime=2s, errors=0
  Process memstore-3: state=Syncing, healthy=true, uptime=2s, errors=0

--- Running for 15 seconds (periodic resyncs will occur) ---
[Periodic health checks logged every 10 seconds...]

--- Updating memstore-2 configuration ---
Initializing MemStore memstore-2 with config: ...

--- Terminating memstore-1 gracefully ---
Stopping MemStore memstore-1 (grace period: 5 seconds)
MemStore memstore-1 stopped successfully

--- Final health check ---
Total processes: 3
Running processes: 2
Terminating processes: 0
  Process memstore-1: state=Finished, healthy=false, uptime=23s
  Process memstore-2: state=Syncing, healthy=true, uptime=23s
  Process memstore-3: state=Syncing, healthy=true, uptime=23s

--- Shutting down process manager ---
Shutdown completed successfully

=== Example complete ===
```

## Key Concepts

### ProcessSyncer Interface
The example implements `ProcessSyncer` for MemStore:
```go
type MemStoreSyncer struct {
    plugins map[procmgr.ProcessID]*memstore.MemStore
}

func (s *MemStoreSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (terminal bool, err error)
func (s *MemStoreSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error
func (s *MemStoreSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error
```

### Process Configuration
Each process has its own configuration:
```go
type ProcessConfig struct {
    ID           procmgr.ProcessID
    PluginConfig *plugin.Config
}
```

### Process Updates
Operations are submitted as updates:
```go
pm.UpdateProcess(procmgr.ProcessUpdate{
    ID:         "memstore-1",
    UpdateType: procmgr.UpdateTypeCreate,
    Config:     &config,
})
```

### Graceful Termination
Processes can be terminated with grace periods:
```go
gracePeriod := int64(5)
pm.UpdateProcess(procmgr.ProcessUpdate{
    ID:         "memstore-1",
    UpdateType: procmgr.UpdateTypeTerminate,
    TerminateOptions: &procmgr.TerminateOptions{
        GracePeriodSecs: &gracePeriod,
    },
})
```

### Health Monitoring
Get comprehensive health status:
```go
health := pm.Health()
for id, procHealth := range health.Processes {
    log.Printf("Process %s: state=%s, healthy=%v, uptime=%v",
        id, procHealth.State, procHealth.Healthy, procHealth.Uptime)
}
```

## Customization

### Different Resync Intervals
```go
pm := procmgr.NewProcessManager(
    procmgr.WithSyncer(syncer),
    procmgr.WithResyncInterval(30*time.Second),  // Resync every 30s
)
```

### Custom Backoff Period
```go
pm := procmgr.NewProcessManager(
    procmgr.WithSyncer(syncer),
    procmgr.WithBackOffPeriod(10*time.Second),  // Max backoff: 10s
)
```

### Metrics Collection
```go
// Implement MetricsCollector interface
type MyMetricsCollector struct {}
func (m *MyMetricsCollector) ProcessStateTransition(id ProcessID, fromState, toState ProcessState) {
    // Record metric...
}

pm := procmgr.NewProcessManager(
    procmgr.WithSyncer(syncer),
    procmgr.WithMetricsCollector(&MyMetricsCollector{}),
)
```

## Architecture Notes

This example follows the Kubernetes Kubelet pod worker pattern:
- Each process gets its own goroutine
- Work queue with priority scheduling for retries
- Exponential backoff with jitter prevents thundering herd
- Context cancellation for graceful interruption
- Five-phase lifecycle (Starting → Syncing → Terminating → Terminated → Finished)

See [RFC-034](../../../docs-cms/rfcs/RFC-034-robust-process-manager.md) for complete design documentation.
