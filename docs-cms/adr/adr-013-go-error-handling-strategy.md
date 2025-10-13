---
date: 2025-10-07
deciders: Core Team
doc_uuid: e8589e41-7426-4a36-9666-2588e2376738
id: adr-013
project_id: prism-data-layer
status: Accepted
tags:
- go
- error-handling
- reliability
- observability
title: Go Error Handling Strategy
---

## Context

Go tooling and CLI utilities require consistent error handling patterns. We need a strategy that:
- Preserves error context through call chains
- Enables debugging without verbose logging
- Follows Go 1.25+ best practices
- Reports errors at handler boundaries
- Supports structured error analysis

## Decision

Adopt modern Go error handling with wrapped context and early error reporting:

1. **Use `fmt.Errorf` with `%w` for error wrapping**
2. **Report errors at the top of handlers** (fail-fast principle)
3. **Add context at each layer** (function name, operation, parameters)
4. **Use sentinel errors** for well-known error conditions
5. **Return errors immediately** rather than accumulating

## Rationale

### Why Error Wrapping

```go
// Modern approach - preserves full context
if err := connectBackend(namespace); err != nil {
    return fmt.Errorf("connectBackend(%s): %w", namespace, err)
}

// Allows callers to unwrap and inspect
if errors.Is(err, ErrBackendUnavailable) {
    // Handle specific error
}
```

**Benefits:**
- Full stack trace without overhead
- Programmatic error inspection with `errors.Is` and `errors.As`
- Clear failure path through logs

### Why Early Error Reporting

```go
// Report errors at the top (fail-fast)
func MigrateData(ctx context.Context, source, dest string) error {
    srcConn, err := openConnection(source)
    if err != nil {
        return fmt.Errorf("MigrateData: open source: %w", err)
    }
    defer srcConn.Close()

    destConn, err := openConnection(dest)
    if err != nil {
        return fmt.Errorf("MigrateData: open dest: %w", err)
    }
    defer destConn.Close()

    // ... continue processing
}
```

**Benefits:**
- Reduces nesting and improves readability
- Makes error paths explicit
- Aligns with Go idioms

### Alternatives Considered

1. **Exception-style panic/recover**
   - Pros: Simpler control flow
   - Cons: Not idiomatic Go, hides errors, harder to debug
   - Rejected: Go community consensus favors explicit errors

2. **Error accumulation patterns**
   - Pros: Can process multiple failures
   - Cons: Harder to reason about, delayed failure detection
   - Rejected: Infrastructure tools should fail fast

3. **Third-party error libraries (pkg/errors)**
   - Pros: Stack traces, additional features
   - Cons: Dependency overhead, stdlib now sufficient
   - Rejected: Go 1.13+ error wrapping is sufficient

## Consequences

### Positive

- Errors carry full context without verbose logging
- Debugging is straightforward (follow the wrapped chain)
- Error handling is testable (check for sentinel errors)
- Aligns with Go 1.25 idioms

### Negative

- Requires discipline to wrap at every layer
- Error messages can become verbose if not carefully structured
- Must decide what context to add at each layer

### Neutral

- Error handling code is explicit (not hidden in abstractions)
- Need to define sentinel errors for known conditions

## Implementation Notes

### Sentinel Errors

Define package-level sentinel errors:

```go
package backend

import "errors"

var (
    ErrBackendUnavailable = errors.New("backend unavailable")
    ErrNamespaceNotFound  = errors.New("namespace not found")
    ErrInvalidConfig      = errors.New("invalid configuration")
)
```

### Error Wrapping Pattern

```go
// Low-level function
func validateConfig(cfg *Config) error {
    if cfg.Namespace == "" {
        return fmt.Errorf("validateConfig: %w: namespace empty", ErrInvalidConfig)
    }
    return nil
}

// Mid-level function
func connectBackend(namespace string) (*Connection, error) {
    cfg, err := loadConfig(namespace)
    if err != nil {
        return nil, fmt.Errorf("connectBackend: load config: %w", err)
    }

    if err := validateConfig(cfg); err != nil {
        return nil, fmt.Errorf("connectBackend: %w", err)
    }

    conn, err := dial(cfg.Endpoint)
    if err != nil {
        return nil, fmt.Errorf("connectBackend: dial %s: %w", cfg.Endpoint, err)
    }

    return conn, nil
}

// Top-level handler
func MigrateNamespace(ctx context.Context, namespace string) error {
    conn, err := connectBackend(namespace)
    if err != nil {
        return fmt.Errorf("MigrateNamespace(%s): %w", namespace, err)
    }
    defer conn.Close()

    return runMigration(ctx, conn)
}
```

### Testing Error Conditions

```go
func TestConnectBackend_InvalidConfig(t *testing.T) {
    _, err := connectBackend("")
    if !errors.Is(err, ErrInvalidConfig) {
        t.Errorf("expected ErrInvalidConfig, got %v", err)
    }
}
```

### Error Context Guidelines

Add context that helps debugging:
- Function name (especially at package boundaries)
- Parameters that identify the operation (namespace, path, endpoint)
- Operation description (what was being attempted)

Avoid adding:
- Redundant context (don't repeat what's already in wrapped error)
- Secrets or sensitive data
- Full object dumps (use identifiers instead)

## References

- [Go Blog: Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Effective Go: Errors](https://go.dev/doc/effective_go#errors)
- ADR-012: Go for Tooling
- org-stream-producer ADR-005: Error Handling Strategy

## Revision History

- 2025-10-07: Initial draft and acceptance (adapted from org-stream-producer)