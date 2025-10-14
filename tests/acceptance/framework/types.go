package framework

import (
	"context"
	"testing"
	"time"
)

// Pattern represents an interface pattern that backends can implement
type Pattern string

const (
	// KeyValue patterns
	PatternKeyValueBasic  Pattern = "KeyValueBasic"
	PatternKeyValueTTL    Pattern = "KeyValueTTL"
	PatternKeyValueScan   Pattern = "KeyValueScan"
	PatternKeyValueAtomic Pattern = "KeyValueAtomic"

	// PubSub patterns
	PatternPubSubBasic    Pattern = "PubSubBasic"
	PatternPubSubOrdering Pattern = "PubSubOrdering"
	PatternPubSubFanout   Pattern = "PubSubFanout"

	// Queue patterns (future)
	PatternQueueBasic Pattern = "QueueBasic"

	// High-level patterns (composites)
	PatternProducer Pattern = "Producer"
	PatternConsumer Pattern = "Consumer"
)

// Backend represents a testable backend implementation
type Backend struct {
	// Name is the human-readable backend name (e.g., "Redis", "PostgreSQL")
	Name string

	// SetupFunc prepares the backend for testing and returns a driver instance
	SetupFunc SetupFunc

	// TeardownFunc cleans up resources after testing (optional, can use cleanup from SetupFunc)
	TeardownFunc TeardownFunc

	// SupportedPatterns lists which patterns this backend implements
	SupportedPatterns []Pattern

	// Capabilities describes optional features this backend supports
	Capabilities Capabilities
}

// SetupFunc prepares a backend for testing
// Returns:
//   - driver: The backend driver instance (cast to appropriate interface in tests)
//   - cleanup: Function to call for cleanup (can be nil if no cleanup needed)
type SetupFunc func(t *testing.T, ctx context.Context) (driver interface{}, cleanup func())

// TeardownFunc performs final cleanup after all tests (optional)
type TeardownFunc func(t *testing.T, ctx context.Context)

// Capabilities defines optional features a backend may support
type Capabilities struct {
	// TTL support
	SupportsTTL bool

	// Scan/iteration support
	SupportsScan bool

	// Atomic operations (CAS, increment, etc.)
	SupportsAtomic bool

	// Transaction support
	SupportsTransactions bool

	// Message ordering guarantees
	SupportsOrdering bool

	// Size limits (0 = unlimited)
	MaxValueSize int64
	MaxKeySize   int

	// Performance characteristics
	ExpectedLatencyP95 time.Duration
	ExpectedThroughput float64 // ops/sec

	// Custom capabilities (backend-specific)
	Custom map[string]interface{}
}

// HasCapability checks if a specific capability is supported
func (c Capabilities) HasCapability(name string) bool {
	switch name {
	case "TTL":
		return c.SupportsTTL
	case "Scan":
		return c.SupportsScan
	case "Atomic":
		return c.SupportsAtomic
	case "Transactions":
		return c.SupportsTransactions
	case "Ordering":
		return c.SupportsOrdering
	default:
		// Check custom capabilities
		if val, ok := c.Custom[name]; ok {
			if boolVal, ok := val.(bool); ok {
				return boolVal
			}
		}
		return false
	}
}

// PatternTest represents a single test case for a pattern
type PatternTest struct {
	// Name is the test name (shown in test output)
	Name string

	// Func is the test function to execute
	Func TestFunc

	// RequiresCapability is an optional capability name that must be supported
	// If not supported, the test will be skipped with a clear message
	RequiresCapability string

	// Timeout is the maximum duration for this test (optional)
	Timeout time.Duration

	// Tags allow grouping tests (e.g., "slow", "flaky", "security")
	Tags []string
}

// TestFunc is the signature for pattern test functions
// Parameters:
//   - t: Standard Go testing.T
//   - driver: Backend driver (cast to appropriate interface)
//   - caps: Backend capabilities (for conditional logic)
type TestFunc func(t *testing.T, driver interface{}, caps Capabilities)

// MultiPatternTestFunc is the signature for multi-pattern test functions
// Parameters:
//   - t: Standard Go testing.T
//   - drivers: Map of pattern name to driver instances
//   - caps: Backend capabilities (for conditional logic)
type MultiPatternTestFunc func(t *testing.T, drivers map[string]interface{}, caps Capabilities)

// MultiPatternTest represents a test case that coordinates multiple patterns
type MultiPatternTest struct {
	// Name is the test name (shown in test output)
	Name string

	// Func is the multi-pattern test function to execute
	Func MultiPatternTestFunc

	// RequiredPatterns maps pattern names to their setup requirements
	// Example: {"producer": "Producer", "consumer": "Consumer"}
	RequiredPatterns map[string]Pattern

	// RequiresCapability is an optional capability name that must be supported
	RequiresCapability string

	// Timeout is the maximum duration for this test (optional)
	Timeout time.Duration

	// Tags allow grouping tests (e.g., "slow", "integration", "end-to-end")
	Tags []string
}

// TestResult captures the outcome of a single test execution
type TestResult struct {
	BackendName string
	PatternName Pattern
	TestName    string
	Passed      bool
	Skipped     bool
	SkipReason  string
	Error       error
	Duration    time.Duration
	Output      string
}

// BenchmarkResult captures performance metrics from a benchmark
type BenchmarkResult struct {
	BackendName     string
	PatternName     Pattern
	BenchmarkName   string
	OpsPerSec       float64
	P50Latency      time.Duration
	P95Latency      time.Duration
	P99Latency      time.Duration
	ErrorRate       float64
	MemoryAllocated int64
	Duration        time.Duration
}

// ComplianceReport summarizes test results across backends and patterns
type ComplianceReport struct {
	Timestamp   time.Time
	TotalTests  int
	PassedTests int
	FailedTests int
	SkippedTests int

	// Results grouped by backend and pattern
	Results map[string]map[Pattern][]TestResult

	// Performance benchmarks
	Benchmarks []BenchmarkResult
}

// PatternScore calculates the compliance score for a backend/pattern combination
func (r *ComplianceReport) PatternScore(backend string, pattern Pattern) float64 {
	results, ok := r.Results[backend][pattern]
	if !ok || len(results) == 0 {
		return 0.0
	}

	passed := 0
	total := 0
	for _, result := range results {
		if !result.Skipped {
			total++
			if result.Passed {
				passed++
			}
		}
	}

	if total == 0 {
		return 0.0
	}

	return float64(passed) / float64(total) * 100.0
}

// OverallScore calculates the overall compliance score for a backend
func (r *ComplianceReport) OverallScore(backend string) float64 {
	patterns := r.Results[backend]
	if len(patterns) == 0 {
		return 0.0
	}

	totalScore := 0.0
	count := 0

	for pattern := range patterns {
		score := r.PatternScore(backend, pattern)
		if score > 0 {
			totalScore += score
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return totalScore / float64(count)
}
