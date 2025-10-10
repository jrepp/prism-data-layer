package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/nats"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	grpcPort := flag.Int("grpc-port", 0, "gRPC control plane port (overrides config)")
	flag.Parse()

	// Initialize logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("nats starting",
		"config_path", *configPath,
		"grpc_port_override", *grpcPort)

	// Load configuration (use defaults if file doesn't exist)
	config, err := core.LoadConfig(*configPath)
	if err != nil {
		slog.Warn("failed to load config, using defaults", "error", err, "config_path", *configPath)
		// Create default config
		config = &core.Config{
			Plugin: core.PluginConfig{
				Name:    "nats",
				Version: "0.1.0",
			},
			ControlPlane: core.ControlPlaneConfig{
				Port: 9002, // Default port for NATS pattern
			},
			Backend: map[string]any{
				"url":               "nats://localhost:4222",
				"max_reconnects":    10,
				"reconnect_wait":    "2s",
				"timeout":           "5s",
				"ping_interval":     "20s",
				"max_pending_msgs":  65536,
				"enable_jetstream": false,
			},
		}
	}

	// Override gRPC port if provided via flag
	if *grpcPort != 0 {
		slog.Info("overriding control plane port from flag",
			"config_port", config.ControlPlane.Port,
			"flag_port", *grpcPort)
		config.ControlPlane.Port = *grpcPort
	}

	// Create NATS plugin
	plugin := nats.New()

	// Bootstrap plugin lifecycle with config
	if err := core.BootstrapWithConfig(plugin, config); err != nil {
		log.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}
