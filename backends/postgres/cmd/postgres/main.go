package main

import (
	"log/slog"
	"os"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/pkg/drivers/postgres"
)

func main() {
	// Get config path from environment
	configPath := os.Getenv("PRISM_PLUGIN_CONFIG")
	if configPath == "" {
		configPath = "/etc/prism/plugin.yaml"
	}

	// Bootstrap the plugin using core package
	// ADR-025: Standard plugin lifecycle management
	plugin := postgres.New()
	if err := core.Bootstrap(plugin, configPath); err != nil {
		slog.Error("plugin failed", "error", err)
		os.Exit(1)
	}
}
