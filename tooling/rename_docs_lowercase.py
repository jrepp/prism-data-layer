#!/usr/bin/env python3
"""Rename documentation files to match Docusaurus link style (lowercase prefixes).

Converts:
  - ADR-001-rust-for-proxy.md -> adr-001-rust-for-proxy.md
  - RFC-015-plugin-tests.md -> rfc-015-plugin-tests.md
  - MEMO-004-backend-guide.md -> memo-004-backend-guide.md

This ensures filenames match the frontmatter ID format (lowercase).

Usage:
    uv run tooling/rename_docs_lowercase.py [--dry-run] [--verbose]
"""

import argparse
import subprocess
import sys
from pathlib import Path

# Import tested functions from test file
from test_rename_docs import (
    generate_new_filename,
    should_rename,
)


def rename_files_in_directory(directory: Path, dry_run: bool = False, verbose: bool = False) -> dict:
    """Rename all documentation files in a directory to lowercase format.

    Args:
        directory: Directory to process
        dry_run: If True, only show what would be renamed
        verbose: If True, print detailed output

    Returns:
        Dictionary with statistics
    """
    stats = {
        "total_files": 0,
        "renamed": 0,
        "skipped": 0,
        "errors": 0,
    }

    if not directory.exists():
        print(f"❌ Error: Directory not found: {directory}")
        return stats

    # Find all markdown files
    md_files = list(directory.rglob("*.md"))
    md_files = [f for f in md_files if "template" not in f.name.lower()]

    if verbose:
        print(f"Found {len(md_files)} markdown files in {directory}")

    for md_file in md_files:
        stats["total_files"] += 1

        filename = md_file.name

        if not should_rename(filename):
            if verbose:
                print(f"  ⊘ Skip: {filename} (already lowercase or not a doc file)")
            stats["skipped"] += 1
            continue

        new_filename = generate_new_filename(filename)

        if new_filename == filename:
            # Should not happen if should_rename returned True
            stats["skipped"] += 1
            continue

        new_path = md_file.parent / new_filename

        if dry_run:
            print(f"  [DRY RUN] Would rename: {filename} -> {new_filename}")
            stats["renamed"] += 1
        else:
            try:
                # Use git mv for case-only renames (macOS is case-insensitive)
                # This works even when source and target are the same on case-insensitive FS
                result = subprocess.run(
                    ["git", "mv", str(md_file), str(new_path)], capture_output=True, text=True, check=False
                )

                if result.returncode == 0:
                    print(f"  ✓ Renamed: {filename} -> {new_filename}")
                    stats["renamed"] += 1
                # Git mv failed, try fallback with temporary name
                elif "already exists" in result.stderr or "are the same" in result.stderr.lower():
                    # Two-step rename for case-insensitive filesystems
                    temp_path = md_file.parent / f"_temp_{new_filename}"

                    # Step 1: Rename to temp
                    subprocess.run(["git", "mv", str(md_file), str(temp_path)], check=True)

                    # Step 2: Rename to final
                    subprocess.run(["git", "mv", str(temp_path), str(new_path)], check=True)

                    print(f"  ✓ Renamed (2-step): {filename} -> {new_filename}")
                    stats["renamed"] += 1
                else:
                    print(f"  ❌ Error renaming {filename}: {result.stderr}")
                    stats["errors"] += 1

            except Exception as e:
                print(f"  ❌ Error renaming {filename}: {e}")
                stats["errors"] += 1

    return stats


def print_summary(stats: dict, dry_run: bool):
    """Print summary of rename operations."""
    print("\n" + "=" * 80)
    print("📊 RENAME SUMMARY")
    print("=" * 80)
    print(f"Files scanned:  {stats['total_files']}")
    print(f"Files renamed:  {stats['renamed']}")
    print(f"Files skipped:  {stats['skipped']}")
    print(f"Errors:         {stats['errors']}")
    print("=" * 80)

    if stats["errors"] > 0:
        print("\n❌ Completed with errors")
        return False
    if stats["renamed"] > 0:
        if dry_run:
            print("\n✅ Dry run complete - no files were renamed")
            print("   Run without --dry-run to apply changes")
            print("\nNext steps:")
            print("1. Run: uv run tooling/rename_docs_lowercase.py")
            print("2. Validate: uv run tooling/validate_docs.py")
            print("3. Commit: git add -A && git commit")
        else:
            print("\n✅ Rename complete!")
            print("\nNext steps:")
            print("1. Review changes: git diff --name-status docs-cms/")
            print("2. Validate docs:   uv run tooling/validate_docs.py")
            print("3. Commit changes:  git add -A && git commit")
        return True
    print("\n✅ All files already in correct format!")
    return True


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Rename docs to match Docusaurus link style (lowercase)")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be renamed without modifying files")
    parser.add_argument("--verbose", "-v", action="store_true", help="Enable verbose output")
    parser.add_argument(
        "--path", type=Path, default=Path("docs-cms"), help="Path to documentation directory (default: docs-cms)"
    )

    args = parser.parse_args()

    if args.dry_run:
        print("🔍 DRY RUN MODE - No files will be modified\n")

    # Rename files
    stats = rename_files_in_directory(args.path, dry_run=args.dry_run, verbose=args.verbose)

    # Print summary
    success = print_summary(stats, args.dry_run)

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
