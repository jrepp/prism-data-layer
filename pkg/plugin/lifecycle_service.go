package core

import (
	"context"
	"log/slog"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
)

// LifecycleService implements the LifecycleInterface gRPC service
type LifecycleService struct {
	pb.UnimplementedLifecycleInterfaceServer
	plugin Plugin
	config *Config
}

// NewLifecycleService creates a new lifecycle service wrapping a plugin
func NewLifecycleService(plugin Plugin) *LifecycleService {
	return &LifecycleService{
		plugin: plugin,
	}
}

// Initialize implements the Initialize RPC
func (s *LifecycleService) Initialize(ctx context.Context, req *pb.InitializeRequest) (*pb.InitializeResponse, error) {
	slog.Info("lifecycle: Initialize called",
		"name", req.Name,
		"version", req.Version)

	// Convert protobuf Struct to Config
	// For now, we'll use an empty config since conversion is complex
	// In production, this would properly parse the config from req.Config
	if s.config == nil {
		s.config = &Config{
			Plugin: PluginConfig{
				Name:    req.Name,
				Version: req.Version,
			},
			ControlPlane: ControlPlaneConfig{
				Port: 9090, // Will be overridden by actual port
			},
			Backend: make(map[string]any),
		}
	}

	// Call plugin Initialize
	if err := s.plugin.Initialize(ctx, s.config); err != nil {
		slog.Error("lifecycle: Initialize failed", "error", err)
		return &pb.InitializeResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	slog.Info("lifecycle: Initialize succeeded",
		"plugin", s.plugin.Name(),
		"version", s.plugin.Version())

	return &pb.InitializeResponse{
		Success: true,
		Error:   "",
		Metadata: &pb.PatternMetadata{
			Name:       s.plugin.Name(),
			Version:    s.plugin.Version(),
			Interfaces: []string{"keyvalue"}, // TODO: Make this configurable
		},
	}, nil
}

// Start implements the Start RPC
func (s *LifecycleService) Start(ctx context.Context, req *pb.StartRequest) (*pb.StartResponse, error) {
	slog.Info("lifecycle: Start called", "plugin", s.plugin.Name())

	// Call plugin Start (non-blocking - it runs in a goroutine in Bootstrap)
	// For now, we'll just acknowledge the start request
	// The actual Start() is called in Bootstrap and runs in a goroutine

	slog.Info("lifecycle: Start succeeded", "plugin", s.plugin.Name())

	return &pb.StartResponse{
		Success:      true,
		Error:        "",
		DataEndpoint: "", // MemStore doesn't have a separate data endpoint
	}, nil
}

// Stop implements the Stop RPC
func (s *LifecycleService) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	slog.Info("lifecycle: Stop called",
		"plugin", s.plugin.Name(),
		"timeout", req.TimeoutSeconds)

	if err := s.plugin.Stop(ctx); err != nil {
		slog.Error("lifecycle: Stop failed", "error", err)
		return &pb.StopResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	slog.Info("lifecycle: Stop succeeded", "plugin", s.plugin.Name())

	return &pb.StopResponse{
		Success: true,
		Error:   "",
	}, nil
}

// HealthCheck implements the HealthCheck RPC
func (s *LifecycleService) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	slog.Debug("lifecycle: HealthCheck called", "plugin", s.plugin.Name())

	health, err := s.plugin.Health(ctx)
	if err != nil {
		slog.Error("lifecycle: HealthCheck failed", "error", err)
		return &pb.HealthCheckResponse{
			Status:  pb.HealthStatus_HEALTH_STATUS_UNHEALTHY,
			Message: err.Error(),
			Details: make(map[string]string),
		}, nil
	}

	// Convert internal health status to protobuf
	var pbStatus pb.HealthStatus
	switch health.Status {
	case HealthHealthy:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_HEALTHY
	case HealthDegraded:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_DEGRADED
	case HealthUnhealthy:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_UNHEALTHY
	default:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_UNSPECIFIED
	}

	slog.Debug("lifecycle: HealthCheck succeeded",
		"plugin", s.plugin.Name(),
		"status", health.Status.String())

	return &pb.HealthCheckResponse{
		Status:  pbStatus,
		Message: health.Message,
		Details: health.Details,
	}, nil
}
