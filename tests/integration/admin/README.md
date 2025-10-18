---
title: Admin Cluster Integration Tests
description: Multi-node Raft cluster testing using process-based test harness
type: testing
status: active
tags: [testing, raft, integration, admin, failover]
---

# Admin Cluster Integration Tests

Process-based integration tests for prism-admin multi-node Raft clusters. Tests real process lifecycle, failure scenarios, and cluster operations.

## Overview

This test suite validates the prism-admin Raft implementation by launching real `prism-admin` processes and testing:

- **Multi-node cluster operations** - 3-node cluster with leader election
- **Failure scenarios** - Leader crashes, node restarts, minority/majority failures
- **State replication** - Raft log replication across nodes
- **API operations** - RegisterProxy, CreateNamespace, Heartbeat, etc.

**Key Innovation**: Uses `procmgr` to manage real process lifecycles, simulating production-like failures (SIGKILL, graceful shutdown, restarts).

## Architecture

```
Test Harness
├── procmgr/               # Process lifecycle manager
│   ├── ProcessManager     # Manages multiple processes
│   └── ManagedProcess     # Single process lifecycle
│
└── admin/
    ├── cluster_harness.go # Admin-specific cluster management
    ├── cluster_test.go    # Integration test suite
    └── README.md          # This file
```

### Components

**procmgr (pkg/testing/procmgr)**:
- Generic process lifecycle management
- Start/Stop/Kill/Restart operations
- Log file capture
- Uptime tracking
- Signal handling (SIGTERM, SIGKILL)

**AdminClusterHarness (tests/integration/admin)**:
- Admin-specific cluster orchestration
- Node lifecycle (StartNode, StopNode, KillNode, RestartNode)
- Leader election waiting
- gRPC client management
- Log retrieval for debugging

## Test Scenarios

### TestAdminClusterBasic
**Duration**: ~15s
**Nodes**: 3
**Tests**:
- Leader election
- RegisterProxy operation
- CreateNamespace with partition assignment
- RegisterLauncher operation
- Proxy heartbeat

### TestAdminClusterLeaderFailover
**Duration**: ~20s
**Nodes**: 3
**Tests**:
- Initial leader election
- Proxy registration via initial leader
- Leader crash (SIGKILL)
- New leader election
- Proxy registration via new leader

**Validates**: Raft leader election after failure

### TestAdminClusterNodeRestart
**Duration**: ~20s
**Nodes**: 3
**Tests**:
- Follower graceful shutdown (SIGTERM)
- Follower restart
- Cluster remains operational
- State preserved after restart

**Validates**: Node rejoin after restart

### TestAdminClusterMinorityFailure
**Duration**: ~15s
**Nodes**: 3 (1 killed)
**Tests**:
- Kill 1 of 3 nodes
- Cluster remains operational (quorum = 2/3)
- Operations succeed via leader

**Validates**: Cluster survives minority failure

### TestAdminClusterMajorityFailure
**Duration**: ~15s
**Nodes**: 3 (2 killed)
**Tests**:
- Kill 2 of 3 nodes
- Cluster becomes unavailable (no quorum)
- Operations fail with timeout/error

**Validates**: Cluster correctly becomes unavailable without quorum

## Running Tests

### Prerequisites

```bash
# Build prism-admin executable
make build-prism-admin

# Executable must be at: build/binaries/prism-admin
```

### Run All Tests

```bash
# Full suite (~15min timeout)
make test-integration-admin

# Or directly
cd tests/integration/admin && go test -v -timeout 15m ./...
```

### Run Specific Test

```bash
cd tests/integration/admin
go test -v -run TestAdminClusterLeaderFailover
```

### Short Mode (Skip Long Tests)

```bash
# Runs basic tests only (~5min)
make test-integration-admin-short

# Or directly
cd tests/integration/admin && go test -v -timeout 5m -short ./...
```

## Port Allocation

Tests use fixed port ranges to avoid conflicts:

| Test Scenario          | HTTP Ports  | gRPC Ports  | Raft Ports  |
|------------------------|-------------|-------------|-------------|
| Basic                  | 18001-18003 | 17001-17003 | 19001-19003 |
| LeaderFailover         | 18011-18013 | 17011-17013 | 19011-19013 |
| NodeRestart            | 18021-18023 | 17021-17023 | 19021-19023 |
| MinorityFailure        | 18031-18033 | 17031-17033 | 19031-19033 |
| MajorityFailure        | 18041-18043 | 17041-17043 | 19041-19043 |

