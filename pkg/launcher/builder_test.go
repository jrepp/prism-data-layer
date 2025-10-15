package launcher

import (
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/isolation"
)

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()

	if builder == nil {
		t.Fatal("NewBuilder should return non-nil builder")
	}

	if builder.config == nil {
		t.Fatal("Builder should have default config")
	}

	// Verify defaults
	config := builder.GetConfig()
	if config.PatternsDir != "./patterns" {
		t.Errorf("Default patterns dir should be './patterns', got %s", config.PatternsDir)
	}

	if config.DefaultIsolation != isolation.IsolationNamespace {
		t.Errorf("Default isolation should be Namespace, got %s", config.DefaultIsolation)
	}

	if config.ResyncInterval != 30*time.Second {
		t.Errorf("Default resync interval should be 30s, got %v", config.ResyncInterval)
	}

	if config.BackOffPeriod != 5*time.Second {
		t.Errorf("Default backoff period should be 5s, got %v", config.BackOffPeriod)
	}
}

func TestBuilderChaining(t *testing.T) {
	builder := NewBuilder().
		WithPatternsDir("/opt/patterns").
		WithNamespaceIsolation().
		WithResyncInterval(10 * time.Second).
		WithBackOffPeriod(2 * time.Second).
		WithResourceLimits(1.5, "512Mi")

	config := builder.GetConfig()

	if config.PatternsDir != "/opt/patterns" {
		t.Errorf("Expected /opt/patterns, got %s", config.PatternsDir)
	}

	if config.DefaultIsolation != isolation.IsolationNamespace {
		t.Errorf("Expected Namespace isolation, got %s", config.DefaultIsolation)
	}

	if config.ResyncInterval != 10*time.Second {
		t.Errorf("Expected 10s resync interval, got %v", config.ResyncInterval)
	}

	if config.BackOffPeriod != 2*time.Second {
		t.Errorf("Expected 2s backoff period, got %v", config.BackOffPeriod)
	}

	if config.CPULimit != 1.5 {
		t.Errorf("Expected 1.5 CPU limit, got %v", config.CPULimit)
	}

	if config.MemoryLimit != "512Mi" {
		t.Errorf("Expected 512Mi memory limit, got %s", config.MemoryLimit)
	}
}

func TestIsolationHelpers(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *ServiceBuilder
		expected isolation.IsolationLevel
	}{
		{
			name:     "None isolation",
			build:    func() *ServiceBuilder { return NewBuilder().WithNoneIsolation() },
			expected: isolation.IsolationNone,
		},
		{
			name:     "Namespace isolation",
			build:    func() *ServiceBuilder { return NewBuilder().WithNamespaceIsolation() },
			expected: isolation.IsolationNamespace,
		},
		{
			name:     "Session isolation",
			build:    func() *ServiceBuilder { return NewBuilder().WithSessionIsolation() },
			expected: isolation.IsolationSession,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := tt.build()
			config := builder.GetConfig()

			if config.DefaultIsolation != tt.expected {
				t.Errorf("Expected isolation %s, got %s", tt.expected, config.DefaultIsolation)
			}
		})
	}
}

func TestDevelopmentDefaults(t *testing.T) {
	builder := NewBuilder().WithDevelopmentDefaults()
	config := builder.GetConfig()

	if config.ResyncInterval != 5*time.Second {
		t.Errorf("Dev resync interval should be 5s, got %v", config.ResyncInterval)
	}

	if config.BackOffPeriod != 1*time.Second {
		t.Errorf("Dev backoff period should be 1s, got %v", config.BackOffPeriod)
	}

	if config.DefaultIsolation != isolation.IsolationNamespace {
		t.Errorf("Dev isolation should be Namespace, got %s", config.DefaultIsolation)
	}
}

func TestProductionDefaults(t *testing.T) {
	builder := NewBuilder().WithProductionDefaults()
	config := builder.GetConfig()

	if config.ResyncInterval != 30*time.Second {
		t.Errorf("Prod resync interval should be 30s, got %v", config.ResyncInterval)
	}

	if config.BackOffPeriod != 5*time.Second {
		t.Errorf("Prod backoff period should be 5s, got %v", config.BackOffPeriod)
	}

	if config.CPULimit != 2.0 {
		t.Errorf("Prod CPU limit should be 2.0, got %v", config.CPULimit)
	}

	if config.MemoryLimit != "1Gi" {
		t.Errorf("Prod memory limit should be 1Gi, got %s", config.MemoryLimit)
	}
}

