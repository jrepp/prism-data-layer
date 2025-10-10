#!/usr/bin/env python3
"""
Add missing 'id' field to ADR frontmatter.

Usage:
    python3 tooling/add_adr_ids.py
"""

import re
from pathlib import Path

def add_id_to_adr(file_path: Path):
    """Add id field to ADR frontmatter if missing."""
    content = file_path.read_text()

    # Check if id field already exists
    if re.search(r'^id:', content, re.MULTILINE):
        print(f"  ✓ {file_path.name} already has id field")
        return False

    # Extract ADR number from filename (ADR-XXX-*.md)
    match = re.match(r'ADR-(\d+)-.*\.md', file_path.name)
    if not match:
        print(f"  ⚠ {file_path.name} doesn't match ADR-XXX-*.md pattern")
        return False

    adr_num = match.group(1)
    id_value = f"adr-{adr_num}"

    # Find frontmatter and add id field after the opening ---
    if content.startswith('---\n'):
        # Add id as first field after ---
        new_content = content.replace('---\n', f'---\nid: {id_value}\n', 1)
        file_path.write_text(new_content)
        print(f"  ✓ Added id: {id_value} to {file_path.name}")
        return True
    else:
        print(f"  ✗ {file_path.name} doesn't have valid frontmatter")
        return False

def main():
    docs_dir = Path(__file__).parent.parent / "docs-cms" / "adr"
    adr_files = sorted(docs_dir.glob("ADR-*.md"))

    print(f"Processing {len(adr_files)} ADR files...\n")

    modified = 0
    for adr_file in adr_files:
        if add_id_to_adr(adr_file):
            modified += 1

    print(f"\n✅ Modified {modified} files")

if __name__ == "__main__":
    main()
