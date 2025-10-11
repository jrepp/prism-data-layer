package core

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	pb "github.com/jrepp/prism-data-layer/patterns/core/gen/prism/pattern"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// ControlPlaneServer provides control plane API for gateway
type ControlPlaneServer struct {
	plugin     Plugin
	port       int
	grpcServer *grpc.Server
	listener   net.Listener
}

// NewControlPlaneServer creates a control plane server
func NewControlPlaneServer(plugin Plugin, port int) *ControlPlaneServer {
	return &ControlPlaneServer{
		plugin: plugin,
		port:   port,
	}
}

// Start begins serving control plane requests
func (s *ControlPlaneServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	// Create gRPC server
	s.grpcServer = grpc.NewServer()

	// Register PatternLifecycle service
	lifecycleService := NewLifecycleService(s.plugin)
	pb.RegisterPatternLifecycleServer(s.grpcServer, lifecycleService)
	slog.Info("registered PatternLifecycle service", "plugin", s.plugin.Name())

	// Register health check service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s.grpcServer, healthServer)

	// Register reflection for debugging
	reflection.Register(s.grpcServer)

	// Start health checker
	go s.healthChecker(ctx, healthServer)

	// Start serving
	go func() {
		slog.Info("control plane listening",
			"port", s.port,
			"plugin", s.plugin.Name())

		if err := s.grpcServer.Serve(listener); err != nil {
			slog.Error("control plane serve error", "error", err)
		}
	}()

	return nil
}

// Port returns the actual port the control plane is listening on
// This is useful when using dynamic port allocation (port 0)
func (s *ControlPlaneServer) Port() int {
	if s.listener != nil {
		addr := s.listener.Addr().(*net.TCPAddr)
		return addr.Port
	}
	return s.port
}

// Stop gracefully stops the control plane server
func (s *ControlPlaneServer) Stop(ctx context.Context) error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	return nil
}

// healthChecker periodically checks plugin health and updates gRPC health service
func (s *ControlPlaneServer) healthChecker(ctx context.Context, healthServer *health.Server) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			healthServer.SetServingStatus(s.plugin.Name(), grpc_health_v1.HealthCheckResponse_NOT_SERVING)
			return
		case <-ticker.C:
			health, err := s.plugin.Health(ctx)
			if err != nil {
				slog.Error("health check failed", "error", err)
				healthServer.SetServingStatus(s.plugin.Name(), grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				continue
			}

			var grpcStatus grpc_health_v1.HealthCheckResponse_ServingStatus
			switch health.Status {
			case HealthHealthy:
				grpcStatus = grpc_health_v1.HealthCheckResponse_SERVING
			case HealthDegraded:
				grpcStatus = grpc_health_v1.HealthCheckResponse_SERVING
			default:
				grpcStatus = grpc_health_v1.HealthCheckResponse_NOT_SERVING
			}

			healthServer.SetServingStatus(s.plugin.Name(), grpcStatus)

			slog.Debug("health check",
				"plugin", s.plugin.Name(),
				"status", health.Status.String(),
				"message", health.Message)
		}
	}
}

// Error creates a gRPC status error
func Error(code codes.Code, message string) error {
	return status.Error(code, message)
}

// Errorf creates a formatted gRPC status error
func Errorf(code codes.Code, format string, args ...interface{}) error {
	return status.Errorf(code, format, args...)
}
