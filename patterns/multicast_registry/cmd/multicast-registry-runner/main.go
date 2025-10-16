package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jrepp/prism-data-layer/patterns/multicast_registry"
	"github.com/jrepp/prism-data-layer/patterns/multicast_registry/backends"
	"github.com/jrepp/prism-data-layer/pkg/drivers/kafka"
	"github.com/jrepp/prism-data-layer/pkg/drivers/nats"
	"github.com/jrepp/prism-data-layer/pkg/drivers/postgres"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"gopkg.in/yaml.v3"
)

// MulticastRegistryRunner runs a multicast registry pattern with dynamically configured backends
type MulticastRegistryRunner struct {
	config      *Config
	coordinator *multicast_registry.Coordinator
	backends    map[string]plugin.Plugin
}

// Config represents the multicast registry configuration loaded from YAML
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

// Slots represents the slot configuration for multicast registry
type Slots struct {
	Registry   *SlotConfig `yaml:"registry,omitempty"`   // KeyValue backend for identity storage
	Messaging  *SlotConfig `yaml:"messaging,omitempty"`  // PubSub backend for multicast delivery
	Durability *SlotConfig `yaml:"durability,omitempty"` // Optional queue backend for persistence
}

// SlotConfig represents a single slot's backend configuration
type SlotConfig struct {
	Backend    string                 `yaml:"backend"`
	Interfaces []string               `yaml:"interfaces"`
	Config     map[string]interface{} `yaml:"config"`
}

func main() {
	var (
		configFile = flag.String("config", "", "Path to multicast registry configuration YAML file")
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
		log.Printf("[MULTICAST-REGISTRY] Connecting to proxy control plane at %s", *proxyAddr)

		runner := &MulticastRegistryRunner{
			backends: make(map[string]plugin.Plugin),
		}

		// Wrap runner as a plugin adapter
		pluginAdapter := &MulticastRegistryPluginAdapter{runner: runner}

		// Create control plane client and connect to proxy
		controlClient := plugin.NewPatternControlPlaneClient(
			*proxyAddr,
			"multicast-registry",
			"0.1.0",
			pluginAdapter,
		)

		if err := controlClient.Connect(ctx); err != nil {
			log.Fatalf("[MULTICAST-REGISTRY] Failed to connect to proxy: %v", err)
		}
		defer controlClient.Close()

		log.Println("[MULTICAST-REGISTRY] ✅ Connected to proxy control plane, waiting for commands...")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("[MULTICAST-REGISTRY] Shutting down...")
		log.Println("[MULTICAST-REGISTRY] ✅ Multicast registry stopped successfully")

	} else {
		// File-based config mode
		if *configFile == "" {
			log.Fatal("Usage: multicast-registry-runner -config <path-to-config.yaml> OR -proxy-addr <proxy-address>")
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
		log.Printf("Running multicast registry pattern for namespace: %s", namespace.Name)
		log.Printf("Pattern: %s (%s)", namespace.Pattern, namespace.PatternVersion)
		log.Printf("Description: %s", namespace.Description)

		// Create and run multicast registry
		runner := &MulticastRegistryRunner{
			config:   config,
			backends: make(map[string]plugin.Plugin),
		}

		// Initialize backends and coordinator
		if err := runner.Initialize(ctx); err != nil {
			log.Fatalf("Failed to initialize: %v", err)
		}

		// Start coordinator
		if err := runner.Start(ctx); err != nil {
			log.Fatalf("Failed to start multicast registry: %v", err)
		}

		log.Println("✅ Multicast registry is running. Press Ctrl+C to stop.")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")

		// Stop coordinator
		if err := runner.Stop(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}

		log.Println("✅ Multicast registry stopped successfully")
	}
}

// loadConfig loads the configuration from a YAML file
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

// Initialize initializes backends and creates the coordinator
func (r *MulticastRegistryRunner) Initialize(ctx context.Context) error {
	namespace := r.config.Namespaces[0]

	// Initialize registry backend (required)
	registryPlugin, err := r.initializeBackend(ctx, "registry", namespace.Slots.Registry)
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}
	r.backends["registry"] = registryPlugin

	// Type assert to KeyValueBasicInterface
	registryBackend, ok := registryPlugin.(plugin.KeyValueBasicInterface)
	if !ok {
		return fmt.Errorf("registry backend does not implement KeyValueBasicInterface")
	}

	// Initialize messaging backend (required)
	messagingPlugin, err := r.initializeBackend(ctx, "messaging", namespace.Slots.Messaging)
	if err != nil {
		return fmt.Errorf("failed to initialize messaging: %w", err)
	}
	r.backends["messaging"] = messagingPlugin

	// Type assert to PubSubInterface
	messagingBackend, ok := messagingPlugin.(plugin.PubSubInterface)
	if !ok {
		return fmt.Errorf("messaging backend does not implement PubSubInterface")
	}

	// Initialize durability backend (optional)
	var durabilityBackend multicast_registry.DurabilityBackend
	if namespace.Slots.Durability != nil {
		durabilityPlugin, err := r.initializeBackend(ctx, "durability", namespace.Slots.Durability)
		if err != nil {
			return fmt.Errorf("failed to initialize durability: %w", err)
		}
		r.backends["durability"] = durabilityPlugin

		// Type assert to QueueInterface
		queueBackend, ok := durabilityPlugin.(plugin.QueueInterface)
		if !ok {
			return fmt.Errorf("durability backend does not implement QueueInterface")
		}
		durabilityBackend = backends.NewQueueDurabilityBackend(queueBackend)
	}

	// Create coordinator configuration
	coordinatorConfig := multicast_registry.DefaultConfig()

	// Override defaults from behavior configuration
	if maxIdentities, ok := namespace.Behavior["max_identities"].(int); ok {
		coordinatorConfig.MaxIdentities = maxIdentities
	}
	if topicPrefix, ok := namespace.Behavior["topic_prefix"].(string); ok {
		coordinatorConfig.Messaging.TopicPrefix = topicPrefix
	}
	if retryAttempts, ok := namespace.Behavior["retry_attempts"].(int); ok {
		coordinatorConfig.Messaging.RetryAttempts = retryAttempts
	}

	// Create backend adapters
	registryAdapter := backends.NewKeyValueRegistryBackend(registryBackend)
	messagingAdapter := backends.NewPubSubMessagingBackend(messagingBackend)

	// Create coordinator
	coordinator, err := multicast_registry.NewCoordinator(
		coordinatorConfig,
		registryAdapter,
		messagingAdapter,
		durabilityBackend,
	)
	if err != nil {
		return fmt.Errorf("failed to create coordinator: %w", err)
	}

	r.coordinator = coordinator
	log.Println("[MULTICAST-REGISTRY] ✅ Coordinator initialized")

	return nil
}

