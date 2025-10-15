package launcher

import (
	"fmt"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
)

// ServiceBuilder provides a fluent interface for constructing a launcher Service.
//
// Usage:
//
//	service, err := launcher.NewBuilder().
//	    WithPatternsDir("./patterns").
//	    WithNamespaceIsolation().
//	    WithResyncInterval(30 * time.Second).
//	    Build()
//
// All builder methods return the builder for method chaining.
type ServiceBuilder struct {
	config *Config
	err    error
}

// NewBuilder creates a new ServiceBuilder with sensible defaults.
//
// Defaults:
//   - PatternsDir: "./patterns"
//   - DefaultIsolation: IsolationNamespace
//   - ResyncInterval: 30 seconds
//   - BackOffPeriod: 5 seconds
//   - CPULimit: 2.0
//   - MemoryLimit: "1Gi"
func NewBuilder() *ServiceBuilder {
	return &ServiceBuilder{
		config: DefaultConfig(),
	}
}

// WithPatternsDir sets the directory to scan for pattern manifests.
//
// Example:
//
//	builder.WithPatternsDir("/opt/prism/patterns")
func (b *ServiceBuilder) WithPatternsDir(dir string) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if dir == "" {
		b.err = fmt.Errorf("patterns directory cannot be empty")
		return b
	}
	b.config.PatternsDir = dir
	return b
}

// WithDefaultIsolation sets the default isolation level for patterns.
//
// Valid levels: IsolationNone, IsolationNamespace, IsolationSession
//
// Example:
//
//	builder.WithDefaultIsolation(isolation.IsolationSession)
func (b *ServiceBuilder) WithDefaultIsolation(level isolation.IsolationLevel) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	b.config.DefaultIsolation = level
	return b
}

// WithNoneIsolation sets default isolation to NONE (shared process for all requests).
//
// Use case: Stateless patterns with no tenant isolation requirements.
//
// Example:
//
//	builder.WithNoneIsolation()
func (b *ServiceBuilder) WithNoneIsolation() *ServiceBuilder {
	return b.WithDefaultIsolation(isolation.IsolationNone)
}

// WithNamespaceIsolation sets default isolation to NAMESPACE (one process per tenant).
//
// Use case: Multi-tenant SaaS requiring fault and resource isolation.
//
// Example:
//
//	builder.WithNamespaceIsolation()
func (b *ServiceBuilder) WithNamespaceIsolation() *ServiceBuilder {
	return b.WithDefaultIsolation(isolation.IsolationNamespace)
}

// WithSessionIsolation sets default isolation to SESSION (one process per user).
//
// Use case: High-security environments or compliance requirements (PCI-DSS, HIPAA).
//
// Example:
//
//	builder.WithSessionIsolation()
func (b *ServiceBuilder) WithSessionIsolation() *ServiceBuilder {
	return b.WithDefaultIsolation(isolation.IsolationSession)
}

// WithResyncInterval sets how often the process manager checks process health.
//
// Lower values: Faster failure detection, higher CPU usage
// Higher values: Slower failure detection, lower CPU usage
//
// Example:
//
//	// Development: fast failure detection
//	builder.WithResyncInterval(5 * time.Second)
//
//	// Production: balanced monitoring
//	builder.WithResyncInterval(30 * time.Second)
func (b *ServiceBuilder) WithResyncInterval(interval time.Duration) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if interval < time.Second {
		b.err = fmt.Errorf("resync interval must be at least 1 second, got %v", interval)
		return b
	}
	b.config.ResyncInterval = interval
	return b
}

// WithBackOffPeriod sets the delay before retrying failed process starts.
//
// Lower values: Faster retries, risk of retry storms
// Higher values: Slower retries, more graceful failure handling
//
// Example:
//
//	// Development: quick retries
//	builder.WithBackOffPeriod(1 * time.Second)
//
//	// Production: avoid retry storms
//	builder.WithBackOffPeriod(5 * time.Second)
func (b *ServiceBuilder) WithBackOffPeriod(period time.Duration) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if period < 0 {
		b.err = fmt.Errorf("backoff period cannot be negative, got %v", period)
		return b
	}
	b.config.BackOffPeriod = period
	return b
}

// WithCPULimit sets the CPU limit for pattern processes.
//
// Value represents CPU cores (e.g., 1.0 = 1 core, 2.5 = 2.5 cores)
//
// Example:
//
//	builder.WithCPULimit(1.5)  // Limit to 1.5 CPU cores
func (b *ServiceBuilder) WithCPULimit(limit float64) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if limit <= 0 {
		b.err = fmt.Errorf("CPU limit must be positive, got %v", limit)
		return b
	}
	b.config.CPULimit = limit
	return b
}

// WithMemoryLimit sets the memory limit for pattern processes.
//
// Accepts sizes like "512Mi", "1Gi", "2G"
//
// Example:
//
//	builder.WithMemoryLimit("512Mi")  // Limit to 512 MiB
func (b *ServiceBuilder) WithMemoryLimit(limit string) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if limit == "" {
		b.err = fmt.Errorf("memory limit cannot be empty")
		return b
	}
	// TODO: Validate memory limit format (512Mi, 1Gi, etc.)
	b.config.MemoryLimit = limit
	return b
}

