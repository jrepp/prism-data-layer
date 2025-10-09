package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents plugin configuration
type Config struct {
	Plugin       PluginConfig       `yaml:"plugin"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	Backend      map[string]any     `yaml:"backend"` // Backend-specific config
}

// PluginConfig contains plugin metadata
type PluginConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// ControlPlaneConfig contains control plane settings
type ControlPlaneConfig struct {
	Port int `yaml:"port"`
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

// GetBackendConfig extracts backend-specific configuration
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
