package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the prism-admin control plane server",
	Long: `Start the prism-admin control plane gRPC server on port 8981.

The control plane server accepts connections from:
- prism-proxy instances (proxy registration, namespace management)
- prism-launcher instances (process lifecycle management)

Example:
  prism-admin serve
  prism-admin serve --port 8981 --db sqlite://~/.prism/admin.db
`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntP("port", "p", 8981, "Control plane gRPC port")
	serveCmd.Flags().String("listen", "0.0.0.0", "Listen address")
	serveCmd.Flags().IntP("metrics-port", "m", 9090, "Prometheus metrics HTTP port")
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.listen", serveCmd.Flags().Lookup("listen"))
	viper.BindPFlag("server.metrics_port", serveCmd.Flags().Lookup("metrics-port"))
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	log := slog.Default()

	// Load configuration
	clusterCfg, controlPlaneCfg, _, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Parse database configuration
	dbURN := viper.GetString("storage.db")
	dbCfg, err := ParseDatabaseURN(dbURN)
	if err != nil {
		return fmt.Errorf("invalid database URN: %w", err)
	}

	// Initialize storage
	fmt.Printf("[INFO] Initializing storage: %s (%s)\n", dbCfg.Type, dbCfg.Path)
	storage, err := NewStorage(ctx, dbCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer storage.Close()
	fmt.Printf("[INFO] Storage initialized successfully\n")

	// Initialize Prometheus metrics
	fmt.Printf("[INFO] Initializing Prometheus metrics\n")
	metrics := NewAdminMetrics("prism_admin")

	// Initialize Admin State Machine (Raft FSM) with storage sync
	fmt.Printf("[INFO] Initializing Admin State Machine with storage sync\n")
	fsm := NewAdminStateMachine(log.With("component", "fsm"), metrics, storage)

	// Initialize Raft Node
	fmt.Printf("[INFO] Initializing Raft node (id=%d, peers=%d)\n",
		clusterCfg.NodeID, len(clusterCfg.Peers))
	raftNode, err := NewRaftNode(clusterCfg.GetRaftConfig(), fsm, metrics, log.With("component", "raft"))
	if err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}

	// Start Raft node
	if err := raftNode.Start(ctx, clusterCfg.BindAddr); err != nil {
		return fmt.Errorf("failed to start raft node: %w", err)
	}
	defer raftNode.Stop()

	// Bootstrap cluster (if not already bootstrapped)
	if err := raftNode.Bootstrap(clusterCfg.Peers); err != nil {
		return fmt.Errorf("failed to bootstrap cluster: %w", err)
	}

	// Wait for leader election
	fmt.Printf("[INFO] Waiting for leader election...\n")
	if err := raftNode.WaitForLeader(5 * time.Second); err != nil {
		return fmt.Errorf("leader election timeout: %w", err)
	}

	if raftNode.IsLeader() {
		fmt.Printf("[INFO] ✅ This node is the LEADER (id=%d)\n", clusterCfg.NodeID)
	} else {
		leaderID := raftNode.GetLeader()
		fmt.Printf("[INFO] This node is a FOLLOWER (leader=%d)\n", leaderID)
	}

	// Initialize Partition Manager (computes ranges on-demand)
	partitionMgr := NewPartitionManager()

	// Create Raft-integrated control plane service
	controlPlane := NewControlPlaneServiceRaft(
		raftNode,
		fsm,
		partitionMgr,
		controlPlaneCfg.ReadConsistency,
		log.With("component", "control_plane"),
	)
	fmt.Printf("[INFO] Raft-integrated control plane service created\n")

	// Start gRPC server
	address := controlPlaneCfg.Listen

	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterControlPlaneServer(grpcServer, controlPlane)
	fmt.Printf("[INFO] gRPC server configured\n")

	// Start Prometheus metrics HTTP server
	metricsPort := viper.GetInt("server.metrics_port")
	metricsAddr := fmt.Sprintf(":%d", metricsPort)
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: promhttp.Handler(),
	}
	fmt.Printf("[INFO] Prometheus metrics endpoint: http://0.0.0.0%s/metrics\n\n", metricsAddr)

	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", "error", err)
		}
	}()

	// Display startup banner
	displayStartupBanner(clusterCfg, controlPlaneCfg, dbCfg, raftNode, metricsPort)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- err
		}
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)

		// Shutdown gRPC server
		grpcServer.GracefulStop()

		// Shutdown metrics server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown metrics server", "error", err)
		}

		return nil
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}

func displayStartupBanner(
	clusterCfg *ClusterConfig,
	controlPlaneCfg *ControlPlaneConfig,
	dbCfg *DatabaseConfig,
	raftNode *RaftNode,
	metricsPort int,
) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("🚀 Prism Admin Control Plane Server with Raft HA\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  Control Plane:  %s\n", controlPlaneCfg.Listen)
	fmt.Printf("  Metrics:        http://0.0.0.0:%d/metrics\n", metricsPort)
	fmt.Printf("  Database:       %s (%s)\n", dbCfg.Type, dbCfg.Path)
	fmt.Printf("  \n")
	fmt.Printf("  Raft Configuration:\n")
	fmt.Printf("    Node ID:      %d\n", clusterCfg.NodeID)
	fmt.Printf("    Cluster Size: %d nodes\n", len(clusterCfg.Peers))
	fmt.Printf("    Raft Addr:    %s\n", clusterCfg.BindAddr)
	fmt.Printf("    Data Dir:     %s\n", clusterCfg.DataDir)

	if raftNode.IsLeader() {
		fmt.Printf("    Role:         🎖️  LEADER\n")
	} else {
		fmt.Printf("    Role:         FOLLOWER (leader=%d)\n", raftNode.GetLeader())
	}

	if clusterCfg.IsSingleNode() {
		fmt.Printf("    Mode:         Single-node (dev)\n")
	} else {
		fmt.Printf("    Mode:         Multi-node cluster\n")
	}

	fmt.Printf("  \n")
	fmt.Printf("  Read Consistency:\n")
	fmt.Printf("    Follower Reads: %v\n", clusterCfg.EnableFollowerReads)
	fmt.Printf("    Max Staleness:  %s\n", clusterCfg.MaxStaleness)
	fmt.Printf("  \n")
	fmt.Printf("  Status:         ✅ Ready\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  Accepting connections from:\n")
	fmt.Printf("    • Proxies (registration, heartbeats, namespace mgmt)\n")
	fmt.Printf("    • Launchers (registration, heartbeats, process mgmt)\n")
	fmt.Printf("    • Clients (namespace provisioning via proxy)\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  RFC-038: Admin Leader Election with Hashicorp Raft\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}
