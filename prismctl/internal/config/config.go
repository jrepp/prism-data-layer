// Package config manages prismctl configuration
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the prismctl configuration
type Config struct {
	OIDC  OIDCConfig  `mapstructure:"oidc"`
	Proxy ProxyConfig `mapstructure:"proxy"`
	Token TokenConfig `mapstructure:"token"`
}

// OIDCConfig holds OIDC provider configuration
type OIDCConfig struct {
	Issuer       string   `mapstructure:"issuer"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	Scopes       []string `mapstructure:"scopes"`
}

// ProxyConfig holds Prism proxy configuration
type ProxyConfig struct {
	URL     string `mapstructure:"url"`
	Timeout int    `mapstructure:"timeout"`
}

// TokenConfig holds token storage configuration
type TokenConfig struct {
	Path string `mapstructure:"path"`
}

// Load loads configuration from file or defaults
func Load() (*Config, error) {
	v := viper.New()

	// Set config name and paths
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("$HOME/.prism")
	v.AddConfigPath(".")

	// Environment variable overrides
	v.SetEnvPrefix("PRISM")
	v.AutomaticEnv()

	// Set defaults for local development
	v.SetDefault("oidc.issuer", "http://localhost:5556/dex")
	v.SetDefault("oidc.client_id", "prismctl")
	v.SetDefault("oidc.client_secret", "prismctl-secret")
	v.SetDefault("oidc.scopes", []string{"openid", "profile", "email", "offline_access"})
	v.SetDefault("proxy.url", "http://localhost:8080")
	v.SetDefault("proxy.timeout", 30)
	v.SetDefault("token.path", filepath.Join(os.Getenv("HOME"), ".prism", "token"))

	// Read config file (ignore if not found - use defaults)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

// EnsurePrismDir ensures the ~/.prism directory exists
func EnsurePrismDir() error {
	prismDir := filepath.Join(os.Getenv("HOME"), ".prism")
	return os.MkdirAll(prismDir, 0755)
}

// DefaultScopes returns the default OIDC scopes
func DefaultScopes() []string {
	return []string{"openid", "profile", "email", "offline_access"}
}
