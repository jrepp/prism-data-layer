package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

// RaftNode wraps Hashicorp Raft with our admin state machine.
// It handles Raft consensus, log replication, and snapshot management.
//
// Reference: RFC-038 Admin Leader Election with Hashicorp Raft
type RaftNode struct {
	// Configuration
	id       string // Node ID ("1", "2", "3")
	bindAddr string // Raft bind address (0.0.0.0:8990)
	dataDir  string // Data directory

	// Raft components
	raft      *raft.Raft
	transport *raft.NetworkTransport
	fsm       *AdminStateMachine

	// Metrics
	metrics       *AdminMetrics
	lastLeaderID  raft.ServerID
	stopMetrics   chan struct{}

	// Logger
	log *slog.Logger
}

// RaftConfig holds configuration for Raft node
type RaftConfig struct {
	ID               uint64            // Node ID (1, 2, 3)
	Cluster          map[uint64]string // Peer URLs (1->http://node1:8990, 2->http://node2:8990)
	DataDir          string            // Data directory
	SnapCount        uint64            // Entries before snapshot (default 10000)
	HeartbeatTick    int               // Heartbeat ticks (default 1)
	ElectionTick     int               // Election timeout ticks (default 10)
}

// NewRaftNode creates a new Raft node with the given configuration
func NewRaftNode(cfg *RaftConfig, fsm *AdminStateMachine, metrics *AdminMetrics, log *slog.Logger) (*RaftNode, error) {
	rn := &RaftNode{
		id:          fmt.Sprintf("%d", cfg.ID),
		dataDir:     cfg.DataDir,
		fsm:         fsm,
		metrics:     metrics,
		stopMetrics: make(chan struct{}),
		log:         log,
	}

	// Setup directories
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return rn, nil
}

// Start starts the Raft node
func (rn *RaftNode) Start(ctx context.Context, bindAddr string) error {
	rn.bindAddr = bindAddr

	// Create Raft configuration
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(rn.id)
	config.Logger = hclog.NewInterceptLogger(&hclog.LoggerOptions{
		Name:   "raft",
		Level:  hclog.Debug,
		Output: os.Stderr,
	})

	// Configure timeouts based on RFC-038 requirements
	// LeaderLeaseTimeout must be <= HeartbeatTimeout per Hashicorp Raft
	config.HeartbeatTimeout = 1000 * time.Millisecond
	config.ElectionTimeout = 3000 * time.Millisecond
	config.CommitTimeout = 50 * time.Millisecond
	config.LeaderLeaseTimeout = 500 * time.Millisecond
	config.SnapshotThreshold = 10000 // Trigger snapshot after 10k entries

	// Setup transport
	addr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve bind address: %w", err)
	}

	transport, err := raft.NewTCPTransport(bindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	rn.transport = transport

	// Setup stable store (BoltDB)
	boltDB, err := raftboltdb.NewBoltStore(filepath.Join(rn.dataDir, "raft.db"))
	if err != nil {
		return fmt.Errorf("failed to create bolt store: %w", err)
	}

	// Setup log store (same as stable store)
	logStore := boltDB

	// Setup snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(rn.dataDir, 3, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Create Raft instance
	ra, err := raft.NewRaft(config, rn.fsm, logStore, boltDB, snapshotStore, transport)
	if err != nil {
		return fmt.Errorf("failed to create raft: %w", err)
	}
	rn.raft = ra

	// Start metrics collection loop
	go rn.collectMetrics()

	rn.log.Info("raft node started",
		"node_id", rn.id,
		"bind_addr", bindAddr,
		"data_dir", rn.dataDir)

	return nil
}

// collectMetrics periodically collects Raft metrics
func (rn *RaftNode) collectMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if rn.raft == nil || rn.metrics == nil {
				continue
			}

			// Get Raft stats
			stats := rn.raft.Stats()

			// Parse state (Follower=0, Candidate=1, Leader=2, Shutdown=3)
			var stateNum uint64
			switch rn.raft.State() {
			case raft.Follower:
				stateNum = 0
			case raft.Candidate:
				stateNum = 1
			case raft.Leader:
				stateNum = 2
			case raft.Shutdown:
				stateNum = 3
			}

			// Parse term and index from stats
			var term, index uint64
			fmt.Sscanf(stats["term"], "%d", &term)
			fmt.Sscanf(stats["last_log_index"], "%d", &index)

			// Update Raft state metrics
			rn.metrics.RecordRaftState(stateNum, term, index)

			// Detect leader changes
			leaderAddr, leaderID := rn.raft.LeaderWithID()
			if leaderID != rn.lastLeaderID && rn.lastLeaderID != "" {
				rn.metrics.RecordLeaderChange()
				rn.log.Info("leader changed",
					"old_leader", rn.lastLeaderID,
					"new_leader", leaderID,
					"leader_addr", leaderAddr)
			}
			rn.lastLeaderID = leaderID

			// Track if we became leader
			if rn.raft.State() == raft.Leader && stateNum == 2 {
				// Leader election completed
				rn.metrics.RecordLeaderElection()
			}

			// Update cluster metrics
			config := rn.raft.GetConfiguration()
			if err := config.Error(); err == nil {
				clusterSize := len(config.Configuration().Servers)
				// TODO: Track healthy peers (requires health checking)
				rn.metrics.UpdateClusterMetrics(clusterSize, clusterSize-1)
			}

			// Update FSM state metrics
			rn.metrics.UpdateAdminStateMetrics(
				len(rn.fsm.state.Proxies),
				len(rn.fsm.state.Launchers),
				len(rn.fsm.state.Namespaces),
				len(rn.fsm.state.Patterns),
			)

		case <-rn.stopMetrics:
			return
		}
	}
}

