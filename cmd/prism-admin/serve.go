package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
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
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.listen", serveCmd.Flags().Lookup("listen"))
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse database configuration
	dbURN := viper.GetString("storage.db")
	dbCfg, err := ParseDatabaseURN(dbURN)
	if err != nil {
		return fmt.Errorf("invalid database URN: %w", err)
	}

	// Initialize storage
	fmt.Printf("Initializing storage: %s (%s)\n", dbCfg.Type, dbCfg.Path)
	storage, err := NewStorage(ctx, dbCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer storage.Close()

	// Create control plane service
	controlPlane := NewControlPlaneService(storage)

	// Start gRPC server
	listenAddr := viper.GetString("server.listen")
	port := viper.GetInt("server.port")
	address := fmt.Sprintf("%s:%d", listenAddr, port)

	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterControlPlaneServer(grpcServer, controlPlane)

	fmt.Printf("🚀 prism-admin control plane server listening on %s\n", address)
	fmt.Printf("   Database: %s (%s)\n", dbCfg.Type, dbCfg.Path)
	fmt.Printf("   Ready to accept proxy and launcher connections\n")

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
		grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}
