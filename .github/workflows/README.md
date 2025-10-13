# GitHub Actions Workflows

This directory contains CI/CD workflows for the Prism data access gateway.

## Workflows

### 1. CI (`ci.yml`)

**Triggers**: Push to `main`, Pull Requests to `main`

Comprehensive CI pipeline that runs on main branch pushes and pull requests.

**Jobs**:

1. **Lint** - Format and lint checks for Rust and Go code
   - Rust: `cargo fmt --check`, `cargo clippy`
   - Go: `go vet` on all packages
   - Duration: ~3 minutes

2. **Test Rust Proxy** - Unit tests for Rust proxy
   - Runs `cargo test --lib`
   - Duration: ~5 minutes

3. **Test Go Patterns** (matrix: core, memstore, redis, nats)
   - Runs unit tests with race detector
   - Generates coverage reports
   - Uploads coverage artifacts
   - Duration: ~3 minutes per pattern

4. **Integration Tests** - End-to-end Go integration tests
   - Tests proxy ↔ backend communication
   - Includes data plane tests (gRPC client → memstore)
   - Duration: ~5 minutes

5. **Acceptance Tests** (matrix: interfaces, redis, nats)
   - Uses testcontainers for real backend instances
   - Tests against Redis and NATS in Docker
   - Duration: ~10 minutes per suite

6. **Validate Documentation**
   - Runs `uv run tooling/validate_docs.py`
   - Checks frontmatter, links, MDX syntax
   - Duration: ~2 minutes

7. **Build All Components**
   - Builds Rust proxy in release mode
   - Builds all Go pattern binaries
   - Uploads artifacts for debugging
   - Duration: ~8 minutes

8. **Coverage Summary**
   - Aggregates all coverage reports
   - Displays summary in GitHub Actions UI

9. **CI Status Check** (required for merge)
   - Gates PR merges on all jobs passing

**Total Duration**: ~30-40 minutes (many jobs run in parallel)

**Artifacts**:
- Coverage reports (7 days retention)
- Proxy binary (7 days retention)
- Pattern binaries (7 days retention)

### 2. Acceptance Tests (`acceptance-tests.yml`) 🆕

**Triggers**:
- Push to `main` with changes to `patterns/**`, `tests/acceptance/**`, or test runner
- Pull Requests with same path filters
- Manual workflow dispatch

**Parallel execution** of acceptance tests with comprehensive Pattern × Backend matrix report.

**Jobs**:

1. **Generate Protobuf Code** - Creates proto artifacts
   - Duration: ~2 minutes

2. **Acceptance Tests (Parallel)** - Runs all pattern/backend combinations
   - Tests MemStore, Redis, NATS backends
   - Tests KeyValue, PubSub patterns
   - Generates matrix report in 3 formats (Terminal, Markdown, JSON)
   - Posts report to PR comments
   - Displays matrix in GitHub Actions summary
   - Duration: ~3-5 minutes (40-60% faster than sequential)

3. **Acceptance Status Check** - Required for merge
   - Gates PR merges on acceptance tests passing

**Total Duration**: ~5-7 minutes

**Matrix Report Example**:
```
🎯 Pattern × Backend Compliance Matrix:

  Pattern          │  MemStore   │   Redis     │   NATS      │ Score
  ─────────────────┼─────────────┼─────────────┼─────────────┼───────
  KeyValue         │  ✅ PASS    │  ✅ PASS    │  ───────    │ 100.0%
  KeyValueTTL      │  ✅ PASS    │  ✅ PASS    │  ───────    │ 100.0%
  KeyValueScan     │  ───────    │  ✅ PASS    │  ───────    │ 100.0%
  PubSubBasic      │  ───────    │  ───────    │  ✅ PASS    │ 100.0%
```

**Artifacts**:
- Matrix report (Markdown) - 30 days retention
- JSON results - 30 days retention
- Terminal output - 7 days retention

**Key Features**:
- ⚡ 40-60% faster than sequential execution
- 📊 Visual Pattern × Backend compliance matrix
- 💬 Automatic PR comments with test results
- 📈 GitHub Actions job summary with matrix
- 🎯 Green/red status for each combination
- 📝 Multiple output formats (Terminal, Markdown, JSON)

### 3. Deploy Docs (`docs.yml`)

**Triggers**: Push to `main` with docs changes, Manual dispatch

Deploys documentation to GitHub Pages.

**Jobs**:

1. **Build** - Validates and builds Docusaurus site
   - Runs `uv run tooling/validate_docs.py`
   - Runs `uv run tooling/build_docs.py`
   - Uploads docs artifact

2. **Deploy** - Deploys to GitHub Pages
   - Publishes to `gh-pages` branch
   - Available at https://yourusername.github.io/data-access/

**Duration**: ~5 minutes

## Workflow Strategy

