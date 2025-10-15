# TDD Workflow Guide

All Go code MUST use TDD with mandatory coverage tracking.

## 3-Phase Cycle

1. **Red**: Write failing test first
2. **Green**: Implement minimal code to pass
3. **Refactor**: Improve while tests pass

## Coverage Requirements

| Component | Minimum | Target |
|-----------|---------|--------|
| Core SDK | 85% | 90%+ |
| Plugins (complex) | 80% | 85%+ |
| Plugins (simple) | 85% | 90%+ |
| Utilities | 90% | 95%+ |

## Quick Commands

```bash
# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# Race detector (MANDATORY in CI)
go test -race ./...
```

## Always Test

- Public APIs
- Business logic
- Error paths
- Edge cases
- Concurrent operations

## Pre-Commit Checklist

- [ ] Tests written BEFORE implementation
- [ ] All tests passing
- [ ] Coverage meets threshold
- [ ] Race detector clean
- [ ] Coverage % in commit message
