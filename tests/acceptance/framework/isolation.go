package framework

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	"github.com/jrepp/prism/pkg/procmgr"
)

// IsolationConfig defines isolation requirements for pattern tests
type IsolationConfig struct {
	// Level defines the isolation strategy
	Level isolation.IsolationLevel

	// Namespace identifies the tenant/namespace
	Namespace string

	// Session identifies the session (for session isolation)
	Session string

	// GracePeriodSec is the termination grace period
	GracePeriodSec int64

	// ProcessOptions are passed to procmgr
	ProcessOptions []procmgr.Option
}

// DefaultIsolationConfig returns default isolation settings (no isolation)
func DefaultIsolationConfig() IsolationConfig {
	return IsolationConfig{
		Level:          isolation.IsolationNone,
		Namespace:      "default",
		Session:        "default",
		GracePeriodSec: 10,
		ProcessOptions: []procmgr.Option{
			procmgr.WithResyncInterval(30 * time.Second),
			procmgr.WithBackOffPeriod(5 * time.Second),
		},
	}
}

// IsolatedBackend wraps a Backend with isolation management
type IsolatedBackend struct {
	Backend
	isolationMgr *isolation.IsolationManager
	config       IsolationConfig
}

// NewIsolatedBackend creates a backend wrapper with isolation support
func NewIsolatedBackend(backend Backend, syncer isolation.ProcessSyncer, config IsolationConfig) *IsolatedBackend {
	return &IsolatedBackend{
		Backend:      backend,
		isolationMgr: isolation.NewIsolationManager(config.Level, syncer, config.ProcessOptions...),
		config:       config,
	}
}

// SetupIsolated wraps the backend's SetupFunc to use isolated processes
func (ib *IsolatedBackend) SetupIsolated(t *testing.T, ctx context.Context, key isolation.IsolationKey) (driver interface{}, cleanup func()) {
	// Create process config
	processConfig := &isolation.ProcessConfig{
		Key:            key,
		BackendConfig:  nil, // Backend-specific config can be passed here
		GracePeriodSec: ib.config.GracePeriodSec,
	}

	// Get or create isolated process
	handle, err := ib.isolationMgr.GetOrCreateProcess(ctx, key, processConfig)
	if err != nil {
		t.Fatalf("Failed to create isolated process: %v", err)
	}

	t.Logf("Using isolated process: %s (level: %s)", handle.ID, ib.config.Level)

	// Call original setup function
	driver, originalCleanup := ib.Backend.SetupFunc(t, ctx)

	// Return wrapped cleanup that terminates the isolated process
	cleanup = func() {
		if originalCleanup != nil {
			originalCleanup()
		}

		// Terminate the isolated process
		termCtx, cancel := context.WithTimeout(context.Background(), time.Duration(ib.config.GracePeriodSec+5)*time.Second)
		defer cancel()

		if err := ib.isolationMgr.TerminateProcess(termCtx, key, ib.config.GracePeriodSec); err != nil {
			t.Logf("Warning: Failed to terminate process %s: %v", handle.ID, err)
		}
	}

	return driver, cleanup
}

// Shutdown gracefully shuts down all isolated processes
func (ib *IsolatedBackend) Shutdown(ctx context.Context) error {
	return ib.isolationMgr.Shutdown(ctx)
}

// Health returns isolation manager health status
func (ib *IsolatedBackend) Health() procmgr.HealthCheck {
	return ib.isolationMgr.Health()
}

// IsolatedTestOptions extends TestOptions with isolation settings
type IsolatedTestOptions struct {
	TestOptions

	// IsolationConfig defines how to isolate processes
	IsolationConfig IsolationConfig

	// NamespaceGenerator creates namespace names for tests (optional)
	// If nil, uses default namespace from config
	NamespaceGenerator func(backendName string, testName string) string

	// SessionGenerator creates session names for tests (optional)
	// If nil, uses default session from config
	SessionGenerator func(backendName string, testName string) string
}

