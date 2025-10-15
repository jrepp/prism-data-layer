# Documentation Best Practices Guide

When creating or modifying documentation, follow these guidelines:

## Frontmatter Templates

**ADR**: title, status, date, deciders, tags, id
**RFC**: title, status, author, created, updated, tags, id
**MEMO**: title, author, created, updated, tags, id, project_id, doc_uuid

## Common Errors & Fixes

| Error | Fix |
|-------|-----|
| Missing field | Add: project_id, doc_uuid, author |
| Unlabeled code block | Add language: ```text or ```bash |
| Duplicate ID | Use next sequential number |
| Unescaped < or > | Escape: \<5 or &lt;5 |
| Wrong link format | Use: [RFC-015](/rfc/rfc-015) |

## Quick Commands

```bash
# Find next available number
ls docs-cms/memos/ | grep -oE '[0-9]+' | sort -n | tail -1

# Generate UUID (MEMO only)
uuidgen | tr '[:upper:]' '[:lower:]'

# Validate docs (MANDATORY before commit)
uv run tooling/validate_docs.py
```

## Workflow

1. Create doc with proper frontmatter
2. Label ALL code blocks
3. Escape < and > in prose
4. Run validation
5. Fix errors
6. Commit only after ✅ SUCCESS

**CRITICAL**: ALWAYS run `uv run tooling/validate_docs.py` before committing. NEVER use `python3` directly - will fail.
