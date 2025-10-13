#!/usr/bin/env -S uv run python3
"""Add UUID to all documentation frontmatter for backend tracking.

This migration script adds a unique 'doc_uuid' field to all ADRs, RFCs, MEMOs,
and general documentation files that don't already have one.

⚠️ MUST use "uv run" - script requires python-frontmatter

Usage:
    uv run tooling/add_uuid_to_docs.py [--dry-run] [--verbose]

Options:
    --dry-run    Show what would be changed without modifying files
    --verbose    Show detailed progress
"""

import argparse
import sys
import uuid
from pathlib import Path

try:
    import frontmatter
except ImportError as e:
    print("\n❌ CRITICAL ERROR: Required dependencies not found", file=sys.stderr)
    print("   Missing: python-frontmatter", file=sys.stderr)
    print("\n   Fix:", file=sys.stderr)
    print("   $ uv sync", file=sys.stderr)
    print("\n   Then run with:", file=sys.stderr)
    print("   $ uv run tooling/add_uuid_to_docs.py", file=sys.stderr)
    print(f"\n   Error details: {e}\n", file=sys.stderr)
    sys.exit(2)


def add_uuid_to_file(file_path: Path, dry_run: bool = False, verbose: bool = False) -> bool:
    """Add doc_uuid to a markdown file's frontmatter.

    Returns True if the file was modified (or would be modified in dry-run mode).
    """
    try:
        # Parse frontmatter
        post = frontmatter.load(file_path)

        # Check if frontmatter exists
        if not post.metadata:
            if verbose:
                print(f"   ⊘ {file_path.name}: No frontmatter, skipping")
            return False

        # Check if doc_uuid already exists
        if "doc_uuid" in post.metadata:
            existing_uuid = post.metadata["doc_uuid"]
            if verbose:
                print(f"   ✓ {file_path.name}: Already has doc_uuid='{existing_uuid}'")
            return False

        # Generate new UUID
        new_uuid = str(uuid.uuid4())

        if dry_run:
            print(f"   [DRY-RUN] Would add doc_uuid='{new_uuid}' to {file_path.name}")
            return True
        # Add doc_uuid to frontmatter
        post.metadata["doc_uuid"] = new_uuid

        # Write back the file
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(frontmatter.dumps(post))

        print(f"   ✅ {file_path.name}: Added doc_uuid='{new_uuid}'")
        return True

    except Exception as e:
        print(f"   ✗ {file_path.name}: Error - {e}", file=sys.stderr)
        return False


def migrate_docs(repo_root: Path, dry_run: bool = False, verbose: bool = False):
    """Migrate all documentation files to include doc_uuid"""
    print("=" * 80)
    print("🔑 Add UUID to Documentation Frontmatter")
    print("=" * 80)
    print("\nPurpose: Add unique identifier for backend document tracking")
    print(f"Mode: {'DRY-RUN (no changes will be made)' if dry_run else 'WRITE (files will be modified)'}")
    print()

    docs_cms = repo_root / "docs-cms"
    if not docs_cms.exists():
        print(f"❌ ERROR: docs-cms directory not found: {docs_cms}", file=sys.stderr)
        sys.exit(1)

    # Track statistics
    total_files = 0
    modified_files = 0

    # Process ADRs
    print("📄 Processing ADRs...")
    adr_dir = docs_cms / "adr"
    if adr_dir.exists():
        for md_file in sorted(adr_dir.glob("*.md")):
            if md_file.name in ["README.md", "index.md"]:
                continue
            total_files += 1
            if add_uuid_to_file(md_file, dry_run, verbose):
                modified_files += 1

    # Process RFCs
    print("\n📄 Processing RFCs...")
    rfc_dir = docs_cms / "rfcs"
    if rfc_dir.exists():
        for md_file in sorted(rfc_dir.glob("*.md")):
            if md_file.name in ["README.md", "index.md"]:
                continue
            total_files += 1
            if add_uuid_to_file(md_file, dry_run, verbose):
                modified_files += 1

    # Process MEMOs
    print("\n📄 Processing MEMOs...")
    memo_dir = docs_cms / "memos"
    if memo_dir.exists():
        for md_file in sorted(memo_dir.glob("*.md")):
            if md_file.name in ["README.md", "index.md"]:
                continue
            total_files += 1
            if add_uuid_to_file(md_file, dry_run, verbose):
                modified_files += 1

    # Process general docs
    print("\n📄 Processing general docs...")
    for md_file in sorted(docs_cms.glob("*.md")):
        if md_file.name not in ["README.md"]:
            total_files += 1
            if add_uuid_to_file(md_file, dry_run, verbose):
                modified_files += 1

    # Summary
    print("\n" + "=" * 80)
    print("📊 Summary")
    print("=" * 80)
    print(f"Total files processed: {total_files}")
    print(f"Files {'that would be ' if dry_run else ''}modified: {modified_files}")
    print(f"Files already up-to-date: {total_files - modified_files}")

    if dry_run and modified_files > 0:
        print("\n💡 Tip: Run without --dry-run to apply changes")
    elif modified_files > 0:
        print("\n✅ Migration complete! Run validation to verify:")
        print("   uv run tooling/validate_docs.py")
    else:
        print("\n✅ All files already have doc_uuid field")

    print()


def main():
    parser = argparse.ArgumentParser(
        description="Add UUID to all documentation frontmatter for backend tracking",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Preview changes without modifying files
    uv run tooling/add_uuid_to_docs.py --dry-run

    # Apply changes with detailed output
    uv run tooling/add_uuid_to_docs.py --verbose

    # Apply changes (production)
    uv run tooling/add_uuid_to_docs.py

What this does:
    ✓ Generates unique UUID for each document
    ✓ Adds 'doc_uuid' field to frontmatter
    ✓ Preserves existing frontmatter and content
    ✓ Skips files that already have doc_uuid
    ✓ Enables backend document tracking and management
        """,
    )

    parser.add_argument("--dry-run", action="store_true", help="Preview changes without modifying files")

    parser.add_argument("--verbose", "-v", action="store_true", help="Show detailed progress")

    args = parser.parse_args()

    repo_root = Path(__file__).parent.parent
    try:
        migrate_docs(repo_root, dry_run=args.dry_run, verbose=args.verbose)
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ ERROR: {e}", file=sys.stderr)
        import traceback

        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