// RunIsolatedPatternTests executes pattern tests with process isolation
func RunIsolatedPatternTests(t *testing.T, pattern Pattern, tests []PatternTest, syncer isolation.ProcessSyncer, opts IsolatedTestOptions) {
	backends := GetBackendsForPattern(pattern)

	if len(backends) == 0 {
		t.Skipf("No backends registered for pattern %s", pattern)
		return
	}

	// Create isolated backend wrappers
	isolatedBackends := make([]*IsolatedBackend, len(backends))
	for i, backend := range backends {
		isolatedBackends[i] = NewIsolatedBackend(backend, syncer, opts.IsolationConfig)
		defer isolatedBackends[i].Shutdown(context.Background())
	}

	for i, backend := range isolatedBackends {
		backend := backend // Capture
		backendName := backends[i].Name

		t.Run(backendName, func(t *testing.T) {
			if !opts.Sequential {
				t.Parallel()
			}

			// Run all tests for this backend
			for _, test := range tests {
				test := test // Capture

				t.Run(test.Name, func(t *testing.T) {
					// Check capability requirements
					if test.RequiresCapability != "" && !backend.HasCapability(test.RequiresCapability) {
						t.Skipf("Backend %s doesn't support capability: %s", backendName, test.RequiresCapability)
						return
					}

					// Generate namespace/session for this test
					namespace := opts.IsolationConfig.Namespace
					if opts.NamespaceGenerator != nil {
						namespace = opts.NamespaceGenerator(backendName, test.Name)
					}

					session := opts.IsolationConfig.Session
					if opts.SessionGenerator != nil {
						session = opts.SessionGenerator(backendName, test.Name)
					}

					key := isolation.IsolationKey{
						Namespace: namespace,
						Session:   session,
					}

					// Setup isolated backend
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					driver, cleanup := backend.SetupIsolated(t, ctx, key)
					if cleanup != nil {
						defer cleanup()
					}

					// Apply test timeout if specified
					if test.Timeout > 0 {
						testCtx, testCancel := context.WithTimeout(context.Background(), test.Timeout)
						defer testCancel()

						// Run test with timeout monitoring
						done := make(chan bool, 1)
						go func() {
							test.Func(t, driver, backend.Capabilities)
							done <- true
						}()

						select {
						case <-done:
							// Test completed
						case <-testCtx.Done():
							t.Fatalf("Test timed out after %v", test.Timeout)
						}
					} else {
						// Run test without timeout
						test.Func(t, driver, backend.Capabilities)
					}
				})
			}
		})
	}
}

// IsolationHealthReporter provides health reporting for isolated processes
type IsolationHealthReporter struct {
	backends map[string]*IsolatedBackend
}

// NewIsolationHealthReporter creates a health reporter
func NewIsolationHealthReporter() *IsolationHealthReporter {
	return &IsolationHealthReporter{
		backends: make(map[string]*IsolatedBackend),
	}
}

// Register adds an isolated backend for health monitoring
func (r *IsolationHealthReporter) Register(name string, backend *IsolatedBackend) {
	r.backends[name] = backend
}

// GetHealth returns health status for all backends
func (r *IsolationHealthReporter) GetHealth() map[string]procmgr.HealthCheck {
	health := make(map[string]procmgr.HealthCheck)
	for name, backend := range r.backends {
		health[name] = backend.Health()
	}
	return health
}

// Report generates a human-readable health report
func (r *IsolationHealthReporter) Report() string {
	report := "=== Isolation Health Report ===\n\n"

	for name, backend := range r.backends {
		health := backend.Health()
		report += fmt.Sprintf("Backend: %s\n", name)
		report += fmt.Sprintf("  Total Processes: %d\n", health.TotalProcesses)
		report += fmt.Sprintf("  Running: %d\n", health.RunningProcesses)
		report += fmt.Sprintf("  Terminating: %d\n", health.TerminatingProcesses)
		report += fmt.Sprintf("  Failed: %d\n", health.FailedProcesses)
		report += fmt.Sprintf("  Work Queue Depth: %d\n", health.WorkQueueDepth)
		report += "\n"

		for id, processHealth := range health.Processes {
			report += fmt.Sprintf("  Process %s:\n", id)
			report += fmt.Sprintf("    State: %s\n", processHealth.State)
			report += fmt.Sprintf("    Healthy: %v\n", processHealth.Healthy)
			report += fmt.Sprintf("    Uptime: %v\n", processHealth.Uptime)
			report += fmt.Sprintf("    Last Sync: %v\n", processHealth.LastSync)
			report += fmt.Sprintf("    Errors: %d\n", processHealth.ErrorCount)
			report += fmt.Sprintf("    Restarts: %d\n", processHealth.RestartCount)
			report += "\n"
		}
	}

	return report
}
