# Testing & Linting Commands

## Parallel Testing (1.7x faster)

```bash
make test-parallel              # All tests
make test-parallel-fast         # Skip acceptance
make test-parallel-fail-fast    # Stop on first failure
```

## Parallel Linting (54-90x faster)

```bash
make lint-parallel              # All 10 categories (3.7s)
make lint-parallel-critical     # Critical only (1-2s)
make lint-fix                   # Auto-fix issues
```

## Development Workflow

```bash
# Fast feedback during development
make lint-parallel-critical && make test-parallel-fast

# Before commit (full validation)
make lint-parallel && make test-parallel

# Debug specific failure
cat test-logs/acceptance-redis.log
```

## 10 Linter Categories

Critical, Security, Style, Quality, Errors, Performance, Bugs, Testing, Maintainability, Misc

See [MEMO-021](/memos/memo-021) for details.
