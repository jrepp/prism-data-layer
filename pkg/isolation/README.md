# isolation - Bulkhead Pattern for Process Isolation

A package that implements the bulkhead pattern using procmgr to isolate requests by namespace or session.

## Overview

The isolation package provides a way to segment requests into separate processes based on isolation levels. This prevents failures in one namespace or session from affecting others, implementing the bulkhead pattern for fault isolation.

## Features

✅ **Three Isolation Levels**:
- **None**: All requests share the same process pool
- **Namespace**: Each namespace gets its own process
- **Session**: Each session gets its own process

✅ **Built on procmgr**:
- Leverages robust process lifecycle management
- Automatic retry with exponential backoff
- Health monitoring and metrics collection
- Graceful termination

✅ **Concurrent Access**:
- Thread-safe process creation and management
- Handles concurrent requests to same namespace/session
- Tested with 10+ concurrent goroutines

## Architecture

### Isolation Levels

```text
IsolationNone:
  Request 1 (ns:A, session:1) ──┐
  Request 2 (ns:B, session:2) ──┼─→ [Process: shared]
  Request 3 (ns:A, session:3) ──┘

IsolationNamespace:
  Request 1 (ns:A, session:1) ──┐
  Request 2 (ns:A, session:2) ──┼─→ [Process: ns:A]
  Request 3 (ns:B, session:1) ────→ [Process: ns:B]

IsolationSession:
  Request 1 (ns:A, session:1) ──┐
  Request 2 (ns:B, session:1) ──┼─→ [Process: session:1]
  Request 3 (ns:A, session:2) ────→ [Process: session:2]
```

### Process IDs

Process IDs are generated based on the isolation level:

- **IsolationNone**: All processes use ID `"shared"`
- **IsolationNamespace**: Process ID is `"ns:<namespace>"`
- **IsolationSession**: Process ID is `"session:<session>"`

This ensures requests are routed to the correct isolated process.

## Usage

### Basic Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/jrepp/prism/pkg/isolation"
    "github.com/jrepp/prism/pkg/procmgr"
)

// Implement ProcessSyncer interface
type MyBackendSyncer struct {
    backends map[procmgr.ProcessID]interface{}
}

