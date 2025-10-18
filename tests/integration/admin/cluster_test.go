package admin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminClusterBasic tests basic 3-node cluster functionality
func TestAdminClusterBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Find prism-admin executable
	executable := findPrismAdminExecutable(t)

	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19001",
		2: "127.0.0.1:19002",
		3: "127.0.0.1:19003",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18001, 17001, 19001, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18002, 17002, 19002, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18003, 17003, 19003, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for leader election
	leaderID, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err, "Leader election failed")
	t.Logf("Leader elected: %s", leaderID)

	// Get leader node
	leader, ok := harness.GetNode(leaderID)
	require.True(t, ok, "Leader node not found")

	// Test 1: Register a proxy via leader
	t.Run("RegisterProxy", func(t *testing.T) {
		resp, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
			ProxyId:      "proxy-01",
			Address:      "proxy-01.local:8080",
			Region:       "us-west-2",
			Version:      "1.0.0",
			Capabilities: []string{"keyvalue", "pubsub"},
			Metadata:     map[string]string{"zone": "us-west-2a"},
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		t.Logf("Proxy registered: %+v", resp)
	})

	// Test 2: Create a namespace
	t.Run("CreateNamespace", func(t *testing.T) {
		resp, err := leader.GRPCClient.CreateNamespace(ctx, &pb.CreateNamespaceRequest{
			Namespace:       "test-namespace",
			RequestingProxy: "proxy-01",
			Principal:       "test-user",
			Config: &pb.NamespaceConfig{
				Metadata: map[string]string{
					"environment": "test",
				},
			},
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		t.Logf("Namespace created: partition=%d, proxy=%s",
			resp.AssignedPartition, resp.AssignedProxy)
	})

	// Test 3: Register a launcher
	t.Run("RegisterLauncher", func(t *testing.T) {
		resp, err := leader.GRPCClient.RegisterLauncher(ctx, &pb.LauncherRegistration{
			LauncherId:   "launcher-01",
			Address:      "launcher-01.local:9090",
			Region:       "us-west-2",
			Version:      "1.0.0",
			ProcessTypes: []string{"consumer", "producer"},
			MaxProcesses: 10,
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		t.Logf("Launcher registered: %+v", resp)
	})

	// Test 4: Send heartbeat
	t.Run("ProxyHeartbeat", func(t *testing.T) {
		resp, err := leader.GRPCClient.Heartbeat(ctx, &pb.ProxyHeartbeat{
			ProxyId:   "proxy-01",
			Timestamp: time.Now().Unix(),
			Resources: &pb.ResourceUsage{
				CpuPercent:     25.5,
				MemoryMb:       512,
				GoroutineCount: 100,
				UptimeSeconds:  3600,
			},
		})
		require.NoError(t, err)
		assert.True(t, resp.Success)
		t.Logf("Heartbeat acknowledged: %s", resp.Message)
	})
}

// TestAdminClusterLeaderFailover tests leader failure and re-election
func TestAdminClusterLeaderFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executable := findPrismAdminExecutable(t)
	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19011",
		2: "127.0.0.1:19012",
		3: "127.0.0.1:19013",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18011, 17011, 19011, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18012, 17012, 19012, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18013, 17013, 19013, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for initial leader
	initialLeader, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err)
	t.Logf("Initial leader: %s", initialLeader)

	// Register a proxy via initial leader
	leader, _ := harness.GetNode(initialLeader)
	resp, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-failover-test",
		Address: "proxy-01.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Kill the leader (simulate crash)
	t.Logf("Killing leader node %s to test failover", initialLeader)
	require.NoError(t, harness.KillNode(initialLeader))

	// Wait for new leader election
	time.Sleep(2 * time.Second) // Give Raft time to detect failure

	newLeader, err := harness.WaitForLeader(ctx, 15*time.Second)
	require.NoError(t, err)
	require.NotEqual(t, initialLeader, newLeader, "New leader should be different from killed leader")
	t.Logf("New leader elected: %s", newLeader)

	// Verify new leader accepts operations
	newLeaderNode, _ := harness.GetNode(newLeader)
	resp2, err := newLeaderNode.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-after-failover",
		Address: "proxy-02.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp2.Success)
	t.Logf("Successfully registered proxy via new leader")
}

// TestAdminClusterNodeRestart tests node restart and rejoin
func TestAdminClusterNodeRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executable := findPrismAdminExecutable(t)
	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19021",
		2: "127.0.0.1:19022",
		3: "127.0.0.1:19023",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18021, 17021, 19021, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18022, 17022, 19022, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18023, 17023, 19023, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for leader
	leaderID, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err)

	// Pick a follower to restart
	var followerID string
	for nodeID := range harness.Nodes {
		if nodeID != leaderID {
			followerID = nodeID
			break
		}
	}

	t.Logf("Stopping follower node %s", followerID)
	require.NoError(t, harness.StopNode(followerID))

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Restart the follower
	t.Logf("Restarting node %s", followerID)
	require.NoError(t, harness.RestartNode(ctx, followerID, peers))
	require.NoError(t, harness.ConnectToNode(ctx, followerID))

	// Wait for node to catch up
	time.Sleep(3 * time.Second)

	// Verify cluster is still functional
	leader, _ := harness.GetNode(leaderID)
	resp, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-after-restart",
		Address: "proxy-03.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	t.Logf("Cluster operational after node restart")
}