// WithResourceLimits sets both CPU and memory limits in one call.
//
// Example:
//
//	builder.WithResourceLimits(2.0, "1Gi")
func (b *ServiceBuilder) WithResourceLimits(cpuLimit float64, memoryLimit string) *ServiceBuilder {
	return b.WithCPULimit(cpuLimit).WithMemoryLimit(memoryLimit)
}

// WithDevelopmentDefaults configures the launcher for local development.
//
// Settings:
//   - ResyncInterval: 5 seconds (fast failure detection)
//   - BackOffPeriod: 1 second (quick retries)
//   - DefaultIsolation: Namespace
//
// Example:
//
//	builder.WithDevelopmentDefaults()
func (b *ServiceBuilder) WithDevelopmentDefaults() *ServiceBuilder {
	return b.
		WithResyncInterval(5 * time.Second).
		WithBackOffPeriod(1 * time.Second).
		WithNamespaceIsolation()
}

// WithProductionDefaults configures the launcher for production deployment.
//
// Settings:
//   - ResyncInterval: 30 seconds (balanced monitoring)
//   - BackOffPeriod: 5 seconds (avoid retry storms)
//   - DefaultIsolation: Namespace
//   - CPULimit: 2.0 cores
//   - MemoryLimit: 1Gi
//
// Example:
//
//	builder.WithProductionDefaults()
func (b *ServiceBuilder) WithProductionDefaults() *ServiceBuilder {
	return b.
		WithResyncInterval(30 * time.Second).
		WithBackOffPeriod(5 * time.Second).
		WithNamespaceIsolation().
		WithResourceLimits(2.0, "1Gi")
}

// WithConfig directly sets the configuration object.
//
// This is useful when you have a pre-built Config struct.
// Note: This replaces all previous builder settings.
//
// Example:
//
//	config := &launcher.Config{
//	    PatternsDir: "./patterns",
//	    DefaultIsolation: isolation.IsolationNamespace,
//	}
//	builder.WithConfig(config)
func (b *ServiceBuilder) WithConfig(config *Config) *ServiceBuilder {
	if b.err != nil {
		return b
	}
	if config == nil {
		b.err = fmt.Errorf("config cannot be nil")
		return b
	}
	b.config = config
	return b
}

// Build creates and initializes the launcher Service.
//
// Returns an error if:
//   - Any builder configuration was invalid
//   - Pattern discovery fails
//   - Service initialization fails
//
// Example:
//
//	service, err := launcher.NewBuilder().
//	    WithPatternsDir("./patterns").
//	    WithNamespaceIsolation().
//	    Build()
//	if err != nil {
//	    log.Fatalf("Failed to build launcher: %v", err)
//	}
//	defer service.Shutdown(context.Background())
func (b *ServiceBuilder) Build() (*Service, error) {
	// Return any accumulated errors
	if b.err != nil {
		return nil, fmt.Errorf("builder validation failed: %w", b.err)
	}

	// Validate final configuration
	if err := b.validateConfig(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create service
	service, err := NewService(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	return service, nil
}

// MustBuild creates the launcher Service and panics on error.
//
// This is useful for simple applications where startup failure
// is unrecoverable.
//
// Example:
//
//	service := launcher.NewBuilder().
//	    WithPatternsDir("./patterns").
//	    WithNamespaceIsolation().
//	    MustBuild()
//	defer service.Shutdown(context.Background())
func (b *ServiceBuilder) MustBuild() *Service {
	service, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build launcher service: %v", err))
	}
	return service
}

// GetConfig returns the current configuration without building the service.
//
// This is useful for inspecting the configuration or passing it to
// other components.
//
// Example:
//
//	config := launcher.NewBuilder().
//	    WithPatternsDir("./patterns").
//	    GetConfig()
func (b *ServiceBuilder) GetConfig() *Config {
	return b.config
}

// validateConfig performs final validation before building the service.
func (b *ServiceBuilder) validateConfig() error {
	if b.config.PatternsDir == "" {
		return fmt.Errorf("patterns directory is required")
	}

	if b.config.ResyncInterval < time.Second {
		return fmt.Errorf("resync interval must be at least 1 second")
	}

	if b.config.BackOffPeriod < 0 {
		return fmt.Errorf("backoff period cannot be negative")
	}

	if b.config.CPULimit <= 0 {
		return fmt.Errorf("CPU limit must be positive")
	}

	if b.config.MemoryLimit == "" {
		return fmt.Errorf("memory limit is required")
	}

	return nil
}

// QuickStart creates a launcher service with minimal configuration.
//
// This is a convenience function for simple use cases where you just
// need a working launcher with default settings.
//
// Example:
//
//	service, err := launcher.QuickStart("./patterns")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer service.Shutdown(context.Background())
func QuickStart(patternsDir string) (*Service, error) {
	return NewBuilder().
		WithPatternsDir(patternsDir).
		WithNamespaceIsolation().
		Build()
}

// MustQuickStart creates a launcher service with minimal configuration
// and panics on error.
//
// Example:
//
//	service := launcher.MustQuickStart("./patterns")
//	defer service.Shutdown(context.Background())
func MustQuickStart(patternsDir string) *Service {
	return NewBuilder().
		WithPatternsDir(patternsDir).
		WithNamespaceIsolation().
		MustBuild()
}
