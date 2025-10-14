package common

import (
	"context"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/testing/backends"
	"github.com/stretchr/testify/require"
)

// TestConfig provides fluent configuration builder for patterns
type TestConfig struct {
	name    string
	version string
	backend map[string]any
}

// NewTestConfig creates a new test configuration builder
func NewTestConfig(patternName string) *TestConfig {
	return &TestConfig{
		name:    patternName,
		version: "0.1.0",
		backend: make(map[string]any),
	}
}

// WithBackendOption adds a backend configuration option
func (tc *TestConfig) WithBackendOption(key string, value any) *TestConfig {
	tc.backend[key] = value
	return tc
}

// WithVersion sets the pattern version
func (tc *TestConfig) WithVersion(version string) *TestConfig {
	tc.version = version
	return tc
}

// Build creates the core.Config
func (tc *TestConfig) Build() *core.Config {
	return &core.Config{
		Plugin: core.PluginConfig{
			Name:    tc.name,
			Version: tc.version,
		},
		Backend: tc.backend,
	}
}

// SetupRedisTest is a convenience wrapper that sets up a Redis pattern with defaults
func SetupRedisTest(t *testing.T, plugin core.Plugin) (*PatternHarness, *backends.RedisBackend) {
	t.Helper()
	ctx := context.Background()

	backend := backends.SetupRedis(t, ctx)
	config := NewTestConfig("redis-test").
		WithBackendOption("address", backend.ConnectionString).
		Build()

	harness := NewPatternHarness(t, plugin, config)
	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	return harness, backend
}

// SetupNATSTest is a convenience wrapper that sets up a NATS pattern with defaults
func SetupNATSTest(t *testing.T, plugin core.Plugin) (*PatternHarness, *backends.NATSBackend) {
	t.Helper()
	ctx := context.Background()

	backend := backends.SetupNATS(t, ctx)
	config := NewTestConfig("nats-test").
		WithBackendOption("url", backend.ConnectionString).
		Build()

	harness := NewPatternHarness(t, plugin, config)
	err := harness.WaitForHealthy(5 * time.Second)
	require.NoError(t, err, "Plugin did not become healthy")

	return harness, backend
}

// WithCleanup is a helper that ensures cleanup is called
func WithCleanup(t *testing.T, setup func() func(), test func()) {
	t.Helper()
	cleanup := setup()
	defer cleanup()
	test()
}

// AssertHealthyWithMessage checks health and provides custom failure message
func AssertHealthyWithMessage(t *testing.T, plugin core.Plugin, ctx context.Context, message string) {
	t.Helper()
	status, err := plugin.Health(ctx)
	require.NoError(t, err, "Health check failed: %s", message)
	require.Equal(t, core.HealthHealthy, status.Status,
		"%s - Plugin status: %s, message: %s", message, status.Status, status.Message)
}

// WaitForCondition polls a condition function until it returns true or timeout
func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool, errorMsg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal(errorMsg)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}
