package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	"github.com/jrepp/prism/pkg/launcher"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/launcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	grpcPort     = flag.Int("grpc-port", 8982, "gRPC server port")
	metricsPort  = flag.Int("metrics-port", 9092, "Metrics server port")
	healthPort   = flag.Int("health-port", 9093, "Health server port")
	patternsDir  = flag.String("patterns-dir", "./patterns", "Patterns directory")
	isolationStr = flag.String("isolation", "namespace", "Default isolation level (none, namespace, session)")
)

func main() {
	flag.Parse()

	log.Printf("Starting Pattern Launcher")
	log.Printf("  gRPC port: %d", *grpcPort)
	log.Printf("  Metrics port: %d", *metricsPort)
	log.Printf("  Health port: %d", *healthPort)
	log.Printf("  Patterns directory: %s", *patternsDir)
	log.Printf("  Default isolation: %s", *isolationStr)

	// Parse isolation level
	isolationLevel := parseIsolationLevel(*isolationStr)

	// Create service configuration
	config := &launcher.Config{
		PatternsDir:      *patternsDir,
		DefaultIsolation: isolationLevel,
		ResyncInterval:   30 * time.Second,
		BackOffPeriod:    5 * time.Second,
		CPULimit:         2.0,
		MemoryLimit:      "1Gi",
	}

	// Create launcher service
	service, err := launcher.NewService(config)
	if err != nil {
		log.Fatalf("Failed to create launcher service: %v", err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterPatternLauncherServer(grpcServer, service)

	// Enable reflection for grpcurl
	reflection.Register(grpcServer)

	// Start gRPC server
	grpcAddr := fmt.Sprintf(":%d", *grpcPort)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", grpcAddr, err)
	}

	go func() {
		log.Printf("gRPC server listening on %s", grpcAddr)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start metrics server
	go func() {
		metricsAddr := fmt.Sprintf(":%d", *metricsPort)
		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", metricsHandler)
		log.Printf("Metrics server listening on %s", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Start health server
	go func() {
		healthAddr := fmt.Sprintf(":%d", *healthPort)
		mux := http.NewServeMux()
		mux.HandleFunc("/health", healthHandler(service))
		mux.HandleFunc("/ready", readyHandler(service))
		log.Printf("Health server listening on %s", healthAddr)
		if err := http.ListenAndServe(healthAddr, mux); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	log.Printf("Pattern Launcher started successfully")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("Shutdown signal received")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop gRPC server
	grpcServer.GracefulStop()

	// Shutdown service
	if err := service.Shutdown(ctx); err != nil {
		log.Printf("Service shutdown error: %v", err)
	}

	log.Printf("Pattern Launcher stopped")
}

func parseIsolationLevel(level string) isolation.IsolationLevel {
	switch level {
	case "none":
		return isolation.IsolationNone
	case "namespace":
		return isolation.IsolationNamespace
	case "session":
		return isolation.IsolationSession
	default:
		log.Printf("Invalid isolation level: %s, defaulting to namespace", level)
		return isolation.IsolationNamespace
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// For Phase 1, return mock metrics
	fmt.Fprint(w, `# HELP pattern_launcher_processes_total Total number of pattern processes
# TYPE pattern_launcher_processes_total gauge
pattern_launcher_processes_total 0

# HELP pattern_launcher_uptime_seconds Launcher uptime in seconds
# TYPE pattern_launcher_uptime_seconds counter
pattern_launcher_uptime_seconds 0
`)
}

func healthHandler(service *launcher.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resp, err := service.Health(ctx, &pb.HealthRequest{
			IncludeProcesses: false,
		})

		if err != nil || !resp.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "unhealthy: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "healthy: %d processes running", resp.RunningProcesses)
	}
}

func readyHandler(service *launcher.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For Phase 1, always ready if service is running
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ready")
	}
}
