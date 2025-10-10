---
title: "ADR-017: Go Structured Logging with slog"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['go', 'logging', 'observability', 'debugging']
---

## Context

Go tooling requires production-grade logging with:
- Structured fields for machine parsing
- Context propagation through call stacks
- High performance (minimal overhead)
- Integration with observability systems
- Standard library compatibility

## Decision

Use **slog** (Go standard library) for structured logging with context management.

## Rationale

### Why slog (Go 1.21+ Standard Library)

- **Standard Library**: No external dependency, guaranteed compatibility
- **Performance**: Designed for high-throughput logging
- **Structured by Default**: Key-value pairs, not string formatting
- **Context Integration**: First-class `context.Context` support
- **Handlers**: JSON, Text, and custom handlers
- **Levels**: Debug, Info, Warn, Error
- **Attributes**: Rich type support (string, int, bool, time, error, etc.)

### Why NOT zap or logrus?

- **zap**: Excellent performance, but slog is now in stdlib with comparable speed
- **logrus**: Mature but slower, maintenance mode, superseded by slog
- **zerolog**: Fast but non-standard API, less idiomatic

### Context Propagation Pattern

Logging flows through context to maintain operation correlation:

```go
// Add logger to context
ctx := log.WithContext(ctx, logger.With("operation", "migrate", "namespace", ns))

// Extract logger from context
logger := log.FromContext(ctx)
logger.Info("starting migration")
```

## Logging Schema

### Log Levels

- **Debug**: Detailed diagnostic information (disabled in production)
- **Info**: General informational messages (normal operations)
- **Warn**: Warning conditions that don't prevent operation
- **Error**: Error conditions that prevent specific operation

### Standard Fields (always present)

```json
{
  "time": "2025-10-07T12:00:00Z",
  "level": "info",
  "msg": "migration completed",
  "service": "prism-migrate",
  "version": "1.0.0"
}
```

### Contextual Fields (operation-specific)

```json
{
  "time": "2025-10-07T12:00:00Z",
  "level": "info",
  "msg": "migration completed",
  "service": "prism-migrate",
  "version": "1.0.0",
  "namespace": "production",
  "operation": "migrate",
  "rows_migrated": 15234,
  "duration_ms": 5230,
  "workers": 8
}
```

### Error Fields

```json
{
  "time": "2025-10-07T12:00:00Z",
  "level": "error",
  "msg": "migration failed",
  "service": "prism-migrate",
  "error": "backend unavailable",
  "error_type": "ErrBackendUnavailable",
  "namespace": "production",
  "retry_count": 3
}
```

## Implementation Pattern

### Package Structure

tools/internal/
  log/
    log.go           # slog wrapper with context helpers
    context.go       # Context management
    log_test.go      # Tests
```text

### Core API

```go
package log

import (
    "context"
    "log/slog"
    "os"
)

var global *slog.Logger

// Init initializes the global logger
func Init(level slog.Level, format string) error {
    var handler slog.Handler

    switch format {
    case "json":
        handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
            Level:     level,
            AddSource: level == slog.LevelDebug,
        })
    case "text":
        handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
            Level:     level,
            AddSource: level == slog.LevelDebug,
        })
    default:
        return fmt.Errorf("unknown log format: %s", format)
    }

    // Add service metadata
    handler = withServiceMetadata(handler)

    global = slog.New(handler)
    slog.SetDefault(global)

    return nil
}

// WithContext adds logger to context
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
    return context.WithValue(ctx, loggerKey{}, logger)
}

// FromContext extracts logger from context (or returns default)
func FromContext(ctx context.Context) *slog.Logger {
    if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
        return logger
    }
    return slog.Default()
}

// With adds fields to logger in context
func With(ctx context.Context, args ...any) context.Context {
    logger := FromContext(ctx).With(args...)
    return WithContext(ctx, logger)
}

type loggerKey struct{}
```text

### Usage Examples

```go
// Initialize at startup
if err := log.Init(slog.LevelInfo, "json"); err != nil {
    panic(err)
}

// Add operation context
ctx := log.WithContext(ctx, slog.Default().With(
    "operation", "migrate",
    "namespace", namespace,
))

// Log with context
logger := log.FromContext(ctx)
logger.Info("starting migration")

// Add more fields
ctx = log.With(ctx, "rows", count)
log.FromContext(ctx).Info("migrated rows")

// Error logging
logger.Error("migration failed",
    "error", err,
    "namespace", namespace,
    "retry", retry,
)

// Debug logging (only in debug mode)
logger.Debug("worker started",
    "worker_id", workerID,
    "queue_size", queueSize,
)
```text

### Performance-Critical Paths

For hot paths, use conditional logging:

```go
if logger.Enabled(ctx, slog.LevelDebug) {
    logger.DebugContext(ctx, "processing item",
        "item_id", id,
        "batch", batchNum,
    )
}
```text

### Testing Pattern

```go
// Custom handler for testing
type TestHandler struct {
    logs []slog.Record
    mu   sync.Mutex
}

func (h *TestHandler) Handle(ctx context.Context, r slog.Record) error {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.logs = append(h.logs, r)
    return nil
}

// Test example
func TestMigrate_Logging(t *testing.T) {
    handler := &TestHandler{}
    logger := slog.New(handler)
    ctx := log.WithContext(context.Background(), logger)

    // Code that logs
    migrate(ctx, "test-namespace")

    // Assert logs
    if len(handler.logs) < 1 {
        t.Error("expected at least 1 log entry")
    }
}
```text

## Logging Guidelines

### DO:
- Use structured fields, not string formatting
- Pass context through call stack
- Log errors with context (namespace, operation, etc.)
- Use appropriate log levels
- Include duration for operations
- Log at service boundaries (start/end of major operations)

### DON'T:
- Log in tight loops
- Log sensitive data (credentials, PII)
- Use global logger (use context instead)
- Format strings with %v (use structured fields)
- Log at Info level for internal function calls

## Log Level Guidelines

### Debug
- Internal function entry/exit
- Variable values during debugging
- Detailed state information
- **Disabled in production**

### Info
- Service start/stop
- Major operation start/complete
- Configuration loaded
- Summary statistics

### Warn
- Degraded performance
- Retryable errors
- Non-fatal issues

### Error
- Failed operations
- Unrecoverable errors
- Connection failures

## Consequences

### Positive

- Zero external dependencies (stdlib)
- Excellent performance
- First-class context support
- Structured logging enforced by API
- Easy testing with custom handlers
- Future-proof (Go stdlib commitment)

### Negative

- slog is relatively new (Go 1.21+)
- Basic functionality (no log rotation, sampling, etc.)

### Mitigations

- Require Go 1.25 (already planned)
- Use external tools for log aggregation (Fluentd, Logstash)

## References

- [slog Documentation](https://pkg.go.dev/log/slog)
- [slog Design Proposal](https://go.googlesource.com/proposal/+/master/design/56345-structured-logging.md)
- ADR-012: Go for Tooling
- ADR-008: Observability Strategy
- org-stream-producer ADR-011: Structured Logging

## Revision History

- 2025-10-07: Initial draft and acceptance (adapted from org-stream-producer)
