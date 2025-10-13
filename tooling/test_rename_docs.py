#!/usr/bin/env python3
"""Unit tests for documentation file renaming to match Docusaurus link style.

Tests the conversion logic before applying any actual file renames.

Usage:
    uv run pytest tooling/test_rename_docs.py -v
"""


import pytest


def extract_doc_prefix(filename: str) -> tuple[str, str, str]:
    """Extract prefix, number, and rest from a documentation filename.

    Args:
        filename: Original filename (e.g., "ADR-001-rust-for-proxy.md")

    Returns:
        Tuple of (prefix, number, rest) or (None, None, None) if not a doc file

    Examples:
        "ADR-001-rust-for-proxy.md" -> ("ADR", "001", "rust-for-proxy.md")
        "RFC-015-plugin-tests.md" -> ("RFC", "015", "plugin-tests.md")
        "MEMO-004-backend-guide.md" -> ("MEMO", "004", "backend-guide.md")
        "adr-001-rust-for-proxy.md" -> ("adr", "001", "rust-for-proxy.md")
        "README.md" -> (None, None, None)
    """
    import re

    # Match ADR-NNN, RFC-NNN, or MEMO-NNN at start of filename (case insensitive)
    match = re.match(r"^(ADR|RFC|MEMO)-(\d+)-(.+)$", filename, re.IGNORECASE)
    if not match:
        return (None, None, None)

    prefix = match.group(1)
    number = match.group(2)
    rest = match.group(3)

    return (prefix, number, rest)


def generate_new_filename(filename: str) -> str:
    """Generate new lowercase filename matching Docusaurus link style.

    Args:
        filename: Original filename (e.g., "ADR-001-rust-for-proxy.md")

    Returns:
        New filename with lowercase prefix (e.g., "adr-001-rust-for-proxy.md")
        Returns original filename if not a doc file.

    Examples:
        "ADR-001-rust-for-proxy.md" -> "adr-001-rust-for-proxy.md"
        "RFC-015-plugin-tests.md" -> "rfc-015-plugin-tests.md"
        "MEMO-004-backend-guide.md" -> "memo-004-backend-guide.md"
        "README.md" -> "README.md"
    """
    prefix, number, rest = extract_doc_prefix(filename)

    if prefix is None:
        # Not a doc file, return unchanged
        return filename

    # Convert prefix to lowercase
    new_prefix = prefix.lower()

    # Reconstruct filename
    return f"{new_prefix}-{number}-{rest}"


def should_rename(filename: str) -> bool:
    """Check if a file needs renaming.

    Args:
        filename: Filename to check

    Returns:
        True if file should be renamed, False otherwise
    """
    prefix, _, _ = extract_doc_prefix(filename)

    if prefix is None:
        return False

    # Check if prefix is uppercase (needs renaming)
    return prefix.isupper()


# Table-based tests using pytest parametrize
@pytest.mark.parametrize(("input_filename", "expected_output"), [
    # ADR files
    ("ADR-001-rust-for-proxy.md", "adr-001-rust-for-proxy.md"),
    ("ADR-050-topaz-policy-authorization.md", "adr-050-topaz-policy-authorization.md"),
    ("ADR-049-podman-container-optimization.md", "adr-049-podman-container-optimization.md"),

    # RFC files
    ("RFC-001-prism-architecture.md", "rfc-001-prism-architecture.md"),
    ("RFC-015-plugin-acceptance-test-framework.md", "rfc-015-plugin-acceptance-test-framework.md"),
    ("RFC-021-poc1-three-plugins-implementation.md", "rfc-021-poc1-three-plugins-implementation.md"),
    ("RFC-026-poc1-keyvalue-memstore-original.md", "rfc-026-poc1-keyvalue-memstore-original.md"),

    # MEMO files
    ("MEMO-001-wal-transaction-flow.md", "memo-001-wal-transaction-flow.md"),
    ("MEMO-008-vault-token-exchange-flow.md", "memo-008-vault-token-exchange-flow.md"),
    ("MEMO-009-topaz-local-authorizer-configuration.md", "memo-009-topaz-local-authorizer-configuration.md"),

    # Already lowercase (should return unchanged)
    ("adr-001-rust-for-proxy.md", "adr-001-rust-for-proxy.md"),
    ("rfc-015-plugin-tests.md", "rfc-015-plugin-tests.md"),
    ("memo-004-backend-guide.md", "memo-004-backend-guide.md"),

    # Non-doc files (should return unchanged)
    ("README.md", "README.md"),
    ("index.md", "index.md"),
    ("intro.md", "intro.md"),
    ("CHANGELOG.md", "CHANGELOG.md"),
    ("000-template.md", "000-template.md"),
])
def test_generate_new_filename(input_filename, expected_output):
    """Test filename generation with table of inputs and expected outputs."""
    result = generate_new_filename(input_filename)
    assert result == expected_output, f"Expected {expected_output}, got {result}"


