package framework

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// ReportFormat defines the output format for reports
type ReportFormat string

const (
	FormatMarkdown ReportFormat = "markdown"
	FormatJSON     ReportFormat = "json"
	FormatText     ReportFormat = "text"
)

// GenerateComplianceMatrix creates a visual matrix showing backend compliance across patterns
func GenerateComplianceMatrix(report *ComplianceReport, w io.Writer, format ReportFormat) error {
	switch format {
	case FormatMarkdown:
		return generateMarkdownMatrix(report, w)
	case FormatText:
		return generateTextMatrix(report, w)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func generateMarkdownMatrix(report *ComplianceReport, w io.Writer) error {
	// Header
	fmt.Fprintf(w, "# Prism Backend Compliance Matrix\n\n")
	fmt.Fprintf(w, "Generated: %s\n\n", report.Timestamp.Format(time.RFC1123))

	// Summary statistics
	fmt.Fprintf(w, "## Summary\n\n")
	fmt.Fprintf(w, "- **Total Tests:** %d\n", report.TotalTests)
	fmt.Fprintf(w, "- **Passed:** %d (%.1f%%)\n", report.PassedTests, float64(report.PassedTests)/float64(report.TotalTests)*100)
	fmt.Fprintf(w, "- **Failed:** %d (%.1f%%)\n", report.FailedTests, float64(report.FailedTests)/float64(report.TotalTests)*100)
	fmt.Fprintf(w, "- **Skipped:** %d (%.1f%%)\n\n", report.SkippedTests, float64(report.SkippedTests)/float64(report.TotalTests)*100)

	// Get all backends and patterns
	backends := make([]string, 0, len(report.Results))
	for backend := range report.Results {
		backends = append(backends, backend)
	}
	sort.Strings(backends)

	// Group patterns by category
	patternsByCategory := groupPatternsByCategory(report)

	// Generate table for each pattern category
	for category, patterns := range patternsByCategory {
		fmt.Fprintf(w, "## %s Patterns\n\n", category)

		// Table header
		fmt.Fprintf(w, "| Backend | ")
		for _, pattern := range patterns {
			fmt.Fprintf(w, "%s | ", patternShortName(pattern))
		}
		fmt.Fprintf(w, "Overall |\n")

		// Table separator
		fmt.Fprintf(w, "|---------|")
		for range patterns {
			fmt.Fprintf(w, "------|")
		}
		fmt.Fprintf(w, "--------|\n")

		// Table rows
		for _, backend := range backends {
			fmt.Fprintf(w, "| **%s** | ", backend)

			for _, pattern := range patterns {
				results, ok := report.Results[backend][pattern]
				if !ok || len(results) == 0 {
					fmt.Fprintf(w, "❌ N/A | ")
					continue
				}

				passed := 0
				total := 0
				for _, r := range results {
					if !r.Skipped {
						total++
						if r.Passed {
							passed++
						}
					}
				}

				if total == 0 {
					fmt.Fprintf(w, "⚠️  0/0 | ")
				} else {
					score := float64(passed) / float64(total) * 100
					icon := getScoreIcon(score)
					fmt.Fprintf(w, "%s %d/%d | ", icon, passed, total)
				}
			}

			// Overall score
			overallScore := report.OverallScore(backend)
			icon := getScoreIcon(overallScore)
			fmt.Fprintf(w, "%s %.0f%% |\n", icon, overallScore)
		}

		fmt.Fprintf(w, "\n")
	}

	// Legend
	fmt.Fprintf(w, "**Legend:**\n")
	fmt.Fprintf(w, "- ✅ All tests passing (100%%)\n")
	fmt.Fprintf(w, "- 🟢 Most tests passing (≥90%%)\n")
	fmt.Fprintf(w, "- 🟡 Some tests passing (≥50%%)\n")
	fmt.Fprintf(w, "- 🟠 Few tests passing (<50%%)\n")
	fmt.Fprintf(w, "- ❌ Pattern not supported or all tests failing\n")
	fmt.Fprintf(w, "- ⚠️  No tests executed (skipped)\n\n")

	// Detailed failures
	if report.FailedTests > 0 {
		fmt.Fprintf(w, "## Detailed Failures\n\n")

		for _, backend := range backends {
			hasFailures := false
			for pattern, results := range report.Results[backend] {
				for _, result := range results {
					if !result.Passed && !result.Skipped {
						if !hasFailures {
							fmt.Fprintf(w, "### %s\n\n", backend)
							hasFailures = true
						}

						fmt.Fprintf(w, "**Pattern:** %s  \n", pattern)
						fmt.Fprintf(w, "**Test:** %s  \n", result.TestName)
						fmt.Fprintf(w, "**Duration:** %v  \n", result.Duration)

						if result.Error != nil {
							fmt.Fprintf(w, "**Error:** `%s`  \n", result.Error.Error())
						}

						if result.Output != "" {
							fmt.Fprintf(w, "**Output:**\n```\n%s\n```\n", result.Output)
						}

						fmt.Fprintf(w, "\n")
					}
				}
			}

			if hasFailures {
				fmt.Fprintf(w, "---\n\n")
			}
		}
	}

	return nil
}

func generateTextMatrix(report *ComplianceReport, w io.Writer) error {
	fmt.Fprintf(w, "Prism Backend Compliance Report\n")
	fmt.Fprintf(w, "================================\n\n")
	fmt.Fprintf(w, "Generated: %s\n\n", report.Timestamp.Format(time.RFC1123))

	backends := make([]string, 0, len(report.Results))
	for backend := range report.Results {
		backends = append(backends, backend)
	}
	sort.Strings(backends)

	for _, backend := range backends {
		fmt.Fprintf(w, "Backend: %s\n", backend)
		fmt.Fprintf(w, "  Overall Score: %.1f%%\n", report.OverallScore(backend))

		for pattern, results := range report.Results[backend] {
			passed := 0
			total := 0
			for _, r := range results {
				if !r.Skipped {
					total++
					if r.Passed {
						passed++
					}
				}
			}

			if total > 0 {
				score := float64(passed) / float64(total) * 100
				fmt.Fprintf(w, "    %s: %d/%d (%.1f%%)\n", pattern, passed, total, score)
			} else {
				fmt.Fprintf(w, "    %s: N/A\n", pattern)
			}
		}
		fmt.Fprintf(w, "\n")
	}

	return nil
}

// GeneratePerformanceComparison creates a performance comparison report
func GeneratePerformanceComparison(benchmarks []BenchmarkResult, w io.Writer, format ReportFormat) error {
	if len(benchmarks) == 0 {
		fmt.Fprintf(w, "No benchmark results available.\n")
		return nil
	}

	// Group by benchmark name
	byName := make(map[string][]BenchmarkResult)
	for _, b := range benchmarks {
		byName[b.BenchmarkName] = append(byName[b.BenchmarkName], b)
	}

	fmt.Fprintf(w, "# Performance Comparison\n\n")

	for name, results := range byName {
		fmt.Fprintf(w, "## %s\n\n", name)

		// Sort by ops/sec descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].OpsPerSec > results[j].OpsPerSec
		})

		// Table header
		fmt.Fprintf(w, "| Backend | Ops/Sec | P50 | P95 | P99 | Errors |\n")
		fmt.Fprintf(w, "|---------|---------|-----|-----|-----|--------|\n")

		// Table rows
		for _, r := range results {
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %.2f%% |\n",
				r.BackendName,
				formatNumber(r.OpsPerSec),
				formatDuration(r.P50Latency),
				formatDuration(r.P95Latency),
				formatDuration(r.P99Latency),
				r.ErrorRate*100,
			)
		}

		fmt.Fprintf(w, "\n")
	}

	return nil
}

