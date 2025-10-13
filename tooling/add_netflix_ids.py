#!/usr/bin/env python3
"""Add missing 'id' field to Netflix docs frontmatter.

Usage:
    python3 tooling/add_netflix_ids.py
"""

import re
from pathlib import Path


def add_id_to_file(file_path: Path):
    """Add id field to frontmatter if missing."""
    content = file_path.read_text()

    # Check if id field already exists
    if re.search(r"^id:", content, re.MULTILINE):
        print(f"  ✓ {file_path.name} already has id field")
        return False

    # Generate id from filename (remove .md)
    id_value = f"netflix-{file_path.stem}"

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
    docs_dir = Path(__file__).parent.parent / "docs-cms" / "netflix"
    netflix_files = sorted(docs_dir.glob("*.md"))

    print(f"Processing {len(netflix_files)} Netflix files...\n")

    modified = 0
    for netflix_file in netflix_files:
        if add_id_to_file(netflix_file):
            modified += 1

    print(f"\n✅ Modified {modified} files")

if __name__ == "__main__":
    main()