@pytest.mark.parametrize(("input_filename", "expected_prefix", "expected_number", "expected_rest"), [
    # Valid doc files (uppercase)
    ("ADR-001-rust-for-proxy.md", "ADR", "001", "rust-for-proxy.md"),
    ("RFC-015-plugin-tests.md", "RFC", "015", "plugin-tests.md"),
    ("MEMO-004-backend-guide.md", "MEMO", "004", "backend-guide.md"),
    ("ADR-050-topaz.md", "ADR", "050", "topaz.md"),

    # Valid doc files (lowercase)
    ("adr-001-rust-for-proxy.md", "adr", "001", "rust-for-proxy.md"),
    ("rfc-015-plugin-tests.md", "rfc", "015", "plugin-tests.md"),
    ("memo-004-backend-guide.md", "memo", "004", "backend-guide.md"),

    # Invalid/non-doc files
    ("README.md", None, None, None),
    ("index.md", None, None, None),
    ("000-template.md", None, None, None),
    ("ADR-malformed.md", None, None, None),
    ("RFC.md", None, None, None),
])
def test_extract_doc_prefix(input_filename, expected_prefix, expected_number, expected_rest):
    """Test prefix extraction with table of inputs and expected outputs."""
    prefix, number, rest = extract_doc_prefix(input_filename)
    assert prefix == expected_prefix
    assert number == expected_number
    assert rest == expected_rest


@pytest.mark.parametrize(("input_filename", "should_rename_expected"), [
    # Uppercase prefixes (should rename)
    ("ADR-001-rust-for-proxy.md", True),
    ("RFC-015-plugin-tests.md", True),
    ("MEMO-004-backend-guide.md", True),

    # Lowercase prefixes (should not rename)
    ("adr-001-rust-for-proxy.md", False),
    ("rfc-015-plugin-tests.md", False),
    ("memo-004-backend-guide.md", False),

    # Non-doc files (should not rename)
    ("README.md", False),
    ("index.md", False),
    ("intro.md", False),
])
def test_should_rename(input_filename, should_rename_expected):
    """Test should_rename logic with table of inputs and expected outputs."""
    result = should_rename(input_filename)
    assert result == should_rename_expected


@pytest.mark.parametrize("input_filename", [
    "ADR-001-rust-for-proxy.md",
    "RFC-015-plugin-tests.md",
    "MEMO-004-backend-guide.md",
])
def test_renamed_file_matches_frontmatter_id(input_filename):
    """Test that renamed files match the expected frontmatter ID format."""
    new_filename = generate_new_filename(input_filename)

    # Extract the ID portion (prefix-number)
    prefix, number, _ = extract_doc_prefix(new_filename)
    expected_id = f"{prefix}-{number}"

    # Verify ID is lowercase
    assert expected_id.islower(), f"Generated ID {expected_id} should be lowercase"

    # Verify format matches frontmatter ID style
    import re
    assert re.match(r"^(adr|rfc|memo)-\d+$", expected_id), \
        f"Generated ID {expected_id} doesn't match frontmatter ID pattern"


def test_idempotent_renaming():
    """Test that renaming is idempotent (applying twice gives same result)."""
    test_cases = [
        "ADR-001-rust-for-proxy.md",
        "RFC-015-plugin-tests.md",
        "MEMO-004-backend-guide.md",
    ]

    for original in test_cases:
        first_rename = generate_new_filename(original)
        second_rename = generate_new_filename(first_rename)

        assert first_rename == second_rename, \
            f"Renaming should be idempotent: {original} -> {first_rename} -> {second_rename}"


def test_no_data_loss():
    """Test that renaming preserves all filename information."""
    test_cases = [
        "ADR-001-rust-for-proxy.md",
        "RFC-015-plugin-acceptance-test-framework.md",
        "MEMO-009-topaz-local-authorizer-configuration.md",
    ]

    for original in test_cases:
        renamed = generate_new_filename(original)

        # Extract parts from both
        orig_prefix, orig_num, orig_rest = extract_doc_prefix(original)
        new_prefix, new_num, new_rest = extract_doc_prefix(renamed)

        # Verify only case changed, not content
        assert orig_prefix.lower() == new_prefix, "Prefix content should be preserved"
        assert orig_num == new_num, "Number should be unchanged"
        assert orig_rest == new_rest, "Rest of filename should be unchanged"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