```
┌─────────────────────────────────────────────────────────────┐
│                     Development Flow                        │
└─────────────────────────────────────────────────────────────┘

  Feature Branch Push
         ↓
    Create PR
         ↓
  ┌──────────────────┴──────────────────┐
  │                                     │
  CI Workflow                 Acceptance Tests Workflow
  (comprehensive)             (if patterns changed)
    ✓ All lint checks           ✓ Parallel pattern tests
    ✓ Unit tests + race         ✓ Matrix report
    ✓ Integration tests         ✓ PR comment
    ✓ Legacy acceptance         ✓ Visual grid
    ✓ Documentation
    ✓ Full builds
    ✓ Coverage reports
  (30-40 min)                 (5-7 min)
         │                                     │
         └──────────────────┬──────────────────┘
                            ↓
                    Both must pass
                            ↓
                     Merge to Main
                            ↓
         ┌──────────────────┴──────────────────┐
         │                                     │
    CI Workflow                          Docs Workflow
    (validation)                         (if docs changed)
```

## Caching Strategy

All workflows use GitHub Actions caching for faster builds:

- **Rust dependencies**: `~/.cargo/` and `proxy/target/`
  - Key: `${{ runner.os }}-cargo-${{ hashFiles('proxy/Cargo.lock') }}`
  - Saves ~3-5 minutes on cache hit

- **Go modules**: Automatic via `actions/setup-go@v5` with `cache: true`
  - Saves ~1-2 minutes on cache hit

- **Node modules**: Automatic via `actions/setup-node@v4` with `cache: 'npm'`
  - Saves ~1 minute on cache hit

## Branch Protection Rules

Recommended branch protection for `main`:

```yaml
Require status checks:
  - ci-status (from ci.yml)
  - acceptance-status (from acceptance-tests.yml, if triggered)
  - build / Build All Components
  - validate-docs / Validate Documentation

Require branches to be up to date: Yes
Require signed commits: Yes (optional)
```

## Local Development

Run the same checks locally before pushing:

```bash
# Run full CI pipeline locally (matches ci.yml)
make ci

# Individual checks
make fmt        # Format all code
make lint       # Lint all code
make test       # Run unit tests
make test-all   # Run all tests (unit + integration + acceptance)

# Parallel acceptance tests (matches acceptance-tests.yml)
make test-acceptance-parallel              # Run with matrix report
make test-acceptance-parallel-report       # Save reports to files
uv run tooling/parallel_acceptance_test.py # Direct invocation
```

## Debugging Failed Workflows

### 1. Download artifacts

```bash
gh run list --workflow=ci.yml
gh run download <run-id>
```

### 2. View logs

```bash
gh run view <run-id> --log
```

### 3. Re-run failed jobs

```bash
gh run rerun <run-id> --failed
```

### 4. Run locally with act

```bash
# Install act: https://github.com/nektos/act
act -j lint                    # Run lint job
act -j test-patterns           # Run pattern tests
act pull_request               # Simulate PR event
```

## Coverage Reports

Coverage reports are generated for:

- Core SDK (`patterns/core`)
- MemStore plugin (`patterns/memstore`)
- Redis plugin (`patterns/redis`)
- NATS plugin (`patterns/nats`)
- Integration tests (`tests/integration`)
- Acceptance tests (`tests/acceptance/*`)

Download from workflow artifacts or view in CI logs.

## Adding New Checks

### Add a new pattern

1. Add to matrix in `test-patterns` job:
   ```yaml
   matrix:
     pattern: [core, memstore, redis, nats, yourpattern]
   ```

2. Add to lint job:
   ```yaml
   - name: Lint Go code
     run: |
       # ... existing patterns ...
       cd ../../patterns/yourpattern && go vet ./...
   ```

3. Update Makefile targets and re-run

### Add a new test suite

1. Create job in `ci.yml`:
   ```yaml
   test-yourtest:
     name: Your Test Suite
     runs-on: ubuntu-latest
     steps:
       - name: Run your tests
         run: cd tests/yourtest && go test -v ./...
   ```

2. Add to `ci-status` dependencies:
   ```yaml
   needs: [lint, test-proxy, ..., test-yourtest]
   ```

## Secrets Required

No secrets are currently required for CI workflows. All tests use:

- Local in-memory backends (memstore)
- Docker containers (Redis, NATS via testcontainers)
- No external service dependencies

## Cost Optimization

Estimated GitHub Actions minutes usage:

- Pre-commit workflow: ~15 minutes per push
- CI workflow: ~40 minutes per PR (parallelized across 9 jobs)
- Docs workflow: ~5 minutes per docs change

**Monthly estimate** (for 100 PRs/month):
- Pre-commits: 100 pushes × 15 min = 1,500 minutes
- CI: 100 PRs × 40 min = 4,000 minutes
- Docs: 20 changes × 5 min = 100 minutes
- **Total**: ~5,600 minutes/month

GitHub Free: 2,000 minutes/month
GitHub Team: 3,000 minutes/month
**Recommendation**: GitHub Team plan or optimize with self-hosted runners

## Performance Tuning

Current optimizations:

1. **Concurrency cancellation**: Old runs cancelled when new commit pushed
2. **Job parallelization**: 9 jobs run concurrently in CI
3. **Dependency caching**: Rust, Go, Node caches save ~5 minutes per run
4. **Incremental builds**: Cargo uses cached artifacts when possible
5. **Conditional execution**: Jobs only run when relevant files change (future enhancement)

Future optimizations:

1. Add `paths` filters to limit when workflows run
2. Use self-hosted runners for private infrastructure
3. Add build artifact reuse between jobs
4. Implement differential testing (only test changed code)
