package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest defines the declarative configuration for a pattern
type Manifest struct {
	// Name of the pattern (e.g., "consumer", "producer")
	Name string `yaml:"name" validate:"required"`

	// Version of the pattern
	Version string `yaml:"version" validate:"required"`

	// Path to executable binary (relative to manifest file)
	Executable string `yaml:"executable" validate:"required"`

	// Default isolation level (can be overridden at launch time)
	IsolationLevel string `yaml:"isolation_level" validate:"required,oneof=none namespace session"`

	// Health check configuration
	HealthCheck HealthCheckConfig `yaml:"healthcheck"`

	// Resource limits
	Resources ResourceConfig `yaml:"resources"`

	// Backend slots required by this pattern
	BackendSlots []BackendSlot `yaml:"backend_slots"`

	// Environment variables for the pattern process
	Environment map[string]string `yaml:"environment"`

	// Optional: Description of the pattern
	Description string `yaml:"description"`

	// Optional: Author/maintainer
	Author string `yaml:"author"`

	// Internal: Absolute path to manifest file (populated during load)
	manifestPath string `yaml:"-"`
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	// HTTP port for health endpoint
	Port int `yaml:"port" validate:"required,min=1,max=65535"`

	// HTTP path for health endpoint
	Path string `yaml:"path" validate:"required"`

	// Interval between health checks
	Interval time.Duration `yaml:"interval" validate:"required"`

	// Timeout for health check request
	Timeout time.Duration `yaml:"timeout"`

	// Number of consecutive failures before marking unhealthy
	FailureThreshold int `yaml:"failure_threshold"`
}

// ResourceConfig defines resource limits
type ResourceConfig struct {
	// CPU limit (cores, e.g., 1.0 = 1 core)
	CPULimit float64 `yaml:"cpu_limit"`

	// Memory limit (e.g., "512Mi", "1Gi")
	MemoryLimit string `yaml:"memory_limit"`

	// Optional: Minimum CPU reservation
	CPURequest float64 `yaml:"cpu_request"`

	// Optional: Minimum memory reservation
	MemoryRequest string `yaml:"memory_request"`
}

// BackendSlot defines a required backend dependency
type BackendSlot struct {
	// Name of the slot (e.g., "storage", "messaging")
	Name string `yaml:"name" validate:"required"`

	// Type of backend (e.g., "postgres", "kafka", "redis")
	Type string `yaml:"type" validate:"required"`

	// Whether this slot is required
	Required bool `yaml:"required"`

	// Optional: Configuration schema for this slot
	ConfigSchema map[string]interface{} `yaml:"config_schema"`
}

// LoadManifest loads a manifest from a YAML file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Store absolute path to manifest
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve manifest path: %w", err)
	}
	manifest.manifestPath = absPath

	// Validate manifest
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return &manifest, nil
}

// Validate checks if the manifest is valid
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}

	if m.Version == "" {
		return fmt.Errorf("version is required")
	}

	if m.Executable == "" {
		return fmt.Errorf("executable is required")
	}

	// Validate isolation level
	switch m.IsolationLevel {
	case "none", "namespace", "session":
		// Valid
	default:
		return fmt.Errorf("invalid isolation_level: %s (must be none, namespace, or session)", m.IsolationLevel)
	}

	// Check if executable exists (relative to manifest directory)
	execPath := m.ExecutablePath()
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("executable not found: %s: %w", execPath, err)
	}

	// Check if executable is executable
	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("executable is not executable: %s (mode: %s)", execPath, info.Mode())
	}

	// Validate health check
	if m.HealthCheck.Port < 1 || m.HealthCheck.Port > 65535 {
		return fmt.Errorf("healthcheck.port must be between 1 and 65535, got: %d", m.HealthCheck.Port)
	}

	if m.HealthCheck.Path == "" {
		return fmt.Errorf("healthcheck.path is required")
	}

	if m.HealthCheck.Interval == 0 {
		// Default to 30 seconds
		m.HealthCheck.Interval = 30 * time.Second
	}

	if m.HealthCheck.Timeout == 0 {
		// Default to 5 seconds
		m.HealthCheck.Timeout = 5 * time.Second
	}

	if m.HealthCheck.FailureThreshold == 0 {
		// Default to 3 failures
		m.HealthCheck.FailureThreshold = 3
	}

	return nil
}

// ExecutablePath returns the absolute path to the executable
func (m *Manifest) ExecutablePath() string {
	if filepath.IsAbs(m.Executable) {
		return m.Executable
	}

	// Resolve relative to manifest directory
	manifestDir := filepath.Dir(m.manifestPath)
	return filepath.Join(manifestDir, m.Executable)
}

// ManifestPath returns the absolute path to the manifest file
func (m *Manifest) ManifestPath() string {
	return m.manifestPath
}

// IsolationLevelProto converts string isolation level to protobuf enum
func (m *Manifest) IsolationLevelProto() int32 {
	switch m.IsolationLevel {
	case "none":
		return 0 // ISOLATION_NONE
	case "namespace":
		return 1 // ISOLATION_NAMESPACE
	case "session":
		return 2 // ISOLATION_SESSION
	default:
		return 0
	}
}
