---
title: "ADR-039: CLI Acceptance Testing with testscript"
status: Accepted
date: 2025-10-09
deciders: [System]
tags: [testing, cli, go, acceptance-testing, developer-experience]
---

## Context

The Prism admin CLI (`prismctl`) requires comprehensive testing to ensure:

1. **Shell-Based Acceptance Tests**: Verify CLI behavior as users would invoke it from the shell
2. **Realistic Integration**: Test actual compiled binaries, not just function calls
3. **Cross-Platform Compatibility**: Ensure CLI works on Linux, macOS, Windows
4. **Regression Prevention**: Catch breaking changes in command flags, output formats, exit codes
5. **Documentation Validation**: Test examples from documentation actually work

**Key Requirements**:
- Tests must invoke CLI as subprocess (no in-process testing)
- Support for testing stdout, stderr, exit codes, and file I/O
- Ability to test interactive sequences and multi-command workflows
- Fast enough for CI/CD (target: &lt;10s for full suite)
- Easy to write and maintain (prefer declarative over imperative)

## Decision

Use **testscript** for CLI acceptance tests, supplemented with table-driven Go tests for unit-level command testing.

**testscript** is a Go library from the Go team that runs txtar-formatted test scripts:

```txtar
# Test: Basic namespace creation
prismctl namespace create test-ns --backend sqlite
stdout 'Created namespace "test-ns"'
! stderr .
[exit 0]

# Verify namespace exists
prismctl namespace list
stdout 'test-ns.*sqlite'
```

## Rationale

### Why testscript?

#### Pros
1. **Shell-Native Syntax**: Tests look like actual shell sessions
2. **Go Team Blessed**: Used for testing Go itself (`go test`, `go mod`, etc.)
3. **Declarative**: Test intent clear from script, not buried in Go code
4. **Txtar Format**: Embedded files, setup/teardown, multi-step workflows
5. **Fast Execution**: Runs in-process but as separate command invocations
6. **Excellent Tooling**: Built-in assertions for stdout, stderr, exit codes, files
7. **Cross-Platform**: Handles path separators, environment variables correctly

#### Cons
1. **Learning Curve**: Txtar format unfamiliar to developers
2. **Limited Debugging**: Failures harder to debug than native Go tests
3. **Less Flexible**: Some complex scenarios easier in pure Go

### Alternatives Considered

#### 1. BATS (Bash Automated Testing System)

```bash
# test_namespace.bats
@test "create namespace" {
  run prismctl namespace create test-ns --backend sqlite
  [ "$status" -eq 0 ]
  [[ "$output" =~ "Created namespace" ]]
}
```

**Pros**:
- Shell-native, familiar to ops teams
- Large ecosystem, widely used
- Excellent for testing shell scripts

**Cons**:
- **Rejected**: Bash-only (no cross-platform)
- Slower than Go-based solutions
- External dependency not in Go ecosystem
- Harder to integrate with `go test`

#### 2. exec.Command + Table-Driven Tests (Pure Go)

```go
func TestNamespaceCreate(t *testing.T) {
    tests := []struct {
        name string
        args []string
        wantStdout string
        wantExitCode int
    }{
        {"basic", []string{"namespace", "create", "test-ns"}, "Created", 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := exec.Command("prismctl", tt.args...)
            out, err := cmd.CombinedOutput()
            // assertions...
        })
    }
}
```

**Pros**:
- Pure Go, no external dependencies
- Full power of Go testing
- Easy debugging

**Cons**:
- **Rejected**: Verbose and imperative
- Harder to read multi-step workflows
- Manual handling of temp directories, cleanup
- More boilerplate per test

#### 3. Ginkgo + Gomega (BDD-style)

```go
var _ = Describe("Namespace", func() {
    It("creates a namespace", func() {
        session := RunCommand("prismctl", "namespace", "create", "test-ns")
        Eventually(session).Should(gexec.Exit(0))
        Expect(session.Out).To(gbytes.Say("Created"))
    })
})
```

**Pros**:
- BDD-style readability
- Rich matchers
- Popular in Kubernetes ecosystem

**Cons**:
- **Rejected**: Heavy framework for CLI testing
- Still requires Go code for each test
- Slower than testscript
- Not as declarative as txtar scripts

## Decision: testscript