// Helper functions

func groupPatternsByCategory(report *ComplianceReport) map[string][]Pattern {
	categories := make(map[string][]Pattern)

	for backend := range report.Results {
		for pattern := range report.Results[backend] {
			category := patternCategory(pattern)
			if !containsPattern(categories[category], pattern) {
				categories[category] = append(categories[category], pattern)
			}
		}
	}

	// Sort patterns within each category
	for category := range categories {
		sort.Slice(categories[category], func(i, j int) bool {
			return string(categories[category][i]) < string(categories[category][j])
		})
	}

	return categories
}

func patternCategory(pattern Pattern) string {
	s := string(pattern)
	if strings.HasPrefix(s, "KeyValue") {
		return "KeyValue"
	}
	if strings.HasPrefix(s, "PubSub") {
		return "PubSub"
	}
	if strings.HasPrefix(s, "Queue") {
		return "Queue"
	}
	return "Other"
}

func patternShortName(pattern Pattern) string {
	s := string(pattern)

	// Remove category prefix
	if strings.HasPrefix(s, "KeyValue") {
		s = strings.TrimPrefix(s, "KeyValue")
	} else if strings.HasPrefix(s, "PubSub") {
		s = strings.TrimPrefix(s, "PubSub")
	} else if strings.HasPrefix(s, "Queue") {
		s = strings.TrimPrefix(s, "Queue")
	}

	if s == "" {
		s = "Basic"
	}

	return s
}

func containsPattern(patterns []Pattern, pattern Pattern) bool {
	for _, p := range patterns {
		if p == pattern {
			return true
		}
	}
	return false
}

func getScoreIcon(score float64) string {
	if score >= 100.0 {
		return "✅"
	} else if score >= 90.0 {
		return "🟢"
	} else if score >= 50.0 {
		return "🟡"
	} else if score > 0.0 {
		return "🟠"
	}
	return "❌"
}

func formatNumber(n float64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", n/1000000)
	} else if n >= 1000 {
		return fmt.Sprintf("%.1fK", n/1000)
	}
	return fmt.Sprintf("%.0f", n)
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000.0)
	} else if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
