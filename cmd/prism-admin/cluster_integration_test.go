package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	adminpb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/admin"
	"google.golang.org/protobuf/proto"
)

// TestSingleNodeCluster tests 1-node cluster with storage sync
func TestSingleNodeCluster(t *testing.T) {
	// Setup test directory
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "node1.db")
	raftDir := filepath.Join(testDir, "node1-raft")

	// Create storage
	storage, err := NewStorage(context.Background(), &DatabaseConfig{
		Type: "sqlite",
		Path: dbPath,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create FSM with storage
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fsm := NewAdminStateMachine(log.With("node", "1"), nil, storage)

	// Create Raft config
	raftCfg := &RaftConfig{
		ID:      1,
		Cluster: map[uint64]string{1: "127.0.0.1:9000"},
		DataDir: raftDir,
	}

	// Create Raft node
	raftNode, err := NewRaftNode(raftCfg, fsm, nil, log.With("component", "raft"))
	if err != nil {
		t.Fatalf("failed to create raft node: %v", err)
	}

	// Start Raft
	if err := raftNode.Start(context.Background(), "127.0.0.1:9000"); err != nil {
		t.Fatalf("failed to start raft: %v", err)
	}
	defer raftNode.Stop()

	// Bootstrap single-node cluster
	if err := raftNode.Bootstrap(raftCfg.Cluster); err != nil {
		t.Fatalf("failed to bootstrap: %v", err)
	}

	// Wait for leader election
	if err := raftNode.WaitForLeader(5 * time.Second); err != nil {
		t.Fatalf("leader election timeout: %v", err)
	}

	if !raftNode.IsLeader() {
		t.Fatal("expected node to be leader in single-node cluster")
	}

	// Test 1: Register a proxy via Raft
	t.Run("RegisterProxy", func(t *testing.T) {
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_RegisterProxy{
				RegisterProxy: &adminpb.RegisterProxyCommand{
					ProxyId: "proxy-01",
					Address: "proxy-01:8080",
					Region:  "us-west-2",
					Version: "1.0.0",
				},
			},
		}

		data, _ := proto.Marshal(cmd)
		if err := raftNode.Propose(context.Background(), data); err != nil {
			t.Fatalf("failed to propose: %v", err)
		}

		// Give Raft time to apply
		time.Sleep(100 * time.Millisecond)

		// Verify FSM state
		proxy, exists := fsm.GetProxy("proxy-01")
		if !exists {
			t.Fatal("proxy not found in FSM")
		}
		if proxy.Address != "proxy-01:8080" {
			t.Errorf("expected address 'proxy-01:8080', got '%s'", proxy.Address)
		}

		// Verify storage sync
		storedProxy, err := storage.GetProxy(context.Background(), "proxy-01")
		if err != nil {
			t.Fatalf("proxy not found in storage: %v", err)
		}
		if storedProxy.Address != "proxy-01:8080" {
			t.Errorf("storage: expected address 'proxy-01:8080', got '%s'", storedProxy.Address)
		}
	})

	// Test 2: Register a launcher
	t.Run("RegisterLauncher", func(t *testing.T) {
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_RegisterLauncher{
				RegisterLauncher: &adminpb.RegisterLauncherCommand{
					LauncherId:   "launcher-01",
					Address:      "launcher-01:9090",
					Region:       "us-west-2",
					Version:      "1.0.0",
					MaxProcesses: 10,
				},
			},
		}

		data, _ := proto.Marshal(cmd)
		if err := raftNode.Propose(context.Background(), data); err != nil {
			t.Fatalf("failed to propose: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// Verify storage sync
		storedLauncher, err := storage.GetLauncher(context.Background(), "launcher-01")
		if err != nil {
			t.Fatalf("launcher not found in storage: %v", err)
		}
		if storedLauncher.MaxProcesses != 10 {
			t.Errorf("expected max_processes 10, got %d", storedLauncher.MaxProcesses)
		}
	})

	// Test 3: Create a namespace
	t.Run("CreateNamespace", func(t *testing.T) {
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_CreateNamespace{
				CreateNamespace: &adminpb.CreateNamespaceCommand{
					Namespace:     "test-ns",
					PartitionId:   1,
					AssignedProxy: "proxy-01",
					Principal:     "test-user",
				},
			},
		}

		data, _ := proto.Marshal(cmd)
		if err := raftNode.Propose(context.Background(), data); err != nil {
			t.Fatalf("failed to propose: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// Verify storage sync
		storedNS, err := storage.GetNamespace(context.Background(), "test-ns")
		if err != nil {
			t.Fatalf("namespace not found in storage: %v", err)
		}
		if storedNS.Name != "test-ns" {
			t.Errorf("expected name 'test-ns', got '%s'", storedNS.Name)
		}
	})
}

// TestThreeNodeCluster tests 3-node cluster with leader election and failover
func TestThreeNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 3-node cluster test in short mode")
	}

	// Setup test directories
	testDir := t.TempDir()
	nodes := make([]*testNode, 3)

	// Create 3 nodes
	cluster := map[uint64]string{
		1: "127.0.0.1:19001",
		2: "127.0.0.1:19002",
		3: "127.0.0.1:19003",
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	for i := 1; i <= 3; i++ {
		nodeID := uint64(i)
		node, err := createTestNode(testDir, nodeID, cluster, log.With("node", i))
		if err != nil {
			t.Fatalf("failed to create node %d: %v", i, err)
		}
		nodes[i-1] = node
		defer node.close()
	}

	// Start all nodes
	for i, node := range nodes {
		if err := node.raft.Start(context.Background(), cluster[uint64(i+1)]); err != nil {
			t.Fatalf("failed to start node %d: %v", i+1, err)
		}
	}

	// Bootstrap cluster (only first node)
	if err := nodes[0].raft.Bootstrap(cluster); err != nil {
		t.Fatalf("failed to bootstrap cluster: %v", err)
	}

	// Wait for leader election
	time.Sleep(2 * time.Second)

	// Find leader
	var leaderIdx int
	var leaderFound bool
	for i, node := range nodes {
		if node.raft.IsLeader() {
			leaderIdx = i
			leaderFound = true
			t.Logf("Node %d is the leader", i+1)
			break
		}
	}

	if !leaderFound {
		t.Fatal("no leader elected in 3-node cluster")
	}

	leader := nodes[leaderIdx]

	// Test 1: Propose command on leader and verify replication to all nodes
	t.Run("ReplicateToAllNodes", func(t *testing.T) {
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_RegisterProxy{
				RegisterProxy: &adminpb.RegisterProxyCommand{
					ProxyId: "proxy-cluster",
					Address: "proxy-cluster:8080",
					Version: "1.0.0",
				},
			},
		}

		data, _ := proto.Marshal(cmd)
		if err := leader.raft.Propose(context.Background(), data); err != nil {
			t.Fatalf("failed to propose: %v", err)
		}

		// Wait for replication
		time.Sleep(500 * time.Millisecond)

		// Verify all nodes have the proxy in FSM
		for i, node := range nodes {
			proxy, exists := node.fsm.GetProxy("proxy-cluster")
			if !exists {
				t.Errorf("node %d: proxy not replicated to FSM", i+1)
				continue
			}
			if proxy.Address != "proxy-cluster:8080" {
				t.Errorf("node %d: expected address 'proxy-cluster:8080', got '%s'", i+1, proxy.Address)
			}

			// Verify storage sync on all nodes
			storedProxy, err := node.storage.GetProxy(context.Background(), "proxy-cluster")
			if err != nil {
				t.Errorf("node %d: proxy not synced to storage: %v", i+1, err)
				continue
			}
			if storedProxy.Address != "proxy-cluster:8080" {
				t.Errorf("node %d: storage address mismatch", i+1)
			}
		}
	})

	// Test 2: Stop leader and verify new leader election
	t.Run("LeaderFailover", func(t *testing.T) {
		oldLeaderID := leader.id
		t.Logf("Stopping leader node %d", oldLeaderID)

		if err := leader.raft.Stop(); err != nil {
			t.Fatalf("failed to stop leader: %v", err)
		}

		// Wait for new leader election (increased timeout to account for election timeout)
		time.Sleep(5 * time.Second)

		// Find new leader
		var newLeaderIdx int
		var newLeaderFound bool
		for i, node := range nodes {
			if uint64(i+1) == oldLeaderID {
				continue // Skip stopped node
			}
			if node.raft.IsLeader() {
				newLeaderIdx = i
				newLeaderFound = true
				t.Logf("Node %d is the new leader", i+1)
				break
			}
		}

		if !newLeaderFound {
			t.Fatal("no new leader elected after failover")
		}

		newLeader := nodes[newLeaderIdx]

		// Propose new command on new leader
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_RegisterProxy{
				RegisterProxy: &adminpb.RegisterProxyCommand{
					ProxyId: "proxy-after-failover",
					Address: "proxy-after-failover:8080",
					Version: "1.0.0",
				},
			},
		}

		data, _ := proto.Marshal(cmd)
		if err := newLeader.raft.Propose(context.Background(), data); err != nil {
			t.Fatalf("failed to propose on new leader: %v", err)
		}

		time.Sleep(500 * time.Millisecond)

		// Verify remaining nodes have new command
		for i, node := range nodes {
			if uint64(i+1) == oldLeaderID {
				continue // Skip stopped node
			}

			_, exists := node.fsm.GetProxy("proxy-after-failover")
			if !exists {
				t.Errorf("node %d: proxy not replicated after failover", i+1)
				continue
			}

			// Verify storage sync
			storedProxy, err := node.storage.GetProxy(context.Background(), "proxy-after-failover")
			if err != nil {
				t.Errorf("node %d: proxy not synced to storage after failover: %v", i+1, err)
			} else if storedProxy.Address != "proxy-after-failover:8080" {
				t.Errorf("node %d: storage address mismatch after failover", i+1)
			}
		}
	})
}

