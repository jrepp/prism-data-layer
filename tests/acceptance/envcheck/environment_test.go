package envcheck_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEnvironmentValidation is the master test that runs all validation checks
func TestEnvironmentValidation(t *testing.T) {
	t.Run("PodmanInstalled", testPodmanInstalled)
	t.Run("PodmanMachineRunning", testPodmanMachineRunning)
	t.Run("DockerHostSet", testDockerHostSet)
	t.Run("TestcontainersConnection", testTestcontainersConnection)
	t.Run("RequiredPortsAvailable", testRequiredPortsAvailable)
	t.Run("TestcontainersCanStartContainer", testTestcontainersCanStartContainer)
	t.Run("NetworkConnectivity", testNetworkConnectivity)
}

// testPodmanInstalled verifies podman binary is available
func testPodmanInstalled(t *testing.T) {
	cmd := exec.Command("podman", "version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "podman is not installed or not in PATH")

	t.Logf("✅ Podman version:\n%s", string(output))
}

// testPodmanMachineRunning verifies podman machine is running (macOS/Windows)
func testPodmanMachineRunning(t *testing.T) {
	// Check if we're on macOS (podman machine is only needed there)
	if os.Getenv("GOOS") == "linux" {
		t.Skip("Skipping podman machine check on Linux")
	}

	cmd := exec.Command("podman", "machine", "list", "--format", "{{.Running}}")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to check podman machine status")

	// Check if any machine is running (output should contain "true")
	running := strings.Contains(string(output), "true")
	require.True(t, running, "Podman machine is not running. Start it with: podman machine start")

	t.Logf("✅ Podman machine is running")
}

// testDockerHostSet verifies DOCKER_HOST environment variable is set
func testDockerHostSet(t *testing.T) {
	dockerHost := os.Getenv("DOCKER_HOST")

	if dockerHost == "" {
		// Check if Docker/Podman daemon is available (even without DOCKER_HOST)
		ctx := context.Background()
		provider, err := testcontainers.NewDockerProvider()
		if err == nil {
			// Testcontainers can connect (Docker Desktop or Podman with default socket)
			defer provider.Close()
			info, _ := provider.DaemonHost(ctx)
			t.Logf("⚠️  DOCKER_HOST not set, but container daemon is available")
			t.Logf("   Using daemon at: %s", info)
			t.Logf("   This is acceptable for local development (likely Docker Desktop)")
			t.Logf("   To explicitly use Podman, set: export DOCKER_HOST=unix://<podman-socket-path>")
			return
		}

		// Neither DOCKER_HOST nor accessible daemon available
		t.Fatal("DOCKER_HOST environment variable is not set and no container daemon is accessible.\n" +
			"Either install Docker Desktop or set DOCKER_HOST to podman socket.\n" +
			"For Podman: export DOCKER_HOST=\"unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')\"")
	}

	// DOCKER_HOST is set - verify it's valid
	require.True(t, strings.HasPrefix(dockerHost, "unix://"),
		"DOCKER_HOST should be a unix socket path (unix://...)")

	// Extract socket path and verify it exists
	socketPath := strings.TrimPrefix(dockerHost, "unix://")
	_, err := os.Stat(socketPath)
	require.NoError(t, err, "DOCKER_HOST socket path does not exist: %s", socketPath)

	t.Logf("✅ DOCKER_HOST is set: %s", dockerHost)
	t.Logf("✅ Socket exists: %s", socketPath)
}

// testTestcontainersConnection verifies testcontainers can connect to Docker/Podman
func testTestcontainersConnection(t *testing.T) {
	ctx := context.Background()

	// Try to get Docker provider info (this validates connection)
	provider, err := testcontainers.NewDockerProvider()
	require.NoError(t, err, "Failed to create testcontainers provider")
	defer provider.Close()

	// Get daemon info
	info, err := provider.DaemonHost(ctx)
	require.NoError(t, err, "Failed to connect to Docker/Podman daemon")

	t.Logf("✅ Testcontainers connected to daemon: %s", info)
}

// testRequiredPortsAvailable checks if required ports are not already in use
func testRequiredPortsAvailable(t *testing.T) {
	requiredPorts := []int{
		4222,  // NATS
		6222,  // NATS cluster
		8222,  // NATS monitoring
		6379,  // Redis
		9090,  // Proxy control plane
		50051, // Pattern control plane
		8980,  // prism-bridge
		5556,  // Dex
		5558,  // Dex gRPC
	}

	unavailablePorts := []int{}

	for _, port := range requiredPorts {
		if !isPortAvailable(port) {
			unavailablePorts = append(unavailablePorts, port)
			t.Logf("⚠️  Port %d is already in use", port)
		} else {
			t.Logf("✅ Port %d is available", port)
		}
	}

	if len(unavailablePorts) > 0 {
		t.Errorf("The following ports are already in use: %v", unavailablePorts)
		t.Logf("Stop services using these ports before running tests.")
		t.Logf("To find what's using a port: lsof -i :<port>")
		t.Logf("To stop all podman containers: podman stop $(podman ps -a -q)")
	}

	assert.Empty(t, unavailablePorts, "All required ports should be available")
}

// testTestcontainersCanStartContainer actually starts a minimal container
func testTestcontainersCanStartContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Log("Starting test container (alpine)...")

	// Start simple alpine container that exits immediately
	req := testcontainers.ContainerRequest{
		Image:      "alpine:latest",
		Cmd:        []string{"echo", "hello"},
		WaitingFor: wait.ForExit(),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start test container")
	defer container.Terminate(ctx)

	// Get logs to verify it ran
	logs, err := container.Logs(ctx)
	require.NoError(t, err, "Failed to get container logs")
	defer logs.Close()

	t.Logf("✅ Successfully started and ran test container")
}

// testNetworkConnectivity checks if we can reach common external services
func testNetworkConnectivity(t *testing.T) {
	// Test DNS resolution
	addrs, err := net.LookupHost("github.com")
	require.NoError(t, err, "DNS resolution failed - network may be down")
	require.NotEmpty(t, addrs, "DNS resolution returned no addresses")
	t.Logf("✅ DNS resolution works: github.com -> %v", addrs)

	// Test external connectivity (try to connect to GitHub)
	conn, err := net.DialTimeout("tcp", "github.com:443", 5*time.Second)
	if err != nil {
		t.Logf("⚠️  External connectivity may be limited: %v", err)
		t.Logf("This is not critical for local tests, but Docker Hub pulls may fail")
	} else {
		conn.Close()
		t.Logf("✅ External connectivity works")
	}
}

// Helper: isPortAvailable checks if a port is available for binding
func isPortAvailable(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