// TestAdminClusterMinorityFailure tests that cluster survives minority node failure
func TestAdminClusterMinorityFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executable := findPrismAdminExecutable(t)
	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19031",
		2: "127.0.0.1:19032",
		3: "127.0.0.1:19033",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18031, 17031, 19031, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18032, 17032, 19032, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18033, 17033, 19033, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for leader
	leaderID, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err)

	// Kill one follower (minority failure)
	var followerID string
	for nodeID := range harness.Nodes {
		if nodeID != leaderID {
			followerID = nodeID
			break
		}
	}

	t.Logf("Killing one follower node %s (1 of 3 nodes)", followerID)
	require.NoError(t, harness.KillNode(followerID))

	// Wait for cluster to detect failure
	time.Sleep(2 * time.Second)

	// Cluster should still be operational with 2/3 nodes
	leader, _ := harness.GetNode(leaderID)
	resp, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-minority-failure",
		Address: "proxy-04.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	t.Logf("Cluster still operational with 2/3 nodes (quorum maintained)")
}

// TestAdminClusterMajorityFailure tests that cluster becomes unavailable on majority failure
func TestAdminClusterMajorityFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executable := findPrismAdminExecutable(t)
	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19041",
		2: "127.0.0.1:19042",
		3: "127.0.0.1:19043",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18041, 17041, 19041, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18042, 17042, 19042, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18043, 17043, 19043, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for leader
	_, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err)

	// Kill two nodes (majority failure)
	var killCount int
	for nodeID := range harness.Nodes {
		if killCount < 2 {
			t.Logf("Killing node %s (%d of 2)", nodeID, killCount+1)
			require.NoError(t, harness.KillNode(nodeID))
			killCount++
		}
	}

	// Wait for cluster to detect failures
	time.Sleep(3 * time.Second)

	// Find the surviving node
	var survivorID string
	for nodeID := range harness.Nodes {
		survivorID = nodeID
		break
	}

	// Cluster should be unavailable (no quorum)
	survivor, _ := harness.GetNode(survivorID)

	// Set short timeout for this operation
	opCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err = survivor.GRPCClient.RegisterProxy(opCtx, &pb.ProxyRegistration{
		ProxyId: "proxy-majority-failure",
		Address: "proxy-05.local:8080",
	})

	// Should fail due to no quorum
	assert.Error(t, err, "Operation should fail without quorum")
	t.Logf("Cluster correctly unavailable with only 1/3 nodes (no quorum): %v", err)
}

