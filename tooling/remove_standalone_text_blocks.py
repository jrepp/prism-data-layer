#!/usr/bin/env python3
"""Remove standalone ```text lines that were mistakenly added.

These lines are not opening or closing actual code blocks - they're artifacts
from the previous fix attempt.

Usage:
    python3 tooling/remove_standalone_text_blocks.py
"""

from pathlib import Path


def remove_standalone_text_blocks(file_path: Path) -> int:
    """Remove standalone ```text lines from a file."""
    content = file_path.read_text()
    lines = content.split("\n")
    new_lines = []
    removed = 0

    for i, line in enumerate(lines):
        # Check if this is a standalone ```text line
        # (i.e., the line before and after are not part of a code block pattern)
        if line.strip() == "```text":
            # Check context - if previous line is not a code block content
            # and next line is not a code block content, this is likely standalone
            prev_line = lines[i-1] if i > 0 else ""
            next_line = lines[i+1] if i < len(lines)-1 else ""

            # If both surrounding lines are blank or markdown content (not code),
            # this is a standalone artifact
            if not prev_line.strip().startswith("```") and (not next_line.strip() or not next_line.startswith(" ")):
                removed += 1
                continue

        new_lines.append(line)

    if removed > 0:
        file_path.write_text("\n".join(new_lines))

    return removed

def main():
    """Remove standalone ```text blocks from all documentation."""
    docs_cms = Path(__file__).parent.parent / "docs-cms"

    directories = ["memos", "adr", "rfcs"]
    total_removed = 0
    total_files = 0

    for directory in directories:
        dir_path = docs_cms / directory
        if not dir_path.exists():
            continue

        for md_file in dir_path.glob("*.md"):
            if md_file.name in ["index.md", "000-template.md", "README.md"]:
                continue

            removed = remove_standalone_text_blocks(md_file)
            if removed > 0:
                print(f"✓ Removed {removed} standalone ```text blocks from {md_file.relative_to(docs_cms)}")
                total_removed += removed
                total_files += 1

    print(f"\n✅ Removed {total_removed} standalone ```text blocks across {total_files} files")

if __name__ == "__main__":
    main()
