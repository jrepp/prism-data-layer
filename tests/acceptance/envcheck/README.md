# Environment Validation Test Suite

## Purpose

This test suite validates the testing infrastructure **before** running acceptance tests. It checks:

1. ✅ **Podman Installation** - Verifies podman is installed and available
2. ✅ **Podman Machine Status** - Checks if podman machine is running (macOS/Windows)
3. ✅ **DOCKER_HOST Configuration** - Ensures testcontainers can connect to podman
4. ✅ **Testcontainers Connection** - Validates testcontainers can communicate with podman
5. ✅ **Required Ports Available** - Checks if test ports are not already in use
6. ✅ **Container Start/Stop** - Actually starts a test container to verify end-to-end functionality
7. ✅ **Network Connectivity** - Validates DNS and external connectivity

## Why This Matters

Without these checks, acceptance tests may fail with cryptic errors like:

- `listen tcp :9090: bind: address already in use` - Port conflict
- `failed to connect to docker daemon` - DOCKER_HOST not set or podman not running
- `container failed to start` - Testcontainers can't communicate with podman
- `dial unix: no such file or directory` - Podman socket path incorrect

**This suite catches these issues early** and provides actionable error messages.

## Usage

### Run Environment Validation

**ALWAYS run this before acceptance tests:**

```bash
cd tests/acceptance/envcheck
go test -v
```

### Expected Output (Success)

```
=== RUN   TestEnvironmentValidation
=== RUN   TestEnvironmentValidation/PodmanInstalled
    environment_test.go:35: ✅ Podman version:
    Client:       Podman Engine
    Version:      5.2.1
    API Version:  5.2.1
    ...
=== RUN   TestEnvironmentValidation/PodmanMachineRunning
    environment_test.go:46: ✅ Podman machine is running
=== RUN   TestEnvironmentValidation/DockerHostSet
    environment_test.go:65: ✅ DOCKER_HOST is set: unix:///Users/jrepp/.local/share/containers/podman/machine/podman.sock
    environment_test.go:66: ✅ Socket exists: /Users/jrepp/.local/share/containers/podman/machine/podman.sock
=== RUN   TestEnvironmentValidation/TestcontainersPodmanConnection
    environment_test.go:77: ✅ Testcontainers connected to daemon: unix:///Users/jrepp/.local/share/containers/podman/machine/podman.sock
=== RUN   TestEnvironmentValidation/RequiredPortsAvailable
    environment_test.go:97: ✅ Port 4222 is available
    environment_test.go:97: ✅ Port 6222 is available
    environment_test.go:97: ✅ Port 8222 is available
    environment_test.go:97: ✅ Port 6379 is available
    environment_test.go:97: ✅ Port 9090 is available
    environment_test.go:97: ✅ Port 50051 is available
    environment_test.go:97: ✅ Port 8980 is available
    environment_test.go:97: ✅ Port 5556 is available
    environment_test.go:97: ✅ Port 5558 is available
=== RUN   TestEnvironmentValidation/TestcontainersCanStartContainer
    environment_test.go:113: Starting test container (alpine)...
    environment_test.go:128: ✅ Successfully started and ran test container
=== RUN   TestEnvironmentValidation/NetworkConnectivity
    environment_test.go:138: ✅ DNS resolution works: github.com -> [140.82.114.4]
    environment_test.go:147: ✅ External connectivity works
--- PASS: TestEnvironmentValidation (5.23s)
PASS
ok  	github.com/jrepp/prism-data-layer/tests/acceptance/envcheck	5.456s
```

### Handling Failures

#### 1. Podman Not Installed

**Error:**
```
podman is not installed or not in PATH
```

**Fix:**
```bash
# macOS
brew install podman

# Linux
sudo dnf install podman  # or apt-get/yum
```

#### 2. Podman Machine Not Running

**Error:**
```
Podman machine is not running. Start it with: podman machine start
```

**Fix:**
```bash
podman machine start
```

#### 3. DOCKER_HOST Not Set

**Error:**
```
DOCKER_HOST environment variable is not set
```

**Fix:**
```bash
# Add to your shell profile (~/.zshrc, ~/.bashrc, etc.)
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"

# Or source the project's env setup
source <(make env)
```

#### 4. Ports Already in Use

