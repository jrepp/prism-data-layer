---
date: 2025-01-16
deciders: Core Team
doc_uuid: 8c2e5f3d-9a1b-4c5e-b6d7-4f8e9a0b1c2d
id: adr-058
project_id: prism-data-layer
status: Accepted
tags:
- proxy
- lifecycle
- shutdown
- drain
- reliability
title: Proxy Drain-on-Shutdown
---

# ADR-058: Proxy Drain-on-Shutdown

## Status

**Accepted** - 2025-01-16

## Context

The prism-proxy needs graceful shutdown behavior when signaled to stop by prism-admin or receiving a termination signal. Current implementation immediately stops accepting connections and kills pattern processes, which can result in:

- Lost in-flight requests from clients
- Aborted backend operations mid-transaction
- Incomplete data writes
- Poor user experience during rolling updates

### Requirements

1. **Frontend Drain Phase**: Stop accepting NEW frontend connections while completing existing requests
2. **Backend Work Completion**: Wait for all backend operations attached to frontend work to complete
3. **Pattern Runner Coordination**: Signal pattern runners to drain (finish current work, reject new work)
4. **Clean Exit**: Only exit when all frontend connections closed AND all pattern processes exited
5. **Timeout Safety**: Force shutdown after timeout to prevent indefinite hangs

### Current Architecture

```text
┌─────────────────┐
│  prism-admin    │ (sends stop command)
└────────┬────────┘
         │ gRPC
         ▼
┌─────────────────┐
│   prism-proxy   │◄──── Frontend gRPC connections (KeyValue, PubSub, etc.)
└────────┬────────┘
         │ Lifecycle gRPC
         ▼
┌─────────────────┐
│ Pattern Runners │ (keyvalue-runner, consumer-runner, etc.)
│  (Go processes) │
└────────┬────────┘
         │
         ▼
    Backends (Redis, NATS, Postgres, etc.)
```

## Decision

### State Machine

Proxy states during shutdown:

```text
Running → Draining → Stopping → Stopped
```

1. **Running**: Normal operation, accepting all connections
2. **Draining**:
   - Reject NEW frontend connections (return UNAVAILABLE)
   - Complete existing frontend requests
   - Signal pattern runners to drain
   - Track active request count
3. **Stopping**:
   - All frontend connections closed
   - Send Stop to pattern runners
   - Wait for pattern processes to exit
4. **Stopped**: Clean exit

### Implementation Components

#### 1. Lifecycle.proto Extension

Add `DrainRequest` message to `lifecycle.proto`:

```protobuf
// Drain request tells pattern to enter drain mode
message DrainRequest {
  // Graceful drain timeout in seconds
  int32 timeout_seconds = 1;

  // Reason for drain (for logging/debugging)
  string reason = 2;
}
```

Add to `ProxyCommand` in `proxy_control_plane.proto`:

```protobuf
message ProxyCommand {
  string correlation_id = 1;

  oneof command {
    // ... existing commands ...
    DrainRequest drain = 8;  // NEW
  }
}
```

#### 2. ProxyServer Drain State

Add connection tracking and drain state to `ProxyServer`:

```rust
pub struct ProxyServer {
    router: Arc&lt;Router&gt;,
    listen_address: String,
    shutdown_tx: Option&lt;oneshot::Sender&lt;()&gt;&gt;,

    // NEW: Drain state
    drain_state: Arc&lt;RwLock&lt;DrainState&gt;&gt;,
    active_connections: Arc&lt;AtomicUsize&gt;,
}

enum DrainState {
    Running,
    Draining { started_at: Instant },
    Stopping,
}
```

#### 3. Frontend Connection Interception

Use tonic interceptor to reject new connections during drain:

```rust
fn connection_interceptor(
    mut req: Request&lt;()&gt;,
    drain_state: Arc&lt;RwLock&lt;DrainState&gt;&gt;,
) -&gt; Result&lt;Request&lt;()&gt;, Status&gt; {
    let state = drain_state.read().await;
    match *state {
        DrainState::Draining { .. } | DrainState::Stopping =&gt; {
            Err(Status::unavailable("Server is draining, not accepting new connections"))
        }
        DrainState::Running =&gt; Ok(req),
    }
}
```

#### 4. Pattern Runner Drain Logic

Pattern runners receive `DrainRequest` via control plane and:

1. Stop accepting new work (return UNAVAILABLE on new RPCs)
2. Complete pending backend operations
3. Send completion signal back to proxy
4. Wait for Stop command

Example in `keyvalue-runner`:

```go
func (a *KeyValuePluginAdapter) Drain(ctx context.Context, timeoutSeconds int32) error {
    log.Printf("[DRAIN] Entering drain mode (timeout: %ds)", timeoutSeconds)

    // Set drain flag
    a.draining.Store(true)

    // Wait for pending operations (with timeout)
    deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
    for a.pendingOps.Load() &gt; 0 {
        if time.Now().After(deadline) {
            log.Printf("[DRAIN] Timeout waiting for %d pending ops", a.pendingOps.Load())
            break
        }
        time.Sleep(50 * time.Millisecond)
    }

    log.Printf("[DRAIN] Drain complete, ready for stop")
    return nil
}
```

