package common

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/stretchr/testify/require"
)

// PatternHarness provides common test utilities for pattern testing
type PatternHarness struct {
	t       *testing.T
	plugin  core.Plugin
	ctx     context.Context
	cancel  context.CancelFunc
	cleanup []func()
}

// NewPatternHarness creates a new test harness for a pattern
func NewPatternHarness(t *testing.T, plugin core.Plugin, config *core.Config) *PatternHarness {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	h := &PatternHarness{
		t:       t,
		plugin:  plugin,
		ctx:     ctx,
		cancel:  cancel,
		cleanup: []func(){},
	}

	// Initialize plugin
	err := plugin.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize plugin")

	// Start plugin
	err = plugin.Start(ctx)
	require.NoError(t, err, "Failed to start plugin")

	// Register cleanup
	h.AddCleanup(func() {
		_ = plugin.Stop(ctx)
	})

	return h
}

// AddCleanup registers a cleanup function
func (h *PatternHarness) AddCleanup(fn func()) {
	h.cleanup = append(h.cleanup, fn)
}

// Cleanup runs all registered cleanup functions
func (h *PatternHarness) Cleanup() {
	h.t.Helper()

	// Run cleanup functions in reverse order
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}

	h.cancel()
}

// Plugin returns the pattern plugin
func (h *PatternHarness) Plugin() core.Plugin {
	return h.plugin
}

// Context returns the test context
func (h *PatternHarness) Context() context.Context {
	return h.ctx
}

// WaitForHealthy polls the plugin until it reports healthy status
func (h *PatternHarness) WaitForHealthy(timeout time.Duration) error {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(h.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus *core.HealthStatus
	var lastErr error
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			// Provide detailed error message with last known state
			if lastErr != nil {
				return &HealthCheckError{
					Timeout:  timeout,
					Attempts: attempts,
					LastErr:  lastErr,
				}
			}
			if lastStatus != nil {
				return &HealthCheckError{
					Timeout:    timeout,
					Attempts:   attempts,
					LastStatus: lastStatus,
				}
			}
			return &HealthCheckError{
				Timeout:  timeout,
				Attempts: attempts,
			}
		case <-ticker.C:
			attempts++
			status, err := h.plugin.Health(h.ctx)
			if err != nil {
				lastErr = err
				continue
			}
			lastStatus = status
			if status.Status == core.HealthHealthy {
				return nil
			}
		}
	}
}

// HealthCheckError provides detailed information about health check failures
type HealthCheckError struct {
	Timeout    time.Duration
	Attempts   int
	LastStatus *core.HealthStatus
	LastErr    error
}

func (e *HealthCheckError) Error() string {
	if e.LastErr != nil {
		return fmt.Sprintf("plugin did not become healthy after %v (%d attempts): last error: %v",
			e.Timeout, e.Attempts, e.LastErr)
	}
	if e.LastStatus != nil {
		return fmt.Sprintf("plugin did not become healthy after %v (%d attempts): last status: %s, message: %s",
			e.Timeout, e.Attempts, e.LastStatus.Status, e.LastStatus.Message)
	}
	return fmt.Sprintf("plugin did not become healthy after %v (%d attempts): no health response",
		e.Timeout, e.Attempts)
}

// AssertHealthy checks that the plugin is healthy
func (h *PatternHarness) AssertHealthy() {
	h.t.Helper()

	status, err := h.plugin.Health(h.ctx)
	require.NoError(h.t, err, "Health check failed")
	require.Equal(h.t, core.HealthHealthy, status.Status, "Plugin not healthy: %s", status.Message)
}
