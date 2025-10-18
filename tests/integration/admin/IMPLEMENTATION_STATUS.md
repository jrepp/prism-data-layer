# Admin Integration Tests - Implementation Status

## Current Status

⚠️ **Tests are ready but require Raft-enabled prism-admin implementation**

The integration test framework and all test scenarios are complete and ready to use. However, the tests currently fail because `prism-admin serve` does not yet support the Raft clustering flags.

## What's Missing in prism-admin

The `prism-admin serve` command needs to be updated to support:

### Required Flags

```bash
--raft-id <id>         # Raft node ID (1, 2, 3, etc.)
--raft-port <port>     # Port for Raft consensus protocol
--http-port <port>     # HTTP API port
--grpc-port <port>     # gRPC control plane port
--data-dir <path>      # Directory for Raft logs and snapshots
--cluster <peers>      # Comma-separated list of Raft peers (e.g., "1=127.0.0.1:9001,2=127.0.0.1:9002")
```

### Current Implementation

```bash
# Current flags (non-Raft)
--port <port>          # Control plane gRPC port (default 8981)
--listen <addr>        # Listen address (default 0.0.0.0)
--metrics-port <port>  # Prometheus metrics port (default 9090)
--db <urn>             # Database URN
```

## What Needs to Be Done

1. **Update serve.go** to integrate RaftNode from RFC-038 implementation:
   - Add Raft configuration flags
   - Initialize RaftNode with FSM (AdminStateMachine)
   - Start Raft transport layer
   - Replace ControlPlaneService with ControlPlaneServiceRaft
   - Handle leader forwarding

2. **Files that need updating**:
   - `cmd/prism-admin/serve.go` - Add Raft flags and initialization
   - Integration already exists in:
     - `cmd/prism-admin/raft.go` - RaftNode implementation ✅
     - `cmd/prism-admin/fsm.go` - AdminStateMachine ✅
     - `cmd/prism-admin/control_plane_raft.go` - Raft-integrated service ✅

## Test Framework Status

✅ **Complete and Ready**:
- Process manager (pkg/testing/procmgr)
- Admin cluster harness (tests/integration/admin)
- 6 comprehensive test scenarios:
  1. TestAdminClusterBasic
  2. TestAdminClusterLeaderFailover
  3. TestAdminClusterNodeRestart
  4. TestAdminClusterMinorityFailure
  5. TestAdminClusterMajorityFailure
  6. TestAdminClusterNetworkPartition

## Timeline

### Phase 1: Minimal Raft Integration (1-2 days)
- Update `serve.go` to start Raft node
- Wire up ControlPlaneServiceRaft
- Bootstrap cluster on first start

### Phase 2: Test Validation (1 day)
- Run integration tests
- Fix any Raft configuration issues
- Validate leader election and failover

### Phase 3: Production Readiness (2-3 days)
- Add graceful shutdown
- Implement health checks
- Add monitoring/observability
- Performance testing

## Running Tests (Future)

Once prism-admin supports Raft:

```bash
# Build with Raft support
make build-prism-admin

# Run all integration tests
make test-integration-admin

# Run specific test
cd tests/integration/admin
go test -v -run TestAdminClusterLeaderFailover
```

## Current Workaround

For now, use the in-process unit tests in `cmd/prism-admin/`:

```bash
cd cmd/prism-admin
go test -v -run TestSingleNodeCluster        # Basic FSM testing
go test -v -run TestThreeNodeCluster         # 3-node Raft cluster
go test -v -run TestLeaderElection           # Leader election
```

These tests work because they directly instantiate RaftNode and FSM without
going through the CLI serve command.

## References

- [RFC-038: Admin Leader Election and High Availability](/rfc/rfc-038)
- [cmd/prism-admin/raft.go](/cmd/prism-admin/raft.go) - RaftNode implementation
- [cmd/prism-admin/control_plane_raft.go](/cmd/prism-admin/control_plane_raft.go) - Raft-integrated gRPC service
- [Integration Test README](./README.md) - Test framework documentation

## Questions?

Contact the team or see ADR-055/ADR-056 for control plane architecture.
