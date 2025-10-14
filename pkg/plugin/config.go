package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main plugin configuration
// See also: plugin.go, backend.go, consumer.go for specific config types
type Config struct {
	Plugin       PluginConfig              `yaml:"plugin"`
	ControlPlane ControlPlaneConfig        `yaml:"control_plane"`
	Backend      map[string]any            `yaml:"backend"`  // Legacy: single backend config
	Backends     map[string]*BackendConfig `yaml:"backends"` // Named backends registry
	Consumer     *ConsumerConfig           `yaml:"consumer"` // Consumer pattern config
}

// LoadConfig loads configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	if config.ControlPlane.Port == 0 {
		config.ControlPlane.Port = 9090
	}

	return &config, nil
}

// GetBackendConfig extracts backend-specific configuration (legacy single backend)
func (c *Config) GetBackendConfig(target interface{}) error {
	// Marshal backend config back to YAML
	data, err := yaml.Marshal(c.Backend)
	if err != nil {
		return fmt.Errorf("failed to marshal backend config: %w", err)
	}

	// Unmarshal into target struct
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal backend config: %w", err)
	}

	return nil
}