// TestAdminClusterNetworkPartition tests network partition (split brain scenario)
func TestAdminClusterNetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test simulates network partition by blocking ports
	// NOTE: Requires ability to manipulate firewall rules (may need sudo in some environments)
	// For now, we simulate partition by killing connections rather than using iptables

	executable := findPrismAdminExecutable(t)
	harness := NewAdminClusterHarness(t, executable)
	defer harness.Cleanup()

	ctx := context.Background()

	// Define 3-node cluster
	peers := map[uint64]string{
		1: "127.0.0.1:19051",
		2: "127.0.0.1:19052",
		3: "127.0.0.1:19053",
	}

	// Start all nodes
	require.NoError(t, harness.StartNode(ctx, "node1", 1, 18051, 17051, 19051, peers))
	require.NoError(t, harness.StartNode(ctx, "node2", 2, 18052, 17052, 19052, peers))
	require.NoError(t, harness.StartNode(ctx, "node3", 3, 18053, 17053, 19053, peers))

	// Connect to all nodes
	require.NoError(t, harness.ConnectToNode(ctx, "node1"))
	require.NoError(t, harness.ConnectToNode(ctx, "node2"))
	require.NoError(t, harness.ConnectToNode(ctx, "node3"))

	// Wait for initial leader
	initialLeader, err := harness.WaitForLeader(ctx, 10*time.Second)
	require.NoError(t, err)
	t.Logf("Initial leader: %s", initialLeader)

	// Register a proxy via initial leader
	leader, _ := harness.GetNode(initialLeader)
	resp, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-partition-test",
		Address: "proxy-01.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Simulate network partition: Kill one node to create minority partition
	// In a real network partition, we'd use iptables/firewall rules to block traffic
	// between specific nodes. For this test, we'll kill one node to simulate
	// losing network connectivity to it.
	var partitionedNode string
	for nodeID := range harness.Nodes {
		if nodeID != initialLeader {
			partitionedNode = nodeID
			break
		}
	}

	t.Logf("Simulating network partition by isolating node %s", partitionedNode)
	require.NoError(t, harness.KillNode(partitionedNode))

	// Wait for cluster to detect partition
	time.Sleep(3 * time.Second)

	// Majority partition (2/3 nodes) should still be operational
	// Try to register another proxy
	resp2, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-after-partition",
		Address: "proxy-02.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp2.Success)
	t.Logf("Majority partition still operational with 2/3 nodes")

	// Heal the partition by restarting the isolated node
	t.Logf("Healing partition by restarting node %s", partitionedNode)
	require.NoError(t, harness.RestartNode(ctx, partitionedNode, peers))
	require.NoError(t, harness.ConnectToNode(ctx, partitionedNode))

	// Wait for node to rejoin
	time.Sleep(3 * time.Second)

	// Verify cluster is fully operational again
	resp3, err := leader.GRPCClient.RegisterProxy(ctx, &pb.ProxyRegistration{
		ProxyId: "proxy-after-heal",
		Address: "proxy-03.local:8080",
	})
	require.NoError(t, err)
	assert.True(t, resp3.Success)
	t.Logf("Cluster operational after partition healed (all 3 nodes)")
}

// Helper function to find prism-admin executable
func findPrismAdminExecutable(t *testing.T) string {
	// Try common locations
	locations := []string{
		"../../../build/binaries/prism-admin",
		"build/binaries/prism-admin",
		"./build/binaries/prism-admin",
	}

	// Get working directory
	wd, err := os.Getwd()
	require.NoError(t, err)

	for _, loc := range locations {
		absPath := filepath.Join(wd, loc)
		if _, err := os.Stat(absPath); err == nil {
			t.Logf("Found prism-admin at: %s", absPath)
			return absPath
		}
	}

	t.Fatal("Could not find prism-admin executable. Run 'make build-prism-admin' first.")
	return ""
}
