package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jrepp/prism-data-layer/patterns/producer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

var (
	configFile = flag.String("config", "", "Producer configuration file (YAML/JSON)")
	address    = flag.String("address", "0.0.0.0:8083", "gRPC server address")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// Setup logging
	level := slog.LevelInfo
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if *configFile == "" {
		log.Fatal("--config flag is required")
	}

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create producer
	prod, err := producer.New(*config)
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}

	// Initialize backend drivers
	messageSink, err := initializeMessageSink(config.Slots.MessageSink)
	if err != nil {
		log.Fatalf("Failed to initialize message sink: %v", err)
	}

	var stateStore plugin.KeyValueBasicInterface
	if config.Behavior.Deduplication {
		stateStore, err = initializeStateStore(config.Slots.StateStore)
		if err != nil {
			log.Fatalf("Failed to initialize state store: %v", err)
		}
	}

	// Bind slots (objectStore is nil for now - claim check pattern not yet implemented in runner)
	if err := prod.BindSlots(messageSink, stateStore, nil); err != nil {
		log.Fatalf("Failed to bind slots: %v", err)
	}

	// Start producer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := prod.Start(ctx); err != nil {
		log.Fatalf("Failed to start producer: %v", err)
	}

	slog.Info("Producer runner started",
		"name", config.Name,
		"address", *address,
		"message_sink", config.Slots.MessageSink.Driver,
		"batch_size", config.Behavior.BatchSize)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigCh

	slog.Info("Shutting down producer...")
	if err := prod.Stop(ctx); err != nil {
		log.Printf("Error stopping producer: %v", err)
	}

	// Print final metrics
	metrics := prod.Metrics()
	slog.Info("Producer metrics",
		"published", metrics.MessagesPublished,
		"failed", metrics.MessagesFailed,
		"deduped", metrics.MessagesDedup,
		"bytes", metrics.BytesPublished,
		"batches", metrics.BatchesPublished)

	slog.Info("Producer shutdown complete")
}

func loadConfig(path string) (*producer.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config producer.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func initializeMessageSink(binding producer.SlotBinding) (interface{}, error) {
	switch binding.Driver {
	case "nats":
		return initializeNATS(binding.Config)
	case "redis":
		return initializeRedis(binding.Config)
	case "memstore":
		return initializeMemStore(binding.Config)
	default:
		return nil, fmt.Errorf("unsupported message sink driver: %s", binding.Driver)
	}
}

func initializeStateStore(binding producer.SlotBinding) (plugin.KeyValueBasicInterface, error) {
	switch binding.Driver {
	case "redis":
		return initializeRedis(binding.Config)
	case "memstore":
		return initializeMemStore(binding.Config)
	default:
		return nil, fmt.Errorf("unsupported state store driver: %s", binding.Driver)
	}
}

func initializeNATS(config map[string]interface{}) (*nats.NATSPattern, error) {
	url, _ := config["url"].(string)
	if url == "" {
		url = "nats://localhost:4222"
	}

	driver := nats.New()

	// Create plugin config
	pluginCfg := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "nats",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"url": url,
		},
	}

	if err := driver.Initialize(context.Background(), pluginCfg); err != nil {
		return nil, err
	}

	return driver, nil
}

func initializeRedis(config map[string]interface{}) (*redis.RedisPattern, error) {
	addr, _ := config["address"].(string)
	if addr == "" {
		addr = "localhost:6379"
	}

	driver := redis.New()

	// Create plugin config
	pluginCfg := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "redis",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"address": addr,
		},
	}

	if err := driver.Initialize(context.Background(), pluginCfg); err != nil {
		return nil, err
	}

	return driver, nil
}

func initializeMemStore(config map[string]interface{}) (*memstore.MemStore, error) {
	maxKeys, _ := config["max_keys"].(float64)
	if maxKeys == 0 {
		maxKeys = 10000
	}

	driver := memstore.New()

	// Create plugin config
	pluginCfg := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "memstore",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"max_keys": int(maxKeys),
		},
	}

	if err := driver.Initialize(context.Background(), pluginCfg); err != nil {
		return nil, err
	}

	return driver, nil
}
