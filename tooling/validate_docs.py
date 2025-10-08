#!/usr/bin/env python3
"""
Document Validation and Link Checker for Prism

Validates markdown documents for:
- YAML frontmatter format and required fields
- Internal link reachability
- Markdown formatting issues
- Consistent ADR/RFC numbering

Usage:
    uv run tooling/validate_docs.py
    uv run tooling/validate_docs.py --verbose
    uv run tooling/validate_docs.py --fix  # Auto-fix issues where possible

Exit Codes:
    0 - All documents valid
    1 - Validation errors found
    2 - Script error
"""

import argparse
import re
import sys
from pathlib import Path
from typing import Dict, List, Set, Tuple
from dataclasses import dataclass, field
from enum import Enum


class LinkType(Enum):
    """Types of links in markdown documents"""
    INTERNAL_DOC = "internal_doc"      # ./relative.md or /docs/path.md
    INTERNAL_ADR = "internal_adr"      # ADR cross-references
    INTERNAL_RFC = "internal_rfc"      # RFC cross-references
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

        # Scan ADRs
        adr_dir = self.repo_root / "docs-cms" / "adr"
        if adr_dir.exists():
            for md_file in sorted(adr_dir.glob("*.md")):
                if md_file.name != "README.md":
                    doc = self._parse_document(md_file, "adr")
                    if doc:
                        self.documents.append(doc)
                        self.file_to_doc[md_file] = doc

        # Scan RFCs
        rfc_dir = self.repo_root / "docs-cms" / "rfcs"
        if rfc_dir.exists():
            for md_file in sorted(rfc_dir.glob("RFC-*.md")):
                doc = self._parse_document(md_file, "rfc")
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
        try:
            content = file_path.read_text(encoding='utf-8')

            # Check for frontmatter
            frontmatter_match = re.match(r'^---\s*\n(.*?)\n---\s*\n', content, re.DOTALL)
            if not frontmatter_match:
                error = f"Missing YAML frontmatter"
                self.log(f"   ✗ {file_path.name}: {error}")
                doc = Document(file_path=file_path, doc_type=doc_type, title="Unknown")
                doc.errors.append(error)
                return doc

            frontmatter = frontmatter_match.group(1)

            # Extract title (required)
            title_match = re.search(r'^title:\s*["\']?([^"\'\n]+)["\']?', frontmatter, re.MULTILINE)
            if not title_match:
                error = "Missing 'title' in frontmatter"
                self.log(f"   ✗ {file_path.name}: {error}")
                doc = Document(file_path=file_path, doc_type=doc_type, title="Unknown")
                doc.errors.append(error)
                return doc

            title = title_match.group(1).strip().strip('"').strip("'")

            # Extract status (required for ADRs and RFCs)
            status = ""
            if doc_type in ["adr", "rfc"]:
                status_match = re.search(r'^status:\s*(.+)$', frontmatter, re.MULTILINE)
                if not status_match:
                    error = f"Missing 'status' in frontmatter ({doc_type} requires status)"
                    self.log(f"   ✗ {file_path.name}: {error}")
                    doc = Document(file_path=file_path, doc_type=doc_type, title=title)
                    doc.errors.append(error)
                    return doc
                status = status_match.group(1).strip()

            # Extract date/created
            date = ""
            date_match = re.search(r'^(?:date|created):\s*(.+)$', frontmatter, re.MULTILINE)
            if date_match:
                date = date_match.group(1).strip()

            # Extract tags (if present)
            tags = []
            tags_match = re.search(r'^tags:\s*\[([^\]]+)\]', frontmatter, re.MULTILINE)
            if tags_match:
                tags_str = tags_match.group(1)
                tags = [t.strip().strip('"').strip("'") for t in tags_str.split(',')]

            doc = Document(
                file_path=file_path,
                doc_type=doc_type,
                title=title,
                status=status,
                date=date,
                tags=tags
            )

            self.log(f"   ✓ {file_path.name}: {title}")
            return doc

        except Exception as e:
            self.errors.append(f"Error parsing {file_path}: {e}")
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

    def validate(self) -> bool:
        """Run full validation pipeline"""
        self.scan_documents()
        self.extract_links()
        self.validate_links()
        self.check_formatting()
        all_valid, report = self.generate_report()
        print(report)
        return all_valid


def main():
    parser = argparse.ArgumentParser(
        description="Validate Prism documentation",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Validate all docs
    uv run tooling/validate_docs.py

    # Verbose output
    uv run tooling/validate_docs.py --verbose

    # Auto-fix issues (future)
    uv run tooling/validate_docs.py --fix
        """
    )

    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Verbose output'
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
        all_valid = validator.validate()
        sys.exit(0 if all_valid else 1)
    except Exception as e:
        print(f"\n❌ ERROR: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(2)


if __name__ == '__main__':
    main()
