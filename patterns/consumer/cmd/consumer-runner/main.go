package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/consumer"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"gopkg.in/yaml.v3"
)

// ConsumerRunner runs a consumer pattern with dynamically configured backends
type ConsumerRunner struct {
	config   *Config
	consumer *consumer.Consumer
	backends map[string]plugin.Plugin
}

// Config represents the consumer configuration loaded from YAML
type Config struct {
	Namespaces []Namespace `yaml:"namespaces"`
}

// Namespace represents a single namespace configuration
type Namespace struct {
	Name           string                 `yaml:"name"`
	Pattern        string                 `yaml:"pattern"`
	PatternVersion string                 `yaml:"pattern_version"`
	Description    string                 `yaml:"description"`
	Slots          Slots                  `yaml:"slots"`
	Behavior       map[string]interface{} `yaml:"behavior"`
}

// Slots represents the slot configuration
type Slots struct {
	MessageSource    *SlotConfig `yaml:"message_source,omitempty"`
	StateStore       *SlotConfig `yaml:"state_store,omitempty"`
	DeadLetterQueue  *SlotConfig `yaml:"dead_letter_queue,omitempty"`
}

// SlotConfig represents a single slot's backend configuration
type SlotConfig struct {
	Backend    string                 `yaml:"backend"`
	Interfaces []string               `yaml:"interfaces"`
	Config     map[string]interface{} `yaml:"config"`
}

func main() {
	var (
		configFile = flag.String("config", "", "Path to consumer configuration YAML file")
		proxyAddr  = flag.String("proxy-addr", "", "Proxy control plane address (e.g., localhost:9090)")
		verbose    = flag.Bool("v", false, "Verbose logging")
	)
	flag.Parse()

	// Set up logging
	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Determine mode: connect to proxy or file-based config
	if *proxyAddr != "" {
		// Proxy mode - connect back to proxy and wait for configuration
		log.Printf("[CONSUMER-RUNNER] Connecting to proxy control plane at %s", *proxyAddr)

		runner := &ConsumerRunner{
			backends: make(map[string]plugin.Plugin),
		}

		// Wrap runner as a plugin adapter
		pluginAdapter := &ConsumerPluginAdapter{runner: runner}

		// Create control plane client and connect to proxy
		controlClient := plugin.NewPatternControlPlaneClient(
			*proxyAddr,
			"consumer",
			"0.1.0",
			pluginAdapter,
		)

		if err := controlClient.Connect(ctx); err != nil {
			log.Fatalf("[CONSUMER-RUNNER] Failed to connect to proxy: %v", err)
		}
		defer controlClient.Close()

		log.Println("[CONSUMER-RUNNER] ✅ Connected to proxy control plane, waiting for commands...")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("[CONSUMER-RUNNER] Shutting down...")
		log.Println("[CONSUMER-RUNNER] ✅ Consumer stopped successfully")

	} else {
		// File-based config mode (original behavior)
		if *configFile == "" {
			log.Fatal("Usage: consumer-runner -config <path-to-config.yaml> OR -proxy-addr <proxy-address>")
		}

		log.Printf("Loading configuration from %s", *configFile)
		config, err := loadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		if len(config.Namespaces) == 0 {
			log.Fatal("No namespaces found in configuration")
		}

		namespace := config.Namespaces[0]
		log.Printf("Running consumer pattern for namespace: %s", namespace.Name)
		log.Printf("Pattern: %s (%s)", namespace.Pattern, namespace.PatternVersion)
		log.Printf("Description: %s", namespace.Description)

		// Create and run consumer
		runner := &ConsumerRunner{
			config:   config,
			backends: make(map[string]plugin.Plugin),
		}

		// Initialize backends and consumer
		if err := runner.Initialize(ctx); err != nil {
			log.Fatalf("Failed to initialize: %v", err)
		}

		// Start consumer
		if err := runner.Start(ctx); err != nil {
			log.Fatalf("Failed to start consumer: %v", err)
		}

		log.Println("✅ Consumer is running. Press Ctrl+C to stop.")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")

		// Stop consumer
		if err := runner.Stop(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}

		log.Println("✅ Consumer stopped successfully")
	}
}

