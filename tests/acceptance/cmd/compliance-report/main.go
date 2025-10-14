package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
)

// Command-line tool to generate pattern compliance reports
// Can be used locally or in CI to visualize pattern test results

func main() {
	var (
		inputFile   = flag.String("input", "", "Input JSON report file")
		outputDir   = flag.String("output", "test-logs", "Output directory for reports")
		formatFlag  = flag.String("format", "all", "Output format: markdown, json, terminal, all")
		showDetails = flag.Bool("details", false, "Show detailed test results")
	)
	flag.Parse()

	// Read input report
	if *inputFile == "" {
		log.Fatal("Error: --input flag is required")
	}

	data, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Error reading input file: %v", err)
	}

	var report framework.ComplianceReport
	if err := json.Unmarshal(data, &report); err != nil {
		log.Fatalf("Error parsing JSON report: %v", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}

	// Generate reports in requested formats
	formats := strings.Split(*formatFlag, ",")
	for _, format := range formats {
		format = strings.TrimSpace(format)

		switch format {
		case "all":
			generateMarkdownReport(&report, *outputDir, *showDetails)
			generateJSONReport(&report, *outputDir)
			generateTerminalReport(&report, *showDetails)
		case "markdown", "md":
			generateMarkdownReport(&report, *outputDir, *showDetails)
		case "json":
			generateJSONReport(&report, *outputDir)
		case "terminal", "stdout":
			generateTerminalReport(&report, *showDetails)
		default:
			log.Printf("Warning: unknown format '%s', skipping", format)
		}
	}

	fmt.Printf("\n✅ Compliance report generated in: %s\n", *outputDir)
}

