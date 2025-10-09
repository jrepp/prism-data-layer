---
title: "ADR-040: Copy-Paste Bootstrap Installer with uv"
status: Accepted
date: 2025-10-09
deciders: [System]
tags: [tooling, bootstrap, installer, developer-experience, uv]
---

## Context

Developers need to quickly spin up a local Prism environment for testing and development. The current setup requires:
1. Cloning the repository
2. Installing multiple dependencies (Docker, Python, uv, Go, Rust)
3. Understanding the monorepo structure
4. Running multiple commands to start services
5. Configuring backends and namespaces manually

This friction creates barriers to entry for new contributors and slows down prototyping.

**Key Requirements**:
- **Zero-Knowledge Start**: Run a single command, get a working Prism environment
- **Copy-Paste Simple**: Should be shareable in Slack/docs as a one-liner
- **Minimal Dependencies**: Only require uv (which installs Python)
- **Includes Admin Console**: Provide web UI for immediate testing
- **Local Backends**: Use SQLite, local Kafka/NATS for realistic testing
- **Fast**: Get running in &lt;2 minutes on fresh machine

**Inspiration**: Similar to how developers can run:
```bash
curl -fsSL https://get.docker.com | sh
```

## Decision

Create a **copy-paste bootstrap installer** using Python + uv that:

1. **Single command installation**:
   ```bash
   curl -sSL https://raw.githubusercontent.com/jrepp/prism-data-layer/main/scripts/install.sh | sh
   ```

2. **Alternative: Direct uv invocation** (no curl needed):
   ```bash
   uv run --with prism-admin install
   ```

3. **What it does**:
   - Installs uv (if not present)
   - Downloads pre-built Prism proxy binary (or builds from source)
   - Starts local backends (SQLite, embedded NATS)
   - Configures default namespace
   - Launches admin web console on http://localhost:8000
   - Opens browser automatically

4. **Admin management via uv**:
   ```bash
   # Start local Prism environment
   uv run prism-admin start

   # Stop environment
   uv run prism-admin stop

   # View status
   uv run prism-admin status

   # Reset environment
   uv run prism-admin reset
   ```

## Rationale

### Why uv for Bootstrap?

#### Pros
1. **Zero Install**: uv itself is a single binary, can be installed via curl
2. **Dependency Management**: Handles Python dependencies automatically
3. **Fast**: Sub-second cold starts
4. **Self-Contained**: No need for pre-existing Python environment
5. **Scriptable**: Python makes complex logic easier than pure bash
6. **Cross-Platform**: Works on Linux, macOS, Windows

#### Cons
1. **Python Required**: Eventually needs Python runtime (but uv handles this)
2. **Not Native**: Go would be faster, but more complex to distribute

### Why Not Docker Compose Alone?

**Pros**:
- Standard approach for multi-service applications
- Well-understood by developers

**Cons**:
- Requires Docker installed
- Slower startup (image pulls)
- Heavier resource usage
- Opaque to new developers (black box)

**Decision**: Use uv as primary installer; provide Docker Compose as alternative

### Why Not Just Shell Script?

**Pros**:
- Universal, no dependencies
- Fast execution

**Cons**:
- Complex logic is brittle in bash
- Hard to handle errors gracefully
- Limited cross-platform support
- Difficult to test

**Decision**: Use shell script for initial curl | sh, delegate to Python for complex logic

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Installation Flow                         │
└─────────────────────────────────────────────────────────────┘

