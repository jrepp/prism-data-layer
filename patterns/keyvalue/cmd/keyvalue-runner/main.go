package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jrepp/prism-data-layer/patterns/keyvalue"
	"github.com/jrepp/prism-data-layer/pkg/drivers/memstore"
	"github.com/jrepp/prism-data-layer/pkg/drivers/redis"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"gopkg.in/yaml.v3"
)

// KeyValueRunner runs a keyvalue pattern with dynamically configured backends
type KeyValueRunner struct {
	config  *Config
	kv      *keyvalue.KeyValue
	backend plugin.Plugin
}

// Config represents the keyvalue configuration loaded from YAML
type Config struct {
	Namespaces []Namespace `yaml:"namespaces"`
}

// Namespace represents a single namespace configuration
type Namespace struct {
	Name           string                 `yaml:"name"`
	Pattern        string                 `yaml:"pattern"`
	PatternVersion string                 `yaml:"pattern_version"`
	Description    string                 `yaml:"description"`
	Backend        string                 `yaml:"backend"`
	BackendConfig  map[string]interface{} `yaml:"backend_config"`
}

func main() {
	var (
		configFile = flag.String("config", "", "Path to keyvalue configuration YAML file")
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
		log.Printf("[KEYVALUE-RUNNER] Connecting to proxy control plane at %s", *proxyAddr)

		runner := &KeyValueRunner{}

		// Wrap runner as a plugin adapter
		pluginAdapter := &KeyValuePluginAdapter{runner: runner}

		// Create control plane client and connect to proxy
		controlClient := plugin.NewPatternControlPlaneClient(
			*proxyAddr,
			"keyvalue",
			"0.1.0",
			pluginAdapter,
		)

		if err := controlClient.Connect(ctx); err != nil {
			log.Fatalf("[KEYVALUE-RUNNER] Failed to connect to proxy: %v", err)
		}
		defer controlClient.Close()

		log.Println("[KEYVALUE-RUNNER] ✅ Connected to proxy control plane, waiting for commands...")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("[KEYVALUE-RUNNER] Shutting down...")
		log.Println("[KEYVALUE-RUNNER] ✅ KeyValue stopped successfully")

	} else {
		// File-based config mode
		if *configFile == "" {
			log.Fatal("Usage: keyvalue-runner -config <path-to-config.yaml> OR -proxy-addr <proxy-address>")
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
		log.Printf("Running keyvalue pattern for namespace: %s", namespace.Name)
		log.Printf("Pattern: %s (%s)", namespace.Pattern, namespace.PatternVersion)
		log.Printf("Description: %s", namespace.Description)
		log.Printf("Backend: %s", namespace.Backend)

		// Create and run keyvalue
		runner := &KeyValueRunner{
			config: config,
		}

		// Initialize backend and keyvalue pattern
		if err := runner.Initialize(ctx); err != nil {
			log.Fatalf("Failed to initialize: %v", err)
		}

		// Start keyvalue
		if err := runner.Start(ctx); err != nil {
			log.Fatalf("Failed to start keyvalue: %v", err)
		}

		log.Println("✅ KeyValue is running. Press Ctrl+C to stop.")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")

		// Stop keyvalue
		if err := runner.Stop(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}

		log.Println("✅ KeyValue stopped successfully")
	}
}

// loadConfig loads the keyvalue configuration from a YAML file
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

// Initialize initializes backend and creates the keyvalue pattern
func (r *KeyValueRunner) Initialize(ctx context.Context) error {
	namespace := r.config.Namespaces[0]

	// Initialize backend driver
	backend, err := r.initializeBackend(ctx, namespace.Backend, namespace.BackendConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize backend: %w", err)
	}
	r.backend = backend

	// Type assert to KeyValueDriver
	kvDriver, ok := backend.(keyvalue.KeyValueDriver)
	if !ok {
		return fmt.Errorf("backend does not implement KeyValueDriver interface")
	}

	// Create keyvalue pattern
	r.kv = keyvalue.NewWithDriver(kvDriver)

	log.Println("[KEYVALUE-RUNNER] ✅ KeyValue pattern created successfully")
	return nil
}

// initializeBackend initializes a backend driver
func (r *KeyValueRunner) initializeBackend(ctx context.Context, backendName string, backendConfig map[string]interface{}) (plugin.Plugin, error) {
	log.Printf("[KEYVALUE-RUNNER] Initializing backend: %s", backendName)

	var backend plugin.Plugin

	switch backendName {
	case "memstore":
		backend = memstore.New()
	case "redis":
		backend = redis.New()
	// TODO: Add more backends (postgres, etc.)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backendName)
	}

	// Create plugin config
	pluginConfig := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    backendName,
			Version: "0.1.0",
		},
		Backend: backendConfig,
	}

	// Initialize backend
	if err := backend.Initialize(ctx, pluginConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize %s: %w", backendName, err)
	}

	// Start backend
	if err := backend.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", backendName, err)
	}

	log.Printf("[KEYVALUE-RUNNER] ✅ %s backend initialized and started", backendName)
	return backend, nil
}

