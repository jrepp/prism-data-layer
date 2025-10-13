#!/usr/bin/env python3
"""Comprehensive documentation migration script for Docusaurus consistency.

Ensures all documents follow the correct pattern:
1. Frontmatter IDs are lowercase (adr-001, rfc-015, memo-004)
2. Filenames match frontmatter ID format (ADR-001-name.md -> adr-001)
3. All internal links use absolute lowercase paths (/adr/adr-001)
4. Title uses proper case in frontmatter

Usage:
    uv run tooling/migrate_docs_format.py [--dry-run] [--verbose]
"""

import argparse
import re
import sys
from pathlib import Path

try:
    import frontmatter
except ImportError:
    print("❌ Error: python-frontmatter not installed")
    print("   This script must be run with: uv run tooling/migrate_docs_format.py")
    sys.exit(2)


class DocMigrator:
    """Migrates documentation to consistent Docusaurus format."""

    def __init__(self, dry_run: bool = False, verbose: bool = False) -> None:
        self.dry_run = dry_run
        self.verbose = verbose
        self.stats = {
            "files_scanned": 0,
            "files_modified": 0,
            "frontmatter_fixed": 0,
            "links_fixed": 0,
            "errors": 0,
        }

    def log(self, message: str, level: str = "info"):
        """Log a message based on verbosity."""
        if level == "error":
            print(f"❌ {message}", file=sys.stderr)
        elif level == "warning":
            print(f"⚠️  {message}")
        elif level == "success":
            print(f"✅ {message}")
        elif self.verbose:
            print(f"ℹ️  {message}")

    def extract_id_from_filename(self, filename: str) -> str | None:
        """Extract lowercase ID from filename.

        Examples:
            ADR-001-rust-for-proxy.md -> adr-001
            RFC-015-plugin-tests.md -> rfc-015
            MEMO-004-backend-guide.md -> memo-004
        """
        # Match ADR-NNN, RFC-NNN, or MEMO-NNN at start of filename
        match = re.match(r"^(ADR|RFC|MEMO)-(\d+)", filename, re.IGNORECASE)
        if match:
            prefix = match.group(1).lower()
            number = match.group(2)
            return f"{prefix}-{number}"
        return None

    def fix_frontmatter(self, post: frontmatter.Post, file_path: Path) -> bool:
        """Fix frontmatter to ensure lowercase ID and proper title format.
        Returns True if changes were made.
        """
        changed = False
        expected_id = self.extract_id_from_filename(file_path.name)

        if not expected_id:
            self.log(f"Skipping {file_path.name} - no standard ID format", "warning")
            return False

        # Fix or add 'id' field
        current_id = post.metadata.get("id")
        if current_id != expected_id:
            if current_id:
                self.log(f"  Fixing ID: {current_id} -> {expected_id}", "info")
            else:
                self.log(f"  Adding ID: {expected_id}", "info")
            post.metadata["id"] = expected_id
            changed = True

        # Ensure title exists and uses proper format
        title = post.metadata.get("title", "")
        if title:
            # Extract just the descriptive part if title includes ID
            # "ADR-001: Rust for Proxy" -> "ADR-001: Rust for Proxy"
            # "Rust for Proxy" -> "ADR-001: Rust for Proxy"
            title_upper = title.upper()
            prefix = expected_id.split("-")[0].upper()
            number = expected_id.split("-")[1]
            expected_prefix = f"{prefix}-{number}:"

            if not title.startswith(prefix):
                # Title missing the ID prefix
                new_title = f"{prefix}-{number}: {title}"
                self.log(f"  Fixing title: '{title}' -> '{new_title}'", "info")
                post.metadata["title"] = new_title
                changed = True
            elif title_upper.startswith(f"{prefix}-{number}") and not title.startswith(expected_prefix):
                # Title has ID but wrong case
                rest_of_title = re.sub(f"^{prefix}-{number}:?\\s*", "", title, flags=re.IGNORECASE)
                new_title = f"{expected_prefix} {rest_of_title}"
                self.log(f"  Normalizing title case: '{title}' -> '{new_title}'", "info")
                post.metadata["title"] = new_title
                changed = True

        return changed

    def fix_links(self, content: str) -> tuple[str, int]:
        """Fix all internal links to use absolute lowercase format.
        Returns (fixed_content, number_of_fixes).
        """
        fixes = 0

        # Fix relative markdown links with .md extension
        # ./ADR-XXX-anything.md -> /adr/adr-XXX
        def fix_relative_adr(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            return f"](/adr/adr-{num})"

        content = re.sub(r"\]\(\.\.?/(?:adr/)?ADR-(\d+)[^)]*\.md\)", fix_relative_adr, content, flags=re.IGNORECASE)

        # RFC links
        def fix_relative_rfc(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            return f"](/rfc/rfc-{num})"

        content = re.sub(r"\]\(\.\.?/(?:rfcs?/)?RFC-(\d+)[^)]*\.md\)", fix_relative_rfc, content, flags=re.IGNORECASE)

        # MEMO links
        def fix_relative_memo(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            return f"](/memos/memo-{num})"

        content = re.sub(r"\]\(\.\.?/(?:memos/)?MEMO-(\d+)[^)]*\.md\)", fix_relative_memo, content, flags=re.IGNORECASE)

        # Fix uppercase in existing absolute links
        # /adr/ADR-XXX -> /adr/adr-XXX
        def fix_case_adr(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            rest = match.group(2) if match.lastindex >= 2 else ""
            return f"/adr/adr-{num}{rest}"

        content = re.sub(r"/adr/ADR-(\d+)([^0-9]|$)", fix_case_adr, content)

        # /rfc/RFC-XXX -> /rfc/rfc-XXX
        def fix_case_rfc(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            rest = match.group(2) if match.lastindex >= 2 else ""
            return f"/rfc/rfc-{num}{rest}"

        content = re.sub(r"/rfc/RFC-(\d+)([^0-9]|$)", fix_case_rfc, content)

        # /memos/MEMO-XXX -> /memos/memo-XXX
        def fix_case_memo(match):
            nonlocal fixes
            fixes += 1
            num = match.group(1)
            rest = match.group(2) if match.lastindex >= 2 else ""
            return f"/memos/memo-{num}{rest}"

        content = re.sub(r"/memos/MEMO-(\d+)([^0-9]|$)", fix_case_memo, content)

        return content, fixes

    def migrate_file(self, file_path: Path) -> bool:
        """Migrate a single file to the correct format.
        Returns True if file was modified.
        """
        self.stats["files_scanned"] += 1

        try:
            # Parse frontmatter
            post = frontmatter.load(file_path)

            # Check if file has frontmatter
            if not post.metadata:
                self.log(f"Skipping {file_path.name} - no frontmatter", "warning")
                return False

            # Track changes
            frontmatter_changed = self.fix_frontmatter(post, file_path)
            content_fixed, links_fixed = self.fix_links(post.content)

            if frontmatter_changed:
                self.stats["frontmatter_fixed"] += 1

            if links_fixed > 0:
                self.stats["links_fixed"] += links_fixed
                post.content = content_fixed

            # Write changes if needed
            if frontmatter_changed or links_fixed > 0:
                if self.dry_run:
                    self.log(
                        f"Would update {file_path.name} (frontmatter: {frontmatter_changed}, links: {links_fixed})",
                        "info",
                    )
                else:
                    # Write file with frontmatter
                    output = frontmatter.dumps(post)
                    file_path.write_text(output, encoding="utf-8")
                    self.log(
                        f"Updated {file_path.name} (frontmatter: {frontmatter_changed}, links: {links_fixed})",
                        "success",
                    )

                self.stats["files_modified"] += 1
                return True

            return False

        except Exception as e:
            self.log(f"Error processing {file_path.name}: {e}", "error")
            self.stats["errors"] += 1
            return False

    def migrate_directory(self, directory: Path):
        """Migrate all markdown files in a directory."""
        if not directory.exists():
            self.log(f"Directory not found: {directory}", "error")
            return

        # Find all markdown files
        md_files = sorted(directory.rglob("*.md"))

        # Filter out template files
        md_files = [f for f in md_files if "template" not in f.name.lower()]

        if not md_files:
            self.log(f"No markdown files found in {directory}", "warning")
            return

        # Get relative path for display
        try:
            display_path = directory.relative_to(Path.cwd())
        except ValueError:
            display_path = directory

        self.log(f"Processing {len(md_files)} files in {display_path}...")

        for md_file in md_files:
            if self.verbose or self.dry_run:
                print(f"\n{'[DRY RUN] ' if self.dry_run else ''}Processing: {md_file.relative_to(directory)}")
            self.migrate_file(md_file)

    def print_summary(self):
        """Print migration summary."""
        print("\n" + "=" * 80)
        print("📊 MIGRATION SUMMARY")
        print("=" * 80)
        print(f"Files scanned:      {self.stats['files_scanned']}")
        print(f"Files modified:     {self.stats['files_modified']}")
        print(f"Frontmatter fixed:  {self.stats['frontmatter_fixed']}")
        print(f"Links fixed:        {self.stats['links_fixed']}")
        print(f"Errors:             {self.stats['errors']}")
        print("=" * 80)

        if self.stats["errors"] > 0:
            print("\n❌ Migration completed with errors")
            return False
        if self.stats["files_modified"] > 0:
            if self.dry_run:
                print("\n✅ Dry run complete - no files were modified")
                print("   Run without --dry-run to apply changes")
            else:
                print("\n✅ Migration complete!")
                print("\nNext steps:")
                print("1. Review changes: git diff docs-cms/")
                print("2. Validate docs:   uv run tooling/validate_docs.py")
                print("3. Commit changes:  git add docs-cms/ && git commit")
            return True
        print("\n✅ All documents already in correct format!")
        return True


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Migrate documentation to consistent Docusaurus format")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be changed without modifying files")
    parser.add_argument("--verbose", "-v", action="store_true", help="Enable verbose output")
    parser.add_argument(
        "--path", type=Path, default=Path("docs-cms"), help="Path to documentation directory (default: docs-cms)"
    )

    args = parser.parse_args()

    # Create migrator
    migrator = DocMigrator(dry_run=args.dry_run, verbose=args.verbose)

    # Migrate documentation
    if args.dry_run:
        print("🔍 DRY RUN MODE - No files will be modified\n")

    migrator.migrate_directory(args.path)
    success = migrator.print_summary()

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
