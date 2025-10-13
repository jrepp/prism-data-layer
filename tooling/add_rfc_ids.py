#!/usr/bin/env python3
"""Add missing 'id' field to RFC frontmatter.

Usage:
    python3 tooling/add_rfc_ids.py
"""

import re
from pathlib import Path


def add_id_to_rfc(file_path: Path):
    """Add id field to RFC frontmatter if missing."""
    content = file_path.read_text()

    # Check if id field already exists
    if re.search(r"^id:", content, re.MULTILINE):
        print(f"  ✓ {file_path.name} already has id field")
        return False

    # Extract RFC number from filename (RFC-XXX-*.md)
    match = re.match(r"RFC-(\d+)-.*\.md", file_path.name)
    if not match:
        print(f"  ⚠ {file_path.name} doesn't match RFC-XXX-*.md pattern")
        return False

    rfc_num = match.group(1)
    id_value = f"rfc-{rfc_num}"

    # Find frontmatter and add id field after the opening ---
    if content.startswith("---\n"):
        # Add id as first field after ---
        new_content = content.replace("---\n", f"---\nid: {id_value}\n", 1)
        file_path.write_text(new_content)
        print(f"  ✓ Added id: {id_value} to {file_path.name}")
        return True
    print(f"  ✗ {file_path.name} doesn't have valid frontmatter")
    return False


def main():
    docs_dir = Path(__file__).parent.parent / "docs-cms" / "rfcs"
    rfc_files = sorted(docs_dir.glob("RFC-*.md"))

    print(f"Processing {len(rfc_files)} RFC files...\n")

    modified = 0
    for rfc_file in rfc_files:
        if add_id_to_rfc(rfc_file):
            modified += 1

    print(f"\n✅ Modified {modified} files")


if __name__ == "__main__":
    main()
