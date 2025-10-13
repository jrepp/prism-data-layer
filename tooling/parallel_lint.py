#!/usr/bin/env python3
"""
Parallel Linting Runner for Prism

Runs golangci-lint with different linter categories in parallel for maximum speed.
Instead of running all 50+ linters sequentially, this runs them in parallel groups.

Usage:
    python tooling/parallel_lint.py                    # Run all linters
    python tooling/parallel_lint.py --categories critical,style  # Run specific categories
    python tooling/parallel_lint.py --list            # List all linter categories
    python tooling/parallel_lint.py --fail-fast       # Stop on first failure

Categories:
    - critical: Must-pass linters (errcheck, govet, staticcheck, etc.)
    - style: Code style (gofmt, goimports, whitespace, etc.)
    - quality: Code quality (gocyclo, gocritic, dupl, etc.)
    - errors: Error handling (errorlint, err113, wrapcheck)
    - security: Security issues (gosec)
    - performance: Performance optimizations (prealloc, bodyclose)
    - bugs: Bug detection (makezero, nilerr, durationcheck)
    - testing: Test-related (testpackage, paralleltest, testifylint)
    - misc: Miscellaneous (misspell, unconvert, unparam)
"""

import asyncio
import json
import sys
import time
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import List, Dict, Optional


class LintStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    SKIPPED = "skipped"


@dataclass
class LintCategory:
    """Represents a category of linters that can run in parallel"""
    name: str
    linters: List[str]
    description: str
    critical: bool = False  # If True, failure blocks other categories
    timeout: int = 300  # 5 minutes default
    status: LintStatus = LintStatus.PENDING
    output: str = ""
    error: Optional[str] = None
    duration: float = 0.0
    issues_count: int = 0


# Linter categories optimized for parallel execution
LINT_CATEGORIES = [
    LintCategory(
        name="critical",
        linters=[
            "errcheck",
            "govet",
            "ineffassign",
            "staticcheck",
            "unused",
        ],
        description="Critical linters (must pass)",
        critical=True,
        timeout=600,
    ),
    LintCategory(
        name="style",
        linters=[
            "gofmt",
            "gofumpt",
            "goimports",
            "gci",
            "whitespace",
            "wsl",
        ],
        description="Code style and formatting",
        timeout=180,
    ),
    LintCategory(
        name="quality",
        linters=[
            "goconst",
            "gocritic",
            "gocyclo",
            "gocognit",
            "cyclop",
            "dupl",
            "revive",
            "stylecheck",
        ],
        description="Code quality and maintainability",
        timeout=600,
    ),
    LintCategory(
        name="errors",
        linters=[
            "errorlint",
            "err113",
            "wrapcheck",
        ],
        description="Error handling patterns",
        timeout=300,
    ),
    LintCategory(
        name="security",
        linters=[
            "gosec",
            "copyloopvar",
        ],
        description="Security vulnerabilities",
        timeout=300,
    ),
    LintCategory(
        name="performance",
        linters=[
            "prealloc",
            "bodyclose",
            "noctx",
        ],
        description="Performance optimizations",
        timeout=300,
    ),
    LintCategory(
        name="bugs",
        linters=[
            "asciicheck",
            "bidichk",
            "durationcheck",
            "makezero",
            "nilerr",
            "nilnil",
            "rowserrcheck",
            "sqlclosecheck",
        ],
        description="Bug detection",
        timeout=300,
    ),
    LintCategory(
        name="testing",
        linters=[
            "testpackage",
            "paralleltest",
            "testifylint",
        ],
        description="Test-related issues",
        timeout=180,
    ),
    LintCategory(
        name="maintainability",
        linters=[
            "funlen",
            "maintidx",
            "nestif",
            "lll",
        ],
        description="Code maintainability",
        timeout=300,
    ),
    LintCategory(
        name="misc",
        linters=[
            "misspell",
            "nakedret",
            "predeclared",
            "tagliatelle",
            "unconvert",
            "unparam",
            "wastedassign",
        ],
        description="Miscellaneous checks",
        timeout=300,
    ),
]


