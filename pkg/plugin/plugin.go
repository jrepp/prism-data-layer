package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// PluginConfig contains plugin metadata
type PluginConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// ControlPlaneConfig contains control plane settings
type ControlPlaneConfig struct {
	Port int `yaml:"port"`
}

// Plugin represents a backend driver lifecycle
type Plugin interface {
	// Name returns the backend driver name (e.g., "redis", "postgres", "kafka")
	Name() string

	// Version returns the backend driver version
	Version() string

	// Initialize prepares the backend driver with configuration
	Initialize(ctx context.Context, config *Config) error

	// Start begins serving requests
	Start(ctx context.Context) error

	// Stop gracefully shuts down the backend driver
	Stop(ctx context.Context) error

	// Health returns the backend driver health status
	Health(ctx context.Context) (*HealthStatus, error)
}

// BackendDriver is a type alias for Plugin to make terminology clearer
type BackendDriver = Plugin

// HealthStatus represents backend driver health
type HealthStatus struct {
	Status  HealthState
	Message string
	Details map[string]string // Backend-specific details
}

// HealthState represents backend driver health state
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

	return BootstrapWithConfig(plugin, config)
}

// BootstrapWithConfig initializes and runs a plugin with a pre-loaded configuration
func BootstrapWithConfig(plugin Plugin, config *Config) error {
	// Create root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("initializing plugin",
		"name", plugin.Name(),
		"version", plugin.Version(),
		"control_plane_port", config.ControlPlane.Port)

	// Initialize plugin
	if err := plugin.Initialize(ctx, config); err != nil {
		slog.Error("failed to initialize plugin", "error", err)
		return fmt.Errorf("failed to initialize plugin: %w", err)
	}
	slog.Info("plugin initialized successfully", "name", plugin.Name())

	// Start control plane server
	slog.Info("starting control plane server", "port", config.ControlPlane.Port)
	controlPlane := NewControlPlaneServer(plugin, config.ControlPlane.Port)
	if err := controlPlane.Start(ctx); err != nil {
		slog.Error("failed to start control plane", "error", err)
		return fmt.Errorf("failed to start control plane: %w", err)
	}
	defer controlPlane.Stop(ctx)
	slog.Info("control plane server started", "port", config.ControlPlane.Port)

	// Start plugin
	slog.Info("starting plugin", "name", plugin.Name())
	errChan := make(chan error, 1)
	go func() {
		if err := plugin.Start(ctx); err != nil {
			slog.Error("plugin start error", "name", plugin.Name(), "error", err)
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
	slog.Info("shutting down plugin", "name", plugin.Name())
	cancel()

	if err := plugin.Stop(ctx); err != nil {
		slog.Error("error stopping plugin", "name", plugin.Name(), "error", err)
		return err
	}

	slog.Info("plugin stopped successfully", "name", plugin.Name())
	return nil
}