// loadConfig loads the consumer configuration from a YAML file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

// Initialize initializes backends and creates the consumer
func (r *ConsumerRunner) Initialize(ctx context.Context) error {
	namespace := r.config.Namespaces[0]

	// Initialize message source backend
	messageSource, err := r.initializeBackend(ctx, "message_source", namespace.Slots.MessageSource)
	if err != nil {
		return fmt.Errorf("failed to initialize message_source: %w", err)
	}
	r.backends["message_source"] = messageSource

	// Initialize state store backend (optional)
	var stateStore plugin.KeyValueBasicInterface
	if namespace.Slots.StateStore != nil {
		backend, err := r.initializeBackend(ctx, "state_store", namespace.Slots.StateStore)
		if err != nil {
			return fmt.Errorf("failed to initialize state_store: %w", err)
		}
		r.backends["state_store"] = backend
		// Type assert to KeyValueBasicInterface
		var ok bool
		stateStore, ok = backend.(plugin.KeyValueBasicInterface)
		if !ok {
			return fmt.Errorf("state_store backend does not implement KeyValueBasicInterface")
		}
	}

	// Initialize dead letter queue backend (optional)
	// TODO: Implement DLQ backend initialization

	// Create consumer configuration
	consumerConfig := consumer.Config{
		Name:        namespace.Name,
		Description: namespace.Description,
		Slots: consumer.SlotConfig{
			MessageSource: consumer.SlotBinding{
				Driver: namespace.Slots.MessageSource.Backend,
				Config: namespace.Slots.MessageSource.Config,
			},
		},
		Behavior: consumer.BehaviorConfig{
			ConsumerGroup: getStringFromBehavior(namespace.Behavior, "consumer_group"),
			Topic:         getStringFromBehavior(namespace.Behavior, "topic"),
			MaxRetries:    getIntFromBehavior(namespace.Behavior, "max_retries"),
			AutoCommit:    getBoolFromBehavior(namespace.Behavior, "auto_commit"),
			BatchSize:     getIntFromBehavior(namespace.Behavior, "batch_size"),
		},
	}

	// Add state store slot if present
	if namespace.Slots.StateStore != nil {
		consumerConfig.Slots.StateStore = consumer.SlotBinding{
			Driver: namespace.Slots.StateStore.Backend,
			Config: namespace.Slots.StateStore.Config,
		}
	}

	// Add dead letter queue slot if present
	if namespace.Slots.DeadLetterQueue != nil {
		dlqBinding := consumer.SlotBinding{
			Driver: namespace.Slots.DeadLetterQueue.Backend,
			Config: namespace.Slots.DeadLetterQueue.Config,
		}
		consumerConfig.Slots.DeadLetterQueue = &dlqBinding
	}

	// Create consumer
	c, err := consumer.New(consumerConfig)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}
	r.consumer = c

	// Bind slots to consumer
	log.Println("[CONSUMER-RUNNER] Binding slots to consumer...")
	// BindSlots(messageSource, stateStore, deadLetter, objectStore)
	// deadLetter and objectStore are not yet implemented in consumer-runner
	if err := r.consumer.BindSlots(messageSource, stateStore, nil, nil); err != nil {
		return fmt.Errorf("failed to bind slots: %w", err)
	}

	// Set a simple message processor
	r.consumer.SetProcessor(func(ctx context.Context, msg *plugin.PubSubMessage) error {
		log.Printf("[CONSUMER-RUNNER] 📨 Processed message: topic=%s, payload=%s", msg.Topic, string(msg.Payload))
		return nil
	})

	return nil
}