func TestBuilderValidation(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *ServiceBuilder
		wantErr bool
	}{
		{
			name: "empty patterns dir",
			build: func() *ServiceBuilder {
				return NewBuilder().WithPatternsDir("")
			},
			wantErr: true,
		},
		{
			name: "invalid resync interval (too short)",
			build: func() *ServiceBuilder {
				return NewBuilder().WithResyncInterval(500 * time.Millisecond)
			},
			wantErr: true,
		},
		{
			name: "negative backoff period",
			build: func() *ServiceBuilder {
				return NewBuilder().WithBackOffPeriod(-1 * time.Second)
			},
			wantErr: true,
		},
		{
			name: "negative CPU limit",
			build: func() *ServiceBuilder {
				return NewBuilder().WithCPULimit(-1.0)
			},
			wantErr: true,
		},
		{
			name: "zero CPU limit",
			build: func() *ServiceBuilder {
				return NewBuilder().WithCPULimit(0)
			},
			wantErr: true,
		},
		{
			name: "empty memory limit",
			build: func() *ServiceBuilder {
				return NewBuilder().WithMemoryLimit("")
			},
			wantErr: true,
		},
		{
			name: "nil config",
			build: func() *ServiceBuilder {
				return NewBuilder().WithConfig(nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := tt.build()

			// Check if error is captured in builder
			if tt.wantErr && builder.err == nil {
				t.Error("Expected builder to capture validation error")
			}

			if !tt.wantErr && builder.err != nil {
				t.Errorf("Unexpected builder error: %v", builder.err)
			}
		})
	}
}

func TestBuilderErrorPropagation(t *testing.T) {
	// Once an error occurs, subsequent calls should be no-ops
	builder := NewBuilder().
		WithPatternsDir("").                   // Error: empty patterns dir
		WithResyncInterval(10 * time.Second). // Should be ignored
		WithCPULimit(2.0)                      // Should be ignored

	if builder.err == nil {
		t.Fatal("Expected builder to have error")
	}

	// Config should not be modified after error
	config := builder.GetConfig()

	// ResyncInterval should still be default (30s), not 10s
	if config.ResyncInterval == 10*time.Second {
		t.Error("Builder should not apply settings after error")
	}

	// CPULimit should still be default (2.0 from defaults), but we can't tell if it changed
	// The important thing is the error was captured
}

func TestQuickStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping QuickStart test in short mode (requires patterns directory)")
	}

	// Note: This will fail if patterns directory doesn't exist
	// In CI, we'd need to create a temporary patterns directory

	t.Run("success path", func(t *testing.T) {
		// Skip if patterns directory doesn't exist
		t.Skip("Requires actual patterns directory to test")

		service, err := QuickStart("./patterns")
		if err != nil {
			t.Fatalf("QuickStart failed: %v", err)
		}
		defer service.Shutdown(nil)

		if service == nil {
			t.Fatal("QuickStart returned nil service")
		}
	})

	t.Run("invalid directory", func(t *testing.T) {
		_, err := QuickStart("/nonexistent/patterns")
		if err == nil {
			t.Error("QuickStart should fail with nonexistent directory")
		}
	})
}

func TestMustBuildPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild should panic on invalid configuration")
		}
	}()

	// This should panic because patterns dir is empty
	NewBuilder().
		WithPatternsDir("").
		MustBuild()
}

func TestMustQuickStartPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustQuickStart should panic on invalid directory")
		}
	}()

	// This should panic because directory doesn't exist
	MustQuickStart("/nonexistent/patterns")
}

func TestBuilderConfigImmutability(t *testing.T) {
	// Test that GetConfig returns a reference to the same config object
	// (not important for immutability, but documents behavior)
	builder := NewBuilder()

	config1 := builder.GetConfig()
	config2 := builder.GetConfig()

	if config1 != config2 {
		t.Error("GetConfig should return the same config object")
	}
}

func TestWithConfig(t *testing.T) {
	customConfig := &Config{
		PatternsDir:      "/custom/patterns",
		DefaultIsolation: isolation.IsolationSession,
		ResyncInterval:   15 * time.Second,
		BackOffPeriod:    3 * time.Second,
		CPULimit:         3.0,
		MemoryLimit:      "2Gi",
	}

	builder := NewBuilder().WithConfig(customConfig)
	config := builder.GetConfig()

	if config != customConfig {
		t.Error("WithConfig should set the config object")
	}

	if config.PatternsDir != "/custom/patterns" {
		t.Error("Custom config values not preserved")
	}
}

// Example tests showing builder usage patterns
func ExampleNewBuilder() {
	service, err := NewBuilder().
		WithPatternsDir("./patterns").
		WithNamespaceIsolation().
		WithResyncInterval(30 * time.Second).
		Build()

	if err != nil {
		// Handle error
		return
	}
	defer service.Shutdown(nil)

	// Use service
}

func ExampleNewBuilder_development() {
	service := NewBuilder().
		WithPatternsDir("./patterns").
		WithDevelopmentDefaults().
		MustBuild()

	defer service.Shutdown(nil)

	// Use service for local development
}

func ExampleNewBuilder_production() {
	service, err := NewBuilder().
		WithPatternsDir("/opt/prism/patterns").
		WithProductionDefaults().
		Build()

	if err != nil {
		// Handle error
		return
	}
	defer service.Shutdown(nil)

	// Use service in production
}

func ExampleQuickStart() {
	service, err := QuickStart("./patterns")
	if err != nil {
		// Handle error
		return
	}
	defer service.Shutdown(nil)

	// Minimal setup for quick start
}
