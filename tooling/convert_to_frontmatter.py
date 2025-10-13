#!/usr/bin/env python3
"""Convert ADR and RFC documents to use YAML frontmatter instead of inline metadata.

Usage:
    python tooling/convert_to_frontmatter.py
"""

import re
from pathlib import Path


def extract_metadata(content: str) -> tuple[dict[str, str], str]:
    """Extract metadata from markdown content and return (metadata, remaining_content)."""
    metadata = {}
    lines = content.split("\n")

    # Extract title from first heading
    title_match = re.match(r"^#\s+(.+)$", lines[0])
    if title_match:
        metadata["title"] = title_match.group(1)
        lines = lines[1:]

    # Extract bold metadata lines
    remaining_lines = []
    in_metadata = True

    for line in lines:
        if in_metadata:
            # Match **Key**: Value
            meta_match = re.match(r"^\*\*([^*]+)\*\*:\s+(.+)$", line)
            if meta_match:
                key = meta_match.group(1).lower().replace(" ", "_")
                value = meta_match.group(2)

                # Handle tags specially - convert to array
                if key == "tags":
                    # Split comma-separated tags
                    tags = [tag.strip() for tag in value.split(",")]
                    metadata[key] = tags
                else:
                    metadata[key] = value
                continue
            if line.strip() == "":
                # Empty line might separate metadata from content
                continue
            # Non-metadata line, stop looking for metadata
            in_metadata = False

        remaining_lines.append(line)

    # Remove leading empty lines from content
    while remaining_lines and remaining_lines[0].strip() == "":
        remaining_lines.pop(0)

    remaining_content = "\n".join(remaining_lines)
    return metadata, remaining_content


def format_frontmatter(metadata: dict[str, str]) -> str:
    """Format metadata as YAML frontmatter."""
    lines = ["---"]

    # Order: title, status, date, author/deciders, tags, other
    ordered_keys = ["title", "status", "date", "author", "deciders", "created", "updated", "tags"]

    for key in ordered_keys:
        if key in metadata:
            value = metadata[key]
            if isinstance(value, list):
                # Format as YAML array
                lines.append(f"{key}: {value}")
            # Quote strings that might have special chars
            elif ":" in value or "#" in value:
                lines.append(f'{key}: "{value}"')
            else:
                lines.append(f"{key}: {value}")

    # Add any remaining keys not in ordered list
    for key, value in metadata.items():
        if key not in ordered_keys:
            if isinstance(value, list):
                lines.append(f"{key}: {value}")
            elif ":" in value or "#" in value:
                lines.append(f'{key}: "{value}"')
            else:
                lines.append(f"{key}: {value}")

    lines.append("---")
    return "\n".join(lines)


def convert_file(file_path: Path) -> bool:
    """Convert a single file to frontmatter format. Returns True if changed."""
    content = file_path.read_text()

    # Skip if already has frontmatter
    if content.startswith("---\n"):
        print(f"  Skipping {file_path.name} (already has frontmatter)")
        return False

    # Extract metadata
    metadata, remaining_content = extract_metadata(content)

    if not metadata:
        print(f"  Skipping {file_path.name} (no metadata found)")
        return False

    # Format new content with frontmatter
    frontmatter = format_frontmatter(metadata)
    new_content = f"{frontmatter}\n\n{remaining_content}"

    # Write back
    file_path.write_text(new_content)
    print(f"  Converted {file_path.name}")
    return True


def main():
    """Convert all ADR and RFC files."""
    repo_root = Path(__file__).parent.parent

    # Convert ADRs
    print("Converting ADRs...")
    adr_dir = repo_root / "docs" / "adr"
    adr_count = 0
    for adr_file in sorted(adr_dir.glob("[0-9]*.md")):
        if convert_file(adr_file):
            adr_count += 1

    print(f"\nConverted {adr_count} ADR files")

    # Convert RFCs
    print("\nConverting RFCs...")
    docs_dir = repo_root / "docs"
    rfc_count = 0
    for rfc_file in sorted(docs_dir.glob("RFC-*.md")):
        if convert_file(rfc_file):
            rfc_count += 1

    print(f"\nConverted {rfc_count} RFC files")
    print(f"\nTotal: {adr_count + rfc_count} files converted")


if __name__ == "__main__":
    main()
