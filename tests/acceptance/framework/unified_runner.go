package framework

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// PatternTestConfig configures how to test a pattern executable
type PatternTestConfig struct {
	// Name of the pattern (e.g., "kafka", "redis", "memstore")
	Name string

	// Address of the running pattern executable
	Address string

	// Configuration to pass to the pattern on initialization
	Config map[string]interface{}

	// Timeout for connecting and initializing
	Timeout time.Duration
}

// RunUnifiedPatternTests is the main entry point for testing a pattern executable
// It discovers interfaces and runs appropriate test suites dynamically
func RunUnifiedPatternTests(t *testing.T, config PatternTestConfig) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	// Discover the pattern's interfaces
	t.Logf("Discovering interfaces for pattern: %s at %s", config.Name, config.Address)
	pattern, err := DiscoverPatternInterfaces(ctx, config.Name, config.Address, config.Config)
	if err != nil {
		t.Fatalf("Failed to discover pattern interfaces: %v", err)
	}
	defer pattern.Cleanup()

	// Get supported interfaces
	interfaces := pattern.GetSupportedInterfaces()
	if len(interfaces) == 0 {
		t.Fatalf("Pattern %s reports no supported interfaces", config.Name)
	}

	t.Logf("Pattern %s supports %d interfaces:", config.Name, len(interfaces))
	for _, iface := range interfaces {
		t.Logf("  - %s", iface)
	}

	// Get test suites for these interfaces
	suites := GetTestSuitesForInterfaces(interfaces)
	if len(suites) == 0 {
		t.Skipf("No test suites registered for any of the %d interfaces", len(interfaces))
		return
	}

	t.Logf("Running %d test suites", len(suites))

	// Run each test suite
	for _, suite := range suites {
		suite := suite // Capture for closure

		t.Run(suite.InterfaceName, func(t *testing.T) {
			t.Logf("Testing interface: %s (%s)", suite.InterfaceName, suite.Description)

			// Run all tests in the suite
			for _, test := range suite.Tests {
				test := test // Capture

				t.Run(test.Name, func(t *testing.T) {
					// Create a test-specific context
					testCtx := context.Background()
					if test.Timeout > 0 {
						var testCancel context.CancelFunc
						testCtx, testCancel = context.WithTimeout(testCtx, test.Timeout)
						defer testCancel()
					}

					// Get driver from pattern connection
					// The test function will cast this to the appropriate interface
					driver := pattern.Connection()

					// Create minimal capabilities (tests shouldn't rely on this for interface-based testing)
					caps := Capabilities{
						Custom: map[string]interface{}{
							"pattern_name":    pattern.Name,
							"pattern_address": pattern.Address,
						},
					}

					// Run test with timeout monitoring if specified
					if test.Timeout > 0 {
						done := make(chan bool, 1)
						go func() {
							test.Func(t, driver, caps)
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
						test.Func(t, driver, caps)
					}
				})
			}
		})
	}
}

// RunUnifiedPatternTestsWithOptions provides more control over test execution
type UnifiedTestOptions struct {
	// Pattern configuration
	Config PatternTestConfig

	// Only run tests for specific interfaces (nil = all discovered interfaces)
	FilterInterfaces []string

	// Skip parallel execution
	Sequential bool

	// Stop on first failure
	FailFast bool

	// Collect detailed results
	CollectResults bool
}