┌──────────────┐
│ User Invokes │
│  install.sh  │
└──────┬───────┘
       │
       ├─ Check for uv
       │   └─ Install uv if missing (curl -sSL https://astral.sh/uv/install.sh | sh)
       │
       ├─ Run uv tool install prism-admin
       │   └─ Downloads prism-admin Python package
       │
       └─ Run uv run prism-admin install
           │
           ├─ Download Prism proxy binary (from GitHub Releases)
           │   └─ Or build from source if no release available
           │
           ├─ Create local data directory (~/.prism/)
           │   ├─ config/       # Configuration files
           │   ├─ data/         # SQLite databases
           │   ├─ logs/         # Log files
           │   └─ bin/          # Downloaded binaries
           │
           ├─ Start local backends
           │   ├─ SQLite (embedded, no external process)
           │   └─ NATS (embedded via prism proxy)
           │
           ├─ Start Prism proxy (background process)
           │   └─ Port 8980 (data plane)
           │   └─ Port 8981 (admin API)
           │
           ├─ Start admin web UI (Python FastAPI, port 8000)
           │   └─ Connects to proxy admin API
           │
           └─ Open browser to http://localhost:8000
```

### Directory Structure

```
~/.prism/                          # Local Prism installation
├── bin/
│   ├── prism-proxy                # Downloaded or built proxy binary
│   └── prism-admin                # Python admin tool (via uv)
├── config/
│   ├── proxy.yaml                 # Proxy configuration
│   └── namespaces.yaml            # Default namespace configs
├── data/
│   ├── config.db                  # SQLite config storage
│   └── namespaces/
│       └── default.db             # Default namespace (SQLite)
├── logs/
│   ├── proxy.log
│   └── admin.log
└── pid/
    ├── proxy.pid
    └── admin.pid
```

### Script: install.sh

```bash
#!/bin/bash
# Prism Bootstrap Installer
# Usage: curl -sSL https://raw.githubusercontent.com/.../install.sh | sh

set -e

echo "🔷 Prism Bootstrap Installer"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Step 1: Check for uv
if ! command -v uv &> /dev/null; then
    echo "📦 Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.cargo/bin:$PATH"
fi

echo "✓ uv installed: $(uv --version)"

# Step 2: Install prism-admin tool
echo "📦 Installing prism-admin..."
uv tool install prism-admin

# Step 3: Run installer
echo "🚀 Setting up Prism environment..."
uv run prism-admin install

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✓ Prism installed successfully!"
echo ""
echo "Admin Console: http://localhost:8000"
echo "Data API: localhost:8980"
echo "Admin API: localhost:8981"
echo ""
echo "Commands:"
echo "  uv run prism-admin status    # View environment status"
echo "  uv run prism-admin stop      # Stop all services"
echo "  uv run prism-admin logs      # View logs"
echo "  uv run prism-admin reset     # Reset environment"
```

### Python Tool: prism-admin

```python
# tooling/prism_admin/__init__.py
#!/usr/bin/env python3
"""
Prism Admin - Local development environment manager

Usage:
    uv run prism-admin install   # Initial setup
    uv run prism-admin start     # Start services
    uv run prism-admin stop      # Stop services
    uv run prism-admin status    # Show status
    uv run prism-admin reset     # Reset environment
    uv run prism-admin logs      # Tail logs
"""

import click
import subprocess
import sys
from pathlib import Path
from rich.console import Console
from rich.table import Table

console = Console()

PRISM_HOME = Path.home() / ".prism"
PRISM_BIN = PRISM_HOME / "bin"
PRISM_DATA = PRISM_HOME / "data"
PRISM_CONFIG = PRISM_HOME / "config"
PRISM_LOGS = PRISM_HOME / "logs"
PRISM_PID = PRISM_HOME / "pid"

@click.group()
def cli():
    """Prism Admin - Local development environment manager"""
    pass

@cli.command()
def install():
    """Install Prism and start local environment"""
    console.print("🔷 [bold cyan]Prism Installation[/bold cyan]")
    console.print("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    # Create directories
    console.print("📁 Creating directories...")
    for directory in [PRISM_BIN, PRISM_DATA, PRISM_CONFIG, PRISM_LOGS, PRISM_PID]:
        directory.mkdir(parents=True, exist_ok=True)

    # Download or build proxy
    console.print("📦 Downloading Prism proxy...")
    download_proxy()

    # Create default configuration
    console.print("⚙️  Creating default configuration...")
    create_default_config()

    # Start services
    console.print("🚀 Starting services...")
    start_services()

    console.print("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    console.print("✓ [bold green]Installation complete![/bold green]")
    console.print(f"\n[bold]Admin Console:[/bold] http://localhost:8000")
    console.print(f"[bold]Data API:[/bold] localhost:8980")
    console.print(f"[bold]Admin API:[/bold] localhost:8981")

    # Open browser
    import webbrowser
    webbrowser.open("http://localhost:8000")

@cli.command()
def start():
    """Start Prism services"""
    console.print("🚀 Starting Prism services...")

    if is_running():
        console.print("[yellow]Prism is already running[/yellow]")
        return

    start_services()
    console.print("✓ [bold green]Prism started[/bold green]")
    console.print(f"Admin Console: http://localhost:8000")

@cli.command()
def stop():
    """Stop Prism services"""
    console.print("⏹  Stopping Prism services...")

    # Stop proxy
    proxy_pid = PRISM_PID / "proxy.pid"
    if proxy_pid.exists():
        pid = int(proxy_pid.read_text())
        try:
            import os
            import signal
            os.kill(pid, signal.SIGTERM)
            proxy_pid.unlink()
        except ProcessLookupError:
            proxy_pid.unlink()

    # Stop admin UI
    admin_pid = PRISM_PID / "admin.pid"
    if admin_pid.exists():
        pid = int(admin_pid.read_text())
        try:
            os.kill(pid, signal.SIGTERM)
            admin_pid.unlink()
        except ProcessLookupError:
            admin_pid.unlink()

    console.print("✓ [bold green]Prism stopped[/bold green]")

@cli.command()
def status():
    """Show Prism environment status"""
    table = Table(title="Prism Environment Status")
    table.add_column("Service", style="cyan")
    table.add_column("Status", style="green")
    table.add_column("Port")
    table.add_column("PID")

    # Check proxy
    proxy_pid = PRISM_PID / "proxy.pid"
    if proxy_pid.exists() and is_process_running(int(proxy_pid.read_text())):
        table.add_row("Proxy", "✓ Running", "8980, 8981", proxy_pid.read_text())
    else:
        table.add_row("Proxy", "✗ Stopped", "-", "-")

    # Check admin UI
    admin_pid = PRISM_PID / "admin.pid"
    if admin_pid.exists() and is_process_running(int(admin_pid.read_text())):
        table.add_row("Admin UI", "✓ Running", "8000", admin_pid.read_text())
    else:
        table.add_row("Admin UI", "✗ Stopped", "-", "-")

    console.print(table)

@cli.command()
def logs():
    """Tail Prism logs"""
    import subprocess

    console.print("📋 Tailing logs (Ctrl+C to stop)...")
    console.print("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    try:
        subprocess.run([
            "tail", "-f",
            str(PRISM_LOGS / "proxy.log"),
            str(PRISM_LOGS / "admin.log")
        ])
    except KeyboardInterrupt:
        pass

@cli.command()
def reset():
    """Reset Prism environment (delete all data)"""
    if not click.confirm("⚠️  This will delete all data. Continue?"):
        return

    # Stop services
    stop.invoke(click.Context(stop))

    # Delete data directory
    import shutil
    if PRISM_HOME.exists():
        shutil.rmtree(PRISM_HOME)

    console.print("✓ [bold green]Environment reset[/bold green]")
    console.print("Run 'uv run prism-admin install' to set up again")

# Helper functions

def download_proxy():
    """Download or build prism proxy binary"""
    import platform
    import requests

    system = platform.system().lower()
    arch = platform.machine().lower()

    # Try to download from GitHub Releases
    release_url = f"https://github.com/jrepp/prism-data-layer/releases/latest/download/prism-proxy-{system}-{arch}"

    try:
        response = requests.get(release_url, stream=True)
        if response.status_code == 200:
            proxy_bin = PRISM_BIN / "prism-proxy"
            with open(proxy_bin, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
            proxy_bin.chmod(0o755)
            console.print(f"✓ Downloaded proxy binary")
            return
    except:
        pass

    # Fallback: Build from source (requires Rust)
    console.print("[yellow]No pre-built binary available, building from source...[/yellow]")
    # TODO: Clone repo and cargo build --release

def create_default_config():
    """Create default Prism configuration"""
    proxy_config = PRISM_CONFIG / "proxy.yaml"
    proxy_config.write_text("""
data_port: 8980
admin_port: 8981

backends:
  - name: sqlite-local
    type: sqlite
    path: ~/.prism/data/namespaces/default.db

namespaces:
  - name: default
    backend: sqlite-local
    pattern: keyvalue
    consistency: eventual
""")

def start_services():
    """Start proxy and admin UI"""
    import subprocess
    import os

    # Start proxy in background
    proxy_bin = PRISM_BIN / "prism-proxy"
    proxy_log = PRISM_LOGS / "proxy.log"

    with open(proxy_log, 'w') as log:
        proc = subprocess.Popen(
            [str(proxy_bin), "--config", str(PRISM_CONFIG / "proxy.yaml")],
            stdout=log,
            stderr=log,
            start_new_session=True
        )
        (PRISM_PID / "proxy.pid").write_text(str(proc.pid))

    # Start admin UI (FastAPI)
    admin_log = PRISM_LOGS / "admin.log"
    with open(admin_log, 'w') as log:
        proc = subprocess.Popen(
            ["uvicorn", "prism_admin.ui:app", "--host", "0.0.0.0", "--port", "8000"],
            stdout=log,
            stderr=log,
            start_new_session=True
        )
        (PRISM_PID / "admin.pid").write_text(str(proc.pid))

def is_running():
    """Check if Prism is currently running"""
    proxy_pid = PRISM_PID / "proxy.pid"
    return proxy_pid.exists() and is_process_running(int(proxy_pid.read_text()))

def is_process_running(pid):
    """Check if process with PID is running"""
    try:
        import os
        import signal
        os.kill(pid, 0)
        return True
    except OSError:
        return False

if __name__ == "__main__":
    cli()
```

### pyproject.toml

```toml
[project]
name = "prism-admin"
version = "0.1.0"
description = "Prism local development environment manager"
authors = [{name = "Prism Team"}]
requires-python = ">=3.10"
dependencies = [
    "click>=8.1.0",
    "rich>=13.0.0",
    "requests>=2.31.0",
    "fastapi>=0.110.0",
    "uvicorn[standard]>=0.29.0",
]

[project.scripts]
prism-admin = "prism_admin:cli"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
```

## Use Cases

### 1. New Developer Onboarding

```bash
# Share this in Slack:
curl -sSL https://raw.githubusercontent.com/.../install.sh | sh

# Or for those with uv installed:
uv run --with prism-admin install
```

**Result**: Working Prism environment in &lt;2 minutes

### 2. Workshop / Demo

```bash
# Presenter shares command at start of workshop
curl -sSL https://prism.io/install | sh

# All participants get identical environment
# Open browser, start testing immediately
```

### 3. CI/CD Integration

```yaml
# .github/workflows/integration-test.yml
- name: Setup Prism
  run: |
    curl -sSL https://.../install.sh | sh

- name: Run integration tests
  run: |
    go test ./integration/...
```

### 4. Daily Development

```bash
# Morning: Start environment
uv run prism-admin start

# Work on features, run tests

# Evening: Stop environment
uv run prism-admin stop

# Clean slate for testing:
uv run prism-admin reset
uv run prism-admin install
```

## Performance Targets

- **Cold install** (no uv): &lt;2 minutes
- **Warm install** (uv cached): &lt;30 seconds
- **Start services**: &lt;5 seconds
- **Stop services**: &lt;2 seconds

## Security Considerations

1. **Binary Verification**: Download binaries with checksum validation
2. **HTTPS Only**: All downloads use HTTPS
3. **Local Only**: Services bind to localhost by default
4. **No Sudo**: Installation to user home directory

```python
def verify_checksum(filepath, expected_sha256):
    """Verify downloaded binary integrity"""
    import hashlib

    sha256 = hashlib.sha256()
    with open(filepath, 'rb') as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256.update(chunk)

    if sha256.hexdigest() != expected_sha256:
        raise ValueError("Checksum verification failed")
```

## Consequences

### Positive

- **Instant Gratification**: Get running immediately
- **Lower Barrier**: No need to understand full architecture
- **Sharable**: Copy-paste in docs, Slack, workshops
- **Consistent**: Everyone gets identical environment
- **CI-Friendly**: Easy to set up test environments
- **uv Integration**: Leverages existing Python tooling

### Negative

- **Maintenance Burden**: Need to keep installer up-to-date
- **Platform Support**: Must test on Linux, macOS, Windows
- **Binary Distribution**: Need CI to build releases
- **Limited Customization**: Default config may not fit all needs

### Neutral

- **Python Dependency**: Uses uv, which is becoming standard
- **Not Production**: This is for local dev/testing only

## Migration Path

### Phase 1: Basic Installer (Week 1)
- Create install.sh script
- Implement prism-admin start/stop/status
- Download pre-built binaries from GitHub Releases

### Phase 2: Admin Console (Week 2)
- Add FastAPI-based admin UI
- Connect to proxy admin API
- Basic namespace/backend management

### Phase 3: Documentation (Week 3)
- Update README with one-liner install
- Create video walkthrough
- Add to getting-started docs

### Phase 4: CI Integration (Week 4)
- Build binaries for releases
- Test installer in CI
- Add integration test workflows

## Open Questions

1. **Windows Support**: Should we support Windows natively or recommend WSL?
2. **Docker Alternative**: Should install.sh detect Docker and offer that path?
3. **Uninstall**: Should we provide `prism-admin uninstall` command?
4. **Multiple Versions**: Support running multiple Prism versions side-by-side?

## References

- [uv Documentation](https://docs.astral.sh/uv/)
- [Docker Install Script](https://get.docker.com/)
- [Homebrew Install Script](https://brew.sh/)
- ADR-004: Local-First Testing
- ADR-012: Go for Tooling
- RFC-003: Admin Interface

## Revision History

- 2025-10-09: Initial draft for copy-paste bootstrap installer with uv
