#!/usr/bin/env python3
"""Parallel Test Runner for Prism

Runs independent test suites in parallel with:
- Fork-join execution model
- Individual log files per test suite
- Fail-fast mode (stop on first failure)
- Real-time progress display
- Comprehensive result summary

Usage:
    # Run all tests in parallel
    uv run tooling/parallel_test.py

    # Run only fast tests (skip acceptance)
    uv run tooling/parallel_test.py --fast

    # Run with fail-fast (stop on first failure)
    uv run tooling/parallel_test.py --fail-fast

    # Run specific test suites
    uv run tooling/parallel_test.py --suites unit,acceptance

    # Show verbose output
    uv run tooling/parallel_test.py --verbose
"""

import argparse
import asyncio
import json
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from pathlib import Path


# ANSI color codes
class Color:
    RESET = "\033[0m"
    BOLD = "\033[1m"
    RED = "\033[31m"
    GREEN = "\033[32m"
    YELLOW = "\033[33m"
    BLUE = "\033[34m"
    MAGENTA = "\033[35m"
    CYAN = "\033[36m"
    GRAY = "\033[90m"


class TestStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    SKIPPED = "skipped"


@dataclass
class TestSuite:
    """Configuration for a test suite"""

    name: str
    command: str
    description: str
    category: str  # "unit", "acceptance", "integration", "lint"
    timeout: int = 300  # 5 minutes default
    depends_on: list[str] = field(default_factory=list)
    parallel_group: str | None = None  # Tests in same group run serially

    # Runtime state
    status: TestStatus = TestStatus.PENDING
    start_time: float | None = None
    end_time: float | None = None
    return_code: int | None = None
    log_file: Path | None = None
    error_message: str | None = None


# Define all test suites
TEST_SUITES = [
    # Unit Tests (fast, parallel)
    TestSuite(
        name="proxy-unit",
        command="cd prism-proxy && cargo test --lib",
        description="Rust proxy unit tests",
        category="unit",
        timeout=120,
    ),
    TestSuite(
        name="core-unit",
        command="cd pkg/plugin && go test -v -cover ./...",
        description="Core SDK unit tests",
        category="unit",
        timeout=60,
    ),
    TestSuite(
        name="memstore-unit",
        command="cd pkg/drivers/memstore && go test -v -cover ./...",
        description="MemStore unit tests",
        category="unit",
        timeout=60,
    ),
    TestSuite(
        name="redis-unit",
        command="cd pkg/drivers/redis && go test -v -cover ./...",
        description="Redis unit tests",
        category="unit",
        timeout=60,
    ),
    TestSuite(
        name="nats-unit",
        command="cd pkg/drivers/nats && go test -v -cover ./...",
        description="NATS unit tests",
        category="unit",
        timeout=60,
    ),
    # Lint Tests (fast, parallel)
    TestSuite(
        name="lint-rust",
        command="cd prism-proxy && cargo clippy -- -D warnings",
        description="Rust linting",
        category="lint",
        timeout=120,
    ),
    TestSuite(
        name="lint-go-memstore",
        command="cd pkg/drivers/memstore && go vet ./...",
        description="Go linting (memstore)",
        category="lint",
        timeout=30,
    ),
    TestSuite(
        name="lint-go-redis",
        command="cd pkg/drivers/redis && go vet ./...",
        description="Go linting (redis)",
        category="lint",
        timeout=30,
    ),
    TestSuite(
        name="lint-go-nats",
        command="cd pkg/drivers/nats && go vet ./...",
        description="Go linting (nats)",
        category="lint",
        timeout=30,
    ),
    TestSuite(
        name="lint-go-core",
        command="cd pkg/plugin && go vet ./...",
        description="Go linting (core)",
        category="lint",
        timeout=30,
    ),
    # Acceptance Tests (slower, parallel but may need containers)
    TestSuite(
        name="acceptance-interfaces",
        command="cd tests/acceptance/interfaces && go test -v -timeout 10m ./...",
        description="Interface-based acceptance tests (multi-backend)",
        category="acceptance",
        timeout=600,
        parallel_group="acceptance",  # Group to avoid container conflicts
    ),
    TestSuite(
        name="acceptance-redis",
        command="cd tests/acceptance/redis && go test -v -timeout 10m ./...",
        description="Redis acceptance tests",
        category="acceptance",
        timeout=600,
        parallel_group="acceptance",
    ),
    TestSuite(
        name="acceptance-nats",
        command="cd tests/acceptance/nats && go test -v -timeout 10m ./...",
        description="NATS acceptance tests",
        category="acceptance",
        timeout=600,
        parallel_group="acceptance",
    ),
    # Integration Tests (medium speed)
    TestSuite(
        name="integration-go",
        command="cd tests/integration && go test -v -timeout 5m ./...",
        description="Go integration tests (proxy-pattern lifecycle)",
        category="integration",
        timeout=300,
        depends_on=["memstore-unit"],  # Needs MemStore binary
    ),
    TestSuite(
        name="integration-rust",
        command="cd proxy && cargo test --test integration_test -- --ignored --nocapture",
        description="Rust proxy integration tests",
        category="integration",
        timeout=300,
        depends_on=["memstore-unit"],
    ),
]