**Chosen for**:
- Declarative shell-like syntax
- Fast execution
- Go team's endorsement
- Perfect fit for CLI acceptance testing

## Implementation

### Directory Structure

tools/
├── cmd/
│   └── prismctl/
│       ├── main.go
│       ├── namespace.go
│       └── ...
├── internal/
│   └── ...
├── testdata/
│   └── script/
│       ├── namespace_create.txtar
│       ├── namespace_list.txtar
│       ├── namespace_delete.txtar
│       ├── session_list.txtar
│       ├── backend_health.txtar
│       └── ...
├── acceptance_test.go         # testscript runner
└── go.mod
```

### Test Runner

```go
// tools/acceptance_test.go
package tools_test

import (
    "os"
    "os/exec"
    "testing"

    "github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
    os.Exit(testscript.RunMain(m, map[string]func() int{
        "prismctl": mainCLI,
    }))
}

func TestScripts(t *testing.T) {
    testscript.Run(t, testscript.Params{
        Dir: "testdata/script",
        Setup: func(env *testscript.Env) error {
            // Set up test environment (mock proxy, temp dirs, etc.)
            env.Setenv("PRISM_ENDPOINT", "localhost:50052")
            env.Setenv("PRISM_CONFIG", env.Getenv("WORK")+"/prism-config.yaml")
            return nil
        },
    })
}

// mainCLI wraps the CLI entry point for testscript
func mainCLI() int {
    if err := rootCmd.Execute(); err != nil {
        return 1
    }
    return 0
}
```

### Example Test Script

```txtar
# testdata/script/namespace_create.txtar

# Test: Create a namespace with explicit configuration
prismctl namespace create my-app \
  --backend sqlite \
  --pattern keyvalue \
  --consistency strong

stdout 'Created namespace "my-app"'
! stderr 'error'

# Test: List namespaces to verify creation
prismctl namespace list
stdout 'my-app.*sqlite.*keyvalue'

# Test: Describe namespace
prismctl namespace describe my-app
stdout 'Namespace: my-app'
stdout 'Backend: sqlite'
stdout 'Pattern: keyvalue'
stdout 'Consistency: strong'

# Test: Delete namespace
prismctl namespace delete my-app --force
stdout 'Deleted namespace "my-app"'

# Verify deletion
prismctl namespace list
! stdout 'my-app'
```

### Advanced Test: Configuration File Discovery

```txtar
# testdata/script/config_discovery.txtar

# Create project config file
-- .prism.yaml --
namespace: my-project
proxy:
  endpoint: localhost:50052
backend:
  type: postgres
  pattern: keyvalue

# Test: CLI discovers config automatically
prismctl namespace create my-project
stdout 'Created namespace "my-project"'
stdout 'Backend: postgres'

# Test: CLI respects config for scoped commands
prismctl config show
stdout 'namespace: my-project'
stdout 'backend:.*postgres'
```

### Multi-Step Workflow Test

```txtar
# testdata/script/shadow_traffic.txtar

# Setup: Create source namespace
prismctl namespace create prod-app --backend postgres

# Setup: Create target namespace
prismctl namespace create prod-app-new --backend redis

# Test: Enable shadow traffic
prismctl shadow enable prod-app \
  --target prod-app-new \
  --percentage 10

stdout 'Shadow traffic enabled'
stdout '10% traffic to prod-app-new'

# Test: Check shadow status
prismctl shadow status prod-app
stdout 'Status: Active'
stdout 'Target: prod-app-new'
stdout '10%.*redis'

# Test: Disable shadow traffic
prismctl shadow disable prod-app
stdout 'Shadow traffic disabled'

# Cleanup
prismctl namespace delete prod-app --force
prismctl namespace delete prod-app-new --force
```

### Error Handling Test

```txtar
# testdata/script/namespace_errors.txtar

# Test: Create namespace with invalid backend
! prismctl namespace create bad-ns --backend invalid-backend
stderr 'error: unsupported backend "invalid-backend"'
[exit 1]

# Test: Delete non-existent namespace
! prismctl namespace delete does-not-exist
stderr 'error: namespace "does-not-exist" not found'
[exit 1]

# Test: Create duplicate namespace
prismctl namespace create duplicate --backend sqlite
! prismctl namespace create duplicate --backend sqlite
stderr 'error: namespace "duplicate" already exists'
[exit 1]

