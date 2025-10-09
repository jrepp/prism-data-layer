# Prism Plugin Watcher

File watcher for Prism backend plugins that automatically rebuilds container images on source changes.

## Features

- **File watching**: Monitors plugin directories for Go source changes
- **Automatic rebuilds**: Triggers Podman builds when files change
- **Debouncing**: Groups rapid changes to avoid redundant builds
- **Optional reload**: Restart plugin containers after successful builds
- **Fast iteration**: Enables rapid development workflow

## Installation

```bash
cd plugins/watcher
go build -o prism-watcher main.go
```

Or install globally:

```bash
go install ./plugins/watcher
```

## Usage

### Basic Watching

```bash
# Watch plugins directory and auto-build on changes
cd plugins
./watcher/prism-watcher

# Watch with verbose output
./watcher/prism-watcher --verbose

# Watch and auto-reload containers
./watcher/prism-watcher --reload
```

### Options

```
Flags:
  --dir string       Plugins directory to watch (default "./plugins")
  --debounce int     Debounce delay in milliseconds (default 1000)
  --build            Automatically build on file changes (default true)
  --reload           Automatically restart containers after build
  --target string    Build target: production or debug (default "production")
  --podman string    Path to podman binary (default "podman")
  -v, --verbose      Verbose output
```

### Examples

```bash
# Watch and build production images
prism-watcher --dir ./plugins

# Watch and build debug images
prism-watcher --target debug

# Watch, build, and reload containers
prism-watcher --reload

# Watch with custom debounce
prism-watcher --debounce 2000

# Disable auto-build (just watch)
prism-watcher --build=false
```

## Development Workflow

### Typical Development Loop

1. **Start watcher**:
   ```bash
   cd plugins
   ./watcher/prism-watcher --reload --verbose
   ```

2. **Edit source code**:
   ```bash
   # Edit postgres plugin
   vim postgres/main.go
   ```

3. **Automatic rebuild**:
   - Watcher detects change
   - Waits 1 second (debounce)
   - Runs `podman build`
   - Optionally restarts container

4. **Iterate** quickly without manual rebuild steps

### Hot Reload Example

Terminal 1 (Watcher):
```bash
$ prism-watcher --reload --verbose
INFO starting prism plugin watcher version=0.1.0 dir=./plugins
INFO watching plugin plugin=postgres path=/plugins/postgres
INFO watching plugin plugin=kafka path=/plugins/kafka
INFO watching plugin plugin=redis path=/plugins/redis
# ... edit postgres/main.go ...
DEBUG file changed file=main.go plugin=postgres op=WRITE
INFO detected changes plugin=postgres
INFO building plugin plugin=postgres target=production
INFO build completed plugin=postgres duration=12.3s
INFO starting plugin container plugin=postgres
INFO plugin container started plugin=postgres container=postgres-plugin
```

Terminal 2 (Testing):
```bash
# Test the reloaded plugin
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check
```

## How It Works

### File Watching

Uses `fsnotify` to watch:
- `plugins/core/**/*.go` - Core package changes trigger rebuild of all plugins
- `plugins/postgres/**/*.go` - Postgres plugin sources
- `plugins/kafka/**/*.go` - Kafka plugin sources
- `plugins/redis/**/*.go` - Redis plugin sources

### Debouncing

Groups rapid changes within the debounce window (default 1 second) to avoid rebuilding on every keystroke.

### Build Process

For each detected change:

1. **Detect plugin**: Determine which plugin directory changed
2. **Debounce**: Wait for debounce period
3. **Build**: Run `podman build --target <target> ...`
4. **Reload** (if `--reload`):
   - Stop existing container
   - Remove old container
   - Start new container with updated image

### Core Package Handling

When `core/` files change, all plugins are rebuilt since they depend on the core package.

## Integration with Makefile

The watcher complements the Makefile:

**Makefile**: One-time builds, CI/CD, releases
```bash
make build              # Build all plugins once
make VERSION=v1.0.0 build  # Tagged builds
```

**Watcher**: Development, hot-reload, rapid iteration
```bash
prism-watcher --reload  # Continuous development
```

## Performance

### Build Times (M1 Mac)

- **Postgres**: ~10-15s (includes Go compile + container build)
- **Kafka**: ~15-20s (includes librdkafka)
- **Redis**: ~8-12s (pure Go, fastest)
- **Core change**: ~30-50s (rebuilds all 3 plugins)

### Debounce Tuning

- **Low debounce (500ms)**: Faster feedback, more builds
- **High debounce (2000ms)**: Fewer builds, wait for multiple changes
- **Default (1000ms)**: Balance between responsiveness and efficiency

## Container Lifecycle

With `--reload` enabled:

```
File Change → Build → Stop Old Container → Start New Container
                                              ↓
                                         Health Check Ready
```

Without `--reload`:

```
File Change → Build → (Manual container restart needed)
```

## Troubleshooting

### Watcher Not Detecting Changes

**Problem**: Files change but no rebuild triggered

**Solutions**:
- Ensure you're editing `.go` or `.mod` files (other files ignored)
- Check watcher is running and watching correct directory
- Verify file is in a watched plugin directory

### Build Failures

**Problem**: Builds fail after file change

**Solutions**:
- Run `go mod download` in plugin directory
- Check syntax errors in source code
- Verify Dockerfiles are present
- Run manual build: `make postgres`

### Container Won't Start

**Problem**: Container starts but immediately exits

**Solutions**:
- Check logs: `podman logs postgres-plugin`
- Verify backend services are running (Postgres, Kafka, Redis)
- Check environment variables are correct
- Try running without `--reload` first

### High CPU Usage

**Problem**: Watcher consuming too much CPU

**Solutions**:
- Increase debounce: `--debounce 2000`
- Ensure not watching large vendor directories
- Check for infinite rebuild loops

## Advanced Usage

### Custom Container Configuration

Edit `reloadPlugin()` function in `main.go` to customize:

```go
case "postgres":
    args = append(args,
        "-e", "DATABASE_URL=postgres://...",
        "-e", "CUSTOM_ENV=value",
        "-v", "/local/data:/data")  // Add volume mounts
```

### Multiple Watchers

Run multiple watchers for different build targets:

Terminal 1:
```bash
prism-watcher --target production --reload
```

Terminal 2:
```bash
prism-watcher --target debug --reload
```

### CI/CD Integration

The watcher is designed for local development. For CI/CD, use the Makefile:

```yaml
# GitHub Actions
- name: Build plugins
  run: |
    cd plugins
    make build
    make test
```

## References

- [fsnotify](https://github.com/fsnotify/fsnotify) - File system notifications
- [ADR-025: Container Plugin Model](/docs-cms/adr/ADR-025-container-plugin-model.md)
- [ADR-026: Distroless Container Images](/docs-cms/adr/ADR-026-distroless-container-images.md)
- [Podman Documentation](https://docs.podman.io/)
