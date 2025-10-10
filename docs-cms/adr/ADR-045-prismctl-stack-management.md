---
title: "ADR-045: prismctl Stack Management Subcommand"
status: Accepted
date: 2025-10-09
deciders: Core Team
tags: [go, cli, tooling, dx, infrastructure, hashicorp]
---

## Context

Prism requires a local development stack with multiple components (Consul, Vault, Kafka, PostgreSQL, etc.). Operators need a simple way to:

1. **Bootstrap** a complete local environment
2. **Start/stop** infrastructure components
3. **Switch** between different infrastructure providers (Hashicorp, AWS, local Docker)
4. **Manage** configuration and credentials for stack components

Previously, we considered creating a separate `hashistack` tool, but this would add another binary to manage and maintain.

## Decision

Add a **`stack` subcommand** to `prismctl` that manages infrastructure provisioning and lifecycle.

**Installation** (single command):
```bash
# Install prismctl with stack management
go install github.com/jrepp/prism-data-layer/tools/cmd/prismctl@latest

# Bootstrap local stack (one-time)
prismctl stack init
```

**Usage**:
```bash
# Initialize stack configuration
prismctl stack init

# Start the default stack (Hashicorp)
prismctl stack start

# Stop the stack
prismctl stack stop

# Check stack health
prismctl stack status

# Switch to different stack provider
prismctl stack use docker-compose
prismctl stack use aws

# List available stack providers
prismctl stack providers
```

## Rationale

### Why Stack Management in prismctl?

#### 1. Single Binary for All Operations

Users install one tool that does everything:

```bash
# One install command
go install github.com/jrepp/prism-data-layer/tools/cmd/prismctl@latest

# All operations available
prismctl namespace list         # Admin operations
prismctl plugin start postgres  # Plugin management
prismctl stack start            # Infrastructure management
```

**Benefits**:
- ✅ No need to manage multiple binaries
- ✅ Consistent CLI patterns across all operations
- ✅ Simplified installation instructions
- ✅ Single version to track and update

#### 2. Natural Command Hierarchy

The `stack` subcommand fits naturally into prismctl's structure:

prismctl
├── namespace      # Manage Prism namespaces
├── plugin         # Manage backend plugins
├── session        # Manage sessions
├── health         # Check Prism health
└── stack          # Manage infrastructure stack
    ├── init       # Initialize stack config
    ├── start      # Start infrastructure
    ├── stop       # Stop infrastructure
    ├── status     # Check stack health
    ├── use        # Switch stack provider
    └── providers  # List available providers
```text

#### 3. Shared Configuration

Stack management shares configuration with other prismctl commands:

```yaml
# ~/.prism/config.yaml
admin:
  endpoint: localhost:8981

stack:
  provider: hashicorp
  consul:
    address: localhost:8500
  vault:
    address: localhost:8200
  postgres:
    host: localhost
    port: 5432
  kafka:
    brokers: [localhost:9092]

plugins:
  postgres:
    image: prism/postgres-plugin:latest
    port: 9090
```text

**Benefits**:
- ✅ Single source of truth for configuration
- ✅ Stack and admin operations see same endpoints
- ✅ Easy to switch environments (dev, staging, prod)

### Pluggable Stack Providers

The `stack` subcommand supports multiple infrastructure providers:

#### 1. Hashicorp (Default)

Uses Consul, Vault, and Nomad for service discovery, secrets, and orchestration:

```bash
# Use Hashicorp stack (default)
prismctl stack init --provider hashicorp

# Starts:
# - Consul (service discovery)
# - Vault (secrets management)
# - PostgreSQL (via Docker)
# - Kafka (via Docker)
# - NATS (via Docker)
```text

**Configuration** (`~/.prism/stacks/hashicorp.yaml`):
```yaml
provider: hashicorp

services:
  consul:
    enabled: true
    mode: dev
    data_dir: ~/.prism/data/consul

  vault:
    enabled: true
    mode: dev
    data_dir: ~/.prism/data/vault
    kv_version: 2

  postgres:
    enabled: true
    image: postgres:16-alpine
    port: 5432
    databases: [prism]

  kafka:
    enabled: true
    image: confluentinc/cp-kafka:latest
    port: 9092

  nats:
    enabled: true
    image: nats:latest
    ports: [4222, 8222]
```text

**Stack operations**:
```go
type HashicorpStack struct {
    config *HashicorpConfig
}

func (s *HashicorpStack) Start(ctx context.Context) error {
    // 1. Start Consul
    if err := s.startConsul(ctx); err != nil {
        return err
    }

    // 2. Start Vault
    if err := s.startVault(ctx); err != nil {
        return err
    }

    // 3. Start databases via Docker
    if err := s.startDatabases(ctx); err != nil {
        return err
    }

    return nil
}
```text

#### 2. Docker Compose

Simple Docker-based local development:

```bash
# Use Docker Compose stack
prismctl stack init --provider docker-compose

