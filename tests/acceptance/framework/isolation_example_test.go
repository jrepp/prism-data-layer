package framework

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrepp/prism/pkg/isolation"
	"github.com/jrepp/prism/pkg/procmgr"
)

// Example ProcessSyncer implementation for testing
type exampleBackendSyncer struct {
	startCount int
	stopCount  int
}

func (s *exampleBackendSyncer) SyncProcess(ctx context.Context, updateType procmgr.UpdateType, config interface{}) (bool, error) {
	s.startCount++
	// Simulate backend startup
	time.Sleep(10 * time.Millisecond)
	return false, nil
}

func (s *exampleBackendSyncer) SyncTerminatingProcess(ctx context.Context, config interface{}, gracePeriodSecs *int64, statusFn procmgr.ProcessStatusFunc) error {
	// Simulate graceful shutdown
	time.Sleep(10 * time.Millisecond)
	return nil
}

func (s *exampleBackendSyncer) SyncTerminatedProcess(ctx context.Context, config interface{}) error {
	s.stopCount++
	// Cleanup resources
	return nil
}

// Example: No isolation - all tests share one process
func ExampleRunIsolatedPatternTests_noIsolation() {
	// Create a mock backend
	backend := Backend{
		Name: "ExampleBackend",
		SetupFunc: func(t *testing.T, ctx context.Context) (interface{}, func()) {
			// Return mock driver
			return "mock-driver", nil
		},
		SupportedPatterns: []Pattern{PatternKeyValueBasic},
		Capabilities: Capabilities{
			SupportsTTL: true,
		},
	}

	// Register backend
	RegisterBackend(backend)
	defer func() {
		// Cleanup: unregister backend
		ClearBackends()
	}()

	// Define tests
	tests := []PatternTest{
		{
			Name: "Test1",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running Test1 with driver:", driver)
			},
		},
		{
			Name: "Test2",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running Test2 with driver:", driver)
			},
		},
	}

	// Create syncer
	syncer := &exampleBackendSyncer{}

	// Configure no isolation (all tests share one process)
	opts := IsolatedTestOptions{
		IsolationConfig: IsolationConfig{
			Level:          isolation.IsolationNone,
			Namespace:      "default",
			Session:        "default",
			GracePeriodSec: 5,
		},
	}

	// Create test instance
	t := &testing.T{}

	// Run tests with no isolation
	RunIsolatedPatternTests(t, PatternKeyValueBasic, tests, syncer, opts)

	fmt.Printf("Backend started %d times (expected: 1 for shared process)\n", syncer.startCount)
	// Output will show that only one process was created for all tests
}

// Example: Namespace isolation - each namespace gets separate process
func ExampleRunIsolatedPatternTests_namespaceIsolation() {
	backend := Backend{
		Name: "ExampleBackend",
		SetupFunc: func(t *testing.T, ctx context.Context) (interface{}, func()) {
			return "mock-driver", nil
		},
		SupportedPatterns: []Pattern{PatternKeyValueBasic},
		Capabilities:      Capabilities{},
	}

	RegisterBackend(backend)
	defer ClearBackends()

	tests := []PatternTest{
		{
			Name: "TenantA_Test",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running test for tenant A")
			},
		},
		{
			Name: "TenantB_Test",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running test for tenant B")
			},
		},
	}

	syncer := &exampleBackendSyncer{}

	// Configure namespace isolation with custom namespace generator
	opts := IsolatedTestOptions{
		IsolationConfig: IsolationConfig{
			Level:          isolation.IsolationNamespace,
			Namespace:      "default",
			GracePeriodSec: 5,
		},
		// Generate different namespace per test
		NamespaceGenerator: func(backendName, testName string) string {
			if testName == "TenantA_Test" {
				return "tenant-a"
			}
			return "tenant-b"
		},
	}

	t := &testing.T{}
	RunIsolatedPatternTests(t, PatternKeyValueBasic, tests, syncer, opts)

	fmt.Printf("Backend started %d times (expected: 2, one per namespace)\n", syncer.startCount)
	// Output will show two processes created (one for tenant-a, one for tenant-b)
}

// Example: Session isolation - each session gets separate process
func ExampleRunIsolatedPatternTests_sessionIsolation() {
	backend := Backend{
		Name: "ExampleBackend",
		SetupFunc: func(t *testing.T, ctx context.Context) (interface{}, func()) {
			return "mock-driver", nil
		},
		SupportedPatterns: []Pattern{PatternKeyValueBasic},
		Capabilities:      Capabilities{},
	}

	RegisterBackend(backend)
	defer ClearBackends()

	tests := []PatternTest{
		{
			Name: "Session1_Test",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running test for session 1")
			},
		},
		{
			Name: "Session2_Test",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running test for session 2")
			},
		},
		{
			Name: "Session1_AnotherTest",
			Func: func(t *testing.T, driver interface{}, caps Capabilities) {
				fmt.Println("Running another test for session 1 (reuses process)")
			},
		},
	}

	syncer := &exampleBackendSyncer{}

	// Configure session isolation with custom session generator
	opts := IsolatedTestOptions{
		IsolationConfig: IsolationConfig{
			Level:          isolation.IsolationSession,
			Session:        "default",
			GracePeriodSec: 5,
		},
		// Generate session ID from test name
		SessionGenerator: func(backendName, testName string) string {
			if testName == "Session1_Test" || testName == "Session1_AnotherTest" {
				return "session-1"
			}
			return "session-2"
		},
	}

	t := &testing.T{}
	RunIsolatedPatternTests(t, PatternKeyValueBasic, tests, syncer, opts)

	fmt.Printf("Backend started %d times (expected: 2, one per session)\n", syncer.startCount)
	// Output will show two processes created (session-1 and session-2)
	// Session1_Test and Session1_AnotherTest reuse the same process
}

// Example: Health monitoring for isolated processes
func ExampleIsolationHealthReporter() {
	backend := Backend{
		Name: "ExampleBackend",
		SetupFunc: func(t *testing.T, ctx context.Context) (interface{}, func()) {
			return "mock-driver", nil
		},
		SupportedPatterns: []Pattern{PatternKeyValueBasic},
	}

	syncer := &exampleBackendSyncer{}

	config := IsolationConfig{
		Level:          isolation.IsolationNamespace,
		Namespace:      "tenant-1",
		GracePeriodSec: 5,
	}

	isolated := NewIsolatedBackend(backend, syncer, config)

	// Create health reporter
	reporter := NewIsolationHealthReporter()
	reporter.Register("ExampleBackend", isolated)

	// Create some isolated processes
	ctx := context.Background()
	key1 := isolation.IsolationKey{Namespace: "tenant-1", Session: "s1"}
	key2 := isolation.IsolationKey{Namespace: "tenant-2", Session: "s2"}

	t := &testing.T{}
	isolated.SetupIsolated(t, ctx, key1)
	isolated.SetupIsolated(t, ctx, key2)

	// Wait for processes to start
	time.Sleep(100 * time.Millisecond)

	// Get health report
	report := reporter.Report()
	fmt.Println(report)
	// Output will show health status of all isolated processes

	// Cleanup
	isolated.Shutdown(ctx)
}