// initializeBackend initializes a backend driver based on the slot configuration
func (r *ConsumerRunner) initializeBackend(ctx context.Context, slotName string, slotConfig *SlotConfig) (plugin.Plugin, error) {
	if slotConfig == nil {
		return nil, fmt.Errorf("slot config is nil")
	}

	log.Printf("[CONSUMER-RUNNER] Initializing %s backend: %s", slotName, slotConfig.Backend)

	var backend plugin.Plugin

	switch slotConfig.Backend {
	case "nats":
		backend = nats.New()
	case "memstore":
		backend = memstore.New()
	// TODO: Add more backends (redis, kafka, postgres)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", slotConfig.Backend)
	}

	// Create plugin config
	pluginConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    slotConfig.Backend,
			Version: "0.1.0",
		},
		Backend: slotConfig.Config,
	}

	// Initialize backend
	if err := backend.Initialize(ctx, pluginConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize %s: %w", slotConfig.Backend, err)
	}

	// Start backend
	if err := backend.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", slotConfig.Backend, err)
	}

	log.Printf("[CONSUMER-RUNNER] ✅ %s backend initialized and started", slotConfig.Backend)
	return backend, nil
}

// Start starts the consumer
func (r *ConsumerRunner) Start(ctx context.Context) error {
	log.Println("[CONSUMER-RUNNER] Starting consumer...")
	if err := r.consumer.Start(ctx); err != nil {
		return err
	}

	// Give it a moment to settle
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Stop stops the consumer and all backends
func (r *ConsumerRunner) Stop(ctx context.Context) error {
	// Stop consumer first
	if r.consumer != nil {
		log.Println("[CONSUMER-RUNNER] Stopping consumer...")
		if err := r.consumer.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop consumer: %w", err)
		}
	}

	// Stop all backends
	for name, backend := range r.backends {
		log.Printf("[CONSUMER-RUNNER] Stopping %s backend...", name)
		if err := backend.Stop(ctx); err != nil {
			log.Printf("[CONSUMER-RUNNER] Warning: failed to stop %s: %v", name, err)
		}
	}

	return nil
}

// Helper functions to extract behavior values
func getStringFromBehavior(behavior map[string]interface{}, key string) string {
	if val, ok := behavior[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getIntFromBehavior(behavior map[string]interface{}, key string) int {
	if val, ok := behavior[key]; ok {
		// Handle both int and float64 (JSON numbers)
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return 0
}

func getBoolFromBehavior(behavior map[string]interface{}, key string) bool {
	if val, ok := behavior[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return false
}

// ConsumerPluginAdapter adapts ConsumerRunner to implement plugin.Plugin interface
// This allows the consumer to be controlled via the control plane protocol
type ConsumerPluginAdapter struct {
	runner *ConsumerRunner
	ctx    context.Context
	name   string
	version string
}

// Initialize implements plugin.Plugin.Initialize
func (a *ConsumerPluginAdapter) Initialize(ctx context.Context, config *plugin.Config) error {
	a.ctx = ctx
	a.name = config.Plugin.Name
	a.version = config.Plugin.Version

	log.Printf("[CONSUMER-RUNNER] ConsumerPluginAdapter: Initialize called with name=%s version=%s", a.name, a.version)

	// Parse slots configuration from config.Backend
	slotsData, ok := config.Backend["slots"]
	if !ok {
		return fmt.Errorf("missing 'slots' in configuration")
	}

	slotsMap, ok := slotsData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("'slots' must be a map")
	}

	// Parse behavior configuration
	behaviorData, ok := config.Backend["behavior"]
	if !ok {
		return fmt.Errorf("missing 'behavior' in configuration")
	}

	behaviorMap, ok := behaviorData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("'behavior' must be a map")
	}

	// Build namespace structure from config
	namespace := Namespace{
		Name:           a.name,
		Pattern:        "consumer",
		PatternVersion: a.version,
		Description:    "Dynamic consumer from control plane",
		Slots:          parseSlots(slotsMap),
		Behavior:       behaviorMap,
	}

	// Create config structure
	a.runner.config = &Config{
		Namespaces: []Namespace{namespace},
	}

	log.Println("[CONSUMER-RUNNER] ConsumerPluginAdapter: Initializing backends and consumer...")
	return a.runner.Initialize(ctx)
}

// Start implements plugin.Plugin.Start
func (a *ConsumerPluginAdapter) Start(ctx context.Context) error {
	log.Println("[CONSUMER-RUNNER] ConsumerPluginAdapter: Start called")
	return a.runner.Start(ctx)
}

// Drain implements plugin.Plugin.Drain
func (a *ConsumerPluginAdapter) Drain(ctx context.Context, timeoutSeconds int32, reason string) (*plugin.DrainMetrics, error) {
	log.Printf("[CONSUMER-RUNNER] ConsumerPluginAdapter: Drain called (timeout=%ds, reason=%s)", timeoutSeconds, reason)

	// For consumer pattern, we delegate drain to the underlying consumer
	// The consumer will complete in-flight message processing before returning
	if a.runner.consumer != nil {
		// Consumer doesn't have Drain yet, but we'll return metrics anyway
		// In the future, the consumer could track in-flight messages
		return &plugin.DrainMetrics{
			DrainedOperations: 0,
			AbortedOperations: 0,
		}, nil
	}

	return &plugin.DrainMetrics{
		DrainedOperations: 0,
		AbortedOperations: 0,
	}, nil
}

// Stop implements plugin.Plugin.Stop
func (a *ConsumerPluginAdapter) Stop(ctx context.Context) error {
	log.Println("[CONSUMER-RUNNER] ConsumerPluginAdapter: Stop called")
	return a.runner.Stop(ctx)
}

// Health implements plugin.Plugin.Health
func (a *ConsumerPluginAdapter) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	if a.runner.consumer == nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthDegraded,
			Message: "consumer not initialized",
		}, nil
	}
	return a.runner.consumer.Health(ctx)
}

