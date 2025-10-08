---
title: "ADR-016: Go CLI and Configuration Management"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['go', 'cli', 'configuration', 'developer-experience']
---

## Context

Prism Go tooling requires robust CLI interfaces with:
- Subcommands for different operations
- Configuration file support
- Environment variable overrides
- Flag parsing with validation
- Consistent UX across tools

## Decision

Use **Cobra** for CLI structure and **Viper** for configuration management.

## Rationale

### Cobra (CLI Framework)
- **Industry Standard**: Used by Kubernetes, Hugo, GitHub CLI, Docker CLI
- **Rich Features**: Subcommands, flags, aliases, help generation
- **POSIX Compliance**: Follows standard CLI conventions
- **Code Generation**: `cobra-cli` scaffolds command structure
- **Testing Support**: Commands are testable units

### Viper (Configuration Management)
- **Layered Configuration**: Flags > Env > Config File > Defaults
- **Multiple Formats**: YAML, JSON, TOML
- **Environment Binding**: Automatic env var mapping
- **Seamless Cobra Integration**: Built to work together

### Configuration Hierarchy (highest to lowest precedence)
1. CLI flags: `--namespace test --backend postgres`
2. Environment variables: `PRISM_NAMESPACE=test PRISM_BACKEND=postgres`
3. Config file: `~/.prism.yaml` or `./prism.yaml`
4. Defaults: Sensible fallbacks

## Configuration Schema

```yaml
# prism.yaml
proxy:
  endpoint: localhost:8980
  timeout: 30s

logging:
  level: info  # debug, info, warn, error
  format: json  # json, text

migrate:
  batch_size: 1000
  workers: 4
```

## CLI Structure

### prism-cli

```bash
prism-cli [command] [flags]

Commands:
  get        Get a value from Prism
  put        Put a value into Prism
  delete     Delete a value from Prism
  scan       Scan values in a namespace
  config     Show resolved configuration

Global Flags:
  -c, --config string      Config file (default: ~/.prism.yaml)
  -e, --endpoint string    Prism proxy endpoint (default: localhost:8980)
      --log-level string   Log level: debug, info, warn, error
      --log-format string  Log format: json, text
```

### prism-migrate

```bash
prism-migrate [command] [flags]

Commands:
  run        Run data migration
  validate   Validate migration configuration
  status     Show migration status

Flags:
  --source string        Source connection string
  --dest string          Destination connection string
  --batch-size int       Batch size (default: 1000)
  --workers int          Concurrent workers (default: NumCPU)
  --dry-run              Validate without migrating
```

### prism-bench

```bash
prism-bench [command] [flags]

Commands:
  load       Run load test
  report     Generate report from results

Flags:
  --duration duration    Test duration (default: 1m)
  --rps int             Target requests per second
  --workers int         Concurrent workers
  --pattern string      Access pattern: random, sequential
```

## Examples

```bash
# Get a value
prism-cli get test user123 profile

# Put a value
prism-cli put test user123 profile '{"name":"Alice"}'

# Scan namespace
prism-cli scan test user123

# Show configuration
prism-cli config

# Run migration
prism-migrate run \
  --source postgres://localhost/old \
  --dest postgres://localhost/new \
  --workers 8

# Load test
prism-bench load --duration 5m --rps 10000
```

## Implementation Notes

### Dependencies

```go
require (
    github.com/spf13/cobra v1.8.1
    github.com/spf13/viper v1.19.0
)
```

### Package Structure

```
tools/
├── cmd/
│   ├── prism-cli/
│   │   ├── main.go        # Entry point
│   │   ├── root.go        # Root command
│   │   ├── get.go         # Get subcommand
│   │   ├── put.go         # Put subcommand
│   │   └── config.go      # Config subcommand
│   ├── prism-migrate/
│   │   ├── main.go
│   │   ├── root.go
│   │   └── run.go
│   └── prism-bench/
│       ├── main.go
│       ├── root.go
│       └── load.go
├── internal/
│   └── config/
│       ├── config.go      # Config types
│       └── loader.go      # Viper integration
```

### Example Implementation

```go
// cmd/prism-cli/root.go
package main

import (
    "log/slog"
    "os"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
    Use:   "prism-cli",
    Short: "Prism command-line interface",
    PersistentPreRun: func(cmd *cobra.Command, args []string) {
        // Initialize logging
        initLogging()
    },
}

func init() {
    cobra.OnInitialize(initConfig)

    rootCmd.PersistentFlags().StringP("config", "c", "", "config file (default: ~/.prism.yaml)")
    rootCmd.PersistentFlags().StringP("endpoint", "e", "localhost:8980", "Prism proxy endpoint")
    rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
    rootCmd.PersistentFlags().String("log-format", "json", "log format (json, text)")

    viper.BindPFlag("proxy.endpoint", rootCmd.PersistentFlags().Lookup("endpoint"))
    viper.BindPFlag("logging.level", rootCmd.PersistentFlags().Lookup("log-level"))
    viper.BindPFlag("logging.format", rootCmd.PersistentFlags().Lookup("log-format"))
}

func initConfig() {
    if cfgFile := rootCmd.PersistentFlags().Lookup("config").Value.String(); cfgFile != "" {
        viper.SetConfigFile(cfgFile)
    } else {
        home, _ := os.UserHomeDir()
        viper.AddConfigPath(home)
        viper.AddConfigPath(".")
        viper.SetConfigName(".prism")
        viper.SetConfigType("yaml")
    }

    viper.SetEnvPrefix("PRISM")
    viper.AutomaticEnv()

    viper.ReadInConfig()
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        slog.Error("command failed", "error", err)
        os.Exit(1)
    }
}
```

## Consequences

### Positive

- Industry-standard tools with large communities
- Rich feature set without custom implementation
- Excellent documentation and examples
- Clear configuration precedence
- Easy testing

### Negative

- Two dependencies (but they work together seamlessly)
- Learning curve for contributors

### Neutral

- Config file watching not needed for CLI tools (useful for daemons)

## References

- [Cobra Documentation](https://github.com/spf13/cobra)
- [Viper Documentation](https://github.com/spf13/viper)
- [12-Factor App Config](https://12factor.net/config)
- ADR-012: Go for Tooling
- org-stream-producer ADR-010: Command-Line Configuration

## Revision History

- 2025-10-07: Initial draft and acceptance (adapted from org-stream-producer)
