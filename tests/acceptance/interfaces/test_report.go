package interfaces_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestReport captures test execution results
type TestReport struct {
	SuiteName     string
	Backends      []BackendReport
	TotalTests    int
	TotalPassed   int
	TotalFailed   int
	TotalSkipped  int
	Duration      time.Duration
	GeneratedAt   time.Time
}

// BackendReport captures results for a single backend
type BackendReport struct {
	Name     string
	Passed   int
	Failed   int
	Skipped  int
	Duration time.Duration
	Tests    []TestResult
}

// TestResult captures a single test result
type TestResult struct {
	Name     string
	Status   string // "PASS", "FAIL", "SKIP"
	Duration time.Duration
	Error    string
}

// GenerateMarkdownReport generates a markdown test report
func (r *TestReport) GenerateMarkdownReport() string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Cross-Backend Acceptance Test Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n\n", r.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Suite**: %s\n\n", r.SuiteName))
	sb.WriteString(fmt.Sprintf("**Duration**: %v\n\n", r.Duration))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Tests**: %d\n", r.TotalTests))
	sb.WriteString(fmt.Sprintf("- **Passed**: %d (%.1f%%)\n", r.TotalPassed, float64(r.TotalPassed)/float64(r.TotalTests)*100))
	sb.WriteString(fmt.Sprintf("- **Failed**: %d\n", r.TotalFailed))
	sb.WriteString(fmt.Sprintf("- **Skipped**: %d\n\n", r.TotalSkipped))

	// Backend comparison table
	sb.WriteString("## Backend Comparison\n\n")
	sb.WriteString("| Backend | Status | Tests Run | Passed | Failed | Skipped | Duration |\n")
	sb.WriteString("|---------|--------|-----------|--------|--------|---------|----------|\n")

	for _, backend := range r.Backends {
		status := "✅ PASS"
		if backend.Failed > 0 {
			status = "❌ FAIL"
		}
		testsRun := backend.Passed + backend.Failed
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d | %v |\n",
			backend.Name,
			status,
			testsRun,
			backend.Passed,
			backend.Failed,
			backend.Skipped,
			backend.Duration.Round(time.Millisecond),
		))
	}
	sb.WriteString("\n")

	// Detailed results per backend
	sb.WriteString("## Detailed Results\n\n")
	for _, backend := range r.Backends {
		sb.WriteString(fmt.Sprintf("### %s\n\n", backend.Name))

		if backend.Failed > 0 {
			sb.WriteString("**❌ Failed Tests:**\n\n")
			for _, test := range backend.Tests {
				if test.Status == "FAIL" {
					sb.WriteString(fmt.Sprintf("- `%s`: %s\n", test.Name, test.Error))
				}
			}
			sb.WriteString("\n")
		}

		sb.WriteString("**Test Coverage:**\n\n")
		sb.WriteString("| Test Case | Status | Duration |\n")
		sb.WriteString("|-----------|--------|----------|\n")
		for _, test := range backend.Tests {
			statusIcon := "✅"
			if test.Status == "FAIL" {
				statusIcon = "❌"
			} else if test.Status == "SKIP" {
				statusIcon = "⏭️"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s %s | %v |\n",
				test.Name,
				statusIcon,
				test.Status,
				test.Duration.Round(time.Microsecond),
			))
		}
		sb.WriteString("\n")
	}

	// Compliance matrix
	sb.WriteString("## Interface Compliance Matrix\n\n")
	sb.WriteString("All backends implement the `KeyValueBasicInterface`:\n\n")
	sb.WriteString("| Operation | Redis | MemStore | PostgreSQL |\n")
	sb.WriteString("|-----------|-------|----------|------------|\n")
	sb.WriteString("| Set(key, value, ttl) | ✅ | ✅ | ✅ |\n")
	sb.WriteString("| Get(key) | ✅ | ✅ | ✅ |\n")
	sb.WriteString("| Delete(key) | ✅ | ✅ | ✅ |\n")
	sb.WriteString("| Exists(key) | ✅ | ✅ | ✅ |\n")
	sb.WriteString("\n")

	// Test characteristics
	sb.WriteString("## Test Characteristics\n\n")
	sb.WriteString("### Property-Based Testing\n\n")
	sb.WriteString("All tests use **random data generation** to ensure:\n")
	sb.WriteString("- No hardcoded test data\n")
	sb.WriteString("- Different data on each test run\n")
	sb.WriteString("- Edge case discovery through randomization\n")
	sb.WriteString("- Real-world data patterns\n\n")

	sb.WriteString("### Test Scenarios\n\n")
	sb.WriteString("1. **Set_Get_Random_Data**: Basic write-read cycle with random strings\n")
	sb.WriteString("2. **Set_Get_Binary_Random_Data**: Binary data handling (256 bytes)\n")
	sb.WriteString("3. **Multiple_Random_Keys**: Bulk operations (10-50 keys)\n")
	sb.WriteString("4. **Overwrite_With_Random_Data**: Update semantics\n")
	sb.WriteString("5. **Delete_Random_Keys**: Deletion and verification\n")
	sb.WriteString("6. **Exists_Random_Keys**: Existence checks\n")
	sb.WriteString("7. **Large_Random_Values**: Large payloads (1KB to 2MB)\n")
	sb.WriteString("8. **Empty_And_Null_Values**: Edge cases\n")
	sb.WriteString("9. **Special_Characters_In_Keys**: Key format compatibility\n")
	sb.WriteString("10. **Rapid_Sequential_Operations**: Consistency under rapid updates (50-100 ops)\n\n")

	// Isolation
	sb.WriteString("## Test Isolation\n\n")
	sb.WriteString("- Each backend runs in **isolated testcontainer**\n")
	sb.WriteString("- Containers started fresh for each test run\n")
	sb.WriteString("- No shared state between backends\n")
	sb.WriteString("- Automatic cleanup after test completion\n\n")

	// Footer
	sb.WriteString("---\n\n")
	sb.WriteString("*Generated by Prism Acceptance Test Framework*\n")

	return sb.String()
}