func generateMarkdownReport(report *framework.ComplianceReport, outputDir string, showDetails bool) {
	filename := filepath.Join(outputDir, "pattern-compliance.md")
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Error creating markdown report: %v", err)
	}
	defer f.Close()

	// Header
	fmt.Fprintf(f, "# Pattern Compliance Report\n\n")
	fmt.Fprintf(f, "**Generated**: %s\n\n", report.Timestamp.Format(time.RFC3339))

	// Overall summary
	fmt.Fprintf(f, "## Summary\n\n")
	fmt.Fprintf(f, "| Metric | Count |\n")
	fmt.Fprintf(f, "|--------|-------|\n")
	fmt.Fprintf(f, "| Total Tests | %d |\n", report.TotalTests)
	fmt.Fprintf(f, "| Passed | %d (%.1f%%) |\n", report.PassedTests, passRate(report))
	fmt.Fprintf(f, "| Failed | %d |\n", report.FailedTests)
	fmt.Fprintf(f, "| Skipped | %d |\n\n", report.SkippedTests)

	// Backend compliance matrix
	fmt.Fprintf(f, "## Backend Compliance Matrix\n\n")
	fmt.Fprintf(f, "| Backend | Overall Score | Patterns Tested | Status |\n")
	fmt.Fprintf(f, "|---------|---------------|-----------------|--------|\n")

	backends := getSortedBackends(report)
	for _, backend := range backends {
		score := report.OverallScore(backend)
		patternCount := len(report.Results[backend])
		status := complianceStatus(score)

		fmt.Fprintf(f, "| %s | %.1f%% | %d | %s |\n", backend, score, patternCount, status)
	}
	fmt.Fprintf(f, "\n")

	// Pattern-by-pattern breakdown
	fmt.Fprintf(f, "## Pattern Compliance Details\n\n")

	for _, backend := range backends {
		fmt.Fprintf(f, "### %s Backend\n\n", backend)
		fmt.Fprintf(f, "| Pattern | Tests | Passed | Failed | Skipped | Score |\n")
		fmt.Fprintf(f, "|---------|-------|--------|--------|---------|-------|\n")

		patterns := getSortedPatterns(report, backend)
		for _, pattern := range patterns {
			results := report.Results[backend][pattern]
			passed, failed, skipped := countResults(results)
			score := report.PatternScore(backend, pattern)

			fmt.Fprintf(f, "| %s | %d | %d | %d | %d | %.1f%% |\n",
				pattern, len(results), passed, failed, skipped, score)
		}
		fmt.Fprintf(f, "\n")

		// Show detailed test results if requested
		if showDetails {
			for _, pattern := range patterns {
				results := report.Results[backend][pattern]
				fmt.Fprintf(f, "#### %s Pattern Tests\n\n", pattern)

				for _, result := range results {
					status := "✅ PASS"
					if result.Skipped {
						status = "⏭️  SKIP: " + result.SkipReason
					} else if !result.Passed {
						status = fmt.Sprintf("❌ FAIL: %v", result.Error)
					}

					fmt.Fprintf(f, "- **%s**: %s (%.2fs)\n", result.TestName, status, result.Duration.Seconds())
				}
				fmt.Fprintf(f, "\n")
			}
		}
	}

	// Multi-pattern tests (coordinated patterns like producer/consumer)
	if hasMultiPatternTests(report) {
		fmt.Fprintf(f, "## Multi-Pattern Tests\n\n")
		fmt.Fprintf(f, "Tests that coordinate multiple patterns (e.g., producer → consumer integration):\n\n")

		for _, backend := range backends {
			// Identify coordinated tests by looking for specific patterns
			producerResults := report.Results[backend][framework.PatternProducer]
			consumerResults := report.Results[backend][framework.PatternConsumer]

			if len(producerResults) > 0 && len(consumerResults) > 0 {
				fmt.Fprintf(f, "### %s: Producer + Consumer\n\n", backend)
				fmt.Fprintf(f, "| Component | Tests | Status |\n")
				fmt.Fprintf(f, "|-----------|-------|--------|\n")

				producerPassed, _, _ := countResults(producerResults)
				consumerPassed, _, _ := countResults(consumerResults)

				prodStatus := "✅ Pass"
				if producerPassed < len(producerResults) {
					prodStatus = "❌ Fail"
				}
				consStatus := "✅ Pass"
				if consumerPassed < len(consumerResults) {
					consStatus = "❌ Fail"
				}

				fmt.Fprintf(f, "| Producer | %d | %s |\n", len(producerResults), prodStatus)
				fmt.Fprintf(f, "| Consumer | %d | %s |\n\n", len(consumerResults), consStatus)
			}
		}
	}

	// Recommendations
	fmt.Fprintf(f, "## Recommendations\n\n")
	recommendations := generateRecommendations(report)
	for _, rec := range recommendations {
		fmt.Fprintf(f, "- %s\n", rec)
	}

	fmt.Printf("✅ Generated markdown report: %s\n", filename)
}

func generateJSONReport(report *framework.ComplianceReport, outputDir string) {
	filename := filepath.Join(outputDir, "pattern-compliance.json")
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Error creating JSON report: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}

	fmt.Printf("✅ Generated JSON report: %s\n", filename)
}

