---
date: 2025-10-09
deciders: Core Team
doc_uuid: e9a89036-3214-4516-b580-bf805e2122fc
id: adr-040
project_id: prism-data-layer
status: Accepted
tags:
- go
- cli
- tooling
- dx
title: Go Binary for Admin CLI (prismctl)
---

## Context

Prism needs an admin CLI for operators to manage namespaces, monitor health, and perform operational tasks. The CLI must be:

1. **Fast**: Sub-50ms startup time for responsive commands
2. **Portable**: Single binary, works everywhere, no runtime dependencies
3. **Simple**: Easy to install and distribute
4. **Professional**: Polished UX expected for infrastructure tooling
5. **Maintainable**: Consistent with backend plugin implementation

## Decision

Build the admin CLI as a **Go binary** named **`prismctl`**, following patterns from successful CLIs like `kubectl`, `docker`, and `gh`.

**Installation**:
```bash
# Download single binary
curl -LO https://github.com/prism/releases/download/v1.0.0/prismctl-$(uname -s)-$(uname -m)
chmod +x prismctl-*
mv prismctl-* /usr/local/bin/prismctl

# Or via Go install
go install github.com/jrepp/prism-data-layer/tools/cmd/prismctl@latest
```

**Usage**:
```bash
prismctl namespace list
prismctl namespace create my-app --description "My application"
prismctl health
prismctl session list
```

## Rationale

### Why Go is Ideal for Admin CLI

#### 1. Single Binary Distribution

Go produces a **single static binary** with no dependencies:

```bash
# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o prismctl-linux-amd64
GOOS=darwin GOARCH=arm64 go build -o prismctl-darwin-arm64
GOOS=windows GOARCH=amd64 go build -o prismctl-windows-amd64.exe
```

**Binary size**: ~10-15MB (comparable to Python with dependencies)

**Advantages**:
- ✅ No Python interpreter required
- ✅ No virtual environment management
- ✅ Works on minimal systems (Alpine, BusyBox)
- ✅ Easy to containerize: `FROM scratch` + binary
- ✅ No PATH issues or version conflicts

#### 2. Blazing Fast Startup

**Performance comparison**:

```bash
# Go binary
$ time prismctl --version
prismctl version 1.0.0
real    0m0.012s  # 12ms

# Python with uv run --with
$ time uv run --with prismctl prism --version
prismctl version 1.0.0
real    0m0.234s  # 234ms (20x slower)
```

For admin operations, 220ms might not matter, but for scripting and automation, it adds up:

```bash
# Loop 100 times
for i in {1..100}; do prismctl namespace list; done

# Go: ~1.2s total (12ms each)
# Python: ~23s total (230ms each)
```

#### 3. Professional CLI Tooling

Go has the best CLI ecosystem:

**Cobra + Viper** (used by Kubernetes, Docker, GitHub CLI):
```go
var rootCmd = &cobra.Command{
    Use:   "prismctl",
    Short: "Admin CLI for Prism data gateway",
}

var namespaceCmd = &cobra.Command{
    Use:   "namespace",
    Short: "Manage namespaces",
}

var namespaceListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all namespaces",
    RunE:  runNamespaceList,
}

func init() {
    rootCmd.AddCommand(namespaceCmd)
    namespaceCmd.AddCommand(namespaceListCmd)
}
```

**Features out of the box**:
- Command completion (bash, zsh, fish, powershell)
- Man page generation
- Markdown docs generation
- Flag parsing with validation
- Subcommand organization
- Configuration file support

#### 4. Consistency with Backend Plugins

The backend plugins are written in Go (ADR-025), so using Go for the CLI:
- ✅ Same language, same patterns, same toolchain
- ✅ Developers only need Go knowledge
- ✅ Can share code/libraries between CLI and plugins
- ✅ Unified build process: `make build` builds everything

#### 5. Cross-Platform Just Works

Go's cross-compilation is trivial:

