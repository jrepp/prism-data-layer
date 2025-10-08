#!/usr/bin/env python3
"""
Build Docusaurus documentation site.

This script automates the Docusaurus build process using npm.
The built site is output to the docs/ directory for GitHub Pages.

Usage:
    uv run tooling/build_docs.py
    uv run tooling/build_docs.py --clean
"""

import argparse
import shutil
import subprocess
import sys
from pathlib import Path


def run_command(cmd: list[str], cwd: Path, check: bool = True) -> subprocess.CompletedProcess:
    """Run a command and stream output."""
    print(f"Running: {' '.join(cmd)}")
    print(f"Working directory: {cwd}")

    result = subprocess.run(
        cmd,
        cwd=cwd,
        check=check,
        text=True,
        capture_output=False,
    )

    return result


def clean_build(repo_root: Path) -> None:
    """Clean previous build artifacts."""
    print("\n=== Cleaning previous build ===")

    # Clean docusaurus cache
    docusaurus_cache = repo_root / "docusaurus" / ".docusaurus"
    if docusaurus_cache.exists():
        print(f"Removing {docusaurus_cache}")
        shutil.rmtree(docusaurus_cache)

    # Note: We don't clean the docs/ output directory because it contains
    # source markdown files. The build will overwrite the built files.

    print("Clean complete")


def check_node() -> bool:
    """Check if Node.js is installed."""
    try:
        result = subprocess.run(
            ["node", "--version"],
            capture_output=True,
            text=True,
            check=True,
        )
        version = result.stdout.strip()
        print(f"Found Node.js {version}")
        return True
    except (subprocess.CalledProcessError, FileNotFoundError):
        print("ERROR: Node.js is not installed or not in PATH")
        print("Install Node.js from https://nodejs.org/")
        return False


def install_dependencies(docusaurus_dir: Path) -> bool:
    """Install npm dependencies."""
    print("\n=== Installing dependencies ===")

    package_lock = docusaurus_dir / "package-lock.json"
    if package_lock.exists():
        # Use npm ci for faster, reproducible installs
        result = run_command(["npm", "ci"], cwd=docusaurus_dir, check=False)
    else:
        # Fallback to npm install
        result = run_command(["npm", "install"], cwd=docusaurus_dir, check=False)

    return result.returncode == 0


def build_site(docusaurus_dir: Path) -> bool:
    """Build the Docusaurus site."""
    print("\n=== Building Docusaurus site ===")

    result = run_command(
        ["npm", "run", "build"],
        cwd=docusaurus_dir,
        check=False,
    )

    return result.returncode == 0


def verify_build(repo_root: Path) -> bool:
    """Verify the build output."""
    print("\n=== Verifying build output ===")

    docs_dir = repo_root / "docs"
    if not docs_dir.exists():
        print(f"ERROR: Output directory not found: {docs_dir}")
        return False

    # Check for key files that should exist
    index_html = docs_dir / "index.html"
    if not index_html.exists():
        print(f"ERROR: index.html not found in {docs_dir}")
        return False

    print(f"✓ Found {index_html}")

    # Check for ADR and RFC directories
    adr_dir = docs_dir / "adr"
    rfc_dir = docs_dir / "rfc"

    if adr_dir.exists():
        print(f"✓ Found ADR section: {adr_dir}")
    else:
        print(f"WARNING: ADR section not found: {adr_dir}")

    if rfc_dir.exists():
        print(f"✓ Found RFC section: {rfc_dir}")
    else:
        print(f"WARNING: RFC section not found: {rfc_dir}")

    print("\nBuild verification complete")
    return True


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Build Docusaurus documentation site",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Build the docs
    uv run tooling/build_docs.py

    # Clean and build
    uv run tooling/build_docs.py --clean
        """,
    )
    parser.add_argument(
        "--clean",
        action="store_true",
        help="Clean build artifacts before building",
    )
    parser.add_argument(
        "--skip-install",
        action="store_true",
        help="Skip npm dependency installation",
    )

    args = parser.parse_args()

    # Get repository root
    repo_root = Path(__file__).parent.parent
    docusaurus_dir = repo_root / "docusaurus"

    print(f"Repository root: {repo_root}")
    print(f"Docusaurus directory: {docusaurus_dir}")

    if not docusaurus_dir.exists():
        print(f"ERROR: Docusaurus directory not found: {docusaurus_dir}")
        return 1

    # Check Node.js
    if not check_node():
        return 1

    # Clean if requested
    if args.clean:
        clean_build(repo_root)

    # Install dependencies
    if not args.skip_install:
        if not install_dependencies(docusaurus_dir):
            print("\nERROR: Failed to install dependencies")
            return 1

    # Build site
    if not build_site(docusaurus_dir):
        print("\nERROR: Build failed")
        return 1

    # Verify build
    if not verify_build(repo_root):
        print("\nERROR: Build verification failed")
        return 1

    print("\n=== Build successful ===")
    print(f"Documentation built to: {repo_root / 'docs'}")
    print("\nTo preview locally:")
    print(f"  cd {docusaurus_dir}")
    print("  npm run serve")

    return 0


if __name__ == "__main__":
    sys.exit(main())