// initializeBackend initializes a backend driver based on the slot configuration
func (r *MulticastRegistryRunner) initializeBackend(ctx context.Context, slotName string, slotConfig *SlotConfig) (plugin.Plugin, error) {
	if slotConfig == nil {
		return nil, fmt.Errorf("slot config is nil")
	}

	log.Printf("[MULTICAST-REGISTRY] Initializing %s backend: %s", slotName, slotConfig.Backend)

	var backend plugin.Plugin

	switch slotConfig.Backend {
	case "redis":
		backend = redis.New()
	case "nats":
		backend = nats.New()
	case "kafka":
		backend = kafka.New()
	case "postgres", "postgresql":
		backend = postgres.New()
	default:
		return nil, fmt.Errorf("unsupported backend: %s (supported: redis, nats, kafka, postgres)", slotConfig.Backend)
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

	log.Printf("[MULTICAST-REGISTRY] ✅ %s backend initialized and started", slotConfig.Backend)
	return backend, nil
}

// Start starts the coordinator
func (r *MulticastRegistryRunner) Start(ctx context.Context) error {
	log.Println("[MULTICAST-REGISTRY] Coordinator ready for operations")
	return nil
}

// Stop stops the coordinator and all backends
func (r *MulticastRegistryRunner) Stop(ctx context.Context) error {
	// Stop coordinator first
	if r.coordinator != nil {
		log.Println("[MULTICAST-REGISTRY] Stopping coordinator...")
		if err := r.coordinator.Close(); err != nil {
			return fmt.Errorf("failed to stop coordinator: %w", err)
		}
	}

	// Stop all backends
	for name, backend := range r.backends {
		log.Printf("[MULTICAST-REGISTRY] Stopping %s backend...", name)
		if err := backend.Stop(ctx); err != nil {
			log.Printf("[MULTICAST-REGISTRY] Warning: failed to stop %s: %v", name, err)
		}
	}

	return nil
}

// MulticastRegistryPluginAdapter adapts MulticastRegistryRunner to implement plugin.Plugin interface
type MulticastRegistryPluginAdapter struct {
	runner  *MulticastRegistryRunner
	ctx     context.Context
	name    string
	version string
}

// Initialize implements plugin.Plugin.Initialize
func (a *MulticastRegistryPluginAdapter) Initialize(ctx context.Context, config *plugin.Config) error {
	a.ctx = ctx
	a.name = config.Plugin.Name
	a.version = config.Plugin.Version

	log.Printf("[MULTICAST-REGISTRY] PluginAdapter: Initialize called with name=%s version=%s", a.name, a.version)

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
		// Behavior is optional, use defaults
		behaviorData = make(map[string]interface{})
	}

	behaviorMap, ok := behaviorData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("'behavior' must be a map")
	}

	// Build namespace structure from config
	namespace := Namespace{
		Name:           a.name,
		Pattern:        "multicast-registry",
		PatternVersion: a.version,
		Description:    "Dynamic multicast registry from control plane",
		Slots:          parseSlots(slotsMap),
		Behavior:       behaviorMap,
	}

	// Create config structure
	a.runner.config = &Config{
		Namespaces: []Namespace{namespace},
	}

	log.Println("[MULTICAST-REGISTRY] PluginAdapter: Initializing backends and coordinator...")
	return a.runner.Initialize(ctx)
}

