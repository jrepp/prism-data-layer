---
title: "RFC-028: prism-probe - CLI Client for Testing and Debugging"
status: Proposed
author: Claude
created: 2025-10-11
updated: 2025-10-11
tags: [cli, testing, debugging, client]
id: rfc-028
---

# RFC-028: prism-probe - CLI Client for Testing and Debugging

## Summary

**prism-probe** is a command-line tool for testing, debugging, and interacting with Prism data access patterns. It provides flexible test scenarios, data generation, and pattern validation without writing code.

**Name rationale**: "probe" is short, memorable, and clearly indicates testing/inspection functionality. Similar to `kubectl`, `redis-cli`, `psql` - single-purpose debugging tools.

## Motivation

**User request:** "we should create a new client command line `client` that gives us some flexible client test scenarios that can be run on the command line against the proxy"

### Current Pain Points

1. **No Quick Testing**: Testing patterns requires writing Go/Rust/Python code
2. **Manual Setup**: Each test scenario needs boilerplate (connection, auth, cleanup)
3. **No Load Testing**: No easy way to generate load against patterns
4. **Debugging is Hard**: Can't easily inspect pattern state or message flow
5. **No Validation**: Can't verify pattern configuration before deploying

### Goals

- **Zero Code Testing**: Test patterns from command line without programming
- **Scenario Library**: Pre-built test scenarios for common use cases
- **Load Generation**: Built-in load testing with configurable profiles
- **Debugging Tools**: Inspect state, trace messages, validate configuration
- **Scriptable**: Composable commands for CI/CD and automation

## Design

### Command Structure

```bash
prism-probe [global-options] <command> [command-options]

Commands:
  keyvalue    Test KeyValue pattern operations
  pubsub      Test PubSub pattern operations
  multicast   Test Multicast Registry pattern operations
  queue       Test Queue pattern operations
  timeseries  Test TimeSeries pattern operations

  scenario    Run predefined test scenarios
  load        Generate load with configurable profiles
  inspect     Inspect pattern state and configuration
  validate    Validate pattern configuration before deployment

Global Options:
  --proxy <url>       Proxy address (default: localhost:50051)
  --auth <token>      Authentication token (default: $PRISM_TOKEN)
  --namespace <ns>    Target namespace (default: default)
  --pattern <name>    Pattern name
  --format <fmt>      Output format: json|yaml|table (default: table)
  --verbose           Enable verbose logging
  --trace             Enable gRPC tracing
```

### Pattern-Specific Commands

#### KeyValue Pattern

```bash
# Set a key
prism-probe keyvalue set --pattern user-cache --key user:123 --value '{"name":"Alice"}'

# Get a key
prism-probe keyvalue get --pattern user-cache --key user:123

# Set with TTL
prism-probe keyvalue set --pattern session-store --key session:abc --value '{}' --ttl 3600

# Delete a key
prism-probe keyvalue delete --pattern user-cache --key user:123

# Batch operations
prism-probe keyvalue mset --pattern user-cache --file users.json
prism-probe keyvalue mget --pattern user-cache --keys user:1,user:2,user:3

# Scan keys
prism-probe keyvalue scan --pattern user-cache --prefix "user:" --limit 100
```

#### PubSub Pattern

```bash
# Publish a message
prism-probe pubsub publish --pattern events --topic user.created --message '{"user_id":"123"}'

# Subscribe to topic (blocks, prints messages)
prism-probe pubsub subscribe --pattern events --topic "user.*"

# Subscribe with filter
prism-probe pubsub subscribe --pattern events --topic "user.*" --filter '{"status":"active"}'

# Publish from file
prism-probe pubsub publish --pattern events --topic orders.new --file order.json

# Publish multiple messages
prism-probe pubsub publish-batch --pattern events --topic test --count 100 --rate 10/sec
```

#### Multicast Registry Pattern

```bash
# Register identity with metadata
prism-probe multicast register --pattern devices \
  --identity device-1 \
  --metadata '{"status":"online","region":"us-west"}' \
  --ttl 300

# Enumerate identities with filter
prism-probe multicast enumerate --pattern devices \
  --filter '{"status":"online"}'

# Multicast message to filtered identities
prism-probe multicast publish --pattern devices \
  --filter '{"status":"online","region":"us-west"}' \
  --message "firmware-update-v2"

# Unregister identity
prism-probe multicast unregister --pattern devices --identity device-1

# Subscribe as consumer (blocks, prints multicasted messages)
prism-probe multicast subscribe --pattern devices --identity device-1
```

