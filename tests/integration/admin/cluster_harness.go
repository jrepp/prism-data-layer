package admin_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/testing/procmgr"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AdminClusterHarness manages a multi-node prism-admin Raft cluster for testing
type AdminClusterHarness struct {
	T          *testing.T
	ProcMgr    *procmgr.ProcessManager
	TestDir    string
	Nodes      map[string]*AdminNode
	Executable string
}

// AdminNode represents a single prism-admin node in the cluster
type AdminNode struct {
	ID           string
	RaftID       uint64
	HTTPPort     int
	GRPCPort     int
	RaftPort     int
	DataDir      string
	LogFile      string
	GRPCClient   pb.ControlPlaneClient
	GRPCConn     *grpc.ClientConn
}

// NewAdminClusterHarness creates a new test harness for admin clusters
func NewAdminClusterHarness(t *testing.T, executable string) *AdminClusterHarness {
	testDir := t.TempDir()

	return &AdminClusterHarness{
		T:          t,
		ProcMgr:    procmgr.NewProcessManager(testDir),
		TestDir:    testDir,
		Nodes:      make(map[string]*AdminNode),
		Executable: executable,
	}
}

// StartNode starts a single prism-admin node
func (h *AdminClusterHarness) StartNode(ctx context.Context, nodeID string, raftID uint64, httpPort, grpcPort, raftPort int, peers map[uint64]string) error {
	dataDir := filepath.Join(h.TestDir, fmt.Sprintf("node-%s", nodeID))
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	logFile := filepath.Join(h.TestDir, fmt.Sprintf("node-%s.log", nodeID))

	node := &AdminNode{
		ID:       nodeID,
		RaftID:   raftID,
		HTTPPort: httpPort,
		GRPCPort: grpcPort,
		RaftPort: raftPort,
		DataDir:  dataDir,
		LogFile:  logFile,
	}

	// Build cluster string for Raft peers
	clusterStr := ""
	for peerID, peerAddr := range peers {
		if clusterStr != "" {
			clusterStr += ","
		}
		clusterStr += fmt.Sprintf("%d=%s", peerID, peerAddr)
	}

	// Start prism-admin process
	cfg := procmgr.ProcessConfig{
		ID:         nodeID,
		Executable: h.Executable,
		Args: []string{
			"serve",
			"--raft-id", fmt.Sprintf("%d", raftID),
			"--http-port", fmt.Sprintf("%d", httpPort),
			"--grpc-port", fmt.Sprintf("%d", grpcPort),
			"--raft-port", fmt.Sprintf("%d", raftPort),
			"--data-dir", dataDir,
			"--cluster", clusterStr,
		},
		LogFile:     logFile,
		StartupWait: 2 * time.Second, // Wait for node to start
	}

	proc, err := h.ProcMgr.Start(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}

	h.T.Logf("Started node %s (PID: %d) - HTTP:%d, gRPC:%d, Raft:%d",
		nodeID, proc.PID(), httpPort, grpcPort, raftPort)

	h.Nodes[nodeID] = node
	return nil
}

// ConnectToNode establishes a gRPC connection to a node
func (h *AdminClusterHarness) ConnectToNode(ctx context.Context, nodeID string) error {
	node, exists := h.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Create gRPC connection
	addr := fmt.Sprintf("localhost:%d", node.GRPCPort)

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
	}

	node.GRPCConn = conn
	node.GRPCClient = pb.NewControlPlaneClient(conn)

	h.T.Logf("Connected to node %s at %s", nodeID, addr)
	return nil
}

// StopNode gracefully stops a single node
func (h *AdminClusterHarness) StopNode(nodeID string) error {
	node, exists := h.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Close gRPC connection
	if node.GRPCConn != nil {
		node.GRPCConn.Close()
		node.GRPCConn = nil
		node.GRPCClient = nil
	}

	// Stop process
	if err := h.ProcMgr.Stop(nodeID); err != nil {
		return err
	}

	h.T.Logf("Stopped node %s", nodeID)
	return nil
}

// KillNode forcefully kills a node (simulates crash)
func (h *AdminClusterHarness) KillNode(nodeID string) error {
	node, exists := h.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Close gRPC connection
	if node.GRPCConn != nil {
		node.GRPCConn.Close()
		node.GRPCConn = nil
		node.GRPCClient = nil
	}

	// Kill process
	if err := h.ProcMgr.Kill(nodeID); err != nil {
		return err
	}

	h.T.Logf("Killed node %s (simulated crash)", nodeID)
	return nil
}

// RestartNode restarts a stopped node
func (h *AdminClusterHarness) RestartNode(ctx context.Context, nodeID string, peers map[uint64]string) error {
	node, exists := h.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Start node with same configuration
	return h.StartNode(ctx, nodeID, node.RaftID, node.HTTPPort, node.GRPCPort, node.RaftPort, peers)
}

// WaitForLeader waits for a leader to be elected
func (h *AdminClusterHarness) WaitForLeader(ctx context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for leader election")
		case <-ticker.C:
			// Check each node to see if it's the leader
			for nodeID, node := range h.Nodes {
				if node.GRPCClient == nil {
					continue
				}

				// Try to get cluster status or perform operation that only leader accepts
				// For now, we'll just check if node responds
				healthCtx, healthCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				_, err := node.GRPCClient.RegisterProxy(healthCtx, &pb.ProxyRegistration{
					ProxyId: "health-check-probe",
					Address: "localhost:0",
				})
				healthCancel()

				// If we get a response (even error), node is responsive
				// In real implementation, we'd have a GetStatus RPC to check leadership
				if err == nil || err.Error() != "context deadline exceeded" {
					h.T.Logf("Node %s appears to be leader or responding", nodeID)
					return nodeID, nil
				}
			}
		}
	}
}

// Cleanup stops all nodes and cleans up resources
func (h *AdminClusterHarness) Cleanup() {
	// Close all gRPC connections
	for _, node := range h.Nodes {
		if node.GRPCConn != nil {
			node.GRPCConn.Close()
		}
	}

	// Stop all processes
	if err := h.ProcMgr.StopAll(); err != nil {
		h.T.Logf("Warning: failed to stop all processes: %v", err)
	}

	h.T.Logf("Cleaned up admin cluster harness")
}

// GetNode returns a node by ID
func (h *AdminClusterHarness) GetNode(nodeID string) (*AdminNode, bool) {
	node, ok := h.Nodes[nodeID]
	return node, ok
}

// GetNodeLogs returns the log file contents for a node
func (h *AdminClusterHarness) GetNodeLogs(nodeID string) (string, error) {
	node, exists := h.Nodes[nodeID]
	if !exists {
		return "", fmt.Errorf("node %s not found", nodeID)
	}

	data, err := os.ReadFile(node.LogFile)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
