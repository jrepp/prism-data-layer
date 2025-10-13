#!/usr/bin/env python3
"""Parallel acceptance test runner for Prism backend patterns.

This script runs acceptance tests in parallel for each backend/pattern combination,
collecting results and generating a comprehensive matrix report showing which backends
support which patterns with pass/fail status.

Usage:
    uv run tooling/parallel_acceptance_test.py [options]

Examples:
    # Run all acceptance tests in parallel
    uv run tooling/parallel_acceptance_test.py

    # Generate JSON report
    uv run tooling/parallel_acceptance_test.py --format json --output acceptance-report.json

    # Test specific backends
    uv run tooling/parallel_acceptance_test.py --backends MemStore,Redis

    # Test specific patterns
    uv run tooling/parallel_acceptance_test.py --patterns KeyValueBasic,KeyValueTTL

    # Fail fast on first error
    uv run tooling/parallel_acceptance_test.py --fail-fast

    # Run sequentially (for debugging)
    uv run tooling/parallel_acceptance_test.py --sequential
"""

import argparse
import asyncio
import json
import subprocess
import sys
import time
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path


class TestStatus(Enum):
    """Test execution status."""

    PASS = "pass"
    FAIL = "fail"
    SKIP = "skip"
    ERROR = "error"
    NOT_SUPPORTED = "not_supported"


@dataclass
class TestResult:
    """Result of running a single pattern/backend test."""

    backend: str
    pattern: str
    status: TestStatus
    duration: float
    output: str = ""
    error: str = ""

    @property
    def passed(self) -> bool:
        """Check if test passed."""
        return self.status == TestStatus.PASS


@dataclass
class AcceptanceReport:
    """Comprehensive report of acceptance test execution."""

    timestamp: str
    duration: float
    results: list[TestResult] = field(default_factory=list)

    @property
    def total_tests(self) -> int:
        """Total number of tests run (excluding not supported)."""
        return len([r for r in self.results if r.status != TestStatus.NOT_SUPPORTED])

    @property
    def passed_tests(self) -> int:
        """Number of tests that passed."""
        return len([r for r in self.results if r.status == TestStatus.PASS])

    @property
    def failed_tests(self) -> int:
        """Number of tests that failed."""
        return len([r for r in self.results if r.status == TestStatus.FAIL])

    @property
    def skipped_tests(self) -> int:
        """Number of tests that were skipped."""
        return len([r for r in self.results if r.status == TestStatus.SKIP])

    @property
    def errored_tests(self) -> int:
        """Number of tests that had errors."""
        return len([r for r in self.results if r.status == TestStatus.ERROR])

    @property
    def backends(self) -> list[str]:
        """List of all tested backends."""
        return sorted(set(r.backend for r in self.results))

    @property
    def patterns(self) -> list[str]:
        """List of all tested patterns."""
        return sorted(set(r.pattern for r in self.results))

    def get_result(self, backend: str, pattern: str) -> TestResult | None:
        """Get result for a specific backend/pattern combination."""
        for result in self.results:
            if result.backend == backend and result.pattern == pattern:
                return result
        return None

    def backend_score(self, backend: str) -> float:
        """Calculate compliance score for a backend (0-100)."""
        backend_results = [r for r in self.results if r.backend == backend and r.status != TestStatus.NOT_SUPPORTED]
        if not backend_results:
            return 0.0
        passed = len([r for r in backend_results if r.status == TestStatus.PASS])
        return (passed / len(backend_results)) * 100.0

    def pattern_score(self, pattern: str) -> float:
        """Calculate compliance score for a pattern across backends (0-100)."""
        pattern_results = [r for r in self.results if r.pattern == pattern and r.status != TestStatus.NOT_SUPPORTED]
        if not pattern_results:
            return 0.0
        passed = len([r for r in pattern_results if r.status == TestStatus.PASS])
        return (passed / len(pattern_results)) * 100.0