class ParallelTestRunner:
    """Orchestrates parallel test execution with fork-join pattern"""

    def __init__(
        self,
        suites: list[TestSuite],
        log_dir: Path,
        fail_fast: bool = False,
        max_parallel: int = 8,
        verbose: bool = False,
    ) -> None:
        self.suites = suites
        self.log_dir = log_dir
        self.fail_fast = fail_fast
        self.max_parallel = max_parallel
        self.verbose = verbose

        # Runtime state
        self.semaphore = asyncio.Semaphore(max_parallel)
        self.parallel_groups: dict[str, asyncio.Lock] = {}
        self.failed = False
        self.results: dict[str, TestSuite] = {}
        self.completion_events: dict[str, asyncio.Event] = {}

        # Create completion events for all suites (for dependency waiting)
        for suite in suites:
            self.completion_events[suite.name] = asyncio.Event()

        # Create log directory
        self.log_dir.mkdir(parents=True, exist_ok=True)

    async def run_all(self) -> bool:
        """Run all test suites in parallel, return True if all passed"""
        print(f"\n{Color.BOLD}🚀 Prism Parallel Test Runner{Color.RESET}")
        print(f"{Color.GRAY}═══════════════════════════════════════════════════{Color.RESET}\n")

        print("📊 Test Configuration:")
        print(f"  • Total suites: {len(self.suites)}")
        print(f"  • Max parallel: {self.max_parallel}")
        print(f"  • Fail-fast: {'enabled' if self.fail_fast else 'disabled'}")
        print(f"  • Log directory: {self.log_dir}")
        print()

        # Group suites by category
        by_category = {}
        for suite in self.suites:
            by_category.setdefault(suite.category, []).append(suite)

        for category, suites in sorted(by_category.items()):
            print(f"  {category.upper()}: {len(suites)} suite(s)")
        print()

        start_time = time.time()

        # Create tasks for all test suites
        tasks = [self.run_suite(suite) for suite in self.suites]

        # Wait for all tasks (or first failure in fail-fast mode)
        try:
            await asyncio.gather(*tasks)
        except KeyboardInterrupt:
            print(f"\n{Color.YELLOW}⚠️  Interrupted by user{Color.RESET}")
            return False

        end_time = time.time()
        duration = end_time - start_time

        # Print summary
        return self.print_summary(duration)

    async def run_suite(self, suite: TestSuite):
        """Run a single test suite"""
        try:
            # Wait for dependencies to complete
            for dep in suite.depends_on:
                if dep in self.completion_events:
                    # Wait for dependency to complete
                    await self.completion_events[dep].wait()

                    # Check if dependency passed
                    dep_suite = next((s for s in self.suites if s.name == dep), None)
                    if dep_suite and dep_suite.status != TestStatus.PASSED:
                        suite.status = TestStatus.SKIPPED
                        suite.error_message = f"Dependency {dep} did not pass"
                        return

            # Check fail-fast
            if self.fail_fast and self.failed:
                suite.status = TestStatus.SKIPPED
                suite.error_message = "Skipped due to fail-fast"
                return

            # Acquire semaphore for parallel execution limit
            async with self.semaphore:
                # If suite is in a parallel group, serialize within that group
                if suite.parallel_group:
                    if suite.parallel_group not in self.parallel_groups:
                        self.parallel_groups[suite.parallel_group] = asyncio.Lock()
                    lock = self.parallel_groups[suite.parallel_group]
                    async with lock:
                        await self._execute_suite(suite)
                else:
                    await self._execute_suite(suite)
        finally:
            # Signal completion (whether passed, failed, or skipped)
            self.completion_events[suite.name].set()

    async def _execute_suite(self, suite: TestSuite):
        """Execute a test suite command"""
        suite.status = TestStatus.RUNNING
        suite.start_time = time.time()
        suite.log_file = self.log_dir / f"{suite.name}.log"

        # Print start message
        icon = self._get_category_icon(suite.category)
        print(f"{icon} {Color.CYAN}{suite.name}{Color.RESET} - {suite.description}")

        try:
            # Run command and capture output
            process = await asyncio.create_subprocess_shell(
                suite.command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                cwd=Path.cwd(),
            )

            # Write output to log file
            with suite.log_file.open("wb") as log:
                while True:
                    line = await process.stdout.readline()
                    if not line:
                        break
                    log.write(line)
                    if self.verbose:
                        print(f"  {Color.GRAY}│{Color.RESET} {line.decode('utf-8', errors='ignore').rstrip()}")

            # Wait for completion with timeout
            try:
                await asyncio.wait_for(process.wait(), timeout=suite.timeout)
                suite.return_code = process.returncode
            except TimeoutError:
                process.kill()
                suite.return_code = -1
                suite.error_message = f"Timeout after {suite.timeout}s"

        except Exception as e:
            suite.return_code = -1
            suite.error_message = str(e)

        suite.end_time = time.time()
        duration = suite.end_time - suite.start_time

        # Determine status
        if suite.return_code == 0:
            suite.status = TestStatus.PASSED
            print(f"  {Color.GREEN}✓{Color.RESET} {suite.name} passed in {duration:.1f}s")
        else:
            suite.status = TestStatus.FAILED
            self.failed = True
            error_info = suite.error_message or f"exit code {suite.return_code}"
            print(f"  {Color.RED}✗{Color.RESET} {suite.name} failed ({error_info})")
            print(f"     Log: {suite.log_file}")

            # Show last 10 lines of error log
            if suite.log_file.exists():
                with suite.log_file.open("r") as f:
                    lines = f.readlines()
                    print(f"\n  {Color.RED}Last 10 lines:{Color.RESET}")
                    for line in lines[-10:]:
                        print(f"    {Color.GRAY}{line.rstrip()}{Color.RESET}")
                print()

        self.results[suite.name] = suite

    def print_summary(self, duration: float) -> bool:
        """Print test summary and return overall success"""
        print(f"\n{Color.BOLD}📋 Test Summary{Color.RESET}")
        print(f"{Color.GRAY}═══════════════════════════════════════════════════{Color.RESET}\n")

        # Count by status
        passed = sum(1 for s in self.suites if s.status == TestStatus.PASSED)
        failed = sum(1 for s in self.suites if s.status == TestStatus.FAILED)
        skipped = sum(1 for s in self.suites if s.status == TestStatus.SKIPPED)

        print(f"  {Color.GREEN}✓{Color.RESET} Passed:  {passed}/{len(self.suites)}")
        print(f"  {Color.RED}✗{Color.RESET} Failed:  {failed}/{len(self.suites)}")
        if skipped > 0:
            print(f"  {Color.YELLOW}⊘{Color.RESET} Skipped: {skipped}/{len(self.suites)}")

        print(f"\n  ⏱️  Total time: {duration:.1f}s")

        # Calculate time savings (estimate sequential time)
        sequential_time = sum((s.end_time - s.start_time) for s in self.suites if s.start_time and s.end_time)
        if sequential_time > duration:
            savings = sequential_time - duration
            speedup = sequential_time / duration if duration > 0 else 1
            print(f"  ⚡ Speedup: {speedup:.1f}x ({savings:.1f}s saved)")

        # Show failed tests
        if failed > 0:
            print(f"\n{Color.RED}Failed Tests:{Color.RESET}")
            for suite in self.suites:
                if suite.status == TestStatus.FAILED:
                    print(f"  • {suite.name}: {suite.error_message or 'see log'}")
                    print(f"    Log: {suite.log_file}")

        # Generate JSON report
        report_file = self.log_dir / "test-report.json"
        self._generate_json_report(report_file, duration)
        print(f"\n📄 JSON report: {report_file}")

        print()
        if failed == 0:
            print(f"{Color.GREEN}✅ All tests passed!{Color.RESET}\n")
            return True
        print(f"{Color.RED}❌ Tests failed{Color.RESET}\n")
        return False

    def _generate_json_report(self, report_file: Path, duration: float):
        """Generate machine-readable JSON report"""
        report = {
            "timestamp": datetime.now().isoformat(),
            "duration": duration,
            "suites": [
                {
                    "name": s.name,
                    "description": s.description,
                    "category": s.category,
                    "status": s.status.value,
                    "start_time": s.start_time,
                    "end_time": s.end_time,
                    "duration": s.end_time - s.start_time if s.start_time and s.end_time else None,
                    "return_code": s.return_code,
                    "error_message": s.error_message,
                    "log_file": str(s.log_file) if s.log_file else None,
                }
                for s in self.suites
            ],
            "summary": {
                "total": len(self.suites),
                "passed": sum(1 for s in self.suites if s.status == TestStatus.PASSED),
                "failed": sum(1 for s in self.suites if s.status == TestStatus.FAILED),
                "skipped": sum(1 for s in self.suites if s.status == TestStatus.SKIPPED),
            },
        }

        with report_file.open("w") as f:
            json.dump(report, f, indent=2)

    @staticmethod
    def _get_category_icon(category: str) -> str:
        """Get icon for test category"""
        icons = {
            "unit": "🧪",
            "acceptance": "🔬",
            "integration": "🔗",
            "lint": "🔍",
        }
        return icons.get(category, "📦")


