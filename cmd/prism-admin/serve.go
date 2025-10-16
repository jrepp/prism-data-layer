package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	serveCmd.Flags().IntP("http-port", "", 8080, "Admin UI HTTP port")
	serveCmd.Flags().String("listen", "0.0.0.0", "Listen address")
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.http_port", serveCmd.Flags().Lookup("http-port"))
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
	fmt.Printf("[INFO] Initializing storage: %s (%s)\n", dbCfg.Type, dbCfg.Path)
	storage, err := NewStorage(ctx, dbCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer storage.Close()
	fmt.Printf("[INFO] Storage initialized successfully\n")

	// Create control plane service
	controlPlane := NewControlPlaneService(storage)
	fmt.Printf("[INFO] Control plane service created\n")

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
	fmt.Printf("[INFO] gRPC server configured\n")

	// Create HTTP server for admin UI
	httpPort := viper.GetInt("server.http_port")
	httpServer := NewHTTPServer(storage, httpPort)
	fmt.Printf("[INFO] HTTP server configured\n\n")

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("🚀 Prism Admin Control Plane Server\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  gRPC API:   %s\n", address)
	fmt.Printf("  Admin UI:   http://localhost:%d\n", httpPort)
	fmt.Printf("  Database:   %s (%s)\n", dbCfg.Type, dbCfg.Path)
	fmt.Printf("  Status:     ✅ Ready\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  gRPC accepts connections from:\n")
	fmt.Printf("    • Proxies (registration, heartbeats, namespace mgmt)\n")
	fmt.Printf("    • Launchers (registration, heartbeats, process mgmt)\n")
	fmt.Printf("    • Clients (namespace provisioning via proxy)\n")
	fmt.Printf("  \n")
	fmt.Printf("  Admin UI accessible at:\n")
	fmt.Printf("    • http://localhost:%d/          (Dashboard)\n", httpPort)
	fmt.Printf("    • http://localhost:%d/proxies   (Proxy status)\n", httpPort)
	fmt.Printf("    • http://localhost:%d/launchers (Launcher status)\n", httpPort)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	errChan := make(chan error, 2) // Increase capacity for both servers

	// Start gRPC server
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Start HTTP server
	go func() {
		if err := httpServer.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)

		// Shutdown HTTP server with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdownCtx)

		// Shutdown gRPC server
		grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		return err
	}
}