class ParallelAcceptanceRunner:
    """Parallel runner for acceptance tests."""

    def __init__(self, repo_root: Path, max_concurrency: int = 8):
        self.repo_root = repo_root
        self.max_concurrency = max_concurrency
        self.semaphore = asyncio.Semaphore(max_concurrency)

    def discover_pattern_tests(self) -> list[Path]:
        """Discover all pattern test directories."""
        patterns_dir = self.repo_root / "tests" / "acceptance" / "patterns"
        if not patterns_dir.exists():
            return []

        pattern_dirs = []
        for item in patterns_dir.iterdir():
            if item.is_dir() and any(item.glob("*_test.go")):
                pattern_dirs.append(item)

        return sorted(pattern_dirs)

    def get_pattern_name(self, pattern_dir: Path) -> str:
        """Get pattern name from directory (e.g., keyvalue -> KeyValueBasic)."""
        # Map directory names to pattern constants
        mapping = {
            "keyvalue": "KeyValue",
            "pubsub": "PubSub",
            "queue": "Queue",
        }
        dir_name = pattern_dir.name
        return mapping.get(dir_name, dir_name.title())

    async def run_pattern_tests(
        self, pattern_dir: Path, backend_filter: set[str] | None = None, fail_fast: bool = False
    ) -> list[TestResult]:
        """Run all tests for a specific pattern directory."""
        async with self.semaphore:
            pattern_name = self.get_pattern_name(pattern_dir)
            print(f"🧪 Running {pattern_name} tests...")

            cmd = ["go", "test", "-v", "-timeout", "10m", "./..."]

            # Add backend filter if specified
            if backend_filter:
                # Run tests with backend name filter (matches test names)
                backends_regex = "|".join(backend_filter)
                cmd.extend(["-run", f".*({backends_regex}).*"])

            start_time = time.time()

            try:
                proc = await asyncio.create_subprocess_exec(
                    *cmd,
                    cwd=pattern_dir,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.STDOUT,
                    env={**subprocess.os.environ, "PRISM_TEST_QUIET": "1"},  # Suppress container logs
                )

                stdout, _ = await proc.communicate()
                output = stdout.decode("utf-8", errors="replace")
                duration = time.time() - start_time

                # Parse Go test output to extract backend-specific results
                results = self._parse_go_test_output(output, pattern_name, duration)

                # If no results parsed, add a summary result
                if not results:
                    status = TestStatus.PASS if proc.returncode == 0 else TestStatus.FAIL
                    results = [
                        TestResult(
                            backend="All",
                            pattern=pattern_name,
                            status=status,
                            duration=duration,
                            output=output,
                        )
                    ]

                # Check fail fast
                if fail_fast and any(not r.passed for r in results):
                    print(f"❌ {pattern_name} tests failed - stopping due to fail-fast")
                    sys.exit(1)

                return results

            except Exception as e:
                duration = time.time() - start_time
                print(f"❌ Error running {pattern_name} tests: {e}")
                return [
                    TestResult(
                        backend="All",
                        pattern=pattern_name,
                        status=TestStatus.ERROR,
                        duration=duration,
                        error=str(e),
                    )
                ]

    def _parse_go_test_output(self, output: str, pattern: str, total_duration: float) -> list[TestResult]:
        """Parse Go test output to extract per-backend results.

        Go test output format:
            === RUN   TestKeyValueBasicPattern
            === RUN   TestKeyValueBasicPattern/MemStore
            === RUN   TestKeyValueBasicPattern/MemStore/SetAndGet
            --- PASS: TestKeyValueBasicPattern/MemStore/SetAndGet (0.01s)
            --- PASS: TestKeyValueBasicPattern/MemStore (0.15s)
            === RUN   TestKeyValueBasicPattern/Redis
            --- PASS: TestKeyValueBasicPattern/Redis (0.42s)
        """
        results = []
        backend_tests: dict[str, dict[str, any]] = {}

        lines = output.split("\n")
        for line in lines:
            # Parse RUN lines to discover backends
            if "=== RUN" in line:
                parts = line.split("/")
                if len(parts) >= 2:
                    backend = parts[1].strip()
                    if backend not in backend_tests:
                        backend_tests[backend] = {"status": TestStatus.PASS, "duration": 0.0}

            # Parse PASS/FAIL lines
            elif "--- PASS:" in line or "--- FAIL:" in line or "--- SKIP:" in line:
                parts = line.split("/")
                if len(parts) >= 2:
                    backend = parts[1].split()[0]
                    status_str = line.split(":")[0].strip().split()[-1]  # PASS, FAIL, or SKIP

                    # Extract duration if present (e.g., "(0.42s)")
                    duration = 0.0
                    if "(" in line and "s)" in line:
                        try:
                            duration_str = line.split("(")[1].split("s)")[0]
                            duration = float(duration_str)
                        except (IndexError, ValueError):
                            pass

                    # Only record top-level backend results (not individual sub-tests)
                    if len(parts) == 2:  # Format: TestName/Backend
                        if status_str == "PASS":
                            backend_tests[backend] = {"status": TestStatus.PASS, "duration": duration}
                        elif status_str == "FAIL":
                            backend_tests[backend] = {"status": TestStatus.FAIL, "duration": duration}
                        elif status_str == "SKIP":
                            backend_tests[backend] = {"status": TestStatus.SKIP, "duration": duration}

        # Convert parsed data to TestResults
        for backend, data in backend_tests.items():
            results.append(
                TestResult(
                    backend=backend,
                    pattern=pattern,
                    status=data["status"],
                    duration=data["duration"],
                    output=output,
                )
            )

        return results

    async def run_all_tests(
        self,
        pattern_filter: set[str] | None = None,
        backend_filter: set[str] | None = None,
        fail_fast: bool = False,
        sequential: bool = False,
    ) -> AcceptanceReport:
        """Run all acceptance tests in parallel or sequentially."""
        start_time = time.time()

        # Discover pattern test directories
        pattern_dirs = self.discover_pattern_tests()
        if not pattern_dirs:
            print("⚠️  No pattern test directories found")
            return AcceptanceReport(timestamp=time.strftime("%Y-%m-%d %H:%M:%S"), duration=0.0)

        # Filter patterns if requested
        if pattern_filter:
            pattern_dirs = [d for d in pattern_dirs if self.get_pattern_name(d) in pattern_filter]

        print(f"📊 Running acceptance tests for {len(pattern_dirs)} patterns")
        print(f"⚡ Concurrency: {1 if sequential else self.max_concurrency}")
        if backend_filter:
            print(f"🎯 Backend filter: {', '.join(sorted(backend_filter))}")
        if pattern_filter:
            print(f"🎯 Pattern filter: {', '.join(sorted(pattern_filter))}")
        print()

        # Run tests
        if sequential:
            all_results = []
            for pattern_dir in pattern_dirs:
                results = await self.run_pattern_tests(pattern_dir, backend_filter, fail_fast)
                all_results.extend(results)
        else:
            # Run in parallel
            tasks = [self.run_pattern_tests(pattern_dir, backend_filter, fail_fast) for pattern_dir in pattern_dirs]
            results_list = await asyncio.gather(*tasks)
            all_results = [r for results in results_list for r in results]

        duration = time.time() - start_time

        return AcceptanceReport(timestamp=time.strftime("%Y-%m-%d %H:%M:%S"), duration=duration, results=all_results)


