# prism-admin

Administrative CLI for managing Prism via the Admin gRPC API (port 8981).

**Features:**
- Namespace management (create, list, delete)
- Backend health monitoring
- Configuration via flags, environment variables, or config file

## Installation

### Build from Source

```bash
cd cmd/prism-admin
go build -o prism-admin .
```

### Install Globally

```bash
cd cmd/prism-admin
go install .
```

## Usage

### Basic Commands

```bash
# List namespaces
prism-admin namespace list

# Create a namespace
prism-admin namespace create my-app \
  --description "My application namespace" \
  --max-sessions 1000

# Delete a namespace
prism-admin namespace delete my-app

# Check backend health
prism-admin health

# Check specific backend
prism-admin health --backend postgres
```

### Configuration

Configuration precedence (highest to lowest):
1. Command-line flags
2. Environment variables (prefix: `PRISM_`)
3. Config file (`~/.prism.yaml` or `./prism.yaml`)
4. Defaults

**Example config file** (`~/.prism.yaml`):

```yaml
admin:
  endpoint: localhost:8981

logging:
  level: info
```

**Environment variables:**

```bash
export PRISM_ADMIN_ENDPOINT=localhost:8981
export PRISM_LOGGING_LEVEL=debug
```

**Command-line flags:**

```bash
prism-admin --endpoint localhost:8981 --log-level debug namespace list
```

### Global Flags

- `--endpoint, -e`: Admin API endpoint (default: `localhost:8981`)
- `--config, -c`: Config file path
- `--log-level`: Log level (debug, info, warn, error)

## Development

### Project Structure

```
cmd/prism-admin/
├── main.go       # Entry point
├── root.go       # Root command + config
├── namespace.go  # Namespace commands
├── health.go     # Health commands
├── go.mod
└── README.md
```

### Dependencies

- **Cobra**: CLI framework
- **Viper**: Configuration management
- **gRPC**: Admin API client

### Testing

```bash
# Run from cmd/prism-admin
go test ./...

# Build
go build .

# Run
./prism-admin --help
```

### Adding New Commands

1. Create a new file (e.g., `session.go`)
2. Define command using Cobra patterns
3. Add command to `rootCmd` in `root.go`
4. Keep code simple and readable

**Example:**

```go
// session.go
package main

import (
    "github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
    Use:   "session",
    Short: "Manage sessions",
}

var sessionListCmd = &cobra.Command{
    Use:   "list",
    Short: "List active sessions",
    RunE:  runSessionList,
}

func init() {
    sessionCmd.AddCommand(sessionListCmd)
}

func runSessionList(cmd *cobra.Command, args []string) error {
    // Implementation
    return nil
}
```

Then in `root.go`:

```go
func init() {
    // ...
    rootCmd.AddCommand(sessionCmd)
}
```

## Architecture

This CLI follows the patterns defined in:
- **ADR-012**: Go for Tooling
- **ADR-016**: Go CLI and Configuration Management
- **ADR-027**: Admin API via gRPC
- **ADR-039**: CLI Acceptance Testing with testscript

### Why Go?

- **Single binary**: No runtime dependencies
- **Fast compilation**: Rapid iteration
- **Strong standard library**: Excellent for CLI tools
- **Protobuf interoperability**: Shares proto definitions with Rust proxy
- **Cross-platform**: Easy to build for Linux, macOS, Windows

### Why Cobra + Viper?

- **Industry standard**: Used by Kubernetes, Docker, Hugo, GitHub CLI
- **Rich features**: Subcommands, flags, help generation
- **Configuration layers**: Flags > Env > Config > Defaults
- **Well documented**: Large community, many examples

## TODO

- [ ] Generate Go protobuf client from `proto/prism/admin/v1/admin.proto`
- [ ] Implement actual gRPC calls to Admin API
- [ ] Add session management commands
- [ ] Add operational commands (maintenance mode, drain)
- [ ] Add shell-based acceptance tests (testscript)
- [ ] Add output formatting (JSON, table, YAML)

## References

- [Cobra Documentation](https://github.com/spf13/cobra)
- [Viper Documentation](https://github.com/spf13/viper)
- [gRPC Go Quick Start](https://grpc.io/docs/languages/go/quickstart/)
- RFC-003: Admin Interface for Prism
- ADR-027: Admin API via gRPC