## Debugging

### View Logs

Each node writes logs to `<testDir>/node-<id>.log`:

```bash
# Logs are in temp directory (shown in test output)
# Example:
cat /tmp/TestAdminClusterBasic*/node-node1.log
```

### Logs in Code

```go
logs, err := harness.GetNodeLogs("node1")
if err != nil {
    t.Logf("Failed to get logs: %v", err)
} else {
    t.Logf("Node1 logs:\n%s", logs)
}
```

### Increase Verbosity

```bash
# Run with -v flag for verbose output
go test -v -run TestAdminClusterBasic
```

## CI Integration

Tests are integrated into CI pipeline:

```yaml
# .github/workflows/ci.yml
- name: Test Admin Cluster Integration
  run: make test-integration-admin-short
```

**Note**: Short mode used in CI to keep build times reasonable.

## Adding New Tests

### 1. Define Port Range

Choose unused port range (see table above):
```go
peers := map[uint64]string{
    1: "127.0.0.1:19051",  // Unique Raft ports
    2: "127.0.0.1:19052",
    3: "127.0.0.1:19053",
}

httpPort := 18051  // HTTP base port
grpcPort := 17051  // gRPC base port
raftPort := 19051  // Raft base port
```

### 2. Create Test Function

```go
func TestAdminClusterMyScenario(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    executable := findPrismAdminExecutable(t)
    harness := NewAdminClusterHarness(t, executable)
    defer harness.Cleanup()

    ctx := context.Background()

    // Define cluster
    peers := map[uint64]string{...}

    // Start nodes
    require.NoError(t, harness.StartNode(ctx, "node1", 1, ...))

    // Your test logic here
}
```

### 3. Test Patterns

**Leader Operations**:
```go
leaderID, err := harness.WaitForLeader(ctx, 10*time.Second)
leader, _ := harness.GetNode(leaderID)
resp, err := leader.GRPCClient.RegisterProxy(ctx, ...)
```

**Failure Injection**:
```go
harness.KillNode("node1")              // Crash
harness.StopNode("node1")              // Graceful shutdown
harness.RestartNode(ctx, "node1", ...) // Restart
```

**Verification**:
```go
require.NoError(t, err)
assert.True(t, resp.Success)
```

## Performance Characteristics

| Test | Duration | Nodes | Operations |
|------|----------|-------|------------|
| Basic | ~15s | 3 | 4 RPCs |
| LeaderFailover | ~20s | 3 | 2 RPCs + crash + election |
| NodeRestart | ~20s | 3 | 1 RPC + restart + rejoin |
| MinorityFailure | ~15s | 2/3 | 1 RPC + crash |
| MajorityFailure | ~15s | 1/3 | 1 RPC (fails) + 2 crashes |

**Total Suite**: ~1-2 minutes (varies with leader election timing)

## Troubleshooting

### "Could not find prism-admin executable"

```bash
# Build the executable
make build-prism-admin

# Verify it exists
ls -lh build/binaries/prism-admin
```

### "Address already in use"

- Another test or prism-admin process is running
- Kill existing processes: `pkill prism-admin`
- Check port availability: `lsof -i :17001`

### "Leader election timeout"

- Increase timeout in test: `harness.WaitForLeader(ctx, 30*time.Second)`
- Check node logs for errors
- Verify network connectivity (localhost)

### Tests hang indefinitely

- Check for deadlocks in harness cleanup
- Ensure `defer harness.Cleanup()` is called
- Increase test timeout: `-timeout 20m`

## References

- [RFC-038: Admin Leader Election and High Availability](/rfc/rfc-038)
- [ADR-055: Control Plane Proxy Registration](/adr/adr-055)
- [ADR-056: Launcher Registration Protocol](/adr/adr-056)
- [Raft Consensus Algorithm](https://raft.github.io/)

## Future Enhancements

- [ ] Network partition simulation (iptables/tc)
- [ ] Disk failure scenarios
- [ ] Log compaction testing
- [ ] Snapshot restore testing
- [ ] Membership changes (add/remove nodes)
- [ ] Performance benchmarks (throughput, latency)
- [ ] Chaos testing (random failure injection)