class ReportFormatter:
    """Formats acceptance test reports in various formats."""

    @staticmethod
    def format_terminal(report: AcceptanceReport) -> str:
        """Format report for terminal output with colors."""
        lines = []

        # Header
        lines.append("=" * 80)
        lines.append("🧪 PRISM ACCEPTANCE TEST MATRIX REPORT")
        lines.append("=" * 80)
        lines.append(f"Timestamp: {report.timestamp}")
        lines.append(f"Duration:  {report.duration:.1f}s")
        lines.append("")

        # Summary
        lines.append("📊 Summary:")
        lines.append(f"  Total Tests:   {report.total_tests}")
        lines.append(
            f"  ✅ Passed:     {report.passed_tests} ({report.passed_tests / max(report.total_tests, 1) * 100:.1f}%)"
        )
        lines.append(f"  ❌ Failed:     {report.failed_tests}")
        lines.append(f"  ⏭️  Skipped:    {report.skipped_tests}")
        if report.errored_tests > 0:
            lines.append(f"  🔥 Errors:     {report.errored_tests}")
        lines.append("")

        # Matrix
        lines.append("🎯 Pattern × Backend Compliance Matrix:")
        lines.append("")

        # Build matrix
        backends = report.backends
        patterns = report.patterns

        if not backends or not patterns:
            lines.append("  No test results to display")
            lines.append("")
            return "\n".join(lines)

        # Calculate column widths
        max_pattern_len = max(len(p) for p in patterns)
        max_backend_len = max(len(b) for b in backends)
        col_width = max(max_backend_len + 2, 12)

        # Header row
        header = f"  {'Pattern':<{max_pattern_len}} │ "
        header += " │ ".join(f"{b:^{col_width}}" for b in backends)
        header += " │ Score"
        lines.append(header)

        # Separator
        sep = "  " + "─" * max_pattern_len + "─┼─"
        sep += "─┼─".join("─" * col_width for _ in backends)
        sep += "─┼───────"
        lines.append(sep)

        # Data rows
        for pattern in patterns:
            row = f"  {pattern:<{max_pattern_len}} │ "
            cells = []

            for backend in backends:
                result = report.get_result(backend, pattern)
                if result is None or result.status == TestStatus.NOT_SUPPORTED:
                    cell = "─" * col_width
                elif result.status == TestStatus.PASS:
                    cell = f"{'✅ PASS':^{col_width}}"
                elif result.status == TestStatus.FAIL:
                    cell = f"{'❌ FAIL':^{col_width}}"
                elif result.status == TestStatus.SKIP:
                    cell = f"{'⏭️  SKIP':^{col_width}}"
                elif result.status == TestStatus.ERROR:
                    cell = f"{'🔥 ERROR':^{col_width}}"
                else:
                    cell = "─" * col_width

                cells.append(cell)

            row += " │ ".join(cells)

            # Add pattern score
            score = report.pattern_score(pattern)
            row += f" │ {score:5.1f}%"

            lines.append(row)

        # Footer separator
        lines.append(sep)

        # Backend scores
        score_row = f"  {'Score':<{max_pattern_len}} │ "
        score_cells = []
        for backend in backends:
            score = report.backend_score(backend)
            score_cells.append(f"{score:>5.1f}%{' ' * (col_width - 7)}")
        score_row += " │ ".join(score_cells)
        score_row += f" │ {report.passed_tests / max(report.total_tests, 1) * 100:5.1f}%"
        lines.append(score_row)

        lines.append("")

        # Legend
        lines.append("Legend:")
        lines.append("  ✅ PASS  - All tests passed")
        lines.append("  ❌ FAIL  - One or more tests failed")
        lines.append("  ⏭️  SKIP  - Tests skipped (missing capability)")
        lines.append("  🔥 ERROR - Test execution error")
        lines.append("  ─────── - Pattern not supported by backend")
        lines.append("")

        # Failures detail
        failures = [r for r in report.results if r.status == TestStatus.FAIL]
        if failures:
            lines.append("=" * 80)
            lines.append("❌ Failed Tests Details:")
            lines.append("=" * 80)
            for result in failures:
                lines.append(f"\n{result.backend} - {result.pattern}:")
                lines.append(f"  Duration: {result.duration:.2f}s")
                if result.error:
                    lines.append(f"  Error: {result.error}")
                # Show relevant failure output (last 10 lines)
                if result.output:
                    output_lines = result.output.strip().split("\n")
                    relevant = output_lines[-10:]
                    lines.append("  Output:")
                    for line in relevant:
                        lines.append(f"    {line}")

        lines.append("=" * 80)
        return "\n".join(lines)

    @staticmethod
    def format_markdown(report: AcceptanceReport) -> str:
        """Format report as Markdown."""
        lines = []

        lines.append("# Prism Acceptance Test Report")
        lines.append("")
        lines.append(f"**Generated:** {report.timestamp}")
        lines.append(f"**Duration:** {report.duration:.1f}s")
        lines.append("")

        # Summary
        lines.append("## Summary")
        lines.append("")
        lines.append(f"- **Total Tests:** {report.total_tests}")
        lines.append(
            f"- **Passed:** {report.passed_tests} ({report.passed_tests / max(report.total_tests, 1) * 100:.1f}%)"
        )
        lines.append(f"- **Failed:** {report.failed_tests}")
        lines.append(f"- **Skipped:** {report.skipped_tests}")
        if report.errored_tests > 0:
            lines.append(f"- **Errors:** {report.errored_tests}")
        lines.append("")

        # Matrix table
        lines.append("## Pattern × Backend Compliance Matrix")
        lines.append("")

        backends = report.backends
        patterns = report.patterns

        if not backends or not patterns:
            lines.append("No test results available")
            return "\n".join(lines)

        # Table header
        header = "| Pattern | " + " | ".join(backends) + " | Score |"
        lines.append(header)

        # Separator
        sep = "| --- | " + " | ".join(["---" for _ in backends]) + " | ---: |"
        lines.append(sep)

        # Data rows
        for pattern in patterns:
            row = f"| {pattern} | "
            cells = []

            for backend in backends:
                result = report.get_result(backend, pattern)
                if result is None or result.status == TestStatus.NOT_SUPPORTED:
                    cells.append("—")
                elif result.status == TestStatus.PASS:
                    cells.append("✅")
                elif result.status == TestStatus.FAIL:
                    cells.append("❌")
                elif result.status == TestStatus.SKIP:
                    cells.append("⏭️")
                elif result.status == TestStatus.ERROR:
                    cells.append("🔥")

            row += " | ".join(cells)
            score = report.pattern_score(pattern)
            row += f" | {score:.1f}% |"
            lines.append(row)

        # Backend scores row
        score_row = "| **Score** | "
        score_cells = []
        for backend in backends:
            score = report.backend_score(backend)
            score_cells.append(f"**{score:.1f}%**")
        score_row += " | ".join(score_cells)
        score_row += f" | **{report.passed_tests / max(report.total_tests, 1) * 100:.1f}%** |"
        lines.append(score_row)

        lines.append("")

        # Legend
        lines.append("### Legend")
        lines.append("")
        lines.append("- ✅ All tests passed")
        lines.append("- ❌ One or more tests failed")
        lines.append("- ⏭️ Tests skipped (missing capability)")
        lines.append("- 🔥 Test execution error")
        lines.append("- — Pattern not supported by backend")
        lines.append("")

        return "\n".join(lines)

    @staticmethod
    def format_json(report: AcceptanceReport) -> str:
        """Format report as JSON."""
        data = {
            "timestamp": report.timestamp,
            "duration": report.duration,
            "summary": {
                "total": report.total_tests,
                "passed": report.passed_tests,
                "failed": report.failed_tests,
                "skipped": report.skipped_tests,
                "errored": report.errored_tests,
            },
            "backends": report.backends,
            "patterns": report.patterns,
            "results": [
                {
                    "backend": r.backend,
                    "pattern": r.pattern,
                    "status": r.status.value,
                    "duration": r.duration,
                    "error": r.error if r.error else None,
                }
                for r in report.results
            ],
            "scores": {
                "backends": {backend: report.backend_score(backend) for backend in report.backends},
                "patterns": {pattern: report.pattern_score(pattern) for pattern in report.patterns},
            },
        }
        return json.dumps(data, indent=2)


