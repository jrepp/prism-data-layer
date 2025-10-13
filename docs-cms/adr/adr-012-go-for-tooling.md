---
date: 2025-10-07
deciders: Core Team
doc_uuid: 4dab61b4-9416-44f9-ab93-36ebc2256e12
id: adr-012
project_id: prism-data-layer
status: Accepted
tags:
- language
- tooling
- cli
- developer-experience
title: Go for Tooling and CLI Utilities
---

## Context

Prism uses Rust for the proxy (performance-critical path) and Python for orchestration. We need to choose a language for:
- CLI utilities and dev tools
- Repository management tools
- Data migration utilities
- Performance testing harnesses
- Potential backend adapters where appropriate

Requirements:
- Fast compilation for rapid iteration
- Single-binary distribution
- Good concurrency primitives
- Protobuf interoperability
- Cross-platform support

## Decision

Use **Go** for tooling, CLI utilities, and select backend adapters.

## Rationale

### Why Go for Tooling

1. **Single Binary Distribution**: No runtime dependencies, easy deployment
2. **Fast Compile Times**: Rapid iteration during development
3. **Strong Standard Library**: Excellent I/O, networking, and HTTP support
4. **Goroutines**: Natural concurrency for parallel operations
5. **Protobuf Interoperability**: First-class protobuf support, can consume same `.proto` files as Rust
6. **Cross-Platform**: Easy cross-compilation for Linux, macOS, Windows

### Use Cases in Prism

#### Primary Use Cases (Go)
- CLI tools (`prism-cli`, `prism-migrate`, `prism-bench`)
- Data migration utilities
- Load testing harnesses
- Repository analysis tools
- Backend health checkers

#### NOT Recommended (Use Rust Instead)
- Proxy core (latency-sensitive, use Rust)
- Hot-path request handlers (use Rust)
- Memory-intensive data processing (use Rust)

#### NOT Recommended (Use Python Instead)
- Code generation from protobuf (Python tooling established)
- Docker Compose orchestration (Python established)
- CI/CD scripting (Python established)

### Alternatives Considered

1. **Rust**
   - Pros: Maximum performance, memory safety, consistency with proxy
   - Cons: Slower compile times, steeper learning curve for tools
   - Rejected: Overkill for CLI tools, slower development velocity

2. **Python**
   - Pros: Rapid development, rich ecosystem
   - Cons: Slow execution, GIL limits concurrency, requires interpreter
   - Rejected: Too slow for data migration/load testing

3. **Node.js**
   - Pros: Fast async I/O, large ecosystem
   - Cons: Memory overhead, callback complexity
   - Rejected: Go's concurrency model clearer for our use cases

## Consequences

### Positive

- Fast compile times enable rapid iteration
- Single binaries simplify distribution (no Docker needed for tools)
- Strong concurrency primitives for parallel operations
- Shared protobuf definitions with Rust proxy
- Growing ecosystem for data infrastructure tools

### Negative

- Another language in the stack (Rust, Python, Go)
- Different error handling patterns than Rust
- Garbage collection pauses (mitigated: not on hot path)

### Neutral

- Learning curve for developers unfamiliar with Go
- Protobuf code generation required (standard across all languages)

## Implementation Notes

### Directory Structure

prism/
├── tooling/               # Python orchestration (unchanged)
├── tools/                 # Go CLI tools (new)
│   ├── cmd/
│   │   ├── prism-cli/    # Main CLI tool
│   │   ├── prism-migrate/ # Data migration
│   │   └── prism-bench/   # Load testing
│   ├── internal/
│   │   ├── config/
│   │   ├── proto/        # Generated Go protobuf code
│   │   └── util/
│   ├── go.mod
│   └── go.sum
```text

### Key Libraries

```
// Protobuf
import "google.golang.org/protobuf/proto"

// CLI framework
import "github.com/spf13/cobra"

// Configuration
import "github.com/spf13/viper"

// Structured logging
import "log/slog"

// Concurrency patterns
import "golang.org/x/sync/errgroup"
```text

### Protobuf Sharing

Generate Go code from the same proto definitions:

```
# Generate Go protobuf code
buf generate --template tools/buf.gen.go.yaml
```text

`tools/buf.gen.go.yaml`:
```
version: v1
plugins:
  - plugin: go
    out: internal/proto
    opt:
      - paths=source_relative
  - plugin: go-grpc
    out: internal/proto
    opt:
      - paths=source_relative
```text

### Example Tool: prism-cli

```
package main

import (
    "context"
    "log/slog"

    "github.com/spf13/cobra"
    "github.com/prism/tools/internal/config"
    "github.com/prism/tools/internal/proto/prism/keyvalue/v1"
)

var rootCmd = &cobra.Command{
    Use:   "prism-cli",
    Short: "Prism command-line interface",
}

var getCmd = &cobra.Command{
    Use:   "get [namespace] [id] [key]",
    Short: "Get a value from Prism",
    Args:  cobra.ExactArgs(3),
    RunE: func(cmd *cobra.Command, args []string) error {
        // Connect to proxy, issue Get request
        // ...
        return nil
    },
}

func main() {
    rootCmd.AddCommand(getCmd)
    if err := rootCmd.Execute(); err != nil {
        slog.Error("command failed", "error", err)
        os.Exit(1)
    }
}
```text

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Protobuf Documentation](https://protobuf.dev/reference/go/go-generated/)
- ADR-003: Protobuf as Single Source of Truth
- org-stream-producer ADR-001: Go for Git Analysis

## Revision History

- 2025-10-07: Initial draft and acceptance

```