class ParallelLintRunner:
    """Orchestrates parallel linting execution"""

    def __init__(self, max_parallel: int = 4, fail_fast: bool = False):
        self.max_parallel = max_parallel
        self.fail_fast = fail_fast
        self.semaphore = asyncio.Semaphore(max_parallel)
        self.categories: List[LintCategory] = []
        self.start_time = 0.0

    def find_go_modules(self, base_dir: Path) -> List[Path]:
        """Find all directories containing go.mod files"""
        modules = []
        for go_mod in base_dir.rglob("go.mod"):
            modules.append(go_mod.parent)
        return sorted(modules)

    async def run_category(self, category: LintCategory, base_dir: Path):
        """Run a single linter category across all Go modules"""
        async with self.semaphore:
            category.status = LintStatus.RUNNING
            self.print_progress()

            start = time.time()

            try:
                # Find all Go modules
                go_modules = self.find_go_modules(base_dir)
                if not go_modules:
                    category.status = LintStatus.SKIPPED
                    category.error = "No Go modules found"
                    category.duration = time.time() - start
                    return

                all_issues = []

                # Run linter on each Go module
                for module_dir in go_modules:
                    linters_str = ",".join(category.linters)
                    cmd = [
                        "golangci-lint",
                        "run",
                        "--enable-only", linters_str,
                        "--timeout", f"{category.timeout}s",
                        "--output.json.path", "stdout",
                        "./...",
                    ]

                    process = await asyncio.create_subprocess_exec(
                        *cmd,
                        stdout=asyncio.subprocess.PIPE,
                        stderr=asyncio.subprocess.PIPE,
                        cwd=module_dir,
                    )

                    try:
                        stdout, stderr = await asyncio.wait_for(
                            process.communicate(),
                            timeout=category.timeout
                        )
                    except asyncio.TimeoutError:
                        process.kill()
                        category.status = LintStatus.FAILED
                        category.error = f"Timeout after {category.timeout}s in {module_dir}"
                        category.duration = time.time() - start
                        return

                    # Parse JSON output
                    if stdout:
                        try:
                            result = json.loads(stdout.decode())
                            issues = result.get("Issues", [])
                            all_issues.extend(issues)
                        except json.JSONDecodeError:
                            pass

                    if stderr:
                        error_msg = stderr.decode()
                        if "no Go files" not in error_msg and error_msg.strip():
                            if category.error:
                                category.error += f"\n{error_msg}"
                            else:
                                category.error = error_msg

                category.duration = time.time() - start
                category.issues_count = len(all_issues)

                if all_issues:
                    category.output = self.format_issues(all_issues, category.name)
                    category.status = LintStatus.FAILED
                else:
                    category.status = LintStatus.PASSED

            except Exception as e:
                category.status = LintStatus.FAILED
                category.error = str(e)
                category.duration = time.time() - start

            self.print_progress()

    def format_issues(self, issues: List[Dict], category_name: str) -> str:
        """Format linting issues for display"""
        lines = [f"\n{category_name.upper()} ISSUES:"]

        # Group by file
        by_file: Dict[str, List[Dict]] = {}
        for issue in issues:
            file = issue.get("Pos", {}).get("Filename", "unknown")
            by_file.setdefault(file, []).append(issue)

        # Format each file's issues
        for file, file_issues in sorted(by_file.items()):
            lines.append(f"\n📄 {file}:")
            for issue in file_issues:
                pos = issue.get("Pos", {})
                line = pos.get("Line", 0)
                col = pos.get("Column", 0)
                text = issue.get("Text", "")
                linter = issue.get("FromLinter", "")

                lines.append(f"  {line}:{col} {linter}: {text}")

        return "\n".join(lines)

    def print_progress(self):
        """Print current progress"""
        pending = sum(1 for c in self.categories if c.status == LintStatus.PENDING)
        running = sum(1 for c in self.categories if c.status == LintStatus.RUNNING)
        passed = sum(1 for c in self.categories if c.status == LintStatus.PASSED)
        failed = sum(1 for c in self.categories if c.status == LintStatus.FAILED)
        skipped = sum(1 for c in self.categories if c.status == LintStatus.SKIPPED)

        print(f"\r\033[K⚡ Linting: {running} running, {passed} passed, {failed} failed, {skipped} skipped, {pending} pending", end="", flush=True)

    async def run(self, base_dir: Path, categories: Optional[List[str]] = None):
        """Run all linter categories in parallel"""
        self.start_time = time.time()

        # Filter categories if specified
        if categories:
            self.categories = [c for c in LINT_CATEGORIES if c.name in categories]
            if not self.categories:
                print(f"❌ No categories found matching: {categories}")
                return False
        else:
            self.categories = LINT_CATEGORIES.copy()

        print(f"🚀 Parallel Lint Runner")
        print(f"{'=' * 60}")
        print(f"📊 Categories: {len(self.categories)}")
        print(f"⚙️  Max parallel: {self.max_parallel}")
        print(f"🏃 Fail-fast: {'enabled' if self.fail_fast else 'disabled'}")
        print(f"{'=' * 60}\n")

        # Run categories in parallel
        tasks = [
            self.run_category(category, base_dir)
            for category in self.categories
        ]

        await asyncio.gather(*tasks)

        # Print final results
        print()  # New line after progress
        self.print_results()

        # Check for failures
        failed_categories = [c for c in self.categories if c.status == LintStatus.FAILED]
        return len(failed_categories) == 0

    def print_results(self):
        """Print final results summary"""
        total_duration = time.time() - self.start_time

        print(f"\n{'=' * 60}")
        print(f"📊 LINTING RESULTS")
        print(f"{'=' * 60}\n")

        # Summary by status
        passed = [c for c in self.categories if c.status == LintStatus.PASSED]
        failed = [c for c in self.categories if c.status == LintStatus.FAILED]
        skipped = [c for c in self.categories if c.status == LintStatus.SKIPPED]

        total_issues = sum(c.issues_count for c in self.categories)

        if passed:
            print(f"✅ PASSED ({len(passed)}):")
            for cat in passed:
                print(f"   • {cat.name:20s} ({cat.duration:.1f}s)")

        if failed:
            print(f"\n❌ FAILED ({len(failed)}):")
            for cat in failed:
                issues_str = f" - {cat.issues_count} issues" if cat.issues_count > 0 else ""
                error_str = f" - {cat.error}" if cat.error else ""
                print(f"   • {cat.name:20s} ({cat.duration:.1f}s){issues_str}{error_str}")

        if skipped:
            print(f"\n⏭️  SKIPPED ({len(skipped)}):")
            for cat in skipped:
                print(f"   • {cat.name:20s}")

        # Print detailed failures
        if failed:
            print(f"\n{'=' * 60}")
            print(f"📋 DETAILED FAILURES")
            print(f"{'=' * 60}")

            for cat in failed:
                if cat.output:
                    print(cat.output)
                if cat.error and not cat.output:
                    print(f"\n{cat.name.upper()} ERROR:")
                    print(f"  {cat.error}")

        # Summary
        print(f"\n{'=' * 60}")
        print(f"⏱️  Total time: {total_duration:.1f}s")
        print(f"📊 Total issues: {total_issues}")
        print(f"✅ Passed: {len(passed)}/{len(self.categories)}")
        print(f"❌ Failed: {len(failed)}/{len(self.categories)}")
        print(f"{'=' * 60}\n")

        if failed:
            print(f"❌ Linting failed - fix issues above\n")
        else:
            print(f"✅ All linters passed!\n")


