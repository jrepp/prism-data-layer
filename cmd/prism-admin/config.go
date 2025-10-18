package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// ClusterConfig holds Raft cluster configuration
type ClusterConfig struct {
	// Node identity
	NodeID       uint64 // Unique node ID (1, 2, 3)
	BindAddr     string // Raft transport address (0.0.0.0:8990)
	AdvertiseAddr string // Address advertised to peers (admin-01.prism.local:8990)

	// Cluster members
	Peers        map[uint64]string // Peer map (1->http://admin-01:8990, 2->http://admin-02:8990)

	// Raft tuning
	HeartbeatTick    int    // Ticks between heartbeats (default 1 = 100ms)
	ElectionTick     int    // Ticks before election timeout (default 10 = 1s)
	SnapshotInterval uint64 // Log entries before snapshot (default 10000)
	SnapshotThreshold uint64 // Snapshot size threshold MB (default 50)
	TrailingLogs     uint64 // Keep this many logs after snapshot (default 10240)

	// Read consistency
	EnableFollowerReads bool   // Allow stale reads from followers
	MaxStaleness        string // Max acceptable staleness (default 200ms)
	LeaseDuration       string // Leader lease duration (default 10s)

	// Storage
	DataDir       string // Raft data directory
	LogRetention  string // How long to keep logs (default 7d)
}

// ControlPlaneConfig holds control plane server configuration
type ControlPlaneConfig struct {
	Listen string // Listen address for control plane gRPC (0.0.0.0:8981)

	// Default read consistency per operation
	ReadConsistency map[string]string
}

// AdminAPIConfig holds admin API configuration
type AdminAPIConfig struct {
	Listen                  string // Listen address for admin API (0.0.0.0:8980)
	DefaultReadConsistency  string // Default for CLI operations (stale, lease-based, linearizable)
}