```bash
# Build for all platforms in one command
make release

# Produces:
dist/
├── prismctl-darwin-amd64
├── prismctl-darwin-arm64
├── prismctl-linux-amd64
├── prismctl-linux-arm64
├── prismctl-windows-amd64.exe
└── checksums.txt
```

No need for:
- Platform-specific Python builds
- Dealing with different Python versions (3.10 vs 3.11)
- Virtual environment setup per platform

#### 6. Easy Distribution

**GitHub Releases** (recommended):
```bash
# Automatically upload binaries
gh release create v1.0.0 \
  dist/prismctl-* \
  --title "prismctl v1.0.0" \
  --notes "Admin CLI for Prism"
```

Users download:
```bash
curl -LO https://github.com/prism/releases/latest/download/prismctl-$(uname -s)-$(uname -m)
chmod +x prismctl-*
sudo mv prismctl-* /usr/local/bin/prismctl
```

**Homebrew** (macOS/Linux):
```ruby
class Prismctl < Formula
  desc "Admin CLI for Prism data gateway"
  homepage "https://prism.io"
  url "https://github.com/prism/prismctl/archive/v1.0.0.tar.gz"

  def install
    system "go", "build", "-o", bin/"prismctl"
  end
end
```

```bash
brew install prismctl
```

**Package managers**:
- **apt/deb**: Create `.deb` package with single binary
- **yum/rpm**: Create `.rpm` package with single binary
- **Chocolatey** (Windows): Simple package with .exe

#### 7. No Runtime Dependencies

Go binary needs **nothing**:
```bash
# Check dependencies (none!)
$ ldd prismctl
    not a dynamic executable

# Works on minimal systems
$ docker run --rm -v $PWD:/app alpine /app/prismctl --version
prismctl version 1.0.0
```

Compare to Python:
```bash
# Python requires:
- Python 3.10+ interpreter
- pip or uv
- C libraries (for some packages like grpcio)
- System dependencies (openssl, etc.)
```

### Implementation: prismctl

**Directory structure**:
tools/
└── cmd/
    └── prismctl/
        ├── main.go           # Entry point
        ├── root.go           # Root command + config
        ├── namespace.go      # Namespace commands
        ├── health.go         # Health commands
        ├── session.go        # Session commands
        └── config.go         # Config management
```text

**Build**:
```
cd tools
go build -o prismctl ./cmd/prismctl
./prismctl --help
```text

**Release build** (optimized):
```
go build -ldflags="-s -w" -o prismctl ./cmd/prismctl
upx prismctl  # Optional: compress binary (10MB → 3MB)
```text

### Configuration

**Default config** (`~/.prism/config.yaml`):
```
admin:
  endpoint: localhost:8981

plugins:
  postgres:
    image: prism/postgres-plugin:latest
    port: 9090
  kafka:
    image: prism/kafka-plugin:latest
    port: 9091
  redis:
    image: prism/redis-plugin:latest
    port: 9092

logging:
  level: info
```text

**Precedence** (Viper):
1. Command-line flags
2. Environment variables (`PRISM_ADMIN_ENDPOINT`)
3. Config file (`~/.prism/config.yaml`)
4. Defaults

### Bootstrap and Installation

**Superseded by ADR-045**: Bootstrap is now handled by `prismctl stack init`.

```
# Install prismctl (includes bootstrap functionality)
go install github.com/jrepp/prism-data-layer/tools/cmd/prismctl@latest

# Initialize environment
prismctl stack init
# Creates ~/.prism directory, config, and stack manifests

# Start infrastructure
prismctl stack start

# Use prismctl for admin operations
prismctl namespace list
prismctl health
```text

Rationale:
- Single binary handles both bootstrap and runtime operations
- No Python dependency for environment setup
- Consistent `go install` installation method
- See ADR-045 for stack management details

### Plugin Management

**Plugin manifests** in `~/.prism/plugins/`:

```
# ~/.prism/plugins/postgres.yaml
name: postgres
image: prism/postgres-plugin:latest
port: 9090
backends: [postgres]
capabilities:
  - keyvalue
  - transactions