# Uses docker-compose.yml for all services
prismctl stack start
```text

**Configuration** (`~/.prism/stacks/docker-compose.yaml`):
```yaml
provider: docker-compose

compose_file: ~/.prism/docker-compose.yml

services:
  postgres: {}
  kafka: {}
  nats: {}
  redis: {}
```text

**Stack operations**:
```go
type DockerComposeStack struct {
    composeFile string
}

func (s *DockerComposeStack) Start(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "docker-compose",
        "-f", s.composeFile,
        "up", "-d",
    )
    return cmd.Run()
}
```text

#### 3. AWS

Cloud-native using AWS services:

```bash
# Use AWS stack
prismctl stack init --provider aws

# Creates:
# - RDS PostgreSQL instance
# - MSK (Kafka) cluster
# - Secrets Manager for credentials
# - VPC, subnets, security groups
```text

**Configuration** (`~/.prism/stacks/aws.yaml`):
```yaml
provider: aws

region: us-west-2

services:
  rds:
    enabled: true
    engine: postgres
    instance_class: db.t3.micro
    allocated_storage: 20

  msk:
    enabled: true
    kafka_version: 3.5.1
    broker_count: 3
    instance_type: kafka.t3.small

  secrets_manager:
    enabled: true
    secrets: [postgres-admin, kafka-creds]
```text

#### 4. Kubernetes

Deploy to Kubernetes cluster:

```bash
# Use Kubernetes stack
prismctl stack init --provider kubernetes

# Applies Helm charts or manifests
prismctl stack start
```text

### Stack Provider Interface

All providers implement a common interface:

```go
type StackProvider interface {
    // Initialize creates configuration files
    Init(ctx context.Context, opts *InitOptions) error

    // Start provisions and starts infrastructure
    Start(ctx context.Context) error

    // Stop tears down infrastructure
    Stop(ctx context.Context) error

    // Status returns health of all components
    Status(ctx context.Context) (*StackStatus, error)

    // GetEndpoints returns service endpoints
    GetEndpoints(ctx context.Context) (*Endpoints, error)
}

type StackStatus struct {
    Healthy  bool
    Services []ServiceStatus
}

type ServiceStatus struct {
    Name    string
    Healthy bool
    Message string
}

type Endpoints struct {
    Consul   string
    Vault    string
    Postgres string
    Kafka    []string
    NATS     string
}
```text

### Implementation Example

**Stack initialization**:
```go
// cmd/prismctl/stack.go
var stackInitCmd = &cobra.Command{
    Use:   "init",
    Short: "Initialize stack configuration",
    RunE:  runStackInit,
}

func runStackInit(cmd *cobra.Command, args []string) error {
    provider := viper.GetString("stack.provider")

    // Create provider
    stack, err := createStackProvider(provider)
    if err != nil {
        return err
    }

    // Initialize configuration
    return stack.Init(cmd.Context(), &InitOptions{
        ConfigDir: getConfigDir(),
    })
}

func createStackProvider(name string) (StackProvider, error) {
    switch name {
    case "hashicorp":
        return &HashicorpStack{}, nil
    case "docker-compose":
        return &DockerComposeStack{}, nil
    case "aws":
        return &AWSStack{}, nil
    case "kubernetes":
        return &KubernetesStack{}, nil
    default:
        return nil, fmt.Errorf("unknown provider: %s", name)
    }
}
```text

**Stack start**:
```go
var stackStartCmd = &cobra.Command{
    Use:   "start",
    Short: "Start infrastructure stack",
    RunE:  runStackStart,
}

func runStackStart(cmd *cobra.Command, args []string) error {
    provider := viper.GetString("stack.provider")
    stack, err := createStackProvider(provider)
    if err != nil {
        return err
    }

    fmt.Printf("Starting %s stack...\n", provider)

    if err := stack.Start(cmd.Context()); err != nil {
        return err
    }

    // Display endpoints
    endpoints, err := stack.GetEndpoints(cmd.Context())
    if err != nil {
        return err
    }

    fmt.Println("\nStack started successfully!")
    fmt.Printf("Consul:    %s\n", endpoints.Consul)
    fmt.Printf("Vault:     %s\n", endpoints.Vault)
    fmt.Printf("Postgres:  %s\n", endpoints.Postgres)
    fmt.Printf("Kafka:     %v\n", endpoints.Kafka)

    return nil
}
```text

## Bootstrap Workflow

### Installation and Setup

```bash
# 1. Install prismctl
go install github.com/jrepp/prism-data-layer/tools/cmd/prismctl@latest

# 2. Initialize stack (creates ~/.prism directory and config)
prismctl stack init

# Output:
# ✓ Created ~/.prism directory
# ✓ Generated config: ~/.prism/config.yaml
# ✓ Generated stack config: ~/.prism/stacks/hashicorp.yaml
# ✓ Generated plugin manifests: ~/.prism/plugins/*.yaml