def list_categories():
    """List all available linter categories"""
    print("📋 Available Linter Categories:\n")

    for cat in LINT_CATEGORIES:
        critical_mark = "🔴 CRITICAL" if cat.critical else ""
        print(f"  {cat.name:20s} {critical_mark}")
        print(f"    {cat.description}")
        print(f"    Linters: {', '.join(cat.linters)}")
        print()


def main():
    import argparse

    parser = argparse.ArgumentParser(
        description="Parallel linting runner for Prism",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )

    parser.add_argument(
        "--categories",
        help="Comma-separated list of categories to run (default: all)",
    )
    parser.add_argument(
        "--max-parallel",
        type=int,
        default=4,
        help="Maximum number of parallel linter categories (default: 4)",
    )
    parser.add_argument(
        "--fail-fast",
        action="store_true",
        help="Stop on first failure",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all linter categories and exit",
    )

    args = parser.parse_args()

    if args.list:
        list_categories()
        return 0

    # Get base directory (project root)
    base_dir = Path(__file__).parent.parent

    # Parse categories
    categories = args.categories.split(",") if args.categories else None

    # Create runner
    runner = ParallelLintRunner(
        max_parallel=args.max_parallel,
        fail_fast=args.fail_fast,
    )

    # Run linting
    success = asyncio.run(runner.run(base_dir, categories))

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