// Name implements plugin.Plugin.Name
func (a *ConsumerPluginAdapter) Name() string {
	return a.name
}

// Version implements plugin.Plugin.Version
func (a *ConsumerPluginAdapter) Version() string {
	return a.version
}

// GetInterfaceDeclarations implements plugin.Plugin.GetInterfaceDeclarations
func (a *ConsumerPluginAdapter) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	// Consumer pattern doesn't directly expose interfaces - it's a composite pattern
	// The proxy doesn't need to know about consumer's internal backend interfaces
	return []*pb.InterfaceDeclaration{}
}

// parseSlots converts a map[string]interface{} to Slots structure
func parseSlots(slotsMap map[string]interface{}) Slots {
	slots := Slots{}

	if msgSourceData, ok := slotsMap["message_source"]; ok {
		if msgSourceMap, ok := msgSourceData.(map[string]interface{}); ok {
			slots.MessageSource = parseSlotConfig(msgSourceMap)
		}
	}

	if stateStoreData, ok := slotsMap["state_store"]; ok {
		if stateStoreMap, ok := stateStoreData.(map[string]interface{}); ok {
			slots.StateStore = parseSlotConfig(stateStoreMap)
		}
	}

	if dlqData, ok := slotsMap["dead_letter_queue"]; ok {
		if dlqMap, ok := dlqData.(map[string]interface{}); ok {
			slots.DeadLetterQueue = parseSlotConfig(dlqMap)
		}
	}

	return slots
}

// parseSlotConfig converts a map[string]interface{} to SlotConfig
func parseSlotConfig(slotMap map[string]interface{}) *SlotConfig {
	config := &SlotConfig{}

	if backend, ok := slotMap["backend"].(string); ok {
		config.Backend = backend
	} else if driver, ok := slotMap["driver"].(string); ok {
		// Support "driver" as alias for "backend"
		config.Backend = driver
	}

	if interfaces, ok := slotMap["interfaces"].([]interface{}); ok {
		for _, iface := range interfaces {
			if str, ok := iface.(string); ok {
				config.Interfaces = append(config.Interfaces, str)
			}
		}
	}

	if configData, ok := slotMap["config"].(map[string]interface{}); ok {
		config.Config = configData
	}

	return config
}