// TestFiveNodeCluster tests 5-node cluster with complex failover scenarios
func TestFiveNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5-node cluster test in short mode")
	}

	// Setup test directories
	testDir := t.TempDir()
	nodes := make([]*testNode, 5)

	// Create 5 nodes
	cluster := map[uint64]string{
		1: "127.0.0.1:29001",
		2: "127.0.0.1:29002",
		3: "127.0.0.1:29003",
		4: "127.0.0.1:29004",
		5: "127.0.0.1:29005",
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	for i := 1; i <= 5; i++ {
		nodeID := uint64(i)
		node, err := createTestNode(testDir, nodeID, cluster, log.With("node", i))
		if err != nil {
			t.Fatalf("failed to create node %d: %v", i, err)
		}
		nodes[i-1] = node
		defer node.close()
	}

	// Start all nodes
	for i, node := range nodes {
		if err := node.raft.Start(context.Background(), cluster[uint64(i+1)]); err != nil {
			t.Fatalf("failed to start node %d: %v", i+1, err)
		}
	}

	// Bootstrap cluster
	if err := nodes[0].raft.Bootstrap(cluster); err != nil {
		t.Fatalf("failed to bootstrap cluster: %v", err)
	}

	// Wait for leader election
	time.Sleep(3 * time.Second)

	// Find leader
	var leaderIdx int
	for i, node := range nodes {
		if node.raft.IsLeader() {
			leaderIdx = i
			t.Logf("Node %d is the leader", i+1)
			break
		}
	}

	// Test: Propose commands and verify all 5 nodes sync to storage
	t.Run("ReplicateToAllFiveNodes", func(t *testing.T) {
		leader := nodes[leaderIdx]

		// Propose multiple commands
		for i := 1; i <= 5; i++ {
			cmd := &adminpb.Command{
				Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
				Timestamp: time.Now().Unix(),
				Issuer:    "test",
				Payload: &adminpb.Command_RegisterProxy{
					RegisterProxy: &adminpb.RegisterProxyCommand{
						ProxyId: fmt.Sprintf("proxy-%d", i),
						Address: fmt.Sprintf("proxy-%d:8080", i),
						Version: "1.0.0",
					},
				},
			}

			data, _ := proto.Marshal(cmd)
			if err := leader.raft.Propose(context.Background(), data); err != nil {
				t.Fatalf("failed to propose command %d: %v", i, err)
			}
		}

		// Wait for replication
		time.Sleep(1 * time.Second)

		// Verify all 5 nodes have all 5 proxies in both FSM and storage
		for nodeIdx, node := range nodes {
			for proxyNum := 1; proxyNum <= 5; proxyNum++ {
				proxyID := fmt.Sprintf("proxy-%d", proxyNum)

				// Check FSM
				proxy, exists := node.fsm.GetProxy(proxyID)
				if !exists {
					t.Errorf("node %d: %s not found in FSM", nodeIdx+1, proxyID)
					continue
				}
				expectedAddr := fmt.Sprintf("proxy-%d:8080", proxyNum)
				if proxy.Address != expectedAddr {
					t.Errorf("node %d: %s address mismatch in FSM", nodeIdx+1, proxyID)
				}

				// Check storage
				storedProxy, err := node.storage.GetProxy(context.Background(), proxyID)
				if err != nil {
					t.Errorf("node %d: %s not synced to storage: %v", nodeIdx+1, proxyID, err)
					continue
				}
				if storedProxy.Address != expectedAddr {
					t.Errorf("node %d: %s address mismatch in storage", nodeIdx+1, proxyID)
				}
			}
		}

		t.Logf("Successfully replicated 5 commands to all 5 nodes (FSM + Storage)")
	})
}