// Start implements plugin.Plugin.Start
func (a *MulticastRegistryPluginAdapter) Start(ctx context.Context) error {
	log.Println("[MULTICAST-REGISTRY] PluginAdapter: Start called")
	return a.runner.Start(ctx)
}

// Drain implements plugin.Plugin.Drain
func (a *MulticastRegistryPluginAdapter) Drain(ctx context.Context, timeoutSeconds int32, reason string) (*plugin.DrainMetrics, error) {
	log.Printf("[MULTICAST-REGISTRY] PluginAdapter: Drain called (timeout=%ds, reason=%s)", timeoutSeconds, reason)

	// For multicast registry, we allow in-flight operations to complete
	// The coordinator handles message delivery and registration atomically
	return &plugin.DrainMetrics{
		DrainedOperations: 0,
		AbortedOperations: 0,
	}, nil
}

// Stop implements plugin.Plugin.Stop
func (a *MulticastRegistryPluginAdapter) Stop(ctx context.Context) error {
	log.Println("[MULTICAST-REGISTRY] PluginAdapter: Stop called")
	return a.runner.Stop(ctx)
}

// Health implements plugin.Plugin.Health
func (a *MulticastRegistryPluginAdapter) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	if a.runner.coordinator == nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthDegraded,
			Message: "coordinator not initialized",
		}, nil
	}
	// TODO: Implement coordinator health check
	return &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "multicast registry operational",
	}, nil
}

// Name implements plugin.Plugin.Name
func (a *MulticastRegistryPluginAdapter) Name() string {
	return a.name
}

// Version implements plugin.Plugin.Version
func (a *MulticastRegistryPluginAdapter) Version() string {
	return a.version
}

// GetInterfaceDeclarations implements plugin.Plugin.GetInterfaceDeclarations
func (a *MulticastRegistryPluginAdapter) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	// Multicast registry is a composite pattern with custom operations
	// It doesn't directly expose standard interfaces like KeyValue or PubSub
	return []*pb.InterfaceDeclaration{
		{
			Name:      "MulticastRegistryInterface",
			ProtoFile: "prism/interfaces/multicast_registry.proto",
			Version:   "v1",
		},
	}
}

// parseSlots converts a map[string]interface{} to Slots structure
func parseSlots(slotsMap map[string]interface{}) Slots {
	slots := Slots{}

	if registryData, ok := slotsMap["registry"]; ok {
		if registryMap, ok := registryData.(map[string]interface{}); ok {
			slots.Registry = parseSlotConfig(registryMap)
		}
	}

	if messagingData, ok := slotsMap["messaging"]; ok {
		if messagingMap, ok := messagingData.(map[string]interface{}); ok {
			slots.Messaging = parseSlotConfig(messagingMap)
		}
	}

	if durabilityData, ok := slotsMap["durability"]; ok {
		if durabilityMap, ok := durabilityData.(map[string]interface{}); ok {
			slots.Durability = parseSlotConfig(durabilityMap)
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