// RunUnifiedPatternTestsWithOptions executes pattern tests with custom options
func RunUnifiedPatternTestsWithOptions(t *testing.T, opts UnifiedTestOptions) *ComplianceReport {
	var report *ComplianceReport
	if opts.CollectResults {
		report = &ComplianceReport{
			Timestamp: time.Now(),
			Results:   make(map[string]map[Pattern][]TestResult),
		}
	}

	if opts.Config.Timeout == 0 {
		opts.Config.Timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Config.Timeout)
	defer cancel()

	// Discover pattern interfaces
	pattern, err := DiscoverPatternInterfaces(ctx, opts.Config.Name, opts.Config.Address, opts.Config.Config)
	if err != nil {
		t.Fatalf("Failed to discover pattern interfaces: %v", err)
	}
	defer pattern.Cleanup()

	// Get supported interfaces
	interfaces := pattern.GetSupportedInterfaces()
	if len(interfaces) == 0 {
		t.Fatalf("Pattern %s reports no supported interfaces", opts.Config.Name)
	}

	// Apply interface filter if specified
	if len(opts.FilterInterfaces) > 0 {
		filtered := make([]string, 0)
		filterMap := make(map[string]bool)
		for _, iface := range opts.FilterInterfaces {
			filterMap[iface] = true
		}

		for _, iface := range interfaces {
			if filterMap[iface] {
				filtered = append(filtered, iface)
			}
		}

		interfaces = filtered
	}

	if len(interfaces) == 0 {
		t.Skipf("No interfaces match filter criteria")
		return report
	}

	// Get test suites
	suites := GetTestSuitesForInterfaces(interfaces)
	if len(suites) == 0 {
		t.Skipf("No test suites registered for specified interfaces")
		return report
	}

	// Run test suites
	for _, suite := range suites {
		suite := suite

		t.Run(suite.InterfaceName, func(t *testing.T) {
			if !opts.Sequential {
				t.Parallel()
			}

			// Initialize results for this suite
			if opts.CollectResults {
				if report.Results[opts.Config.Name] == nil {
					report.Results[opts.Config.Name] = make(map[Pattern][]TestResult)
				}
			}

			driver := pattern.Connection()
			caps := Capabilities{
				Custom: map[string]interface{}{
					"pattern_name":    pattern.Name,
					"pattern_address": pattern.Address,
				},
			}

			// Run tests
			for _, test := range suite.Tests {
				test := test

				result := TestResult{
					BackendName: opts.Config.Name,
					PatternName: suite.Pattern,
					TestName:    test.Name,
				}

				t.Run(test.Name, func(t *testing.T) {
					startTime := time.Now()

					defer func() {
						result.Duration = time.Since(startTime)

						if r := recover(); r != nil {
							result.Passed = false
							result.Error = fmt.Errorf("panic: %v", r)
							t.Errorf("Test panicked: %v", r)
						}

						if opts.CollectResults {
							if result.Error != nil || t.Failed() {
								result.Passed = false
								report.FailedTests++
							} else {
								result.Passed = true
								report.PassedTests++
							}
							report.TotalTests++
							report.Results[opts.Config.Name][suite.Pattern] = append(
								report.Results[opts.Config.Name][suite.Pattern],
								result,
							)
						}

						if opts.FailFast && (result.Error != nil || t.Failed()) {
							t.Fatalf("Stopping on first failure: %s", test.Name)
						}
					}()

					test.Func(t, driver, caps)
				})
			}
		})
	}

	// Automatically write GitHub Actions summary if report was collected
	if opts.CollectResults && report != nil {
		if err := WriteGitHubActionsSummary(report); err != nil {
			t.Logf("Warning: Failed to write GitHub Actions summary: %v", err)
		}
	}

	return report
}

// DiscoverAndPrintInterfaces is a utility function to discover and print supported interfaces
// Useful for debugging and understanding what a pattern supports
func DiscoverAndPrintInterfaces(ctx context.Context, config PatternTestConfig) error {
	pattern, err := DiscoverPatternInterfaces(ctx, config.Name, config.Address, config.Config)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}
	defer pattern.Cleanup()

	fmt.Printf("Pattern: %s\n", pattern.Metadata.Name)
	fmt.Printf("Version: %s\n", pattern.Metadata.Version)
	fmt.Printf("Interfaces: %d\n\n", len(pattern.Metadata.Interfaces))

	for i, iface := range pattern.Metadata.Interfaces {
		fmt.Printf("%d. %s\n", i+1, iface.Name)
		if iface.ProtoFile != "" {
			fmt.Printf("   Proto: %s\n", iface.ProtoFile)
		}
		if iface.Version != "" {
			fmt.Printf("   Version: %s\n", iface.Version)
		}
		if len(iface.Metadata) > 0 {
			fmt.Printf("   Metadata:\n")
			for k, v := range iface.Metadata {
				fmt.Printf("     %s: %s\n", k, v)
			}
		}
		fmt.Println()
	}

	return nil
}