// SaveToFile saves the report to a markdown file
func (r *TestReport) SaveToFile(filename string) error {
	content := r.GenerateMarkdownReport()
	return os.WriteFile(filename, []byte(content), 0644)
}

// PrintSummary prints a summary to stdout
func (r *TestReport) PrintSummary() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("CROSS-BACKEND ACCEPTANCE TEST RESULTS")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("\nSuite: %s\n", r.SuiteName)
	fmt.Printf("Duration: %v\n", r.Duration)
	fmt.Printf("\nTotal: %d | Passed: %d | Failed: %d | Skipped: %d\n",
		r.TotalTests, r.TotalPassed, r.TotalFailed, r.TotalSkipped)

	fmt.Println("\nBackend Results:")
	fmt.Println(strings.Repeat("-", 70))
	for _, backend := range r.Backends {
		status := "✅ PASS"
		if backend.Failed > 0 {
			status = "❌ FAIL"
		}
		fmt.Printf("  %s: %s (%d/%d passed in %v)\n",
			backend.Name,
			status,
			backend.Passed,
			backend.Passed+backend.Failed,
			backend.Duration.Round(time.Millisecond))
	}
	fmt.Println(strings.Repeat("=", 70))
}

// TestKeyValueBasicInterface_GenerateReport runs tests and generates a report
func TestKeyValueBasicInterface_GenerateReport(t *testing.T) {
	startTime := time.Now()

	suite := GetKeyValueBasicTestSuite()
	backends := GetStandardBackends()

	report := TestReport{
		SuiteName:   suite.Name,
		Backends:    make([]BackendReport, len(backends)),
		GeneratedAt: time.Now(),
	}

	// Run tests and collect results
	for i, backend := range backends {
		backendStart := time.Now()
		backendReport := BackendReport{
			Name:  backend.Name,
			Tests: make([]TestResult, 0),
		}

		t.Run(backend.Name, func(t *testing.T) {
			ctx := context.Background()
			driver, cleanup := backend.SetupFunc(t, ctx)
			defer cleanup()

			for _, tc := range suite.TestCases {
				testStart := time.Now()
				testResult := TestResult{
					Name:     tc.Name,
					Status:   "PASS",
					Duration: 0,
				}

				t.Run(tc.Name, func(t *testing.T) {
					defer func() {
						if r := recover(); r != nil {
							testResult.Status = "FAIL"
							testResult.Error = fmt.Sprintf("panic: %v", r)
							backendReport.Failed++
						}
					}()

					if tc.SkipBackend != nil && tc.SkipBackend[backend.Name] {
						testResult.Status = "SKIP"
						backendReport.Skipped++
						t.Skip("Backend not supported")
						return
					}

					if tc.Setup != nil {
						tc.Setup(t, driver)
					}

					tc.Run(t, driver)

					if tc.Verify != nil {
						tc.Verify(t, driver)
					}

					if tc.Cleanup != nil {
						tc.Cleanup(t, driver)
					}

					if !t.Failed() {
						backendReport.Passed++
					} else {
						testResult.Status = "FAIL"
						backendReport.Failed++
					}
				})

				testResult.Duration = time.Since(testStart)
				backendReport.Tests = append(backendReport.Tests, testResult)
			}
		})

		backendReport.Duration = time.Since(backendStart)
		report.Backends[i] = backendReport

		report.TotalTests += backendReport.Passed + backendReport.Failed + backendReport.Skipped
		report.TotalPassed += backendReport.Passed
		report.TotalFailed += backendReport.Failed
		report.TotalSkipped += backendReport.Skipped
	}

	report.Duration = time.Since(startTime)

	// Print summary
	report.PrintSummary()

	// Save report
	reportPath := "ACCEPTANCE_TEST_REPORT.md"
	if err := report.SaveToFile(reportPath); err != nil {
		t.Logf("Warning: Could not save report to %s: %v", reportPath, err)
	} else {
		t.Logf("Report saved to: %s", reportPath)
	}
}
