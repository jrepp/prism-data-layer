# Prism Tooling

This directory contains scripts and tools for maintaining the Prism documentation and codebase.

## Documentation Validation

### validate_docs.py

**⚠️ CRITICAL: Run this before pushing documentation changes!**

Comprehensive validation that catches issues before they break the GitHub Pages build.

```bash
# Full validation (recommended before pushing)
uv run tooling/validate_docs.py

# Quick check (skip build, faster iteration)
uv run tooling/validate_docs.py --skip-build

# Verbose output for debugging
uv run tooling/validate_docs.py --verbose
```

**What it checks**:
1. **YAML Frontmatter**: Required fields (id, title, status for ADRs/RFCs)
2. **Link Validity**: Internal and external link reachability
3. **MDX Compatibility**: Detects unescaped `<` and `>` that break MDX parsing
4. **Cross-Plugin Links**: Identifies problematic relative links across plugins
5. **TypeScript Typecheck**: Validates Docusaurus configuration
6. **Full Build**: Runs Docusaurus build to catch compilation errors

**Exit codes**:
- `0`: All checks passed, safe to push
- `1`: Validation failed, fix issues before pushing
- `2`: Script error

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
# 1. Run full validation (includes build check)
uv run tooling/validate_docs.py

# 2. If validation passes, commit and push
git add .
git commit -m "Your commit message"
git push origin main

# Quick iteration (skip slow build check during development)
uv run tooling/validate_docs.py --skip-build
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
echo "Running documentation validation..."
uv run tooling/validate_docs.py --skip-build
if [ $? -ne 0 ]; then
    echo "❌ Validation failed. Fix issues before committing."
    exit 1
fi
echo "✅ Validation passed. Run full check before push: uv run tooling/validate_docs.py"
```

Make executable:
```bash
chmod +x .git/hooks/pre-commit
```

**Note**: Pre-commit uses `--skip-build` for speed. Always run full validation before pushing!

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
# Run full validation
uv run tooling/validate_docs.py

# Quick validation during development
uv run tooling/validate_docs.py --skip-build

# Test Docusaurus build directly
cd docusaurus && npm run build

# Serve locally to test
npm run serve
```

## Scripts Reference

| Script | Purpose | Usage |
|--------|---------|-------|
| `validate_docs.py` | Comprehensive doc validation | `uv run tooling/validate_docs.py` |
| `bootstrap.py` | (Future) Setup script | `uv run tooling/bootstrap.py` |

## Support

For issues or questions:
- Check [CLAUDE.md](../CLAUDE.md) for project guidance
- Review [GitHub Actions logs](https://github.com/jrepp/prism-data-layer/actions)
- File an issue in the repository