# Cleanup
prismctl namespace delete duplicate --force
```

### JSON Output Test

```txtar
# testdata/script/json_output.txtar

# Create test namespace
prismctl namespace create json-test --backend sqlite

# Test: JSON output format
prismctl namespace list --output json
stdout '{"namespaces":\['
stdout '{"name":"json-test"'
stdout '"backend":"sqlite"'

# Test: Parse JSON with jq (if available)
[exec:jq] prismctl namespace list --output json
stdout '"name":.*"json-test"'

# Cleanup
prismctl namespace delete json-test --force
```

## Testing Strategy

### Test Categories

1. **Smoke Tests** (Fast, &lt;1s total)
   - `prismctl --help`
   - `prismctl --version`
   - Basic command validation

2. **Unit Tests** (Go table-driven tests)
   - Flag parsing
   - Configuration loading
   - Output formatting

3. **Acceptance Tests** (testscript, ~5-10s)
   - End-to-end CLI workflows
   - Integration with mock proxy
   - Error handling paths

4. **Integration Tests** (Against real proxy, slower)
   - Full stack: CLI → Proxy → Backend
   - Separate CI job (not in `go test`)

### Test Organization

testdata/script/
├── smoke/              # Fast smoke tests
│   ├── help.txtar
│   └── version.txtar
├── namespace/          # Namespace management
│   ├── create.txtar
│   ├── list.txtar
│   ├── describe.txtar
│   ├── update.txtar
│   └── delete.txtar
├── backend/            # Backend operations
│   ├── health.txtar
│   └── stats.txtar
├── session/            # Session management
│   ├── list.txtar
│   └── trace.txtar
├── config/             # Configuration
│   ├── discovery.txtar
│   └── validation.txtar
└── errors/             # Error scenarios
    ├── invalid_args.txtar
    └── connection_errors.txtar
```

### CI Integration

```yaml
# .github/workflows/cli-tests.yml
name: CLI Acceptance Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Build CLI
        run: cd tools && go build ./cmd/prismctl

      - name: Run acceptance tests
        run: cd tools && go test -v ./acceptance_test.go

      - name: Upload test results
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: test-results
          path: tools/testdata/script/**/*.log
```

## Performance Targets

- **Smoke tests**: &lt;1s total
- **Acceptance test suite**: &lt;10s total
- **Individual test**: &lt;500ms average
- **Parallel execution**: 4x faster (use `t.Parallel()`)

## Debugging Failed Tests

```bash
# Run single test
cd tools
go test -v -run TestScripts/namespace_create

# Show verbose output
go test -v -run TestScripts/namespace_create -testscript.verbose

# Update golden files
go test -v -run TestScripts/namespace_create -testscript.update
```

## Consequences

### Positive

- Declarative tests that read like shell sessions
- Fast execution (in-process but subprocess-like)
- Cross-platform support out of the box
- Easy to write and maintain
- Excellent for testing CLI UX
- Catches regressions in output formats

### Negative

- Learning curve for txtar format
- Debugging failures less intuitive than pure Go
- Limited access to Go testing utilities inside scripts
- Some complex scenarios still need Go table tests

### Neutral

- Two testing approaches (testscript + Go tests)
- Requires discipline to choose right tool for each test

## Migration Path

### Phase 1: Smoke Tests (Week 1)
- Implement testscript runner
- Add basic smoke tests (help, version, invalid commands)
- Verify CI integration

### Phase 2: Core Commands (Week 2)
- Namespace CRUD tests
- Backend health tests
- Basic error handling

### Phase 3: Advanced Workflows (Week 3)
- Shadow traffic tests
- Multi-step workflows
- Configuration discovery

### Phase 4: Full Coverage (Week 4)
- Session management tests
- Metrics tests
- Edge cases and error scenarios

## References

- [testscript Documentation](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [Txtar Format](https://pkg.go.dev/golang.org/x/tools/txtar)
- [Go Command Testing](https://github.com/golang/go/tree/master/src/cmd/go/testdata/script)
- ADR-012: Go for Tooling
- ADR-015: Go Testing Strategy
- ADR-016: Go CLI Configuration

## Revision History

- 2025-10-09: Initial draft proposing testscript for CLI acceptance tests