async def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Parallel acceptance test runner for Prism backend patterns",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )

    parser.add_argument("--backends", help="Comma-separated list of backends to test (e.g., MemStore,Redis)")

    parser.add_argument("--patterns", help="Comma-separated list of patterns to test (e.g., KeyValueBasic,KeyValueTTL)")

    parser.add_argument(
        "--concurrency",
        type=int,
        default=8,
        help="Maximum number of concurrent test executions (default: 8)",
    )

    parser.add_argument("--sequential", action="store_true", help="Run tests sequentially instead of in parallel")

    parser.add_argument("--fail-fast", action="store_true", help="Stop on first test failure")

    parser.add_argument(
        "--format",
        choices=["terminal", "markdown", "json"],
        default="terminal",
        help="Output format (default: terminal)",
    )

    parser.add_argument("--output", type=Path, help="Write report to file (default: stdout)")

    args = parser.parse_args()

    # Get repository root
    repo_root = Path(__file__).parent.parent

    # Parse filters
    backend_filter = set(args.backends.split(",")) if args.backends else None
    pattern_filter = set(args.patterns.split(",")) if args.patterns else None

    # Run tests
    runner = ParallelAcceptanceRunner(repo_root, max_concurrency=args.concurrency)

    try:
        report = await runner.run_all_tests(
            pattern_filter=pattern_filter,
            backend_filter=backend_filter,
            fail_fast=args.fail_fast,
            sequential=args.sequential,
        )
    except KeyboardInterrupt:
        print("\n\n⚠️  Tests interrupted by user")
        sys.exit(130)

    # Format report
    if args.format == "terminal":
        formatted = ReportFormatter.format_terminal(report)
    elif args.format == "markdown":
        formatted = ReportFormatter.format_markdown(report)
    else:  # json
        formatted = ReportFormatter.format_json(report)

    # Output report
    if args.output:
        args.output.write_text(formatted)
        print(f"✅ Report written to: {args.output}")
    else:
        print(formatted)

    # Exit with appropriate code
    sys.exit(0 if report.failed_tests == 0 and report.errored_tests == 0 else 1)


if __name__ == "__main__":
    asyncio.run(main())
