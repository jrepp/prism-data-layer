package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteGitHubActionsSummary writes a markdown report suitable for GitHub Actions $GITHUB_STEP_SUMMARY
// If GITHUB_STEP_SUMMARY env var is set, writes to that file. Otherwise writes to stdout.
func WriteGitHubActionsSummary(report *ComplianceReport) error {
	// Check if running in GitHub Actions
	summaryFile := os.Getenv("GITHUB_STEP_SUMMARY")

	if summaryFile == "" {
		// Not in GitHub Actions, write to file in test-logs/
		summaryFile = "test-logs/acceptance-report.md"
		if err := os.MkdirAll(filepath.Dir(summaryFile), 0755); err != nil {
			return fmt.Errorf("failed to create test-logs directory: %w", err)
		}
	}

	// Open file for writing
	f, err := os.OpenFile(summaryFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open summary file: %w", err)
	}
	defer f.Close()

	// Generate markdown report
	if err := GenerateComplianceMatrix(report, f, FormatMarkdown); err != nil {
		return fmt.Errorf("failed to generate compliance matrix: %w", err)
	}

	fmt.Printf("✅ Wrote GitHub Actions summary to: %s\n", summaryFile)
	return nil
}

// WriteGitHubActionsArtifact writes detailed test results to a JSON artifact
// This can be uploaded as a GitHub Actions artifact for later analysis
func WriteGitHubActionsArtifact(report *ComplianceReport, filename string) error {
	// Ensure test-logs directory exists
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file for writing
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open artifact file: %w", err)
	}
	defer f.Close()

	// Write JSON report
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	fmt.Printf("✅ Wrote test artifact to: %s\n", filename)
	return nil
}