#### 5. Shutdown Orchestration

New `ProxyServer::drain_and_shutdown()` method:

```rust
pub async fn drain_and_shutdown(&amp;mut self, timeout: Duration) -&gt; Result&lt;()&gt; {
    // Phase 1: Enter drain mode
    {
        let mut state = self.drain_state.write().await;
        *state = DrainState::Draining { started_at: Instant::now() };
    }
    info!("🔸 Entering DRAIN mode");

    // Phase 2: Signal pattern runners to drain
    self.router.pattern_manager.drain_all_patterns(timeout).await?;

    // Phase 3: Wait for frontend connections to complete
    let poll_interval = Duration::from_millis(100);
    let deadline = Instant::now() + timeout;

    while self.active_connections.load(Ordering::Relaxed) &gt; 0 {
        if Instant::now() &gt; deadline {
            warn!("⏱️  Drain timeout, {} connections still active",
                  self.active_connections.load(Ordering::Relaxed));
            break;
        }
        sleep(poll_interval).await;
    }

    info!("✅ All frontend connections drained");

    // Phase 4: Stop pattern runners
    {
        let mut state = self.drain_state.write().await;
        *state = DrainState::Stopping;
    }
    info!("🔹 Entering STOPPING mode");

    self.router.pattern_manager.stop_all_patterns().await?;

    // Phase 5: Shutdown gRPC server
    if let Some(tx) = self.shutdown_tx.take() {
        let _ = tx.send(());
    }

    info!("✅ Proxy shutdown complete");
    Ok(())
}
```

### Admin Control Plane Integration

Admin sends drain command via new RPC:

```protobuf
service AdminControlPlane {
  // ... existing RPCs ...

  // Initiate graceful drain and shutdown
  rpc DrainProxy(DrainProxyRequest) returns (DrainProxyResponse);
}

message DrainProxyRequest {
  int32 timeout_seconds = 1;
  string reason = 2;
}

message DrainProxyResponse {
  bool success = 1;
  string message = 2;
}
```

### Timeout Handling

- **Default drain timeout**: 30 seconds
- **Configurable via**: Admin request or environment variable `PRISM_DRAIN_TIMEOUT_SECONDS`
- **Behavior on timeout**: Force shutdown with warning logs
- **Per-component timeouts**:
  - Frontend connections: 30s
  - Pattern runners: 30s
  - Backend operations: Determined by pattern runner logic

## Consequences

### Positive

- ✅ **Zero data loss**: All in-flight operations complete before shutdown
- ✅ **Graceful rolling updates**: Kubernetes can drain pods safely
- ✅ **Better observability**: Clear state transitions logged
- ✅ **Configurable timeouts**: Operators control drain duration
- ✅ **Backwards compatible**: Existing Stop behavior preserved as fallback

### Negative

- ⚠️ **Increased shutdown time**: From instant to 30+ seconds
- ⚠️ **Complexity**: More state tracking and coordination logic
- ⚠️ **Potential timeout issues**: Slow backends can cause forced shutdowns

### Risks

- **Stuck drains**: If backend operations hang, timeout must force shutdown
  - *Mitigation*: Configurable timeouts, forced kill after 2x timeout
- **Connection leaks**: If connections aren't tracked properly
  - *Mitigation*: Comprehensive integration tests with connection counting

## Implementation Plan

1. **Phase 1**: Protobuf changes (lifecycle.proto, proxy_control_plane.proto)
2. **Phase 2**: ProxyServer drain state and connection tracking
3. **Phase 3**: Pattern runner drain logic (plugin SDK changes)
4. **Phase 4**: Admin control plane drain RPC
5. **Phase 5**: Integration tests with real backend operations
6. **Phase 6**: Documentation and runbooks

## Testing Strategy

### Unit Tests

- State transitions (Running → Draining → Stopping → Stopped)
- Connection counting accuracy
- Timeout enforcement

### Integration Tests

1. **Happy path**: Start proxy, send requests, drain, verify completion
2. **Timeout path**: Long-running operations, verify forced shutdown
3. **Connection rejection**: New connections during drain return UNAVAILABLE
4. **Pattern coordination**: Multiple pattern runners drain in parallel

### Load Testing

- 1000 concurrent connections
- Trigger drain mid-load
- Measure: completion rate, drain duration, error rate

## References

- RFC-016: Local Development Infrastructure (shutdown patterns)
- ADR-048: Local Signoz Observability (shutdown tracing)
- ADR-055: Control Plane Connectivity (admin → proxy communication)
- Kubernetes Pod Lifecycle: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-termination

## Related Work

Similar patterns in industry:

- **Envoy**: `drain_listeners` + `drain_connections_on_host_removal`
- **gRPC Go**: `GracefulStop()` drains connections before shutdown
- **Kubernetes**: `preStop` hooks + `terminationGracePeriodSeconds`
