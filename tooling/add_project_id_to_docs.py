#!/usr/bin/env -S uv run python3
"""
Add project_id to all documentation frontmatter.

This migration script adds the 'project_id' field to all ADRs, RFCs, MEMOs,
and general documentation files that don't already have it.

⚠️ MUST use "uv run" - script requires python-frontmatter

Usage:
    uv run tooling/add_project_id_to_docs.py [--dry-run] [--verbose]

Options:
    --dry-run    Show what would be changed without modifying files
    --verbose    Show detailed progress
"""

import argparse
import sys
from pathlib import Path

try:
    import frontmatter
    import yaml
except ImportError as e:
    print("\n❌ CRITICAL ERROR: Required dependencies not found", file=sys.stderr)
    print("   Missing: python-frontmatter", file=sys.stderr)
    print("\n   Fix:", file=sys.stderr)
    print("   $ uv sync", file=sys.stderr)
    print("\n   Then run with:", file=sys.stderr)
    print("   $ uv run tooling/add_project_id_to_docs.py", file=sys.stderr)
    print(f"\n   Error details: {e}\n", file=sys.stderr)
    sys.exit(2)


def load_project_config(repo_root: Path) -> dict:
    """Load docs-project.yaml configuration"""
    config_path = repo_root / "docs-cms" / "docs-project.yaml"
    if not config_path.exists():
        print(f"❌ ERROR: Configuration file not found: {config_path}", file=sys.stderr)
        sys.exit(1)

    with open(config_path, 'r') as f:
        return yaml.safe_load(f)


def add_project_id_to_file(file_path: Path, project_id: str, dry_run: bool = False, verbose: bool = False) -> bool:
    """
    Add project_id to a markdown file's frontmatter.

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

        # Check if project_id already exists
        if 'project_id' in post.metadata:
            if verbose:
                existing_id = post.metadata['project_id']
                if existing_id == project_id:
                    print(f"   ✓ {file_path.name}: Already has correct project_id='{project_id}'")
                else:
                    print(f"   ⚠️  {file_path.name}: Has different project_id='{existing_id}' (expected '{project_id}')")
            return False

        # Add project_id
        post.metadata['project_id'] = project_id

        if dry_run:
            print(f"   [DRY-RUN] Would add project_id='{project_id}' to {file_path.name}")
            return True
        else:
            # Write back the file
            with open(file_path, 'w', encoding='utf-8') as f:
                f.write(frontmatter.dumps(post))

            print(f"   ✅ {file_path.name}: Added project_id='{project_id}'")
            return True

    except Exception as e:
        print(f"   ✗ {file_path.name}: Error - {e}", file=sys.stderr)
        return False


def migrate_docs(repo_root: Path, dry_run: bool = False, verbose: bool = False):
    """Migrate all documentation files to include project_id"""
    print("="*80)
    print("📦 Add project_id to Documentation Frontmatter")
    print("="*80)

    # Load configuration
    config = load_project_config(repo_root)
    project_id = config['project']['id']

    print(f"\nProject ID: '{project_id}'")
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
            if add_project_id_to_file(md_file, project_id, dry_run, verbose):
                modified_files += 1

    # Process RFCs
    print("\n📄 Processing RFCs...")
    rfc_dir = docs_cms / "rfcs"
    if rfc_dir.exists():
        for md_file in sorted(rfc_dir.glob("*.md")):
            if md_file.name in ["README.md", "index.md"]:
                continue
            total_files += 1
            if add_project_id_to_file(md_file, project_id, dry_run, verbose):
                modified_files += 1

    # Process MEMOs
    print("\n📄 Processing MEMOs...")
    memo_dir = docs_cms / "memos"
    if memo_dir.exists():
        for md_file in sorted(memo_dir.glob("*.md")):
            if md_file.name in ["README.md", "index.md"]:
                continue
            total_files += 1
            if add_project_id_to_file(md_file, project_id, dry_run, verbose):
                modified_files += 1

    # Process general docs
    print("\n📄 Processing general docs...")
    for md_file in sorted(docs_cms.glob("*.md")):
        if md_file.name not in ["README.md"]:
            total_files += 1
            if add_project_id_to_file(md_file, project_id, dry_run, verbose):
                modified_files += 1

    # Summary
    print("\n" + "="*80)
    print(f"📊 Summary")
    print("="*80)
    print(f"Total files processed: {total_files}")
    print(f"Files {'that would be ' if dry_run else ''}modified: {modified_files}")
    print(f"Files already up-to-date: {total_files - modified_files}")

    if dry_run and modified_files > 0:
        print(f"\n💡 Tip: Run without --dry-run to apply changes")
    elif modified_files > 0:
        print(f"\n✅ Migration complete! Run validation to verify:")
        print(f"   uv run tooling/validate_docs.py")
    else:
        print(f"\n✅ All files already have project_id field")

    print()


def main():
    parser = argparse.ArgumentParser(
        description="Add project_id to all documentation frontmatter",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Preview changes without modifying files
    uv run tooling/add_project_id_to_docs.py --dry-run

    # Apply changes with detailed output
    uv run tooling/add_project_id_to_docs.py --verbose

    # Apply changes (production)
    uv run tooling/add_project_id_to_docs.py

What this does:
    ✓ Reads project ID from docs-cms/docs-project.yaml
    ✓ Adds 'project_id' field to frontmatter of all docs
    ✓ Preserves existing frontmatter and content
    ✓ Skips files that already have project_id
        """
    )

    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Preview changes without modifying files'
    )

    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Show detailed progress'
    )

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


if __name__ == '__main__':
    main()