### Scenario Command

Pre-built test scenarios for common patterns:

```bash
# List available scenarios
prism-probe scenario list

# Run a scenario
prism-probe scenario run --name user-session-flow --pattern user-cache

# Run with custom parameters
prism-probe scenario run --name user-session-flow \
  --pattern user-cache \
  --params '{"users":100,"duration":"5m"}'

# Available scenarios:
prism-probe scenario list

NAME                    PATTERN             DESCRIPTION
user-session-flow       keyvalue            Simulates user login/logout with session caching
event-fanout            pubsub              Publishes events, verifies all subscribers receive
device-registration     multicast-registry  Registers devices, multicasts commands, measures latency
order-processing        queue               Produces orders, consumes with workers, tracks completion
metrics-ingestion       timeseries          Writes time-series data, queries aggregations
```

### Load Command

Generate load with configurable profiles:

```bash
# Constant rate load test
prism-probe load run --pattern user-cache \
  --operation keyvalue.set \
  --rate 1000/sec \
  --duration 60s

# Ramp-up profile (0 → 5000 RPS over 2 minutes)
prism-probe load run --pattern events \
  --operation pubsub.publish \
  --profile ramp-up \
  --start-rate 0 \
  --end-rate 5000/sec \
  --ramp-duration 2m \
  --steady-duration 5m

# Spike profile (baseline → spike → baseline)
prism-probe load run --pattern devices \
  --operation multicast.register \
  --profile spike \
  --baseline-rate 100/sec \
  --spike-rate 10000/sec \
  --spike-duration 30s

# Custom profile from file
prism-probe load run --pattern orders \
  --operation queue.enqueue \
  --profile-file load-profile.yaml

# Example profile file (load-profile.yaml):
phases:
  - name: warmup
    rate: 100/sec
    duration: 1m

  - name: ramp-up
    start_rate: 100/sec
    end_rate: 5000/sec
    duration: 5m

  - name: steady
    rate: 5000/sec
    duration: 10m

  - name: ramp-down
    start_rate: 5000/sec
    end_rate: 0/sec
    duration: 2m
```

### Inspect Command

Debugging and observability:

```bash
# Inspect pattern configuration
prism-probe inspect config --pattern user-cache

# Inspect pattern state (backend-specific)
prism-probe inspect state --pattern devices \
  --backend registry \
  --query '{"status":"online"}'

# Trace a request (follows request through proxy → plugin → backend)
prism-probe inspect trace --pattern events \
  --operation pubsub.publish \
  --payload '{"test":true}'

# Show pattern metrics
prism-probe inspect metrics --pattern user-cache

# Show backend health
prism-probe inspect health --pattern user-cache --backend redis
```

### Validate Command

Pre-deployment validation:

```bash
# Validate pattern configuration file
prism-probe validate config --file pattern.yaml

# Validate backend connectivity
prism-probe validate backend --pattern user-cache

# Validate schema (if message_schema is configured)
prism-probe validate schema --pattern devices --message-file event.json

# Validate load test plan
prism-probe validate load-profile --file load-profile.yaml
```

## Implementation

### Technology Stack

