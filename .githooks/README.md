# Git Hooks for Prism

This directory contains git hooks for the Prism project.

## Installation

To use these hooks, run:

```bash
git config core.hooksPath .githooks
```

Or add to your git config:

```bash
git config --local core.hooksPath .githooks
```

## Available Hooks

### pre-commit

Validates documentation before committing:
- Checks YAML frontmatter format
- Validates internal links
- Checks markdown formatting

**Skip validation** (use sparingly):
```bash
git commit --no-verify
```

## Requirements

- `uv` must be installed
- Python 3.11+

## Troubleshooting

If hooks aren't running:
```bash
# Verify hooks path
git config --get core.hooksPath

# Should output: .githooks

# Re-install if needed
git config core.hooksPath .githooks
```
