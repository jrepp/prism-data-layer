package framework

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// RunPatternTests executes a suite of tests for a specific pattern against all compatible backends
// This is the main entry point for pattern test files
func RunPatternTests(t *testing.T, pattern Pattern, tests []PatternTest) {
	backends := GetBackendsForPattern(pattern)

	if len(backends) == 0 {
		t.Skipf("No backends registered for pattern %s", pattern)
		return
	}

	for _, backend := range backends {
		backend := backend // Capture for parallel execution

		t.Run(backend.Name, func(t *testing.T) {
			// Allow parallel execution across backends
			t.Parallel()

			// Setup backend
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			driver, cleanup := backend.SetupFunc(t, ctx)
			if cleanup != nil {
				defer cleanup()
			}

			// Run all tests for this backend
			for _, test := range tests {
				test := test // Capture for closure

				t.Run(test.Name, func(t *testing.T) {
					// Check capability requirements
					if test.RequiresCapability != "" && !backend.HasCapability(test.RequiresCapability) {
						t.Skipf("Backend %s doesn't support capability: %s", backend.Name, test.RequiresCapability)
						return
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

// RunPatternTestsWithOptions provides more control over test execution
type TestOptions struct {
	// Backends to test (nil = all backends for pattern)
	Backends []string

	// Skip parallel execution
	Sequential bool

	// Global timeout for all tests
	Timeout time.Duration

	// Stop on first failure
	FailFast bool

	// Collect detailed results
	CollectResults bool
}

// RunPatternTestsWithOptions executes pattern tests with custom options
func RunPatternTestsWithOptions(t *testing.T, pattern Pattern, tests []PatternTest, opts TestOptions) *ComplianceReport {
	var report *ComplianceReport
	if opts.CollectResults {
		report = &ComplianceReport{
			Timestamp: time.Now(),
			Results:   make(map[string]map[Pattern][]TestResult),
		}
	}

	// Get backends to test
	var backends []Backend
	if len(opts.Backends) > 0 {
		// Test only specified backends
		for _, name := range opts.Backends {
			if backend, ok := GetBackend(name); ok && backend.SupportsPattern(pattern) {
				backends = append(backends, backend)
			} else {
				t.Logf("Warning: Backend %s not found or doesn't support pattern %s", name, pattern)
			}
		}
	} else {
		// Test all backends for pattern
		backends = GetBackendsForPattern(pattern)
	}

	if len(backends) == 0 {
		t.Skipf("No backends available for pattern %s", pattern)
		return report
	}

	// Apply global timeout if specified
	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Execute tests for each backend
	for _, backend := range backends {
		backend := backend // Capture

		t.Run(backend.Name, func(t *testing.T) {
			if !opts.Sequential {
				t.Parallel()
			}

			// Setup backend
			driver, cleanup := backend.SetupFunc(t, ctx)
			if cleanup != nil {
				defer cleanup()
			}

			// Initialize results for this backend
			if opts.CollectResults {
				if report.Results[backend.Name] == nil {
					report.Results[backend.Name] = make(map[Pattern][]TestResult)
				}
			}

			// Run all tests
			for _, test := range tests {
				test := test // Capture

				result := TestResult{
					BackendName: backend.Name,
					PatternName: pattern,
					TestName:    test.Name,
				}

				t.Run(test.Name, func(t *testing.T) {
					startTime := time.Now()

					// Check capability
					if test.RequiresCapability != "" && !backend.HasCapability(test.RequiresCapability) {
						result.Skipped = true
						result.SkipReason = fmt.Sprintf("Missing capability: %s", test.RequiresCapability)
						t.Skipf(result.SkipReason)

						if opts.CollectResults {
							result.Duration = time.Since(startTime)
							report.Results[backend.Name][pattern] = append(report.Results[backend.Name][pattern], result)
							report.SkippedTests++
						}
						return
					}

					// Run test
					defer func() {
						result.Duration = time.Since(startTime)

						if r := recover(); r != nil {
							result.Passed = false
							result.Error = fmt.Errorf("panic: %v", r)
							t.Errorf("Test panicked: %v", r)
						}

						if opts.CollectResults {
							if !result.Skipped {
								if result.Error != nil || t.Failed() {
									result.Passed = false
									report.FailedTests++
								} else {
									result.Passed = true
									report.PassedTests++
								}
								report.TotalTests++
							}
							report.Results[backend.Name][pattern] = append(report.Results[backend.Name][pattern], result)
						}

						// Fail fast if requested
						if opts.FailFast && (result.Error != nil || t.Failed()) {
							t.Fatalf("Stopping on first failure: %s", test.Name)
						}
					}()

					test.Func(t, driver, backend.Capabilities)
				})
			}
		})
	}

	return report
}

// RunSingleBackendTests executes tests for a specific backend only
// Useful for focused testing during development
func RunSingleBackendTests(t *testing.T, backendName string, pattern Pattern, tests []PatternTest) {
	backend, ok := GetBackend(backendName)
	if !ok {
		t.Fatalf("Backend %s not registered", backendName)
	}

	if !backend.SupportsPattern(pattern) {
		t.Fatalf("Backend %s doesn't support pattern %s", backendName, pattern)
	}

	ctx := context.Background()
	driver, cleanup := backend.SetupFunc(t, ctx)
	if cleanup != nil {
		defer cleanup()
	}

	for _, test := range tests {
		test := test // Capture

		t.Run(test.Name, func(t *testing.T) {
			if test.RequiresCapability != "" && !backend.HasCapability(test.RequiresCapability) {
				t.Skipf("Backend doesn't support capability: %s", test.RequiresCapability)
				return
			}

			test.Func(t, driver, backend.Capabilities)
		})
	}
}
