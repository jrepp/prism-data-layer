#!/usr/bin/env python3
"""Comprehensive fix for all code fence errors in MEMO-036.

Fixes:
1. Unlabeled shell script blocks (``` should be ```bash)
2. Unlabeled YAML/text blocks (``` should be ```yaml or ```text)
3. Closing fences with extra text (```text should be ```)
4. Go code blocks (``` should be ```go)
"""

import sys
from pathlib import Path


def fix_all_code_fences(content: str) -> tuple[str, int]:
    """Fix all code fence errors comprehensively.

    Strategy:
    1. Track code block state
    2. For opening fences without labels, infer label from content
    3. For closing fences with labels, remove the label

    Returns:
        Tuple of (fixed_content, number_of_fixes)
    """
    lines = content.split("\n")
    fixed_lines = []
    fixes = 0
    in_code_block = False

    for i, line in enumerate(lines):
        stripped = line.strip()

        # Check if this is a code fence line
        if stripped.startswith("```"):
            fence_content = stripped[3:].strip()

            if not in_code_block:
                # Opening fence
                if not fence_content:
                    # No language label - need to infer from next lines
                    # Look ahead to determine appropriate label
                    label = infer_language_label(lines, i)
                    indent = len(line) - len(line.lstrip())
                    fixed_lines.append(" " * indent + f"```{label}")
                    fixes += 1
                    in_code_block = True
                else:
                    # Has language label - keep as is
                    fixed_lines.append(line)
                    in_code_block = True
            # Closing fence
            elif fence_content:
                # Closing fence has extra text - remove it
                indent = len(line) - len(line.lstrip())
                fixed_lines.append(" " * indent + "```")
                fixes += 1
                in_code_block = False
            else:
                # Clean closing fence - keep as is
                fixed_lines.append(line)
                in_code_block = False
        else:
            fixed_lines.append(line)

    return "\n".join(fixed_lines), fixes


def infer_language_label(lines: list[str], fence_index: int) -> str:
    """Infer the appropriate language label based on the code block content.

    Returns appropriate label: bash, yaml, go, text
    """
    # Look at next 5 lines to determine language
    next_lines = lines[fence_index + 1 : fence_index + 6]
    content = "\n".join(next_lines).lower()

    # Check for shell script patterns
    shell_patterns = [
        "curl",
        "kubectl",
        "helm",
        "make",
        "cd ",
        "export",
        "brew install",
        "sudo",
        "chmod",
        "mkdir",
        "#!/bin",
        "go run",
        "docker",
        "git ",
        "npm ",
        "go version",
    ]
    if any(pattern in content for pattern in shell_patterns):
        return "bash"

    # Check for YAML patterns
    yaml_patterns = ["apiversion:", "kind:", "metadata:", "spec:", "- name:"]
    if any(pattern in content for pattern in yaml_patterns):
        return "yaml"

    # Check for Go patterns
    go_patterns = ["package ", "import (", "func ", "type ", "return "]
    if any(pattern in content for pattern in go_patterns):
        return "go"

    # Check for output/plain text patterns
    text_patterns = ["name ", "ready", "status", "age", "expected output", "#"]
    if any(pattern in content for pattern in text_patterns):
        return "text"

    # Default to text
    return "text"


def main():
    memo_path = Path("/Users/jrepp/dev/data-access/docs-cms/memos/MEMO-036-kubernetes-operator-development.md")

    if not memo_path.exists():
        print(f"❌ File not found: {memo_path}")
        return 1

    print(f"📖 Reading {memo_path.name}...")
    content = memo_path.read_text()

    print("🔧 Fixing all code fence errors comprehensively...")
    fixed_content, num_fixes = fix_all_code_fences(content)

    if num_fixes == 0:
        print("✅ No code fence errors found!")
        return 0

    print(f"📝 Applied {num_fixes} fixes")

    # Write fixed content
    memo_path.write_text(fixed_content)
    print(f"✅ Fixed {memo_path.name}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