// testNode represents a test Raft node with storage
type testNode struct {
	id      uint64
	raft    *RaftNode
	fsm     *AdminStateMachine
	storage *Storage
}

func createTestNode(baseDir string, nodeID uint64, cluster map[uint64]string, log *slog.Logger) (*testNode, error) {
	nodeDir := filepath.Join(baseDir, fmt.Sprintf("node%d", nodeID))
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return nil, err
	}

	// Create storage
	dbPath := filepath.Join(nodeDir, "admin.db")
	storage, err := NewStorage(context.Background(), &DatabaseConfig{
		Type: "sqlite",
		Path: dbPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create FSM with storage
	fsm := NewAdminStateMachine(log.With("component", "fsm"), nil, storage)

	// Create Raft config
	raftCfg := &RaftConfig{
		ID:      nodeID,
		Cluster: cluster,
		DataDir: filepath.Join(nodeDir, "raft"),
	}

	// Create Raft node
	raftNode, err := NewRaftNode(raftCfg, fsm, nil, log.With("component", "raft"))
	if err != nil {
		storage.Close()
		return nil, fmt.Errorf("failed to create raft node: %w", err)
	}

	return &testNode{
		id:      nodeID,
		raft:    raftNode,
		fsm:     fsm,
		storage: storage,
	}, nil
}

func (n *testNode) close() {
	if n.raft != nil {
		n.raft.Stop()
	}
	if n.storage != nil {
		n.storage.Close()
	}
}