// Start starts the keyvalue pattern
func (r *KeyValueRunner) Start(ctx context.Context) error {
	log.Println("[KEYVALUE-RUNNER] Starting keyvalue pattern...")
	if err := r.kv.Start(ctx); err != nil {
		return err
	}

	return nil
}

// Stop stops the keyvalue pattern and backend
func (r *KeyValueRunner) Stop(ctx context.Context) error {
	// Stop keyvalue first
	if r.kv != nil {
		log.Println("[KEYVALUE-RUNNER] Stopping keyvalue pattern...")
		if err := r.kv.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop keyvalue: %w", err)
		}
	}

	// Stop backend
	if r.backend != nil {
		log.Println("[KEYVALUE-RUNNER] Stopping backend...")
		if err := r.backend.Stop(ctx); err != nil {
			log.Printf("[KEYVALUE-RUNNER] Warning: failed to stop backend: %v", err)
		}
	}

	return nil
}

// KeyValuePluginAdapter adapts KeyValueRunner to implement plugin.Plugin interface
// This allows the keyvalue pattern to be controlled via the control plane protocol
type KeyValuePluginAdapter struct {
	runner  *KeyValueRunner
	ctx     context.Context
	name    string
	version string
}

// Initialize implements plugin.Plugin.Initialize
func (a *KeyValuePluginAdapter) Initialize(ctx context.Context, config *plugin.Config) error {
	a.ctx = ctx
	a.name = config.Plugin.Name
	a.version = config.Plugin.Version

	log.Printf("[KEYVALUE-RUNNER] KeyValuePluginAdapter: Initialize called with name=%s version=%s", a.name, a.version)

	// Parse backend configuration
	backendName, ok := config.Backend["backend"].(string)
	if !ok {
		return fmt.Errorf("missing 'backend' in configuration")
	}

	backendConfig, ok := config.Backend["backend_config"].(map[string]interface{})
	if !ok {
		backendConfig = make(map[string]interface{})
	}

	// Build namespace structure from config
	namespace := Namespace{
		Name:           a.name,
		Pattern:        "keyvalue",
		PatternVersion: a.version,
		Description:    "Dynamic keyvalue from control plane",
		Backend:        backendName,
		BackendConfig:  backendConfig,
	}

	// Create config structure
	a.runner.config = &Config{
		Namespaces: []Namespace{namespace},
	}

	log.Println("[KEYVALUE-RUNNER] KeyValuePluginAdapter: Initializing backend and keyvalue...")
	return a.runner.Initialize(ctx)
}

// Start implements plugin.Plugin.Start
func (a *KeyValuePluginAdapter) Start(ctx context.Context) error {
	log.Println("[KEYVALUE-RUNNER] KeyValuePluginAdapter: Start called")
	return a.runner.Start(ctx)
}

// Stop implements plugin.Plugin.Stop
func (a *KeyValuePluginAdapter) Stop(ctx context.Context) error {
	log.Println("[KEYVALUE-RUNNER] KeyValuePluginAdapter: Stop called")
	return a.runner.Stop(ctx)
}

// Health implements plugin.Plugin.Health
func (a *KeyValuePluginAdapter) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	if a.runner.kv == nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthDegraded,
			Message: "keyvalue not initialized",
		}, nil
	}
	return a.runner.kv.Health(ctx)
}

// Name implements plugin.Plugin.Name
func (a *KeyValuePluginAdapter) Name() string {
	return a.name
}

// Version implements plugin.Plugin.Version
func (a *KeyValuePluginAdapter) Version() string {
	return a.version
}

// GetInterfaceDeclarations implements plugin.Plugin.GetInterfaceDeclarations
func (a *KeyValuePluginAdapter) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	// KeyValue pattern exposes keyvalue_basic and keyvalue_ttl interfaces
	return []*pb.InterfaceDeclaration{
		{
			Name:      "KeyValueBasicInterface",
			ProtoFile: "prism/interfaces/keyvalue/keyvalue_basic.proto",
		},
		{
			Name:      "KeyValueTTLInterface",
			ProtoFile: "prism/interfaces/keyvalue/keyvalue_ttl.proto",
		},
	}
}