def main():
    parser = argparse.ArgumentParser(
        description="Parallel test runner for Prism",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run all tests in parallel
  uv run tooling/parallel_test.py

  # Run only fast tests (skip acceptance)
  uv run tooling/parallel_test.py --fast

  # Run with fail-fast (stop on first failure)
  uv run tooling/parallel_test.py --fail-fast

  # Run specific categories
  uv run tooling/parallel_test.py --categories unit,lint

  # Limit parallelism
  uv run tooling/parallel_test.py --max-parallel 4
        """,
    )

    parser.add_argument(
        "--fast",
        action="store_true",
        help="Skip slow acceptance tests (run only unit + lint)",
    )
    parser.add_argument(
        "--fail-fast",
        action="store_true",
        help="Stop on first failure",
    )
    parser.add_argument(
        "--categories",
        type=str,
        help="Comma-separated list of categories to run (unit,lint,acceptance,integration)",
    )
    parser.add_argument(
        "--suites",
        type=str,
        help="Comma-separated list of specific suite names to run",
    )
    parser.add_argument(
        "--max-parallel",
        type=int,
        default=8,
        help="Maximum number of parallel test suites (default: 8)",
    )
    parser.add_argument(
        "--log-dir",
        type=Path,
        default=Path("test-logs"),
        help="Directory for test logs (default: ./test-logs)",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Show verbose output (test output in real-time)",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all available test suites and exit",
    )

    args = parser.parse_args()

    # List mode
    if args.list:
        print(f"\n{Color.BOLD}Available Test Suites:{Color.RESET}\n")
        by_category = {}
        for suite in TEST_SUITES:
            by_category.setdefault(suite.category, []).append(suite)

        for category in sorted(by_category.keys()):
            print(f"{Color.BOLD}{category.upper()}:{Color.RESET}")
            for suite in by_category[category]:
                print(f"  • {suite.name}: {suite.description}")
                if suite.depends_on:
                    print(f"    Depends on: {', '.join(suite.depends_on)}")
            print()
        return 0

    # Filter suites
    suites = TEST_SUITES.copy()

    if args.fast:
        # Skip acceptance tests in fast mode
        suites = [s for s in suites if s.category != "acceptance"]
        print(f"{Color.YELLOW}Fast mode: skipping acceptance tests{Color.RESET}")

    if args.categories:
        categories = set(args.categories.split(","))
        suites = [s for s in suites if s.category in categories]
        print(f"Running categories: {', '.join(sorted(categories))}")

    if args.suites:
        suite_names = set(args.suites.split(","))
        suites = [s for s in suites if s.name in suite_names]
        print(f"Running suites: {', '.join(sorted(suite_names))}")

    if not suites:
        print(f"{Color.RED}No test suites selected{Color.RESET}")
        return 1

    # Run tests
    runner = ParallelTestRunner(
        suites=suites,
        log_dir=args.log_dir,
        fail_fast=args.fail_fast,
        max_parallel=args.max_parallel,
        verbose=args.verbose,
    )

    success = asyncio.run(runner.run_all())
    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
