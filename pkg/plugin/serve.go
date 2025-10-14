package core

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"time"
)

// ServeOptions configures backend driver execution
type ServeOptions struct {
	// DefaultName is the default backend driver name if not specified in config
	DefaultName string

	// DefaultVersion is the default backend driver version
	DefaultVersion string

	// DefaultPort is the default control plane port if not specified
	// Use 0 for dynamic port allocation
	DefaultPort int

	// ConfigPath is the path to the configuration file
	// Can be overridden by -config flag
	ConfigPath string

	// MetricsPort is the port for the Prometheus metrics endpoint
	// Set to 0 to disable metrics HTTP server
	MetricsPort int

	// EnableTracing enables OpenTelemetry tracing
	EnableTracing bool

	// TraceExporter specifies the trace exporter ("stdout", "jaeger", "otlp")
	// Default: "stdout" (for development)
	TraceExporter string
}

// ServeBackendDriver is the main entrypoint for backend driver executables.
// It handles all boilerplate:
// - Flag parsing
// - Config loading (with defaults)
// - Dynamic port allocation
// - Logging setup
// - Lifecycle management (Initialize → Start → Stop)
//
// This should be called from main() in backend driver executables:
//
//	func main() {
//	    core.ServeBackendDriver(memstore.New, core.ServeOptions{
//	        DefaultName:    "memstore",
//	        DefaultVersion: "0.1.0",
//	        DefaultPort:    9090,
//	        ConfigPath:     "config.yaml",
//	    })
//	}
//
// The driver factory function receives the loaded config and should return
// a Plugin (BackendDriver) instance. The driver should NOT connect to any
// external resources in the factory - all connections should happen in
// Initialize() or Start() lifecycle methods.
func ServeBackendDriver(driverFactory func() Plugin, opts ServeOptions) {
	// Parse flags
	configPath := flag.String("config", opts.ConfigPath, "Path to configuration file")
	grpcPort := flag.Int("grpc-port", opts.DefaultPort, "gRPC control plane port (0 for dynamic allocation)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	metricsPort := flag.Int("metrics-port", opts.MetricsPort, "Prometheus metrics port (0 to disable)")
	enableTracing := flag.Bool("enable-tracing", opts.EnableTracing, "Enable OpenTelemetry tracing")
	traceExporter := flag.String("trace-exporter", opts.TraceExporter, "Trace exporter (stdout, jaeger, otlp)")
	flag.Parse()

	// Initialize logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("backend driver starting",
		"name", opts.DefaultName,
		"version", opts.DefaultVersion,
		"config_path", *configPath,
		"grpc_port", *grpcPort)

	// Load configuration (use defaults if file doesn't exist)
	config, err := LoadConfig(*configPath)
	if err != nil {
		slog.Warn("failed to load config, using defaults",
			"error", err,
			"config_path", *configPath)
		// Create default config
		config = &Config{
			Plugin: PluginConfig{
				Name:    opts.DefaultName,
				Version: opts.DefaultVersion,
			},
			ControlPlane: ControlPlaneConfig{
				Port: opts.DefaultPort,
			},
			Backend: make(map[string]any),
		}
	}

	// Handle dynamic port allocation if requested
	if *grpcPort == 0 {
		// Allocate a random available port
		port, err := allocateDynamicPort()
		if err != nil {
			slog.Error("failed to allocate dynamic port", "error", err)
			os.Exit(1)
		}
		slog.Info("allocated dynamic port", "port", port)
		config.ControlPlane.Port = port
	} else if *grpcPort != opts.DefaultPort {
		// Override port if explicitly provided
		slog.Info("overriding control plane port from flag",
			"config_port", config.ControlPlane.Port,
			"flag_port", *grpcPort)
		config.ControlPlane.Port = *grpcPort
	}

	// Initialize observability (tracing, metrics, health endpoints)
	obsConfig := &ObservabilityConfig{
		ServiceName:    opts.DefaultName,
		ServiceVersion: opts.DefaultVersion,
		MetricsPort:    *metricsPort,
		EnableTracing:  *enableTracing,
		TraceExporter:  *traceExporter,
	}
	observability := NewObservabilityManager(obsConfig)

	ctx := context.Background()
	if err := observability.Initialize(ctx); err != nil {
		slog.Error("failed to initialize observability", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := observability.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown observability", "error", err)
		}
	}()

	// Create backend driver instance
	slog.Info("creating backend driver instance",
		"name", opts.DefaultName,
		"version", opts.DefaultVersion)
	driver := driverFactory()

	// Validate driver metadata matches options
	if driver.Name() != opts.DefaultName {
		slog.Warn("driver name mismatch",
			"expected", opts.DefaultName,
			"actual", driver.Name())
	}

	// Bootstrap driver lifecycle with config
	slog.Info("bootstrapping backend driver",
		"name", driver.Name(),
		"version", driver.Version(),
		"control_plane_port", config.ControlPlane.Port,
		"metrics_port", *metricsPort,
		"tracing_enabled", *enableTracing)

	if err := BootstrapWithConfig(driver, config); err != nil {
		log.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("backend driver shut down successfully",
		"name", driver.Name(),
		"version", driver.Version())
}

// allocateDynamicPort finds an available port by binding to :0 and returning the allocated port
func allocateDynamicPort() (int, error) {
	// Bind to port 0 to let the OS assign an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to allocate dynamic port: %w", err)
	}
	defer listener.Close()

	// Extract the port that was allocated
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
