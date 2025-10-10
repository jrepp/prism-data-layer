#!/usr/bin/env -S uv run python3
"""
Document Validation and Link Checker for Prism

Validates markdown documents for:
- YAML frontmatter format and required fields
- Internal link reachability
- Markdown formatting issues
- Consistent ADR/RFC numbering
- MDX compilation compatibility
- Docusaurus build validation

⚠️ CRITICAL: Run this before pushing documentation changes!
⚠️ MUST use "uv run" - script requires pydantic and python-frontmatter

Usage:
    uv run tooling/validate_docs.py
    uv run tooling/validate_docs.py --verbose
    uv run tooling/validate_docs.py --skip-build  # Skip Docusaurus build check
    uv run tooling/validate_docs.py --fix  # Auto-fix issues where possible

    OR run directly (shebang will invoke uv):
    ./tooling/validate_docs.py

Exit Codes:
    0 - All documents valid
    1 - Validation errors found
    2 - Missing dependencies (must use "uv run")
"""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Dict, List, Set, Tuple
from dataclasses import dataclass, field
from enum import Enum

try:
    import frontmatter
    from pydantic import ValidationError
    from tooling.doc_schemas import (
        ADRFrontmatter, RFCFrontmatter, MemoFrontmatter, GenericDocFrontmatter
    )
    ENHANCED_VALIDATION = True
except ImportError as e:
    print("\n❌ CRITICAL ERROR: Required dependencies not found", file=sys.stderr)
    print("   Missing: python-frontmatter and/or pydantic", file=sys.stderr)
    print("   These are REQUIRED for proper frontmatter validation.", file=sys.stderr)
    print("\n   Fix:", file=sys.stderr)
    print("   $ uv sync", file=sys.stderr)
    print("\n   Then run validation with:", file=sys.stderr)
    print("   $ uv run tooling/validate_docs.py", file=sys.stderr)
    print(f"\n   Error details: {e}\n", file=sys.stderr)
    sys.exit(2)


class LinkType(Enum):
    """Types of links in markdown documents"""
    INTERNAL_DOC = "internal_doc"      # ./relative.md or /docs/path.md
    INTERNAL_ADR = "internal_adr"      # ADR cross-references
    INTERNAL_RFC = "internal_rfc"      # RFC cross-references
    DOCUSAURUS_PLUGIN = "docusaurus_plugin"  # Cross-plugin links (e.g., /prism-data-layer/netflix/...)
    EXTERNAL = "external"              # http(s)://
    ANCHOR = "anchor"                  # #section
    UNKNOWN = "unknown"


@dataclass
class Document:
    """Represents a Prism documentation file"""
    file_path: Path
    doc_type: str  # "adr", "rfc", or "doc"
    title: str
    status: str = ""
    date: str = ""
    tags: List[str] = field(default_factory=list)
    links: List['Link'] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)

    def __hash__(self):
        return hash(str(self.file_path))


@dataclass
class Link:
    """Represents a link in a document"""
    source_doc: Path
    target: str
    line_number: int
    link_type: LinkType
    is_valid: bool = False
    error_message: str = ""

    def __str__(self):
        status = "✓" if self.is_valid else "✗"
        return f"{status} {self.source_doc.name}:{self.line_number} -> {self.target}"