- **Language**: Go (same as patterns, easy distribution as single binary)
- **CLI Framework**: [cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper) (industry standard)
- **gRPC Client**: Use generated client from `proto/` definitions
- **Output Formatting**: [tablewriter](https://github.com/olekukonko/tablewriter) for tables, stdlib `encoding/json` for JSON/YAML

### Directory Structure

```text
cli/prism-probe/
├── cmd/
│   ├── root.go              # Root command, global flags
│   ├── keyvalue.go          # keyvalue subcommands
│   ├── pubsub.go            # pubsub subcommands
│   ├── multicast.go         # multicast subcommands
│   ├── queue.go             # queue subcommands
│   ├── timeseries.go        # timeseries subcommands
│   ├── scenario.go          # scenario subcommands
│   ├── load.go              # load subcommands
│   ├── inspect.go           # inspect subcommands
│   └── validate.go          # validate subcommands
├── pkg/
│   ├── client/              # gRPC client wrappers
│   ├── scenarios/           # Pre-built test scenarios
│   ├── load/                # Load generation engine
│   ├── format/              # Output formatters (table, JSON, YAML)
│   └── trace/               # Request tracing utilities
├── examples/                # Example configuration files
├── main.go
├── go.mod
└── README.md
```

### Example Implementation: KeyValue Set

```go
// cmd/keyvalue.go
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/prism/cli/prism-probe/pkg/client"
)

var keyvalueSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a key-value pair",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse flags
		pattern, _ := cmd.Flags().GetString("pattern")
		key, _ := cmd.Flags().GetString("key")
		value, _ := cmd.Flags().GetString("value")
		ttl, _ := cmd.Flags().GetDuration("ttl")

		// Create client
		c, err := client.NewProxyClient(
			globalFlags.ProxyURL,
			globalFlags.Namespace,
			globalFlags.AuthToken,
		)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer c.Close()

		// Execute operation
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = c.KeyValue().Set(ctx, pattern, key, []byte(value), ttl)
		if err != nil {
			return fmt.Errorf("set failed: %w", err)
		}

		// Output result
		if globalFlags.Format == "json" {
			fmt.Printf(`{"status":"ok","key":"%s"}\n`, key)
		} else {
			fmt.Printf("✓ Set %s = %s\n", key, value)
		}

		return nil
	},
}

func init() {
	keyvalueCmd.AddCommand(keyvalueSetCmd)

	keyvalueSetCmd.Flags().String("pattern", "", "Pattern name (required)")
	keyvalueSetCmd.MarkFlagRequired("pattern")

	keyvalueSetCmd.Flags().String("key", "", "Key to set (required)")
	keyvalueSetCmd.MarkFlagRequired("key")

	keyvalueSetCmd.Flags().String("value", "", "Value to set (required)")
	keyvalueSetCmd.MarkFlagRequired("value")

	keyvalueSetCmd.Flags().Duration("ttl", 0, "TTL duration (e.g., 3600s, 1h, 0 for no expiration)")
}
```

### Example Scenario: User Session Flow

```go
// pkg/scenarios/user_session_flow.go
package scenarios

import (
	"context"
	"fmt"
	"time"
	"math/rand"
)

type UserSessionFlow struct {
	Pattern string
	Users   int
	Duration time.Duration
}

func (s *UserSessionFlow) Run(ctx context.Context, client *ProxyClient) error {
	fmt.Printf("Running user session flow scenario\n")
	fmt.Printf("  Pattern: %s\n", s.Pattern)
	fmt.Printf("  Users: %d\n", s.Users)
	fmt.Printf("  Duration: %s\n", s.Duration)

	start := time.Now()
	operations := 0

	for time.Since(start) < s.Duration {
		// Randomly pick a user
		userID := rand.Intn(s.Users)
		key := fmt.Sprintf("session:%d", userID)

		// 70% login, 20% access, 10% logout
		action := rand.Float64()

		if action < 0.7 {
			// Login: create session
			session := fmt.Sprintf(`{"user_id":%d,"logged_in_at":%d}`, userID, time.Now().Unix())
			err := client.KeyValue().Set(ctx, s.Pattern, key, []byte(session), 1*time.Hour)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			operations++
		} else if action < 0.9 {
			// Access: read session
			_, err := client.KeyValue().Get(ctx, s.Pattern, key)
			if err != nil && err != ErrNotFound {
				return fmt.Errorf("access failed: %w", err)
			}
			operations++
		} else {
			// Logout: delete session
			err := client.KeyValue().Delete(ctx, s.Pattern, key)
			if err != nil {
				return fmt.Errorf("logout failed: %w", err)
			}
			operations++
		}

		// Rate limiting: ~100 ops/sec per user
		time.Sleep(time.Duration(s.Users) * 10 * time.Millisecond)
	}

	elapsed := time.Since(start)
	opsPerSec := float64(operations) / elapsed.Seconds()

	fmt.Printf("\nScenario complete:\n")
	fmt.Printf("  Operations: %d\n", operations)
	fmt.Printf("  Duration: %s\n", elapsed)
	fmt.Printf("  Rate: %.2f ops/sec\n", opsPerSec)

	return nil
}
```

## Examples

### Example 1: Quick KeyValue Test

```bash
# Set a value
$ prism-probe keyvalue set --pattern user-cache --key user:alice --value '{"name":"Alice","age":30}'
✓ Set user:alice = {"name":"Alice","age":30}

# Get the value
$ prism-probe keyvalue get --pattern user-cache --key user:alice
KEY         VALUE
user:alice  {"name":"Alice","age":30}

# Delete the value
$ prism-probe keyvalue delete --pattern user-cache --key user:alice
✓ Deleted user:alice
```

### Example 2: Multicast Registry Test

```bash
# Register 3 devices
$ prism-probe multicast register --pattern iot \
  --identity device-1 \
  --metadata '{"status":"online","region":"us-west"}'
✓ Registered device-1

$ prism-probe multicast register --pattern iot \
  --identity device-2 \
  --metadata '{"status":"offline","region":"us-west"}'
✓ Registered device-2

$ prism-probe multicast register --pattern iot \
  --identity device-3 \
  --metadata '{"status":"online","region":"eu-west"}'
✓ Registered device-3

# Enumerate online devices
$ prism-probe multicast enumerate --pattern iot \
  --filter '{"status":"online"}'

IDENTITY    STATUS    REGION     REGISTERED_AT
device-1    online    us-west    2025-10-11T10:30:00Z
device-3    online    eu-west    2025-10-11T10:30:15Z

# Multicast to online devices in us-west
$ prism-probe multicast publish --pattern iot \
  --filter '{"status":"online","region":"us-west"}' \
  --message "firmware-update-v2"

✓ Multicasted to 1 identities (delivered: 1, failed: 0)
```

### Example 3: Load Test

```bash
# Run 5-minute load test with ramp-up
$ prism-probe load run --pattern user-cache \
  --operation keyvalue.set \
  --profile ramp-up \
  --start-rate 0 \
  --end-rate 5000/sec \
  --ramp-duration 2m \
  --steady-duration 3m

Load Test: keyvalue.set on pattern user-cache
Profile: ramp-up (0 → 5000/sec over 2m, steady 3m)

[====================] 100% | 5m0s | 750,000 ops | 2,500/sec | Latency: p50=2.1ms p95=8.3ms p99=15.2ms

Results:
  Total Operations: 750,000
  Success Rate: 99.98%
  Throughput: 2,500 ops/sec
  Latency:
    p50: 2.1ms
    p95: 8.3ms
    p99: 15.2ms
    p999: 23.1ms
  Errors: 150 (0.02%)
```

### Example 4: CI/CD Validation

```bash
# In CI pipeline: validate pattern before deployment
$ prism-probe validate config --file deploy/patterns/user-cache.yaml
✓ Syntax valid
✓ Schema valid
✓ Backend configuration valid

$ prism-probe validate backend --pattern user-cache
✓ Redis connection successful (localhost:6379)
✓ Read/write test passed
✓ Latency within threshold (p99 < 10ms)

$ echo "Pattern validated, proceeding with deployment"
```

## Benefits

1. **Rapid Prototyping**: Test patterns in seconds without writing code
2. **Load Testing**: Built-in load generation with realistic profiles
3. **Debugging**: Inspect state, trace requests, validate configuration
4. **CI/CD Integration**: Automated validation before deployment
5. **Documentation**: Commands serve as executable documentation
6. **Cross-Platform**: Single binary for Linux/macOS/Windows

## Open Questions

1. **Auth Integration**: How to handle different auth methods (mTLS, OAuth2, API keys)?
2. **Schema Awareness**: Should probe auto-generate message payloads from schemas?
3. **Distributed Load**: Support distributed load generation across multiple machines?
4. **UI Mode**: Add interactive TUI (terminal UI) mode with live dashboards?

## Recommendations

**POC 4 (Week 2)**:
- Implement basic commands: `keyvalue`, `multicast`, `inspect config`
- Single binary for macOS (development target)
- JSON/table output formats

**POC 5 (Week 3)**:
- Add `scenario` and `load` commands
- Implement 3 pre-built scenarios
- Load testing with ramp-up profiles

**Production**:
- All pattern types supported
- Distributed load testing
- Interactive TUI mode with live metrics
- CI/CD integration examples
- Cross-platform builds (Linux, macOS, Windows, Docker)

## Related

- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017)
- [MEMO-008: Message Schema Configuration](/memos/memo-008)
- [ADR-040: CLI-First Tooling](/adr/adr-040)
