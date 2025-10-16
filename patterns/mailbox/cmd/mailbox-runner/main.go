package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jrepp/prism-data-layer/patterns/mailbox"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/drivers/sqlite"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "mailbox.yaml", "Path to mailbox configuration file")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Configure logging
	level := slog.LevelInfo
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err, "path", *configPath)
		os.Exit(1)
	}

	slog.Info("starting mailbox-runner",
		"name", config.Name,
		"topic", config.Behavior.Topic,
		"database", config.Storage.DatabasePath)

	// Create mailbox pattern
	mb, err := mailbox.New(*config)
	if err != nil {
		slog.Error("failed to create mailbox", "error", err)
		os.Exit(1)
	}

	// Initialize backends
	ctx := context.Background()

	// Initialize NATS message source
	natsDriver := nats.New()
	natsConfig := &plugin.Config{
		Backend: map[string]interface{}{
			"url": "nats://localhost:4222",
		},
	}
	if err := natsDriver.Initialize(ctx, natsConfig); err != nil {
		slog.Error("failed to initialize NATS driver", "error", err)
		os.Exit(1)
	}
	if err := natsDriver.Start(ctx); err != nil {
		slog.Error("failed to start NATS driver", "error", err)
		os.Exit(1)
	}
	defer natsDriver.Stop(ctx)

	// Initialize SQLite storage backend
	sqliteDriver := sqlite.New()
	sqliteConfig := &plugin.Config{
		Backend: map[string]interface{}{
			"database_path":  config.Storage.DatabasePath,
			"table_name":     config.Storage.TableName,
			"retention_days": float64(config.Storage.RetentionDays),
		},
	}
	if err := sqliteDriver.Initialize(ctx, sqliteConfig); err != nil {
		slog.Error("failed to initialize SQLite driver", "error", err)
		os.Exit(1)
	}
	if err := sqliteDriver.Start(ctx); err != nil {
		slog.Error("failed to start SQLite driver", "error", err)
		os.Exit(1)
	}
	defer sqliteDriver.Stop(ctx)

	// Get TableWriterInterface and TableReaderInterface from SQLite driver
	tableWriter, ok := sqliteDriver.(plugin.TableWriterInterface)
	if !ok {
		slog.Error("SQLite driver does not implement TableWriterInterface")
		os.Exit(1)
	}

	tableReader, ok := sqliteDriver.(plugin.TableReaderInterface)
	if !ok {
		slog.Error("SQLite driver does not implement TableReaderInterface")
		os.Exit(1)
	}

	// Get message source interface from NATS driver
	var messageSource interface{}
	if pubsub, ok := natsDriver.(plugin.PubSubInterface); ok {
		messageSource = pubsub
	} else if queue, ok := natsDriver.(plugin.QueueInterface); ok {
		messageSource = queue
	} else {
		slog.Error("NATS driver does not implement PubSubInterface or QueueInterface")
		os.Exit(1)
	}

	// Bind slots to mailbox pattern
	if err := mb.BindSlots(messageSource, tableWriter, tableReader); err != nil {
		slog.Error("failed to bind slots", "error", err)
		os.Exit(1)
	}

	// Start mailbox
	if err := mb.Start(ctx); err != nil {
		slog.Error("failed to start mailbox", "error", err)
		os.Exit(1)
	}

	slog.Info("mailbox started successfully",
		"name", config.Name,
		"topic", config.Behavior.Topic)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Periodically log statistics
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Wait for termination signal or ticker
	for {
		select {
		case sig := <-sigCh:
			slog.Info("received signal, shutting down", "signal", sig)
			if err := mb.Stop(ctx); err != nil {
				slog.Error("error stopping mailbox", "error", err)
			}
			return

		case <-ticker.C:
			// Log statistics
			stats, err := mb.GetStats(ctx)
			if err != nil {
				slog.Warn("failed to get stats", "error", err)
				continue
			}

			slog.Info("mailbox statistics", "stats", stats)

			// Check health
			health, err := mb.Health(ctx)
			if err != nil {
				slog.Warn("failed to get health", "error", err)
				continue
			}

			if health.Status != plugin.HealthHealthy {
				slog.Warn("mailbox health degraded",
					"status", health.Status,
					"message", health.Message,
					"details", health.Details)
			}
		}
	}
}

// loadConfig loads the mailbox configuration from a YAML file.
func loadConfig(path string) (*mailbox.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config mailbox.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}