class PrismDocValidator:
    """Validates Prism documentation"""

    def __init__(self, repo_root: Path, verbose: bool = False, fix: bool = False):
        self.repo_root = repo_root.resolve()
        self.verbose = verbose
        self.fix = fix
        self.documents: List[Document] = []
        self.file_to_doc: Dict[Path, Document] = {}
        self.all_links: List[Link] = []
        self.errors: List[str] = []

    def log(self, message: str, force: bool = False):
        """Log if verbose or forced"""
        if self.verbose or force:
            print(message)

    def scan_documents(self):
        """Scan all markdown files"""
        self.log("\n📂 Scanning documents...")

        # Filename patterns (adr-NNN-name-with-dashes.md or ADR-NNN-name-with-dashes.md)
        # Accept both lowercase (new standard) and uppercase (legacy) formats
        adr_pattern = re.compile(r'^(adr|ADR)-(\d{3})-(.+)\.md$', re.IGNORECASE)
        rfc_pattern = re.compile(r'^(rfc|RFC)-(\d{3})-(.+)\.md$', re.IGNORECASE)
        memo_pattern = re.compile(r'^(memo|MEMO)-(\d{3})-(.+)\.md$', re.IGNORECASE)

        # Scan ADRs
        adr_dir = self.repo_root / "docs-cms" / "adr"
        if adr_dir.exists():
            for md_file in sorted(adr_dir.glob("*.md")):
                # Skip README and index files (landing pages)
                if md_file.name in ["README.md", "index.md"]:
                    continue

                match = adr_pattern.match(md_file.name)
                if not match:
                    self.errors.append(f"Invalid ADR filename: {md_file.name} (expected: adr-NNN-name-with-dashes.md or ADR-NNN-name-with-dashes.md)")
                    self.log(f"   ✗ {md_file.name}: Invalid filename format")
                    continue

                prefix, num, slug = match.groups()
                # Skip template files (000)
                if num == "000":
                    self.log(f"   ⊘ {md_file.name}: Skipping template file")
                    continue

                doc = self._parse_document(md_file, "adr")
                if doc:
                    self.documents.append(doc)
                    self.file_to_doc[md_file] = doc

        # Scan RFCs
        rfc_dir = self.repo_root / "docs-cms" / "rfcs"
        if rfc_dir.exists():
            for md_file in sorted(rfc_dir.glob("*.md")):
                # Skip index files and validate filename format
                if md_file.name in ["README.md", "index.md"]:
                    continue

                match = rfc_pattern.match(md_file.name)
                if not match:
                    self.errors.append(f"Invalid RFC filename: {md_file.name} (expected: rfc-NNN-name-with-dashes.md or RFC-NNN-name-with-dashes.md)")
                    self.log(f"   ✗ {md_file.name}: Invalid filename format")
                    continue

                prefix, num, slug = match.groups()
                # Skip template files (000)
                if num == "000":
                    self.log(f"   ⊘ {md_file.name}: Skipping template file")
                    continue

                doc = self._parse_document(md_file, "rfc")
                if doc:
                    self.documents.append(doc)
                    self.file_to_doc[md_file] = doc

        # Scan MEMOs
        memo_dir = self.repo_root / "docs-cms" / "memos"
        if memo_dir.exists():
            for md_file in sorted(memo_dir.glob("*.md")):
                # Skip index files and validate filename format
                if md_file.name in ["README.md", "index.md"]:
                    continue

                match = memo_pattern.match(md_file.name)
                if not match:
                    self.errors.append(f"Invalid MEMO filename: {md_file.name} (expected: memo-NNN-name-with-dashes.md or MEMO-NNN-name-with-dashes.md)")
                    self.log(f"   ✗ {md_file.name}: Invalid filename format")
                    continue

                prefix, num, slug = match.groups()
                # Skip template files (000)
                if num == "000":
                    self.log(f"   ⊘ {md_file.name}: Skipping template file")
                    continue

                doc = self._parse_document(md_file, "memo")
                if doc:
                    self.documents.append(doc)
                    self.file_to_doc[md_file] = doc

        # Scan general docs
        docs_dir = self.repo_root / "docs-cms"
        if docs_dir.exists():
            for md_file in docs_dir.glob("*.md"):
                if md_file.name not in ["README.md"]:
                    doc = self._parse_document(md_file, "doc")
                    if doc:
                        self.documents.append(doc)
                        self.file_to_doc[md_file] = doc

        self.log(f"   Found {len(self.documents)} documents")

    def _parse_document(self, file_path: Path, doc_type: str) -> Document | None:
        """Parse a markdown file and validate frontmatter"""
        return self._parse_document_enhanced(file_path, doc_type)

    def _parse_document_enhanced(self, file_path: Path, doc_type: str) -> Document | None:
        """Parse document with python-frontmatter and pydantic validation"""
        try:
            # Parse frontmatter
            post = frontmatter.load(file_path)

            if not post.metadata:
                error = "Missing YAML frontmatter"
                self.log(f"   ✗ {file_path.name}: {error}")
                doc = Document(file_path=file_path, doc_type=doc_type, title="Unknown")
                doc.errors.append(error)
                return doc

            # Validate against schema
            schema = None
            try:
                if doc_type == "adr":
                    schema = ADRFrontmatter(**post.metadata)
                elif doc_type == "rfc":
                    schema = RFCFrontmatter(**post.metadata)
                elif doc_type == "memo":
                    schema = MemoFrontmatter(**post.metadata)
                else:
                    # Generic validation for other docs
                    schema = GenericDocFrontmatter(**post.metadata)

            except ValidationError as e:
                # Pydantic validation errors - very detailed
                doc = Document(
                    file_path=file_path,
                    doc_type=doc_type,
                    title=post.metadata.get('title', 'Unknown'),
                    status=post.metadata.get('status', ''),
                    date=str(post.metadata.get('date', post.metadata.get('created', ''))),
                    tags=post.metadata.get('tags', [])
                )

                for error in e.errors():
                    field = '.'.join(str(loc) for loc in error['loc'])
                    msg = error['msg']
                    error_type = error['type']

                    # Format user-friendly error message
                    if error_type == 'literal_error':
                        # Extract allowed values from message
                        doc.errors.append(f"Frontmatter field '{field}': {msg}")
                    else:
                        doc.errors.append(f"Frontmatter field '{field}': {msg}")

                    self.log(f"   ✗ {file_path.name}: {field} - {msg}")

                return doc

            # Success - create document
            doc = Document(
                file_path=file_path,
                doc_type=doc_type,
                title=post.metadata.get('title', 'Unknown'),
                status=post.metadata.get('status', ''),
                date=str(post.metadata.get('date', post.metadata.get('created', ''))),
                tags=post.metadata.get('tags', [])
            )

            self.log(f"   ✓ {file_path.name}: {doc.title}")
            return doc

        except Exception as e:
            self.errors.append(f"Error parsing {file_path}: {e}")
            self.log(f"   ✗ {file_path.name}: {e}")
            return None

    def extract_links(self):
        """Extract all links from documents"""
        self.log("\n🔗 Extracting links...")

        for doc in self.documents:
            links = self._extract_links_from_file(doc.file_path)
            doc.links = links
            self.all_links.extend(links)

        self.log(f"   Found {len(self.all_links)} total links")

    def _extract_links_from_file(self, file_path: Path) -> List[Link]:
        """Extract markdown links from a file"""
        links = []

        try:
            content = file_path.read_text(encoding='utf-8')
            lines = content.split('\n')

            in_code_fence = False
            code_fence_pattern = re.compile(r'^```')
            link_pattern = re.compile(r'\[([^\]]+)\]\(([^)]+)\)')

            for line_num, line in enumerate(lines, start=1):
                # Toggle code fence
                if code_fence_pattern.match(line):
                    in_code_fence = not in_code_fence
                    continue

                if in_code_fence:
                    continue

                # Remove inline code
                line_without_code = re.sub(r'`[^`]+`', '', line)

                for match in link_pattern.finditer(line_without_code):
                    link_target = match.group(2)

                    # Skip mailto and data links
                    if link_target.startswith(('mailto:', 'data:')):
                        continue

                    link_type = self._classify_link(link_target, file_path)

                    link = Link(
                        source_doc=file_path,
                        target=link_target,
                        line_number=line_num,
                        link_type=link_type
                    )
                    links.append(link)

        except Exception as e:
            self.errors.append(f"Error extracting links from {file_path}: {e}")

        return links

    def _classify_link(self, target: str, source_path: Path) -> LinkType:
        """Classify link by target"""
        if target.startswith(('http://', 'https://')):
            return LinkType.EXTERNAL
        elif target.startswith('#'):
            return LinkType.ANCHOR
        elif target.startswith('/prism-data-layer/'):
            # Docusaurus cross-plugin links (e.g., /prism-data-layer/netflix/scale)
            return LinkType.DOCUSAURUS_PLUGIN
        elif target.startswith(('/adr/', '/rfc/', '/memos/', '/docs/', '/netflix/')):
            # Docusaurus plugin routes (e.g., /adr/ADR-046, /rfc/RFC-001, /memos/MEMO-003)
            return LinkType.DOCUSAURUS_PLUGIN
        elif 'adr/' in target or target.startswith('./') and 'docs/adr' in str(source_path):
            return LinkType.INTERNAL_ADR
        elif 'rfc' in target.lower() or target.startswith('./') and 'docs/rfcs' in str(source_path):
            return LinkType.INTERNAL_RFC
        elif target.endswith('.md') or target.startswith(('./', '../')):
            return LinkType.INTERNAL_DOC
        else:
            return LinkType.UNKNOWN

    def validate_links(self):
        """Validate all links"""
        self.log("\n✓ Validating links...")

        for link in self.all_links:
            if link.link_type == LinkType.EXTERNAL:
                link.is_valid = True
                continue

            if link.link_type == LinkType.ANCHOR:
                link.is_valid = True
                continue

            if link.link_type == LinkType.DOCUSAURUS_PLUGIN:
                # Cross-plugin links are valid (e.g., /prism-data-layer/netflix/...)
                link.is_valid = True
                continue

            if link.link_type in [LinkType.INTERNAL_DOC, LinkType.INTERNAL_ADR, LinkType.INTERNAL_RFC]:
                self._validate_internal_link(link)
            else:
                link.is_valid = False
                link.error_message = f"Unknown link type: {link.target}"

    def _validate_internal_link(self, link: Link):
        """Validate internal document link"""
        target = link.target.split('#')[0]  # Remove anchor

        # Handle relative paths
        if target.startswith(('./', '../')):
            source_dir = link.source_doc.parent
            target_path = (source_dir / target).resolve()

            if not target.endswith('.md'):
                target_path = Path(str(target_path) + '.md')

            if target_path.exists():
                link.is_valid = True
            else:
                link.is_valid = False
                link.error_message = f"File not found: {target_path}"

        # Handle absolute paths
        elif target.startswith('/'):
            target_path = self.repo_root / target.lstrip('/')
            if target_path.exists():
                link.is_valid = True
            else:
                link.is_valid = False
                link.error_message = f"File not found: {target_path}"

        else:
            link.is_valid = False
            link.error_message = f"Ambiguous link format: {target}"

    def check_mdx_compilation(self):
        """Check MDX compilation using @mdx-js/mdx compiler"""
        self.log("\n🔧 Checking MDX compilation...")

        # Check if Node.js is available
        try:
            subprocess.run(['node', '--version'], capture_output=True, timeout=5)
        except (FileNotFoundError, subprocess.TimeoutExpired):
            self.log("   ⚠️  Node.js not found, skipping MDX compilation check")
            return True

        # Check if validate_mdx.mjs exists
        mdx_validator = self.repo_root / 'docusaurus' / 'validate_mdx.mjs'
        if not mdx_validator.exists():
            self.log("   ⚠️  validate_mdx.mjs not found, skipping MDX compilation check")
            return True

        # Collect all document paths
        file_paths = [str(doc.file_path) for doc in self.documents]

        if not file_paths:
            self.log("   ⚠️  No documents to validate")
            return True

        try:
            # Call Node.js validator
            result = subprocess.run(
                ['node', str(mdx_validator)] + file_paths,
                capture_output=True,
                text=True,
                timeout=60
            )

            # Parse JSON results
            try:
                results = json.loads(result.stdout)
            except json.JSONDecodeError:
                error = "Failed to parse MDX validation results"
                self.errors.append(error)
                self.log(f"   ✗ {error}")
                if self.verbose:
                    self.log(f"      Output: {result.stdout}")
                    self.log(f"      Error: {result.stderr}")
                return False

            # Process results
            has_errors = False
            for file_result in results:
                file_path = Path(file_result['file'])

                # Find corresponding document
                doc = self.file_to_doc.get(file_path)
                if not doc:
                    continue

                if not file_result['valid']:
                    has_errors = True
                    error_msg = file_result.get('reason', file_result.get('message', 'Unknown MDX error'))
                    line = file_result.get('line')

                    if line:
                        error = f"MDX compilation error at line {line}: {error_msg}"
                    else:
                        error = f"MDX compilation error: {error_msg}"

                    doc.errors.append(error)
                    self.log(f"   ✗ {doc.file_path.name}: {error}")

            if not has_errors:
                self.log("   ✓ All documents compile as valid MDX")
                return True
            else:
                return False

        except subprocess.TimeoutExpired:
            error = "MDX validation timed out"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False
        except Exception as e:
            error = f"Error running MDX validation: {e}"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False

    def check_mdx_compatibility(self):
        """Check for MDX parsing issues (unescaped special characters)"""
        self.log("\n🔧 Checking MDX compatibility...")

        # MDX doesn't like unescaped < and > in markdown
        problematic_patterns = [
            (r'^\s*[-*]\s+.*<\d+', 'Unescaped < before number (use &lt; or backticks)'),
            (r':\s+<\d+', 'Unescaped < after colon (use &lt; or backticks)'),
            (r'^\s*[-*]\s+.*>\d+', 'Unescaped > before number (use &gt; or backticks)'),
        ]

        mdx_issues_found = False

        for doc in self.documents:
            try:
                content = doc.file_path.read_text(encoding='utf-8')
                lines = content.split('\n')

                in_code_fence = False
                code_fence_pattern = re.compile(r'^```')

                for line_num, line in enumerate(lines, start=1):
                    # Toggle code fence
                    if code_fence_pattern.match(line):
                        in_code_fence = not in_code_fence
                        continue

                    if in_code_fence:
                        continue

                    # Remove inline code
                    line_without_code = re.sub(r'`[^`]+`', '', line)

                    for pattern, issue_desc in problematic_patterns:
                        if re.search(pattern, line_without_code):
                            error = f"Line {line_num}: {issue_desc}"
                            doc.errors.append(error)
                            mdx_issues_found = True
                            self.log(f"   ✗ {doc.file_path.name}:{line_num} - {issue_desc}")

            except Exception as e:
                doc.errors.append(f"Error checking MDX compatibility: {e}")

        if not mdx_issues_found:
            self.log("   ✓ No MDX syntax issues found")

    def check_cross_plugin_links(self):
        """Check for problematic cross-plugin links"""
        self.log("\n🔗 Checking cross-plugin links...")

        cross_plugin_pattern = re.compile(r'\[([^\]]+)\]\((\.\.\/){2,}[^)]+\)')
        issues_found = False

        for doc in self.documents:
            try:
                content = doc.file_path.read_text(encoding='utf-8')
                matches = list(cross_plugin_pattern.finditer(content))

                if matches:
                    issues_found = True
                    error = f"Found {len(matches)} cross-plugin link(s) - use absolute GitHub URLs instead"
                    doc.errors.append(error)
                    self.log(f"   ⚠️  {doc.file_path.name}: {error}")

            except Exception as e:
                doc.errors.append(f"Error checking cross-plugin links: {e}")

        if not issues_found:
            self.log("   ✓ No problematic cross-plugin links found")

    def check_typescript_config(self):
        """Run TypeScript typecheck on Docusaurus config"""
        self.log("\n🔍 Running TypeScript typecheck...")

        docusaurus_dir = self.repo_root / 'docusaurus'
        if not docusaurus_dir.exists():
            self.log("   ⚠️  Docusaurus directory not found, skipping typecheck")
            return True

        original_dir = os.getcwd()
        try:
            os.chdir(docusaurus_dir)

            result = subprocess.run(
                ['npm', 'run', 'typecheck'],
                capture_output=True,
                text=True,
                timeout=60
            )

            if result.returncode == 0:
                self.log("   ✓ TypeScript typecheck passed")
                return True
            else:
                error = "TypeScript typecheck failed"
                self.errors.append(error)
                self.log(f"   ✗ {error}")
                if self.verbose:
                    self.log(f"      {result.stderr}")
                return False

        except subprocess.TimeoutExpired:
            error = "TypeScript typecheck timed out"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False
        except FileNotFoundError:
            self.log("   ⚠️  npm not found, skipping typecheck")
            return True
        except Exception as e:
            error = f"Error running typecheck: {e}"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False
        finally:
            os.chdir(original_dir)

    def check_docusaurus_build(self, skip_build: bool = False):
        """Run Docusaurus build to catch compilation errors"""
        if skip_build:
            self.log("\n⏭️  Skipping Docusaurus build check (--skip-build)")
            return True

        self.log("\n🏗️  Running Docusaurus build validation...")
        self.log("   This may take a minute...")

        docusaurus_dir = self.repo_root / 'docusaurus'
        if not docusaurus_dir.exists():
            self.log("   ⚠️  Docusaurus directory not found, skipping build check")
            return True

        original_dir = os.getcwd()
        try:
            os.chdir(docusaurus_dir)

            result = subprocess.run(
                ['npm', 'run', 'build'],
                capture_output=True,
                text=True,
                timeout=300  # 5 minutes
            )

            output = result.stdout + result.stderr

            # Extract warnings
            warning_pattern = re.compile(r'Warning:\s+(.+)')
            warnings = warning_pattern.findall(output)

            if result.returncode == 0:
                self.log(f"   ✓ Docusaurus build succeeded")
                if warnings:
                    self.log(f"   ⚠️  Build completed with {len(warnings)} warning(s)")
                    if self.verbose:
                        for warning in warnings[:5]:
                            self.log(f"      {warning}")
                        if len(warnings) > 5:
                            self.log(f"      ... and {len(warnings) - 5} more warnings")
                return True
            else:
                # Extract error details
                error_pattern = re.compile(r'Error:\s+(.+)')
                errors = error_pattern.findall(output)

                error_msg = "Docusaurus build failed"
                self.errors.append(error_msg)
                self.log(f"   ✗ {error_msg}")

                if errors:
                    for error in errors[:3]:
                        self.log(f"      {error}")
                        self.errors.append(f"Build error: {error}")
                elif self.verbose:
                    # Show last 500 chars if no specific error found
                    self.log(f"      {output[-500:]}")

                return False

        except subprocess.TimeoutExpired:
            error = "Docusaurus build timed out (5 minutes)"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False
        except FileNotFoundError:
            self.log("   ⚠️  npm not found, skipping build check")
            return True
        except Exception as e:
            error = f"Error running build: {e}"
            self.errors.append(error)
            self.log(f"   ✗ {error}")
            return False
        finally:
            os.chdir(original_dir)

    def check_code_blocks(self):
        """Check code block formatting - balanced and properly labeled

        Rules (per CommonMark/MDX spec):
        - Opening fence: ``` followed by optional language (e.g., ```python, ```text)
        - Closing fence: ``` with NO language or other text
        - Content: Everything between opening and closing is treated as content
        """
        self.log("\n📝 Checking code blocks...")

        total_valid = 0
        total_invalid = 0

        for doc in self.documents:
            try:
                content = doc.file_path.read_text(encoding='utf-8')
                lines = content.split('\n')

                in_code_block = False
                in_frontmatter = False
                frontmatter_count = 0
                opening_line = None
                opening_language = None
                doc_valid_blocks = 0
                doc_invalid_blocks = 0

                for line_num, line in enumerate(lines, start=1):
                    stripped = line.strip()

                    # Track frontmatter (first --- to second ---)
                    if stripped == '---':
                        frontmatter_count += 1
                        if frontmatter_count == 1:
                            in_frontmatter = True
                        elif frontmatter_count == 2:
                            in_frontmatter = False
                        continue

                    # Skip frontmatter content
                    if in_frontmatter:
                        continue

                    # Check if this line is a code fence (must start with exactly ``` or more backticks)
                    # Per CommonMark: fence must be at least 3 backticks
                    fence_match = re.match(r'^(`{3,})', stripped)
                    if not fence_match:
                        continue

                    # This is a fence line
                    fence_backticks = fence_match.group(1)
                    remainder = stripped[len(fence_backticks):].strip()

                    if not in_code_block:
                        # Opening fence
                        if not remainder:
                            # Bare opening fence - INVALID (must have language)
                            error = f"Line {line_num}: Opening code fence missing language (use ```text for plain text)"
                            doc.errors.append(error)
                            self.log(f"   ✗ {doc.file_path.name}:{line_num} - Opening fence without language")
                            doc_invalid_blocks += 1
                            total_invalid += 1
                            # Still track as opening to detect closing
                            in_code_block = True
                            opening_line = line_num
                            opening_language = '<none>'
                        else:
                            # Valid opening with language
                            in_code_block = True
                            opening_line = line_num
                            opening_language = remainder.split()[0] if remainder else '<none>'
                    else:
                        # Closing fence
                        if remainder:
                            # Closing fence has extra text - INVALID
                            # Per CommonMark: closing fence should have no info string
                            error = f"Line {line_num}: Closing code fence has extra text (```{remainder}), should be just ```"
                            doc.errors.append(error)
                            self.log(f"   ✗ {doc.file_path.name}:{line_num} - Closing fence with text '```{remainder}'")
                            doc_invalid_blocks += 1
                            total_invalid += 1
                        else:
                            # Valid closing fence
                            doc_valid_blocks += 1
                            total_valid += 1

                        # Mark block as closed regardless
                        in_code_block = False
                        opening_line = None
                        opening_language = None

                # Check for unclosed code block
                if in_code_block:
                    error = f"Unclosed code block starting at line {opening_line} (```{opening_language})"
                    doc.errors.append(error)
                    self.log(f"   ✗ {doc.file_path.name} - Unclosed block at line {opening_line}")
                    doc_invalid_blocks += 1
                    total_invalid += 1

                # Report per-document summary
                if doc_valid_blocks > 0 or doc_invalid_blocks > 0:
                    if doc_invalid_blocks == 0:
                        self.log(f"   ✓ {doc.file_path.name}: {doc_valid_blocks} valid code blocks")
                    else:
                        self.log(f"   ✗ {doc.file_path.name}: {doc_valid_blocks} valid, {doc_invalid_blocks} invalid")

            except Exception as e:
                doc.errors.append(f"Error checking code blocks: {e}")
                self.log(f"   ✗ {doc.file_path.name}: Exception - {e}")

        self.log(f"\n   Total: {total_valid} valid code blocks, {total_invalid} invalid code blocks across {len(self.documents)} documents")

    def check_formatting(self):
        """Check markdown formatting issues"""
        self.log("\n📝 Checking formatting...")

        for doc in self.documents:
            try:
                content = doc.file_path.read_text(encoding='utf-8')
                lines = content.split('\n')

                # Check for trailing whitespace
                for line_num, line in enumerate(lines, start=1):
                    if line.rstrip() != line:
                        doc.errors.append(f"Line {line_num}: Trailing whitespace")

                # Check for multiple blank lines
                blank_count = 0
                for line_num, line in enumerate(lines, start=1):
                    if not line.strip():
                        blank_count += 1
                        if blank_count > 2:
                            doc.errors.append(f"Line {line_num}: More than 2 consecutive blank lines")
                    else:
                        blank_count = 0

            except Exception as e:
                doc.errors.append(f"Error checking formatting: {e}")

    def generate_report(self) -> Tuple[bool, str]:
        """Generate validation report"""
        lines = []
        lines.append("\n" + "="*80)
        lines.append("📊 PRISM DOCUMENTATION VALIDATION REPORT")
        lines.append("="*80)

        # Summary
        total_docs = len(self.documents)
        docs_with_errors = sum(1 for d in self.documents if d.errors)
        total_links = len(self.all_links)
        valid_links = sum(1 for l in self.all_links if l.is_valid)
        broken_links = total_links - valid_links

        lines.append(f"\n📄 Documents scanned: {total_docs}")
        lines.append(f"   ADRs: {sum(1 for d in self.documents if d.doc_type == 'adr')}")
        lines.append(f"   RFCs: {sum(1 for d in self.documents if d.doc_type == 'rfc')}")
        lines.append(f"   MEMOs: {sum(1 for d in self.documents if d.doc_type == 'memo')}")
        lines.append(f"   Docs: {sum(1 for d in self.documents if d.doc_type == 'doc')}")

        lines.append(f"\n🔗 Total links: {total_links}")
        lines.append(f"   Valid: {valid_links}")
        lines.append(f"   Broken: {broken_links}")

        # Link breakdown
        link_counts = {}
        for link in self.all_links:
            link_counts[link.link_type] = link_counts.get(link.link_type, 0) + 1

        lines.append(f"\n📋 Link Types:")
        for link_type, count in sorted(link_counts.items(), key=lambda x: x[1], reverse=True):
            lines.append(f"   {link_type.value}: {count}")

        # Tags summary (union of all tags)
        all_tags: Dict[str, int] = {}
        for doc in self.documents:
            for tag in doc.tags:
                all_tags[tag] = all_tags.get(tag, 0) + 1

        if all_tags:
            lines.append(f"\n🏷️  Tags (union across all documents): {len(all_tags)} unique tags")
            # Show top 15 tags by usage
            sorted_tags = sorted(all_tags.items(), key=lambda x: x[1], reverse=True)
            for tag, count in sorted_tags[:15]:
                lines.append(f"   {tag}: {count} document(s)")
            if len(sorted_tags) > 15:
                lines.append(f"   ... and {len(sorted_tags) - 15} more tags")

        # Document errors
        if docs_with_errors > 0:
            lines.append(f"\n❌ DOCUMENTS WITH ERRORS ({docs_with_errors}):")
            lines.append("-"*80)

            for doc in self.documents:
                if doc.errors:
                    lines.append(f"\n📄 {doc.file_path.relative_to(self.repo_root)}")
                    lines.append(f"   Title: {doc.title}")
                    for error in doc.errors:
                        lines.append(f"   ✗ {error}")

        # Broken links
        if broken_links > 0:
            lines.append(f"\n❌ BROKEN LINKS ({broken_links}):")
            lines.append("-"*80)

            broken_by_doc: Dict[Path, List[Link]] = {}
            for link in self.all_links:
                if not link.is_valid:
                    if link.source_doc not in broken_by_doc:
                        broken_by_doc[link.source_doc] = []
                    broken_by_doc[link.source_doc].append(link)

            for doc_path, doc_links in sorted(broken_by_doc.items()):
                lines.append(f"\n📄 {doc_path.relative_to(self.repo_root)}")
                for link in doc_links:
                    lines.append(f"   Line {link.line_number}: {link.target}")
                    lines.append(f"      → {link.error_message}")

        # Validation-level errors (TypeScript, build, etc.)
        if self.errors:
            lines.append(f"\n❌ VALIDATION ERRORS ({len(self.errors)}):")
            lines.append("-"*80)
            for error in self.errors:
                lines.append(f"   ✗ {error}")

        # Final status
        lines.append("\n" + "="*80)
        if docs_with_errors == 0 and broken_links == 0 and not self.errors:
            lines.append("✅ SUCCESS: All documents valid!")
            all_valid = True
        else:
            lines.append("❌ FAILURE: Validation errors found")
            all_valid = False
        lines.append("="*80 + "\n")

        return all_valid, "\n".join(lines)

    def validate(self, skip_build: bool = False) -> bool:
        """Run full validation pipeline"""
        self.scan_documents()
        self.extract_links()
        self.validate_links()
        self.check_code_blocks()  # NEW: Check code block balance and labeling
        self.check_mdx_compilation()  # NEW: Check MDX compilation with @mdx-js/mdx
        self.check_mdx_compatibility()
        self.check_cross_plugin_links()
        self.check_formatting()

        # Build validation (can be skipped for faster checks)
        build_passed = self.check_typescript_config()
        build_passed = self.check_docusaurus_build(skip_build) and build_passed

        all_valid, report = self.generate_report()
        print(report)
        return all_valid and build_passed


