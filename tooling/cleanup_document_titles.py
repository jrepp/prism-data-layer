#!/usr/bin/env python3
"""Clean up document titles to remove redundant ID prefixes.

This script removes the "PREFIX-NNN:" part from document titles in frontmatter
since the ID is already present in the 'id' field and will be displayed by
the sidebar presentation layer.

Usage:
    uv run tooling/cleanup_document_titles.py [--dry-run]

Examples:
    Before: title: "MEMO-001: WAL Transaction Flow"
    After:  title: "WAL Transaction Flow"

The ID prefix is removed from:
- ADRs (ADR-001, ADR-002, etc.)
- RFCs (RFC-001, RFC-002, etc.)
- MEMOs (MEMO-001, MEMO-002, etc.)
"""

import argparse
import re
import sys
from pathlib import Path

import frontmatter


def cleanup_title(title: str) -> tuple[str, bool]:
    """Remove document ID prefix from title.

    Args:
        title: Original title (e.g., "MEMO-001: Title")

    Returns:
        Tuple of (cleaned_title, was_modified)
    """
    # Match patterns like "ADR-001:", "RFC-010:", "MEMO-022:", etc.
    pattern = r"^([A-Z]+-\d+):\s*(.+)$"
    match = re.match(pattern, title)

    if match:
        return match.group(2).strip(), True

    return title, False


def process_file(file_path: Path, dry_run: bool = False) -> tuple[bool, str]:
    """Process a single markdown file to clean up its title.

    Args:
        file_path: Path to the markdown file
        dry_run: If True, don't write changes

    Returns:
        Tuple of (was_modified, message)
    """
    try:
        # Read file with frontmatter
        doc = frontmatter.load(file_path)

        if "title" not in doc:
            return False, f"No title field in {file_path}"

        original_title = doc["title"]
        cleaned_title, was_modified = cleanup_title(original_title)

        if not was_modified:
            return False, f"No change needed for {file_path.name}"

        if dry_run:
            return True, f"Would update {file_path.name}:\n  Before: {original_title}\n  After:  {cleaned_title}"

        # Update title
        doc["title"] = cleaned_title

        # Write back
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(frontmatter.dumps(doc))

        return True, f"Updated {file_path.name}: {original_title} → {cleaned_title}"

    except Exception as e:
        return False, f"Error processing {file_path}: {e}"


def process_directory(directory: Path, dry_run: bool = False) -> tuple[int, int]:
    """Process all markdown files in a directory.

    Args:
        directory: Directory to process
        dry_run: If True, don't write changes

    Returns:
        Tuple of (modified_count, total_count)
    """
    modified = 0
    total = 0

    # Find all .md files except templates and index
    md_files = [f for f in directory.glob("*.md") if f.name not in {"000-template.md", "index.md"}]

    print(f"\nProcessing {directory.name}/")
    print("-" * 60)

    for file_path in sorted(md_files):
        total += 1
        was_modified, message = process_file(file_path, dry_run)
        if was_modified:
            modified += 1
            print(f"✓ {message}")

    return modified, total


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Clean up document titles to remove redundant ID prefixes")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be changed without modifying files",
    )

    args = parser.parse_args()

    # Get repository root
    repo_root = Path(__file__).parent.parent
    docs_cms = repo_root / "docs-cms"

    if not docs_cms.exists():
        print(f"❌ docs-cms directory not found at {docs_cms}", file=sys.stderr)
        sys.exit(1)

    # Process each document type
    directories = [
        docs_cms / "adr",
        docs_cms / "rfcs",
        docs_cms / "memos",
    ]

    total_modified = 0
    total_processed = 0

    for directory in directories:
        if directory.exists():
            modified, processed = process_directory(directory, args.dry_run)
            total_modified += modified
            total_processed += processed
        else:
            print(f"⚠️  Directory not found: {directory}")

    # Summary
    print("\n" + "=" * 60)
    if args.dry_run:
        print(f"Dry run complete: {total_modified}/{total_processed} files would be modified")
    else:
        print(f"✅ Cleanup complete: {total_modified}/{total_processed} files modified")


if __name__ == "__main__":
    main()
