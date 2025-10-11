package core

import "fmt"

// BackendConfig represents a named backend configuration
// Backends can be referenced by consumers using their config path
type BackendConfig struct {
	Name             string         `yaml:"name"`              // Human-readable name
	ConfigPath       string         `yaml:"config_path"`       // Unique path identifier (e.g., /us-east-1/order-sqs)
	Type             string         `yaml:"type"`              // Backend type (nats, kafka, sqs, redis, etc.)
	ConnectionString string         `yaml:"connection_string"` // Connection details
	Config           map[string]any `yaml:"config"`            // Backend-specific configuration
}

// GetBackend retrieves a named backend by config path
func (c *Config) GetBackend(configPath string) (*BackendConfig, error) {
	if c.Backends == nil {
		return nil, fmt.Errorf("no backends configured")
	}

	backend, ok := c.Backends[configPath]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", configPath)
	}

	return backend, nil
}

// ListBackends returns all configured backend config paths
func (c *Config) ListBackends() []string {
	if c.Backends == nil {
		return nil
	}

	paths := make([]string, 0, len(c.Backends))
	for path := range c.Backends {
		paths = append(paths, path)
	}
	return paths
}
