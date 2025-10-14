package plugin

import (
	"context"
	"fmt"
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
		"version", req.Version,
		"has_config", req.Config != nil)

	// Parse configuration from protobuf Struct
	var config *Config
	var err error

	if req.Config != nil {
		// Convert protobuf Struct to Config
		config, err = ParseConfigFromStruct(req.Name, req.Version, req.Config)
		if err != nil {
			slog.Error("lifecycle: failed to parse config", "error", err)
			return &pb.InitializeResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to parse config: %v", err),
			}, nil
		}
		slog.Info("lifecycle: parsed config successfully",
			"backend_keys", len(config.Backend))
	} else {
		// No config provided - use minimal config
		slog.Warn("lifecycle: no config provided, using minimal config")
		config = &Config{
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

	s.config = config

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
			Interfaces: s.plugin.GetInterfaceDeclarations(),
		},
	}, nil
}

// Start implements the Start RPC
func (s *LifecycleService) Start(ctx context.Context, req *pb.StartRequest) (*pb.StartResponse, error) {
	slog.Info("lifecycle: Start called", "plugin", s.plugin.Name())

	// Call plugin Start
	if err := s.plugin.Start(ctx); err != nil {
		slog.Error("lifecycle: Start failed", "error", err)
		return &pb.StartResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	slog.Info("lifecycle: Start succeeded", "plugin", s.plugin.Name())

	return &pb.StartResponse{
		Success:      true,
		Error:        "",
		DataEndpoint: "", // Can be used for data plane endpoint if pattern exposes one
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