// LoadConfig loads configuration from viper
func LoadConfig() (*ClusterConfig, *ControlPlaneConfig, *AdminAPIConfig, error) {
	// Cluster configuration
	cluster := &ClusterConfig{
		NodeID:              viper.GetUint64("cluster.node_id"),
		HeartbeatTick:       viper.GetInt("cluster.raft.heartbeat_tick"),
		ElectionTick:        viper.GetInt("cluster.raft.election_tick"),
		SnapshotInterval:    viper.GetUint64("cluster.raft.snapshot_interval"),
		SnapshotThreshold:   viper.GetUint64("cluster.raft.snapshot_size_mb"),
		TrailingLogs:        viper.GetUint64("cluster.raft.trailing_logs"),
		EnableFollowerReads: viper.GetBool("cluster.raft.enable_follower_reads"),
		MaxStaleness:        viper.GetString("cluster.raft.max_staleness"),
		LeaseDuration:       viper.GetString("cluster.raft.lease_duration"),
		DataDir:             viper.GetString("cluster.data_dir"),
		LogRetention:        viper.GetString("cluster.log_retention"),
	}

	// Get raft port first
	raftPort := viper.GetInt("cluster.raft_port")
	if raftPort == 0 {
		raftPort = 8990
	}

	// Build bind address from raft-addr and raft-port
	// Default to localhost instead of 0.0.0.0 for security
	// Note: We'll potentially update this after parsing peers if needed
	raftAddr := viper.GetString("cluster.bind_addr")
	if raftAddr == "" {
		raftAddr = "127.0.0.1"
	}
	tempBindAddr := fmt.Sprintf("%s:%d", raftAddr, raftPort)

	// Parse peers from comma-separated string or slice FIRST
	var peersStr string
	if viper.IsSet("cluster.peers") {
		// Could be a string or slice, handle both
		if peerSlice := viper.GetStringSlice("cluster.peers"); len(peerSlice) > 0 {
			peersStr = peerSlice[0] // If it's a slice, take first element
		} else {
			peersStr = viper.GetString("cluster.peers")
		}
	}

	cluster.Peers = make(map[uint64]string)

	if peersStr != "" {
		// Parse peers from string (e.g., "1=localhost:19001,2=localhost:19002,3=localhost:19003")
		cluster.Peers = parsePeersFromEnv(peersStr)
	}

	// Auto-compute node ID if not specified
	if cluster.NodeID == 0 {
		if len(cluster.Peers) == 0 {
			// Single-node mode: default to ID 1
			cluster.NodeID = 1
			fmt.Println("[INFO] No --raft-id specified, defaulting to 1 (single-node mode)")
		} else {
			// Multi-node mode: find first available ID by matching bind address to peer list
			localAddr := fmt.Sprintf("%s:%d", raftAddr, raftPort)
			for id, peerAddr := range cluster.Peers {
				// Check if this peer address matches our bind address
				if peerAddr == localAddr || peerAddr == fmt.Sprintf("localhost:%d", raftPort) ||
				   peerAddr == fmt.Sprintf("127.0.0.1:%d", raftPort) {
					cluster.NodeID = id
					fmt.Printf("[INFO] No --raft-id specified, auto-detected as %d based on bind address %s\n", id, localAddr)
					break
				}
			}

			// If still not found, use the first peer ID as fallback
			if cluster.NodeID == 0 {
				for id := range cluster.Peers {
					cluster.NodeID = id
					fmt.Printf("[INFO] No --raft-id specified and no match found, using first peer ID: %d\n", id)
					break
				}
			}
		}
	}

	// Build advertise address AFTER parsing peers
	advertiseAddr := viper.GetString("cluster.advertise_addr")
	if advertiseAddr == "" && cluster.NodeID > 0 {
		// Try to extract from peers if available
		if len(cluster.Peers) > 0 && cluster.Peers[cluster.NodeID] != "" {
			// Peer addresses are already in TCP format (no http:// prefix)
			advertiseAddr = cluster.Peers[cluster.NodeID]
		} else {
			// Default to localhost:<raft-port> for testing
			advertiseAddr = fmt.Sprintf("localhost:%d", raftPort)
		}
	}
	cluster.AdvertiseAddr = advertiseAddr

	// If bind address is 0.0.0.0 and we have an advertise address, use the advertise
	// address for binding. This is necessary because Hashicorp Raft doesn't allow
	// 0.0.0.0 as an advertisable address.
	// Note: Default is now 127.0.0.1, but user might explicitly set 0.0.0.0
	if raftAddr == "0.0.0.0" && advertiseAddr != "" {
		// Extract the host from advertise address
		host, _, err := net.SplitHostPort(advertiseAddr)
		if err == nil {
			tempBindAddr = fmt.Sprintf("%s:%d", host, raftPort)
			fmt.Printf("[INFO] Bind address was 0.0.0.0, using advertise address host: %s\n", host)
		}
	}
	cluster.BindAddr = tempBindAddr

	// Environment variable overrides (for Docker/K8s)
	if nodeIDStr := os.Getenv("PRISM_NODE_ID"); nodeIDStr != "" {
		if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil {
			cluster.NodeID = id
		}
	}
	if bindAddr := os.Getenv("PRISM_BIND_ADDR"); bindAddr != "" {
		cluster.BindAddr = bindAddr
	}
	if advertiseAddr := os.Getenv("PRISM_ADVERTISE_ADDR"); advertiseAddr != "" {
		cluster.AdvertiseAddr = advertiseAddr
	}
	if peersStr := os.Getenv("PRISM_PEERS"); peersStr != "" {
		cluster.Peers = parsePeersFromEnv(peersStr)
	}

	// Defaults
	if cluster.HeartbeatTick == 0 {
		cluster.HeartbeatTick = 1
	}
	if cluster.ElectionTick == 0 {
		cluster.ElectionTick = 10
	}
	if cluster.SnapshotInterval == 0 {
		cluster.SnapshotInterval = 10000
	}
	if cluster.SnapshotThreshold == 0 {
		cluster.SnapshotThreshold = 50
	}
	if cluster.TrailingLogs == 0 {
		cluster.TrailingLogs = 10240
	}
	if cluster.MaxStaleness == "" {
		cluster.MaxStaleness = "200ms"
	}
	if cluster.LeaseDuration == "" {
		cluster.LeaseDuration = "10s"
	}
	if cluster.DataDir == "" {
		cluster.DataDir = "/var/lib/prism-admin/raft"
	}
	if cluster.LogRetention == "" {
		cluster.LogRetention = "7d"
	}

	// Control plane configuration
	controlPlane := &ControlPlaneConfig{
		ReadConsistency: viper.GetStringMapString("control_plane.read_consistency"),
	}

	// Build listen address from CLI flag or config
	grpcPort := viper.GetInt("control_plane.listen")
	if grpcPort == 0 {
		grpcPort = 8981 // Default gRPC port
	}
	// Handle both "8981" and "0.0.0.0:8981" formats
	if grpcPort < 65536 {
		controlPlane.Listen = fmt.Sprintf("0.0.0.0:%d", grpcPort)
	} else {
		// If it's not a valid port, try to parse as string
		listenStr := viper.GetString("control_plane.listen")
		if listenStr != "" {
			controlPlane.Listen = listenStr
		} else {
			controlPlane.Listen = "0.0.0.0:8981"
		}
	}

	// Default consistency levels per operation
	if len(controlPlane.ReadConsistency) == 0 {
		controlPlane.ReadConsistency = map[string]string{
			"proxy_heartbeat":       "stale",
			"proxy_registration":    "linearizable",
			"namespace_get":         "stale",
			"namespace_list":        "stale",
			"namespace_create":      "linearizable",
			"pattern_assignment":    "linearizable",
			"launcher_heartbeat":    "stale",
			"launcher_registration": "linearizable",
		}
	}

	// Admin API configuration
	adminAPI := &AdminAPIConfig{
		DefaultReadConsistency: viper.GetString("admin_api.default_read_consistency"),
	}

	// Build listen address from CLI flag or config
	httpPort := viper.GetInt("admin_api.listen")
	if httpPort == 0 {
		httpPort = 8980 // Default HTTP port
	}
	// Handle both "8980" and "0.0.0.0:8980" formats
	if httpPort < 65536 {
		adminAPI.Listen = fmt.Sprintf("0.0.0.0:%d", httpPort)
	} else {
		// If it's not a valid port, try to parse as string
		listenStr := viper.GetString("admin_api.listen")
		if listenStr != "" {
			adminAPI.Listen = listenStr
		} else {
			adminAPI.Listen = "0.0.0.0:8980"
		}
	}

	if adminAPI.DefaultReadConsistency == "" {
		adminAPI.DefaultReadConsistency = "stale"
	}

	// Validate configuration
	if err := validateClusterConfig(cluster); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid cluster config: %w", err)
	}

	return cluster, controlPlane, adminAPI, nil
}