```text

**CLI integration**:
```
# List available plugins
prismctl plugin list

# Start plugin
prismctl plugin start postgres

# Stop plugin
prismctl plugin stop postgres

# Plugin health
prismctl plugin health postgres
```text

**Go implementation**:
```
func runPluginStart(cmd *cobra.Command, args []string) error {
    pluginName := args[0]

    // Load manifest
    manifest, err := loadPluginManifest(pluginName)
    if err != nil {
        return err
    }

    // Start container
    return startPluginContainer(manifest)
}

func loadPluginManifest(name string) (*PluginManifest, error) {
    path := filepath.Join(os.Getenv("HOME"), ".prism", "plugins", name+".yaml")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var manifest PluginManifest
    if err := yaml.Unmarshal(data, &manifest); err != nil {
        return nil, err
    }

    return &manifest, nil
}
```text

## Consequences

### Positive

- **Fast**: 12ms startup vs 230ms for Python
- **Simple**: Single binary, no dependencies
- **Professional**: Industry-standard CLI patterns (Cobra/Viper)
- **Consistent**: Same language as backend plugins
- **Portable**: Works everywhere Go compiles
- **Easy distribution**: GitHub releases, Homebrew, package managers
- **Small footprint**: ~10-15MB binary vs Python + deps

### Negative

- **Platform-specific builds**: Must compile for each OS/arch
  - *Mitigation*: Automated via CI/CD (make release)
- **Binary size**: 10-15MB vs 2-3MB for minimal Python
  - *Acceptable*: Trivial for infrastructure tooling
- **Less dynamic**: Can't hot-reload code like Python
  - *Not needed*: CLI tools don't need hot-reload

### Neutral

- **Language choice**: Go vs Python is preference for admin tool
  - *Decision*: Go aligns better with plugin ecosystem and performance requirements

## Comparison: Go vs Python for Admin CLI

| Aspect | Go (prismctl) | Python (with uv) |
|--------|---------------|------------------|
| **Startup time** | 12ms | 230ms |
| **Binary size** | 10-15MB | 5-10MB + Python |
| **Dependencies** | None | Python 3.10+ |
| **Installation** | Single binary | pip/uv + package |
| **Cross-platform** | Build per platform | Universal (with Python) |
| **CLI framework** | Cobra (kubectl-style) | Click/Typer |
| **Distribution** | GitHub releases | PyPI |
| **Updates** | Download new binary | pip/uv upgrade |
| **Consistency** | Matches Go plugins | Different language |
| **Community** | Docker, k8s, gh use Go | Many Python CLIs exist |

**Verdict**: Go is the better choice for infrastructure CLI tools that prioritize performance, portability, and professional UX.

## Implementation Plan

1. **Rename**: `tools/cmd/prism-admin` → `tools/cmd/prismctl`
2. **Update**: Binary name from `prism-admin` to `prismctl`
3. **Add**: Plugin management commands to prismctl
4. **Add**: Stack management subcommand (see ADR-045)
5. **Document**: Installation and usage in README
6. **Release**: Automated builds via GitHub Actions

## References

- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [Viper Configuration](https://github.com/spf13/viper)
- [kubectl Design](https://kubernetes.io/docs/reference/kubectl/)
- [GitHub CLI Design](https://cli.github.com/)
- ADR-012: Go for Tooling
- ADR-016: Go CLI and Configuration Management
- ADR-025: Container Plugin Model
- ADR-045: prismctl Stack Management Subcommand
- RFC-010: Admin Protocol with OIDC

## Revision History

- 2025-10-09: Initial acceptance with Go binary approach
- 2025-10-09: Updated to reference ADR-045 for stack management (bootstrap now via prismctl)

```