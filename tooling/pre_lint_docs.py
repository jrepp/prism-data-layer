#!/usr/bin/env python3
"""
Pre-lint validation for documentation before pushing to GitHub.

This script validates:
1. Markdown link validity (internal references)
2. MDX compilation (checks for MDX syntax errors)
3. Docusaurus build warnings

Run this before pushing documentation changes to catch issues early.
"""

import subprocess
import sys
import re
from pathlib import Path
from typing import List, Tuple

# ANSI color codes
RED = '\033[91m'
GREEN = '\033[92m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'

def print_header(msg: str):
    print(f"\n{BLUE}{'='*80}{RESET}")
    print(f"{BLUE}{msg}{RESET}")
    print(f"{BLUE}{'='*80}{RESET}\n")

def print_error(msg: str):
    print(f"{RED}❌ {msg}{RESET}")

def print_warning(msg: str):
    print(f"{YELLOW}⚠️  {msg}{RESET}")

def print_success(msg: str):
    print(f"{GREEN}✅ {msg}{RESET}")

def check_mdx_special_chars(root_dir: Path) -> List[Tuple[str, int, str]]:
    """Check for problematic characters that break MDX parsing."""
    issues = []

    # MDX doesn't like unescaped < in markdown lists/text
    problematic_patterns = [
        (r'^\s*[-*]\s+.*<\d+', 'Unescaped < before number (use &lt; or backticks)'),
        (r':\s+<\d+', 'Unescaped < after colon (use &lt; or backticks)'),
    ]

    md_files = list(root_dir.glob('**/*.md'))

    for md_file in md_files:
        if '.docusaurus' in str(md_file) or 'node_modules' in str(md_file):
            continue

        with open(md_file, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                for pattern, issue_desc in problematic_patterns:
                    if re.search(pattern, line):
                        issues.append((str(md_file.relative_to(root_dir)), line_num, issue_desc))

    return issues

def check_internal_links(root_dir: Path) -> List[str]:
    """Check for broken internal markdown links."""
    issues = []

    # Patterns for internal links that might break in Docusaurus
    cross_plugin_pattern = re.compile(r'\[([^\]]+)\]\((\.\.\/){2,}[^)]+\)')

    md_files = list(root_dir.glob('**/*.md'))

    for md_file in md_files:
        if '.docusaurus' in str(md_file) or 'node_modules' in str(md_file):
            continue

        with open(md_file, 'r', encoding='utf-8') as f:
            content = f.read()
            matches = cross_plugin_pattern.findall(content)

            if matches:
                issues.append(
                    f"{md_file.relative_to(root_dir)}: Found {len(matches)} cross-plugin link(s) "
                    f"(use absolute GitHub URLs instead)"
                )

    return issues

def run_docusaurus_typecheck() -> Tuple[bool, str]:
    """Run TypeScript type checking on Docusaurus config."""
    try:
        result = subprocess.run(
            ['npm', 'run', 'typecheck'],
            capture_output=True,
            text=True,
            timeout=60
        )
        return result.returncode == 0, result.stderr
    except subprocess.TimeoutExpired:
        return False, "Typecheck timed out"
    except Exception as e:
        return False, str(e)

def run_docusaurus_build_check() -> Tuple[bool, List[str]]:
    """Run a Docusaurus build to check for MDX errors and warnings."""
    warnings = []

    try:
        # Run build and capture output
        result = subprocess.run(
            ['npm', 'run', 'build'],
            capture_output=True,
            text=True,
            timeout=300
        )

        # Parse output for warnings and errors
        output = result.stdout + result.stderr

        # Extract warnings
        warning_pattern = re.compile(r'Warning:\s+(.+)')
        for match in warning_pattern.finditer(output):
            warnings.append(match.group(1))

        # Check for build success
        if result.returncode != 0:
            # Extract error details
            error_pattern = re.compile(r'Error:\s+(.+)')
            errors = error_pattern.findall(output)
            return False, errors if errors else [output[-500:]]  # Last 500 chars if no specific error

        return True, warnings

    except subprocess.TimeoutExpired:
        return False, ["Build timed out after 5 minutes"]
    except Exception as e:
        return False, [str(e)]

def main():
    # Get repository root
    repo_root = Path(__file__).parent.parent
    docs_cms = repo_root / 'docs-cms'
    docusaurus_dir = repo_root / 'docusaurus'

    print_header("🔍 PRISM DOCUMENTATION PRE-LINT VALIDATION")

    # Change to docusaurus directory for npm commands
    import os
    original_dir = os.getcwd()
    os.chdir(docusaurus_dir)

    all_passed = True

    # 1. Check for MDX special characters
    print_header("1. Checking for MDX special characters...")
    mdx_issues = check_mdx_special_chars(docs_cms)

    if mdx_issues:
        print_error(f"Found {len(mdx_issues)} MDX syntax issue(s):")
        for file, line, issue in mdx_issues:
            print(f"  {file}:{line} - {issue}")
        all_passed = False
    else:
        print_success("No MDX syntax issues found")

    # 2. Check internal links
    print_header("2. Checking internal markdown links...")
    link_issues = check_internal_links(docs_cms)

    if link_issues:
        print_warning(f"Found {len(link_issues)} potential link issue(s):")
        for issue in link_issues:
            print(f"  {issue}")
        print_warning("Consider using absolute GitHub URLs for cross-plugin references")
    else:
        print_success("No problematic internal links found")

    # 3. TypeScript typecheck
    print_header("3. Running TypeScript typecheck...")
    typecheck_passed, typecheck_error = run_docusaurus_typecheck()

    if typecheck_passed:
        print_success("TypeScript typecheck passed")
    else:
        print_error(f"TypeScript typecheck failed:\n{typecheck_error}")
        all_passed = False

    # 4. Docusaurus build check
    print_header("4. Running Docusaurus build validation...")
    print("This may take a minute...")

    build_passed, build_output = run_docusaurus_build_check()

    if build_passed:
        print_success("Docusaurus build succeeded")
        if build_output:
            print_warning(f"Build completed with {len(build_output)} warning(s):")
            for warning in build_output[:5]:  # Show first 5 warnings
                print(f"  {warning}")
            if len(build_output) > 5:
                print(f"  ... and {len(build_output) - 5} more warnings")
    else:
        print_error("Docusaurus build failed:")
        for error in build_output:
            print(f"  {error}")
        all_passed = False

    # Restore original directory
    os.chdir(original_dir)

    # Final summary
    print_header("📊 VALIDATION SUMMARY")

    if all_passed:
        print_success("All validation checks passed!")
        print("\n✨ Documentation is ready to push to GitHub\n")
        return 0
    else:
        print_error("Some validation checks failed")
        print("\n❌ Please fix the issues above before pushing\n")
        return 1

if __name__ == '__main__':
    sys.exit(main())