// parsePeersFromEnv parses peers from environment variable
// Format: "1=admin-01:8990,2=admin-02:8990,3=admin-03:8990"
// Or: "admin-01:8990,admin-02:8990,admin-03:8990" (auto-assign IDs)
// NOTE: Raft uses TCP transport, so we DON'T add http:// prefix
func parsePeersFromEnv(peersStr string) map[uint64]string {
	peers := make(map[uint64]string)
	parts := strings.Split(peersStr, ",")

	for i, part := range parts {
		part = strings.TrimSpace(part)

		// Check if format is "id=url"
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			id, err := strconv.ParseUint(kv[0], 10, 64)
			if err != nil {
				continue
			}
			// Strip any http:// or https:// prefix if present
			addr := strings.TrimPrefix(kv[1], "http://")
			addr = strings.TrimPrefix(addr, "https://")
			peers[id] = addr
		} else {
			// Auto-assign IDs (1, 2, 3...)
			id := uint64(i + 1)
			// Strip any http:// or https:// prefix if present
			addr := strings.TrimPrefix(part, "http://")
			addr = strings.TrimPrefix(addr, "https://")
			peers[id] = addr
		}
	}

	return peers
}

// validateClusterConfig validates cluster configuration
func validateClusterConfig(cfg *ClusterConfig) error {
	// NodeID should have been auto-computed by LoadConfig if not specified
	if cfg.NodeID == 0 {
		return fmt.Errorf("failed to determine node_id (should have been auto-computed)")
	}

	if cfg.BindAddr == "" {
		return fmt.Errorf("bind_addr is required")
	}

	// Check if single-node mode (no peers = standalone)
	if len(cfg.Peers) == 0 {
		fmt.Println("[INFO] Running in single-node mode (no peers configured)")
		// Add self as only peer for single-node cluster
		// Use advertise addr if provided, otherwise use bind addr
		addr := cfg.AdvertiseAddr
		if addr == "" {
			addr = cfg.BindAddr
		}
		// Don't add http:// prefix - Raft uses TCP transport
		cfg.Peers = map[uint64]string{
			cfg.NodeID: addr,
		}
		// Set advertise addr to match
		if cfg.AdvertiseAddr == "" {
			cfg.AdvertiseAddr = addr
		}
		return nil
	}

	// Multi-node cluster validation
	if cfg.AdvertiseAddr == "" {
		return fmt.Errorf("advertise_addr is required for multi-node clusters")
	}

	// Validate advertise address is valid
	if !strings.Contains(cfg.AdvertiseAddr, ":") {
		return fmt.Errorf("invalid advertise_addr: must include port (e.g., 'admin-01:8990')")
	}

	// Validate this node is in peer list
	if _, ok := cfg.Peers[cfg.NodeID]; !ok {
		return fmt.Errorf("this node (id=%d) not found in peers list", cfg.NodeID)
	}

	// Validate odd number of nodes for Raft quorum
	if len(cfg.Peers)%2 == 0 {
		return fmt.Errorf("cluster must have odd number of nodes (3, 5, 7), got %d", len(cfg.Peers))
	}

	return nil
}

// IsSingleNode returns true if running in single-node mode
func (cfg *ClusterConfig) IsSingleNode() bool {
	return len(cfg.Peers) == 1
}

// GetRaftConfig converts ClusterConfig to RaftConfig
func (cfg *ClusterConfig) GetRaftConfig() *RaftConfig {
	return &RaftConfig{
		ID:            cfg.NodeID,
		Cluster:       cfg.Peers,
		DataDir:       cfg.DataDir,
		SnapCount:     cfg.SnapshotInterval,
		HeartbeatTick: cfg.HeartbeatTick,
		ElectionTick:  cfg.ElectionTick,
	}
}
