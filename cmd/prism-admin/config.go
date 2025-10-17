package main

import (
	"fmt"
	"net/url"
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
		BindAddr:            viper.GetString("cluster.bind_addr"),
		AdvertiseAddr:       viper.GetString("cluster.advertise_addr"),
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

	// Parse peers from comma-separated string or map
	peers := viper.GetStringSlice("cluster.peers")
	cluster.Peers = make(map[uint64]string)

	if len(peers) > 0 {
		// Format: "admin-01.prism.local:8990,admin-02.prism.local:8990,admin-03.prism.local:8990"
		for i, peer := range peers {
			nodeID := uint64(i + 1) // 1-indexed
			cluster.Peers[nodeID] = fmt.Sprintf("http://%s", peer)
		}
	}

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
		Listen:          viper.GetString("control_plane.listen"),
		ReadConsistency: viper.GetStringMapString("control_plane.read_consistency"),
	}

	if controlPlane.Listen == "" {
		controlPlane.Listen = "0.0.0.0:8981"
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
		Listen:                 viper.GetString("admin_api.listen"),
		DefaultReadConsistency: viper.GetString("admin_api.default_read_consistency"),
	}

	if adminAPI.Listen == "" {
		adminAPI.Listen = "0.0.0.0:8980"
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
// Format: "1=http://admin-01:8990,2=http://admin-02:8990,3=http://admin-03:8990"
// Or: "admin-01:8990,admin-02:8990,admin-03:8990" (auto-assign IDs)
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
			peers[id] = ensureHTTPScheme(kv[1])
		} else {
			// Auto-assign IDs (1, 2, 3...)
			id := uint64(i + 1)
			peers[id] = ensureHTTPScheme(part)
		}
	}

	return peers
}

// ensureHTTPScheme adds http:// if no scheme present
func ensureHTTPScheme(addr string) string {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}

// validateClusterConfig validates cluster configuration
func validateClusterConfig(cfg *ClusterConfig) error {
	if cfg.NodeID == 0 {
		return fmt.Errorf("node_id is required (must be > 0)")
	}

	if cfg.BindAddr == "" {
		return fmt.Errorf("bind_addr is required")
	}

	if cfg.AdvertiseAddr == "" {
		return fmt.Errorf("advertise_addr is required")
	}

	// Validate advertise address is valid URL
	if _, err := url.Parse(cfg.AdvertiseAddr); err != nil {
		return fmt.Errorf("invalid advertise_addr: %w", err)
	}

	// Check if single-node mode (no peers = standalone)
	if len(cfg.Peers) == 0 {
		fmt.Println("[INFO] Running in single-node mode (no peers configured)")
		// Add self as only peer for single-node cluster
		cfg.Peers = map[uint64]string{
			cfg.NodeID: ensureHTTPScheme(cfg.AdvertiseAddr),
		}
		return nil
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