// Bootstrap bootstraps a new Raft cluster with the given configuration
func (rn *RaftNode) Bootstrap(peers map[uint64]string) error {
	// Convert peers to Raft server configuration
	var servers []raft.Server
	for id, addr := range peers {
		servers = append(servers, raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("%d", id)),
			Address: raft.ServerAddress(addr),
		})
	}

	configuration := raft.Configuration{
		Servers: servers,
	}

	// Bootstrap the cluster
	future := rn.raft.BootstrapCluster(configuration)
	if err := future.Error(); err != nil {
		// Ignore error if already bootstrapped
		if err != raft.ErrCantBootstrap {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
		}
	}

	rn.log.Info("cluster bootstrapped", "servers", len(servers))
	return nil
}

// Stop stops the Raft node
func (rn *RaftNode) Stop() error {
	if rn.raft == nil {
		return nil
	}

	// Stop metrics collection (idempotent with select)
	select {
	case <-rn.stopMetrics:
		// Already closed
	default:
		close(rn.stopMetrics)
	}

	future := rn.raft.Shutdown()
	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to shutdown raft: %w", err)
	}

	rn.log.Info("raft node stopped")
	return nil
}

// Propose proposes a new command to Raft
func (rn *RaftNode) Propose(ctx context.Context, data []byte) error {
	start := time.Now()

	future := rn.raft.Apply(data, 3*time.Second)
	err := future.Error()

	// Record metrics
	duration := time.Since(start)
	if rn.metrics != nil {
		rn.metrics.RecordRaftProposal(err == nil, duration)
	}

	if err != nil {
		return fmt.Errorf("failed to apply: %w", err)
	}
	return nil
}

// IsLeader returns true if this node is the leader
func (rn *RaftNode) IsLeader() bool {
	return rn.raft.State() == raft.Leader
}

// GetLeader returns the current leader ID
func (rn *RaftNode) GetLeader() uint64 {
	leaderAddr, _ := rn.raft.LeaderWithID()
	if leaderAddr == "" {
		return 0
	}
	// Parse ID from address
	var id uint64
	fmt.Sscanf(string(leaderAddr), "%d", &id)
	return id
}

// GetLeaderAddr returns the leader's control plane address (for forwarding)
func (rn *RaftNode) GetLeaderAddr() string {
	addr, _ := rn.raft.LeaderWithID()
	if addr == "" {
		return ""
	}
	// Convert Raft address to control plane address
	// Raft: admin-01:8990 → Control Plane: admin-01:8980
	host, _, err := net.SplitHostPort(string(addr))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:8980", host)
}

// WaitForLeader waits for a leader to be elected
func (rn *RaftNode) WaitForLeader(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if rn.IsLeader() || rn.GetLeader() != 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for leader election")
}

// ====================================================================
// Hashicorp logger adapter
// ====================================================================

type hashicorpLogger struct {
	log *slog.Logger
}

func newHashicorpLogger(log *slog.Logger) *hashicorpLogger {
	return &hashicorpLogger{log: log}
}

func (l *hashicorpLogger) Trace(msg string, args ...interface{}) {
	l.log.Debug(msg, argsToAttrs(args)...)
}

func (l *hashicorpLogger) Debug(msg string, args ...interface{}) {
	l.log.Debug(msg, argsToAttrs(args)...)
}

func (l *hashicorpLogger) Info(msg string, args ...interface{}) {
	l.log.Info(msg, argsToAttrs(args)...)
}

func (l *hashicorpLogger) Warn(msg string, args ...interface{}) {
	l.log.Warn(msg, argsToAttrs(args)...)
}

func (l *hashicorpLogger) Error(msg string, args ...interface{}) {
	l.log.Error(msg, argsToAttrs(args)...)
}

func (l *hashicorpLogger) Log(level hclog.Level, msg string, args ...interface{}) {
	switch level {
	case hclog.Trace, hclog.Debug:
		l.log.Debug(msg, argsToAttrs(args)...)
	case hclog.Info:
		l.log.Info(msg, argsToAttrs(args)...)
	case hclog.Warn:
		l.log.Warn(msg, argsToAttrs(args)...)
	case hclog.Error:
		l.log.Error(msg, argsToAttrs(args)...)
	}
}

func (l *hashicorpLogger) IsTrace() bool { return false }
func (l *hashicorpLogger) IsDebug() bool { return true }
func (l *hashicorpLogger) IsInfo() bool  { return true }
func (l *hashicorpLogger) IsWarn() bool  { return true }
func (l *hashicorpLogger) IsError() bool { return true }
func (l *hashicorpLogger) GetLevel() hclog.Level { return hclog.Debug }
func (l *hashicorpLogger) SetLevel(level hclog.Level) {}

func (l *hashicorpLogger) With(args ...interface{}) any {
	return &hashicorpLogger{log: l.log.With(argsToAttrs(args)...)}
}

func (l *hashicorpLogger) Named(name string) any {
	return &hashicorpLogger{log: l.log.With("name", name)}
}

func (l *hashicorpLogger) ImpliedArgs() []interface{} { return nil }

func argsToAttrs(args []interface{}) []interface{} {
	// Convert Hashicorp's key-value pairs to slog attributes
	attrs := make([]interface{}, 0, len(args))
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			attrs = append(attrs, args[i], args[i+1])
		}
	}
	return attrs
}
