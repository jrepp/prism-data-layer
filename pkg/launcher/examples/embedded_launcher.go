package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	"github.com/jrepp/prism/pkg/launcher"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
)

// Example: Embedded launcher service
//
// This example demonstrates how to:
// 1. Create and configure a launcher service programmatically
// 2. Start a gRPC server for the launcher
// 3. Handle graceful shutdown
// 4. Export metrics for monitoring

func main() {
	// 1. Create launcher configuration
	config := &launcher.Config{
		PatternsDir:      "./patterns",
		DefaultIsolation: isolation.IsolationNamespace,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
		CPULimit:         2.0,
		MemoryLimit:      "1Gi",
	}

	// 2. Create launcher service
	fmt.Println("Creating pattern launcher service...")
	service, err := launcher.NewService(config)
	if err != nil {
		log.Fatalf("Failed to create launcher service: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal: %v\n", sig)
		fmt.Println("Shutting down gracefully...")

		// Shutdown launcher service
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := service.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}

		cancel()
	}()

	// 3. Create gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterPatternLauncherServer(grpcServer, service)

	// 4. Start listening
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	fmt.Printf("✓ Pattern launcher service started\n")
	fmt.Printf("  Patterns directory: %s\n", config.PatternsDir)
	fmt.Printf("  Default isolation: %s\n", config.DefaultIsolation)
	fmt.Printf("  gRPC address: %s\n", listener.Addr())
	fmt.Println()

	// 5. Export initial metrics
	fmt.Println("Initial metrics:")
	metrics := service.GetMetrics()
	fmt.Printf("  Total processes: %d\n", metrics.TotalProcesses)
	fmt.Printf("  Running processes: %d\n", metrics.RunningProcesses)
	fmt.Printf("  Uptime: %.0fs\n\n", metrics.UptimeSeconds)

	// 6. Start metrics exporter (separate goroutine)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Export metrics in Prometheus format
				prometheusMetrics := service.ExportPrometheusMetrics()

				// In production, this would be exposed via HTTP endpoint
				// For example, serve on /metrics endpoint
				fmt.Println("=== Metrics Export ===")
				fmt.Println(prometheusMetrics)
				fmt.Println()
			}
		}
	}()

	// 7. Serve gRPC
	fmt.Println("Serving gRPC requests on :8080...")
	fmt.Println("Press Ctrl+C to shutdown gracefully")

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("Shutting down gRPC server...")
		grpcServer.GracefulStop()
		fmt.Println("✓ Shutdown complete")
	case err := <-errCh:
		if err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}
}