**Error:**
```
The following ports are already in use: [4222, 6379, 9090]
To find what's using a port: lsof -i :<port>
To stop all podman containers: podman stop $(podman ps -a -q)
```

**Fix:**
```bash
# Find what's using the port
lsof -i :9090

# Kill specific process
kill <PID>

# Or stop all podman containers
podman stop $(podman ps -a -q)
podman rm $(podman ps -a -q)
```

#### 5. Testcontainers Can't Connect

**Error:**
```
Failed to create testcontainers provider
```

**Fix:**
1. Verify DOCKER_HOST is set correctly
2. Check podman machine is running: `podman machine list`
3. Verify socket exists: `ls -la $(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')`
4. Restart podman machine: `podman machine stop && podman machine start`

## Integration with CI/CD

### GitHub Actions

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23.1'

      # Install and start podman
      - name: Install Podman
        run: |
          sudo apt-get update
          sudo apt-get install -y podman
          podman info

      # Environment validation (CRITICAL)
      - name: Validate Test Environment
        run: |
          cd tests/acceptance/envcheck
          go test -v

      # Only run acceptance tests if environment is valid
      - name: Run Acceptance Tests
        run: |
          cd tests/acceptance
          go test -v ./...
```

### Local Development Workflow

```bash
# 1. Clean environment
podman stop $(podman ps -a -q) 2>/dev/null
podman rm $(podman ps -a -q) 2>/dev/null

# 2. Validate environment
cd tests/acceptance/envcheck
go test -v

# 3. If validation passes, run acceptance tests
cd ../
go test -v ./patterns/consumer/ -run TestConsumerProxyArchitecture
```

## Makefile Integration

Add to root `Makefile`:

```makefile
.PHONY: test-env-validate
test-env-validate:
	@echo "🔍 Validating test environment..."
	@cd tests/acceptance/envcheck && go test -v

.PHONY: test-acceptance
test-acceptance: test-env-validate
	@echo "✅ Environment validated, running acceptance tests..."
	@cd tests/acceptance && go test -v ./...

.PHONY: clean-podman
clean-podman:
	@echo "🧹 Cleaning all podman containers..."
	@podman stop $$(podman ps -a -q) 2>/dev/null || true
	@podman rm $$(podman ps -a -q) 2>/dev/null || true
	@echo "✅ All podman containers stopped and removed"
```

Then use:

```bash
# Clean environment
make clean-podman

# Validate before running tests
make test-env-validate

# Run acceptance tests (validates first)
make test-acceptance
```

## Continuous Monitoring

You can run environment validation in a loop to monitor health:

```bash
# Run validation every 30 seconds
while true; do
  cd tests/acceptance/envcheck
  go test -v
  sleep 30
done
```

## Debugging Tips

### Enable Verbose Testcontainers Logging

```bash
export TESTCONTAINERS_RYUK_VERBOSE=true
export TESTCONTAINERS_LOG_LEVEL=debug
go test -v
```

### Check Podman System Info

```bash
podman info
podman machine inspect
```

### Verify Docker Socket Permissions

```bash
ls -la $(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')
```

### Test Direct Docker API Access

```bash
curl --unix-socket $(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}') http://localhost/version
```

## Architecture

This validation suite is designed to be:

1. **Fast**: Completes in <10 seconds
2. **Non-destructive**: Only checks, doesn't modify anything (except test container)
3. **Self-contained**: No dependencies on other test packages
4. **Actionable**: Clear error messages with fix instructions
5. **Portable**: Works on macOS, Linux, Windows (with podman machine)

## Future Enhancements

Potential additions:

- [ ] Check disk space for container images
- [ ] Validate Go version compatibility
- [ ] Check for conflicting Docker installations
- [ ] Verify resource limits (ulimits, file descriptors)
- [ ] Validate network bridge configuration
- [ ] Check for required kernel modules (Linux)
- [ ] Validate firewall rules don't block container networking
- [ ] Check for VPN interference with container networking

## Related Documentation

- [ADR-049: Podman Container Optimization](/adr/adr-049)
- [MEMO-007: Podman Scratch Container Demo](/memos/memo-007)
- [MEMO-020: Parallel Testing and Build Hygiene](/memos/memo-020)
- [Testcontainers Documentation](https://golang.testcontainers.org/)
- [Podman Documentation](https://docs.podman.io/)
