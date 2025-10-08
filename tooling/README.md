# Prism Tooling

This directory contains scripts and tools for maintaining the Prism documentation and codebase.

## Documentation Validation

### validate_docs.py

Validates documentation structure, frontmatter, and link integrity.

```bash
# Run documentation validation
uv run tooling/validate_docs.py
```

**What it checks**:
- Document frontmatter (id, title, sidebar_label, status)
- Internal and external links
- Broken references
- Document counts (ADRs, RFCs, Docs)

### pre_lint_docs.py

**⚠️ REQUIRED BEFORE PUSHING DOCUMENTATION CHANGES**

Pre-lint validation to catch issues before they break the GitHub Pages build.

```bash
# Run pre-lint validation before pushing
uv run tooling/pre_lint_docs.py
```

**What it checks**:
1. **MDX Special Characters**: Detects unescaped `<` characters that break MDX parsing
2. **Internal Links**: Identifies cross-plugin markdown links that won't work in Docusaurus
3. **TypeScript Typecheck**: Validates Docusaurus configuration
4. **Build Validation**: Runs full Docusaurus build to catch compilation errors

**Exit codes**:
- `0`: All checks passed, safe to push
- `1`: Validation failed, fix issues before pushing

### Common MDX Issues

#### Unescaped Special Characters

❌ **Incorrect** (will break MDX):
```markdown
- **Latency**: <100ms
- **Throughput**: >50k RPS
```

✅ **Correct** (use HTML entities or backticks):
```markdown
- **Latency**: &lt;100ms
- **Throughput**: &gt;50k RPS
```

Or use backticks:
```markdown
- **Latency**: `<100ms`
- **Throughput**: `>50k RPS`
```

#### Cross-Plugin Links

❌ **Incorrect** (won't work in Docusaurus):
```markdown
See [CLAUDE.md](../../CLAUDE.md) for details
```

✅ **Correct** (use absolute GitHub URLs):
```markdown
See [CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md) for details
```

## Workflow

### Before Committing Documentation

```bash
# 1. Validate documentation structure
uv run tooling/validate_docs.py

# 2. Run pre-lint checks (catches build errors)
uv run tooling/pre_lint_docs.py

# 3. If both pass, commit and push
git add .
git commit -m "Your commit message"
git push origin main
```

### GitHub Pages Build

After pushing, GitHub Actions will:
1. Install dependencies
2. Run Docusaurus build
3. Deploy to GitHub Pages

The search interface will appear once the build completes successfully.

**Check build status**: [GitHub Actions](https://github.com/jrepp/prism-data-layer/actions)

## Troubleshooting

### Search Not Showing Up

The search interface requires:
1. `@easyops-cn/docusaurus-search-local` package installed ✅
2. Theme configured in `docusaurus.config.ts` ✅
3. Successful build (index generation happens during build)

If search is missing:
- Check GitHub Actions build status
- Look for MDX compilation errors in build logs
- Verify theme configuration in `docusaurus.config.ts`

### Build Failing on GitHub Actions

1. Run pre-lint locally: `uv run tooling/pre_lint_docs.py`
2. Fix any reported issues
3. Test build locally: `cd docusaurus && npm run build`
4. Push after local build succeeds

### Link Warnings

Docusaurus shows warnings for:
- Links to files outside the plugin directory
- Links to non-existent anchors
- Relative links crossing plugin boundaries

**Solution**: Convert to absolute GitHub URLs for external references.

## CI/CD Integration

### Pre-commit Hook

Add to `.git/hooks/pre-commit`:

```bash
#!/bin/bash
echo "Running documentation pre-lint validation..."
uv run tooling/pre_lint_docs.py
if [ $? -ne 0 ]; then
    echo "❌ Pre-lint validation failed. Fix issues before committing."
    exit 1
fi
```

Make executable:
```bash
chmod +x .git/hooks/pre-commit
```

### GitHub Actions

The repository includes a workflow that:
1. Builds Docusaurus site
2. Runs validation checks
3. Deploys to GitHub Pages

See `.github/workflows/deploy.yml` for details.

## Development

### Adding New Validation Rules

Edit `pre_lint_docs.py` to add new checks:

```python
def check_custom_rule(root_dir: Path) -> List[str]:
    """Check for custom rule violations."""
    issues = []
    # Your validation logic here
    return issues

# Add to main():
custom_issues = check_custom_rule(docs_cms)
if custom_issues:
    # Handle issues
```

### Testing Locally

```bash
# Run validation on specific directory
python tooling/pre_lint_docs.py

# Test Docusaurus build
cd docusaurus && npm run build

# Serve locally to test
npm run serve
```

## Scripts Reference

| Script | Purpose | Usage |
|--------|---------|-------|
| `validate_docs.py` | Structure validation | `uv run tooling/validate_docs.py` |
| `pre_lint_docs.py` | Pre-push validation | `uv run tooling/pre_lint_docs.py` |
| `bootstrap.py` | (Future) Setup script | `uv run tooling/bootstrap.py` |

## Support

For issues or questions:
- Check [CLAUDE.md](../CLAUDE.md) for project guidance
- Review [GitHub Actions logs](https://github.com/jrepp/prism-data-layer/actions)
- File an issue in the repository
