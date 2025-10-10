package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Plugin represents a backend plugin lifecycle
type Plugin interface {
	// Name returns the plugin name (e.g., "postgres", "kafka")
	Name() string

	// Version returns the plugin version
	Version() string

	// Initialize prepares the plugin with configuration
	Initialize(ctx context.Context, config *Config) error

	// Start begins serving requests
	Start(ctx context.Context) error

	// Stop gracefully shuts down the plugin
	Stop(ctx context.Context) error

	// Health returns the plugin health status
	Health(ctx context.Context) (*HealthStatus, error)
}

// HealthStatus represents plugin health
type HealthStatus struct {
	Status  HealthState
	Message string
	Details map[string]string
}

// HealthState represents health state
type HealthState int

const (
	HealthUnknown HealthState = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

func (h HealthState) String() string {
	switch h {
	case HealthHealthy:
		return "HEALTHY"
	case HealthDegraded:
		return "DEGRADED"
	case HealthUnhealthy:
		return "UNHEALTHY"
	default:
		return "UNKNOWN"
	}
}

// Bootstrap initializes and runs a plugin with lifecycle management
func Bootstrap(plugin Plugin, configPath string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("plugin starting",
		"name", plugin.Name(),
		"version", plugin.Version(),
		"config", configPath)

	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize plugin
	if err := plugin.Initialize(ctx, config); err != nil {
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}

	// Start control plane server
	controlPlane := NewControlPlaneServer(plugin, config.ControlPlane.Port)
	if err := controlPlane.Start(ctx); err != nil {
		return fmt.Errorf("failed to start control plane: %w", err)
	}
	defer controlPlane.Stop(ctx)

	// Start plugin
	errChan := make(chan error, 1)
	go func() {
		if err := plugin.Start(ctx); err != nil {
			errChan <- fmt.Errorf("plugin error: %w", err)
		}
	}()

	slog.Info("plugin ready",
		"name", plugin.Name(),
		"control_plane_port", config.ControlPlane.Port)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		slog.Error("plugin failed", "error", err)
		return err
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	slog.Info("shutting down plugin")
	cancel()

	if err := plugin.Stop(ctx); err != nil {
		slog.Error("error stopping plugin", "error", err)
		return err
	}

	slog.Info("plugin stopped successfully")
	return nil
}
