#!/usr/bin/env python3
"""Add frontmatter to README.md and index.md files in docs-cms.

Usage:
    python3 tooling/add_readme_frontmatter.py
"""

from pathlib import Path

FRONTMATTER_TEMPLATES = {
    "adr/README.md": {
        "id": "adr-index",
        "title": "Architecture Decision Records",
        "sidebar_label": "ADR Index",
    },
    "adr/index.md": {
        "id": "adr-home",
        "title": "Architecture Decision Records Home",
        "sidebar_label": "ADR Home",
    },
    "rfcs/README.md": {
        "id": "rfc-index",
        "title": "Request for Comments",
        "sidebar_label": "RFC Index",
    },
    "memos/README.md": {
        "id": "memo-index",
        "title": "Technical Memos",
        "sidebar_label": "Memo Index",
    },
    "netflix/README.md": {
        "id": "netflix-index",
        "title": "Netflix Data Gateway Research",
        "sidebar_label": "Netflix Index",
    },
}


def has_frontmatter(content: str) -> bool:
    """Check if content already has frontmatter."""
    return content.startswith("---\n")


def add_frontmatter(file_path: Path, metadata: dict):
    """Add frontmatter to a file."""
    content = file_path.read_text()

    if has_frontmatter(content):
        print(f"  ✓ {file_path.relative_to(file_path.parent.parent.parent)} already has frontmatter")
        return False

    # Build frontmatter
    frontmatter_lines = ["---"]
    for key, value in metadata.items():
        frontmatter_lines.append(f'{key}: "{value}"')
    frontmatter_lines.append("---")
    frontmatter_lines.append("")

    new_content = "\n".join(frontmatter_lines) + content
    file_path.write_text(new_content)
    print(f"  ✓ Added frontmatter to {file_path.relative_to(file_path.parent.parent.parent)}")
    return True


def main():
    docs_dir = Path(__file__).parent.parent / "docs-cms"

    print("Adding frontmatter to README/index files...\n")

    modified = 0
    for file_rel_path, metadata in FRONTMATTER_TEMPLATES.items():
        file_path = docs_dir / file_rel_path
        if file_path.exists():
            if add_frontmatter(file_path, metadata):
                modified += 1
        else:
            print(f"  ⚠ {file_rel_path} doesn't exist")

    print(f"\n✅ Modified {modified} files")


if __name__ == "__main__":
    main()