def main():
    parser = argparse.ArgumentParser(
        description="Validate Prism documentation (⚠️ CRITICAL: Run before pushing docs!)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Full validation (recommended before pushing)
    uv run tooling/validate_docs.py

    # Quick check (skip slow build validation)
    uv run tooling/validate_docs.py --skip-build

    # Verbose output
    uv run tooling/validate_docs.py --verbose

    # Auto-fix issues (future)
    uv run tooling/validate_docs.py --fix

What this checks:
    ✓ YAML frontmatter format
    ✓ Internal link validity
    ✓ MDX syntax compatibility
    ✓ Cross-plugin link issues
    ✓ TypeScript compilation
    ✓ Full Docusaurus build
        """
    )

    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Verbose output'
    )

    parser.add_argument(
        '--skip-build',
        action='store_true',
        help='Skip Docusaurus build check (faster, but less thorough)'
    )

    parser.add_argument(
        '--fix',
        action='store_true',
        help='Auto-fix issues where possible (not yet implemented)'
    )

    args = parser.parse_args()

    repo_root = Path(__file__).parent.parent
    validator = PrismDocValidator(repo_root=repo_root, verbose=args.verbose, fix=args.fix)

    try:
        all_valid = validator.validate(skip_build=args.skip_build)
        sys.exit(0 if all_valid else 1)
    except Exception as e:
        print(f"\n❌ ERROR: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(2)


if __name__ == '__main__':
    main()