func (s *MyBackendSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (bool, error) {
    // Start/update your backend
    log.Printf("Starting backend with config: %v", config)
    return false, nil
}

func (s *MyBackendSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
    // Stop backend gracefully
    log.Printf("Stopping backend (grace period: %d seconds)", *gracePeriodSecs)
    return nil
}

func (s *MyBackendSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
    // Cleanup resources
    log.Printf("Cleaning up backend")
    return nil
}

func main() {
    // Create isolation manager with namespace isolation
    syncer := &MyBackendSyncer{
        backends: make(map[procmgr.ProcessID]interface{}),
    }

    im := isolation.NewIsolationManager(
        isolation.IsolationNamespace,
        syncer,
        procmgr.WithResyncInterval(30*time.Second),
        procmgr.WithBackOffPeriod(5*time.Second),
    )
    defer im.Shutdown(context.Background())

    // Get or create process for namespace
    ctx := context.Background()
    key := isolation.IsolationKey{
        Namespace: "tenant-1",
        Session:   "session-123",
    }

    config := &isolation.ProcessConfig{
        Key:            key,
        BackendConfig:  map[string]string{"endpoint": "redis://localhost:6379"},
        GracePeriodSec: 10,
    }

    handle, err := im.GetOrCreateProcess(ctx, key, config)
    if err != nil {
        log.Fatalf("Failed to create process: %v", err)
    }

    log.Printf("Process created: %s (healthy: %v)", handle.ID, handle.Health)

    // Use the process...
    time.Sleep(5 * time.Second)

    // Terminate process gracefully
    if err := im.TerminateProcess(ctx, key, 10); err != nil {
        log.Printf("Failed to terminate: %v", err)
    }
}
```

### Namespace Isolation Example

```go
// Different namespaces get different processes
im := isolation.NewIsolationManager(isolation.IsolationNamespace, syncer)

key1 := isolation.IsolationKey{Namespace: "tenant-1", Session: "s1"}
key2 := isolation.IsolationKey{Namespace: "tenant-2", Session: "s2"}
key3 := isolation.IsolationKey{Namespace: "tenant-1", Session: "s3"} // Same as key1

handle1, _ := im.GetOrCreateProcess(ctx, key1, config1)
handle2, _ := im.GetOrCreateProcess(ctx, key2, config2)
handle3, _ := im.GetOrCreateProcess(ctx, key3, config3)

// handle1 and handle3 use the same process (ns:tenant-1)
// handle2 uses a different process (ns:tenant-2)
assert.Equal(t, handle1.ID, handle3.ID)
assert.NotEqual(t, handle1.ID, handle2.ID)
```

### Session Isolation Example

```go
// Different sessions get different processes
im := isolation.NewIsolationManager(isolation.IsolationSession, syncer)

key1 := isolation.IsolationKey{Namespace: "tenant-1", Session: "session-1"}
key2 := isolation.IsolationKey{Namespace: "tenant-1", Session: "session-2"}
key3 := isolation.IsolationKey{Namespace: "tenant-2", Session: "session-1"} // Same as key1

handle1, _ := im.GetOrCreateProcess(ctx, key1, config1)
handle2, _ := im.GetOrCreateProcess(ctx, key2, config2)
handle3, _ := im.GetOrCreateProcess(ctx, key3, config3)

// handle1 and handle3 use the same process (session:session-1)
// handle2 uses a different process (session:session-2)
assert.Equal(t, handle1.ID, handle3.ID)
assert.NotEqual(t, handle1.ID, handle2.ID)
```

## API Reference

### Core Types

```go
// IsolationLevel defines how requests are isolated
type IsolationLevel int

const (
    IsolationNone      IsolationLevel = iota
    IsolationNamespace
    IsolationSession
)

// IsolationKey identifies the namespace and session for a request
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

// ProcessHandle represents a handle to a managed process
type ProcessHandle struct {
    ID     procmgr.ProcessID
    Key    IsolationKey
    Config *ProcessConfig
    Health bool
}

// IsolationManager manages process isolation
type IsolationManager struct {
    // ... internal fields
}
```

### Core Functions

```go
// NewIsolationManager creates a new isolation manager
func NewIsolationManager(level IsolationLevel, syncer ProcessSyncer, opts ...procmgr.Option) *IsolationManager

// GetOrCreateProcess gets an existing process or creates a new one
func (im *IsolationManager) GetOrCreateProcess(ctx context.Context, key IsolationKey, config *ProcessConfig) (*ProcessHandle, error)

// GetProcess returns a handle to an existing process
func (im *IsolationManager) GetProcess(key IsolationKey) *ProcessHandle

// TerminateProcess gracefully terminates a process
func (im *IsolationManager) TerminateProcess(ctx context.Context, key IsolationKey, gracePeriodSec int64) error

// ListProcesses returns handles to all managed processes
func (im *IsolationManager) ListProcesses() []*ProcessHandle

// Health returns health information for all processes
func (im *IsolationManager) Health() procmgr.HealthCheck

// Shutdown gracefully shuts down all processes
func (im *IsolationManager) Shutdown(ctx context.Context) error

// Level returns the current isolation level
func (im *IsolationManager) Level() IsolationLevel
```

## Testing

### Run Tests

```bash
go test -v -timeout 90s
```

### Test Coverage

All 10 tests pass:
- ✅ IsolationLevel string representation
- ✅ ProcessID generation for all levels
- ✅ None isolation (shared process)
- ✅ Namespace isolation (separate processes per namespace)
- ✅ Session isolation (separate processes per session)
- ✅ GetProcess for existing processes
- ✅ TerminateProcess with graceful shutdown
- ✅ ListProcesses showing all processes
- ✅ Health reporting
- ✅ Concurrent access from 10 goroutines

## Integration with Pattern Runner

The isolation package is designed to integrate with the pattern runner framework:

```go
// Pattern runner can use isolation manager to route requests
type PatternRunner struct {
    isolationMgr *isolation.IsolationManager
}

func (pr *PatternRunner) RouteRequest(ctx context.Context, req Request) (*Response, error) {
    key := isolation.IsolationKey{
        Namespace: req.Namespace,
        Session:   req.SessionID,
    }

    // Get or create isolated process for this request
    handle, err := pr.isolationMgr.GetOrCreateProcess(ctx, key, config)
    if err != nil {
        return nil, err
    }

    // Route request to the isolated process
    return pr.executeInProcess(ctx, handle, req)
}
```

## Benefits

1. **Fault Isolation**: Failures in one namespace/session don't affect others
2. **Resource Isolation**: Each namespace/session can have separate resource limits
3. **Security**: Better isolation between tenants or sessions
4. **Observability**: Per-namespace/session metrics and health tracking
5. **Graceful Degradation**: Failed processes can be restarted independently

## References

1. [procmgr package](../procmgr/README.md) - Underlying process management
2. [RFC-034: Robust Process Manager Package](../../docs-cms/rfcs/RFC-034-robust-process-manager.md)
3. [Bulkhead Pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/bulkhead) - Microsoft Architecture Patterns

## License

Part of the Prism Data Access Gateway project.