func generateTerminalReport(report *framework.ComplianceReport, showDetails bool) {
	fmt.Println("\n╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║         PATTERN COMPLIANCE REPORT                              ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nGenerated: %s\n\n", report.Timestamp.Format(time.RFC822))

	// Summary
	fmt.Println("SUMMARY:")
	fmt.Printf("  Total Tests:   %d\n", report.TotalTests)
	fmt.Printf("  Passed:        %d (%.1f%%)\n", report.PassedTests, passRate(report))
	fmt.Printf("  Failed:        %d\n", report.FailedTests)
	fmt.Printf("  Skipped:       %d\n\n", report.SkippedTests)

	// Compliance matrix
	fmt.Println("BACKEND COMPLIANCE MATRIX:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Backend\tOverall Score\tPatterns\tStatus")
	fmt.Fprintln(w, "-------\t-------------\t--------\t------")

	backends := getSortedBackends(report)
	for _, backend := range backends {
		score := report.OverallScore(backend)
		patternCount := len(report.Results[backend])
		status := complianceStatus(score)

		fmt.Fprintf(w, "%s\t%.1f%%\t%d\t%s\n", backend, score, patternCount, status)
	}
	w.Flush()
	fmt.Println()

	// Pattern breakdown
	if showDetails {
		for _, backend := range backends {
			fmt.Printf("\n%s BACKEND:\n", strings.ToUpper(backend))
			patterns := getSortedPatterns(report, backend)

			for _, pattern := range patterns {
				results := report.Results[backend][pattern]
				passed, failed, skipped := countResults(results)
				score := report.PatternScore(backend, pattern)

				fmt.Printf("  %s: %d tests, %d passed, %d failed, %d skipped (%.1f%%)\n",
					pattern, len(results), passed, failed, skipped, score)

				for _, result := range results {
					if result.Passed {
						fmt.Printf("    ✅ %s (%.2fs)\n", result.TestName, result.Duration.Seconds())
					} else if result.Skipped {
						fmt.Printf("    ⏭️  %s (skipped: %s)\n", result.TestName, result.SkipReason)
					} else {
						fmt.Printf("    ❌ %s: %v (%.2fs)\n", result.TestName, result.Error, result.Duration.Seconds())
					}
				}
			}
		}
	}

	fmt.Println()
}

// Helper functions

func passRate(report *framework.ComplianceReport) float64 {
	if report.TotalTests == 0 {
		return 0.0
	}
	return float64(report.PassedTests) / float64(report.TotalTests) * 100.0
}

func complianceStatus(score float64) string {
	switch {
	case score >= 95.0:
		return "🟢 Excellent"
	case score >= 85.0:
		return "🟡 Good"
	case score >= 70.0:
		return "🟠 Fair"
	default:
		return "🔴 Needs Work"
	}
}

func getSortedBackends(report *framework.ComplianceReport) []string {
	backends := make([]string, 0, len(report.Results))
	for backend := range report.Results {
		backends = append(backends, backend)
	}
	sort.Strings(backends)
	return backends
}

func getSortedPatterns(report *framework.ComplianceReport, backend string) []framework.Pattern {
	patterns := make([]framework.Pattern, 0, len(report.Results[backend]))
	for pattern := range report.Results[backend] {
		patterns = append(patterns, pattern)
	}
	sort.Slice(patterns, func(i, j int) bool {
		return string(patterns[i]) < string(patterns[j])
	})
	return patterns
}

func countResults(results []framework.TestResult) (passed, failed, skipped int) {
	for _, result := range results {
		if result.Skipped {
			skipped++
		} else if result.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

func hasMultiPatternTests(report *framework.ComplianceReport) bool {
	for _, patterns := range report.Results {
		if len(patterns) > 1 {
			return true
		}
	}
	return false
}

func generateRecommendations(report *framework.ComplianceReport) []string {
	recommendations := []string{}

	// Check for low overall pass rate
	if passRate(report) < 80.0 {
		recommendations = append(recommendations,
			fmt.Sprintf("⚠️  Overall pass rate is %.1f%% - consider investigating failing tests", passRate(report)))
	}

	// Check for backends with low compliance
	for backend, patterns := range report.Results {
		score := report.OverallScore(backend)
		if score < 70.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("🔴 %s backend has low compliance (%.1f%%) - needs attention", backend, score))
		} else if score < 85.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("🟠 %s backend compliance is %.1f%% - room for improvement", backend, score))
		}

		// Check for patterns with no tests
		for pattern := range patterns {
			results := report.Results[backend][pattern]
			if len(results) == 0 {
				recommendations = append(recommendations,
					fmt.Sprintf("⚠️  %s backend has no tests for %s pattern", backend, pattern))
			}
		}
	}

	// Check for skipped tests
	if report.SkippedTests > report.TotalTests/4 {
		recommendations = append(recommendations,
			fmt.Sprintf("⚠️  High number of skipped tests (%d) - may indicate missing capabilities or broken setup", report.SkippedTests))
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ All patterns show good compliance - keep up the great work!")
	}

	return recommendations
}