# 3. Start the stack
prismctl stack start

# Output:
# Starting Hashicorp stack...
# ✓ Starting Consul (dev mode)
# ✓ Starting Vault (dev mode)
# ✓ Starting PostgreSQL (Docker)
# ✓ Starting Kafka (Docker)
# ✓ Starting NATS (Docker)
#
# Stack started successfully!
# Consul:    http://localhost:8500
# Vault:     http://localhost:8200
# Postgres:  localhost:5432
# Kafka:     [localhost:9092]

# 4. Use prismctl for admin operations
prismctl health
prismctl namespace create my-app
```text

### One-Command Bootstrap

Combine init + start:

```bash
# Bootstrap everything in one command
prismctl stack bootstrap

# Equivalent to:
# prismctl stack init
# prismctl stack start
```text

## Configuration Files

After `prismctl stack init`, the structure is:

~/.prism/
├── config.yaml                    # Main prismctl config
├── stacks/
│   ├── hashicorp.yaml             # Hashicorp stack config
│   ├── docker-compose.yaml        # Docker Compose config
│   ├── aws.yaml                   # AWS stack config
│   └── kubernetes.yaml            # Kubernetes stack config
├── plugins/
│   ├── postgres.yaml              # PostgreSQL plugin manifest
│   ├── kafka.yaml                 # Kafka plugin manifest
│   └── redis.yaml                 # Redis plugin manifest
└── data/                          # Stack data directories
    ├── consul/
    ├── vault/
    └── postgres/
```

## Stack Management Commands

```bash
# Initialize configuration
prismctl stack init [--provider <name>]

# Bootstrap (init + start)
prismctl stack bootstrap

# Start infrastructure
prismctl stack start

# Stop infrastructure
prismctl stack stop

# Check status
prismctl stack status

# Get endpoints
prismctl stack endpoints

# Switch provider
prismctl stack use <provider>

# List providers
prismctl stack providers

# Clean up (removes data)
prismctl stack clean [--all]
```

## Consequences

### Positive

- **Single binary**: `go install` gets everything needed
- **Consistent UX**: Stack management uses same patterns as other prismctl commands
- **Pluggable**: Easy to add new stack providers
- **Shared config**: Stack and admin operations use same configuration
- **Fast bootstrap**: One command to get complete dev environment
- **Flexible**: Supports local dev (Docker), cloud (AWS), and enterprise (Hashicorp)

### Negative

- **Binary size**: Stack management adds ~2-3MB to prismctl binary
  - *Acceptable*: Still single binary &lt;20MB total
- **Complexity**: More code in one repository
  - *Mitigated*: Stack providers are pluggable modules
- **Provider dependencies**: Each provider may require external tools (docker, aws-cli, kubectl)
  - *Documented*: Clear requirements per provider

### Neutral

- **Installation method**: `go install` vs separate download
  - *Decision*: `go install` is simpler and handles updates

## Alternatives Considered

### 1. Separate `hashistack` Binary

Create a dedicated `hashistack` tool for Hashicorp infrastructure.

**Rejected because**:
- Requires users to install two binaries
- Separate versioning and release process
- Configuration split between tools
- Duplicates infrastructure management code

### 2. Python Bootstrap Script Only

Keep `tooling/bootstrap.py` as the only bootstrap method.

**Rejected because**:
- Python dependency for dev environment setup
- Slower startup (230ms vs 12ms)
- Can't be distributed as single binary
- Doesn't integrate with prismctl admin operations

### 3. Shell Scripts

Provide shell scripts for stack management.

**Rejected because**:
- Platform-specific (bash vs powershell)
- Harder to maintain
- No type safety
- Poor error handling

## Implementation Plan

1. **Add stack package**: `tools/internal/stack/`
   - Define `StackProvider` interface
   - Implement Hashicorp provider
   - Implement Docker Compose provider

2. **Add stack commands**: `tools/cmd/prismctl/stack.go`
   - `init`, `start`, `stop`, `status`, `use`, `providers`

3. **Update bootstrap**:
   - `prismctl stack init` replaces `uv run tooling/bootstrap.py`

4. **Documentation**:
   - Update README with `go install` instructions
   - Document each stack provider

5. **Testing**:
   - Integration tests for each provider
   - CI pipeline for stack bootstrapping

## References

- ADR-040: Go Binary for Admin CLI
- ADR-012: Go for Tooling
- ADR-016: Go CLI and Configuration Management
- ADR-025: Container Plugin Model
- [Consul Documentation](https://www.consul.io/docs)
- [Vault Documentation](https://www.vaultproject.io/docs)
- [Docker Compose](https://docs.docker.com/compose/)

## Revision History

- 2025-10-09: Initial acceptance with prismctl stack subcommand approach
