#!/usr/bin/env python3
"""Generate GitHub Actions summary for acceptance test results.

This script collects acceptance test results from matrix jobs and generates
a comprehensive summary report for the GitHub Actions workflow summary page.

Usage:
    # Collect results from JSON files
    uv run tooling/acceptance_summary.py \
        --results-dir build/acceptance-results \
        --output-file $GITHUB_STEP_SUMMARY

    # With custom coverage directory
    uv run tooling/acceptance_summary.py \
        --results-dir build/acceptance-results \
        --coverage-dir build/coverage-reports \
        --output-file $GITHUB_STEP_SUMMARY

Environment Variables:
    GITHUB_STEP_SUMMARY: GitHub Actions summary file (set automatically)
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path


def parse_coverage_file(coverage_path: Path) -> float | None:
    """Parse Go coverage file and return coverage percentage."""
    if not coverage_path.exists():
        return None

    try:
        result = subprocess.run(
            ["go", "tool", "cover", "-func", str(coverage_path)],
            capture_output=True,
            text=True,
            check=True,
        )

        # Parse output for "total" line
        for line in result.stdout.split("\n"):
            if "total:" in line:
                # Expected format: total: (statements) XX.X%
                parts = line.split()
                if len(parts) >= 3:
                    coverage_str = parts[-1].rstrip("%")
                    return float(coverage_str)

    except (subprocess.CalledProcessError, ValueError, IndexError):
        pass

    return None


def collect_results(results_dir: Path, coverage_dir: Path) -> dict:
    """Collect all acceptance test results from JSON files.

    Args:
        results_dir: Directory containing result JSON files (one per pattern)
        coverage_dir: Directory containing coverage.out files

    Returns:
        Dictionary with aggregated results:
        {
            "patterns": {
                "memstore": {
                    "status": "passed" | "failed" | "error",
                    "duration": 12.5,
                    "coverage": 85.3,
                    "tests_passed": 10,
                    "tests_failed": 0,
                    "output": "test output..."
                },
                ...
            },
            "summary": {
                "total_patterns": 3,
                "passed": 3,
                "failed": 0,
                "total_duration": 45.2,
                "average_coverage": 84.5
            }
        }
    """
    results = {"patterns": {}, "summary": {"total_patterns": 0, "passed": 0, "failed": 0, "total_duration": 0.0}}

    # Collect results from JSON files
    if results_dir.exists():
        for result_file in results_dir.glob("*.json"):
            pattern_name = result_file.stem.replace("acceptance-", "")

            try:
                with result_file.open() as f:
                    data = json.load(f)

                # Extract status, duration, test counts
                status = data.get("status", "unknown")
                duration = data.get("duration", 0.0)
                tests_passed = data.get("tests_passed", 0)
                tests_failed = data.get("tests_failed", 0)
                output = data.get("output", "")

                # Find corresponding coverage file
                coverage = None
                if coverage_dir.exists():
                    coverage_path = coverage_dir / f"coverage-acceptance-{pattern_name}" / "coverage.out"
                    coverage = parse_coverage_file(coverage_path)

                results["patterns"][pattern_name] = {
                    "status": status,
                    "duration": duration,
                    "coverage": coverage,
                    "tests_passed": tests_passed,
                    "tests_failed": tests_failed,
                    "output": output,
                }

                results["summary"]["total_patterns"] += 1
                results["summary"]["total_duration"] += duration
                if status == "passed":
                    results["summary"]["passed"] += 1
                elif status == "failed":
                    results["summary"]["failed"] += 1

            except (json.JSONDecodeError, KeyError) as e:
                print(f"⚠️  Failed to parse {result_file}: {e}", file=sys.stderr)
                continue

    # Calculate average coverage
    coverages = [p["coverage"] for p in results["patterns"].values() if p["coverage"] is not None]
    if coverages:
        results["summary"]["average_coverage"] = sum(coverages) / len(coverages)
    else:
        results["summary"]["average_coverage"] = None

    return results


def generate_markdown_summary(results: dict) -> str:
    """Generate GitHub-flavored Markdown summary.

    Args:
        results: Aggregated results from collect_results()

    Returns:
        Markdown string for GitHub Actions summary
    """
    lines = []

    summary = results["summary"]
    patterns = results["patterns"]

    # Overall status banner
    if summary["failed"] == 0:
        lines.append("## ✅ Acceptance Tests Passed")
    else:
        lines.append("## ❌ Acceptance Tests Failed")

    lines.append("")

    # Summary stats
    lines.append("### 📊 Summary")
    lines.append("")
    lines.append(f"- **Total Patterns:** {summary['total_patterns']}")
    lines.append(f"- **Passed:** {summary['passed']} ✅")
    lines.append(f"- **Failed:** {summary['failed']} ❌")
    lines.append(f"- **Duration:** {summary['total_duration']:.1f}s")

    if summary["average_coverage"] is not None:
        lines.append(f"- **Average Coverage:** {summary['average_coverage']:.1f}%")

    lines.append("")

    # Pattern results table
    lines.append("### 🎯 Pattern Results")
    lines.append("")
    lines.append("| Pattern | Status | Duration | Coverage | Tests |")
    lines.append("|---------|--------|----------|----------|-------|")

    for pattern_name in sorted(patterns.keys()):
        pattern = patterns[pattern_name]

        # Status emoji
        if pattern["status"] == "passed":
            status = "✅ Passed"
        elif pattern["status"] == "failed":
            status = "❌ Failed"
        else:
            status = "⚠️ Error"

        # Duration
        duration_str = f"{pattern['duration']:.1f}s"

        # Coverage
        coverage_str = f"{pattern['coverage']:.1f}%" if pattern["coverage"] is not None else "N/A"

        # Test counts
        tests_str = f"{pattern['tests_passed']} passed"
        if pattern["tests_failed"] > 0:
            tests_str += f", {pattern['tests_failed']} failed"

        lines.append(f"| {pattern_name} | {status} | {duration_str} | {coverage_str} | {tests_str} |")

    lines.append("")

    # Failed tests detail (if any)
    failed_patterns = {name: p for name, p in patterns.items() if p["status"] == "failed"}
    if failed_patterns:
        lines.append("### ❌ Failed Pattern Details")
        lines.append("")

        for pattern_name, pattern in failed_patterns.items():
            lines.append(f"#### {pattern_name}")
            lines.append("")
            lines.append("<details>")
            lines.append(f"<summary>View {pattern_name} output</summary>")
            lines.append("")
            lines.append("```text")
            # Show last 50 lines of output
            output_lines = pattern["output"].strip().split("\n")
            relevant_lines = output_lines[-50:]
            lines.append("\n".join(relevant_lines))
            lines.append("```")
            lines.append("</details>")
            lines.append("")

    # Coverage detail
    if any(p["coverage"] is not None for p in patterns.values()):
        lines.append("### 📈 Coverage Details")
        lines.append("")

        # Sort patterns by coverage (highest first)
        patterns_with_coverage = [(name, p) for name, p in patterns.items() if p["coverage"] is not None]
        patterns_with_coverage.sort(key=lambda x: x[1]["coverage"], reverse=True)

        for pattern_name, pattern in patterns_with_coverage:
            coverage = pattern["coverage"]
            # Visual bar (20 chars width, scaled to 100%)
            filled = int(coverage / 5)  # 5% per character
            coverage_bar = "█" * filled + "░" * (20 - filled)
            lines.append(f"- **{pattern_name}:** `{coverage_bar}` {coverage:.1f}%")

    lines.append("")

    return "\n".join(lines)


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Generate GitHub Actions summary for acceptance tests",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    parser.add_argument(
        "--results-dir",
        type=Path,
        default=Path("build/acceptance-results"),
        help="Directory containing result JSON files (default: build/acceptance-results)",
    )

    parser.add_argument(
        "--coverage-dir",
        type=Path,
        default=Path("build/coverage-reports"),
        help="Directory containing coverage files (default: build/coverage-reports)",
    )

    parser.add_argument(
        "--output-file",
        type=Path,
        help="Output file for summary (default: stdout, use $GITHUB_STEP_SUMMARY for Actions)",
    )

    args = parser.parse_args()

    # Collect results
    print(f"📊 Collecting results from {args.results_dir}")
    results = collect_results(args.results_dir, args.coverage_dir)

    if results["summary"]["total_patterns"] == 0:
        print("⚠️  No acceptance test results found", file=sys.stderr)
        sys.exit(1)

    # Generate summary
    summary_md = generate_markdown_summary(results)

    # Write output
    if args.output_file:
        args.output_file.write_text(summary_md)
        print(f"✅ Summary written to {args.output_file}")
    else:
        print(summary_md)

    # Exit with appropriate code
    sys.exit(0 if results["summary"]["failed"] == 0 else 1)


if __name__ == "__main__":
    main()
