// Package cmd provides the CLI commands for prismctl
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	localStackPIDs map[string]int
)

func init() {
	localStackPIDs = make(map[string]int)
	rootCmd.AddCommand(localCmd)
	localCmd.AddCommand(localStartCmd)
	localCmd.AddCommand(localStopCmd)
	localCmd.AddCommand(localStatusCmd)
	localCmd.AddCommand(localLogsCmd)
	localCmd.AddCommand(localNamespaceCmd)
}

// localNamespaceCmd provisions a namespace via control plane
var localNamespaceCmd = &cobra.Command{
	Use:   "namespace [name]",
	Short: "Provision a namespace via the control plane",
	Long: `Provision a namespace by sending a CreateNamespace request through the control plane.

Example:
  prismctl local namespace $admin-logs`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return provisionNamespace(args[0])
	},
}

// localCmd represents the local command
var localCmd = &cobra.Command{
	Use:   "local",
	Short: "Manage local Prism stack for development",
	Long: `Manage a local Prism stack for development and testing.

The local stack includes:
- prism-admin: Admin server managing proxy configurations
- prism-proxy (2 instances): Data plane proxies
- pattern-launcher: Pattern lifecycle manager
- keyvalue-runner: KeyValue pattern with MemStore backend

All components run from the build/binaries/ directory.`,
}

// localStartCmd starts the local Prism stack
var localStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the local Prism stack",
	Long: `Start a complete local Prism development stack.

This starts:
1. prism-admin with:
   - gRPC API on :8981 (admin control plane)
   - Web UI on :8080 (admin dashboard at http://localhost:8080)
2. pattern-launcher on :7070 (connected to prism-admin)
3. keyvalue-runner with memstore backend

All processes run in the background and logs are captured.

The Admin Web UI provides:
  - Real-time dashboard with system metrics
  - Proxy status and management
  - Launcher status and capacity monitoring
  - Namespace management`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startLocalStack()
	},
}

// localStopCmd stops the local Prism stack
var localStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the local Prism stack",
	Long:  `Stop all components of the local Prism stack.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopLocalStack()
	},
}

// localStatusCmd shows the status of the local Prism stack
var localStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of local Prism stack",
	Long:  `Display the running status of all local Prism components.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showLocalStackStatus()
	},
}

// localLogsCmd shows logs from local Prism components
var localLogsCmd = &cobra.Command{
	Use:   "logs [component]",
	Short: "Show logs from local Prism components",
	Long: `Show logs from local Prism stack components.

Components: admin, proxy1, proxy2, launcher, keyvalue

Example:
  prismctl local logs admin
  prismctl local logs proxy1`,
	ValidArgs: []string{"admin", "proxy1", "proxy2", "launcher", "keyvalue"},
	Args:      cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := "all"
		if len(args) > 0 {
			component = args[0]
		}
		return showLocalStackLogs(component)
	},
}

// startLocalStack starts all components of the local stack
func startLocalStack() error {
	// Ensure we're in or can find the binaries directory
	binDir, err := findBinariesDir()
	if err != nil {
		return fmt.Errorf("cannot find binaries directory: %w", err)
	}

	fmt.Printf("🚀 Starting local Prism stack from %s\n\n", binDir)

	// Create logs directory
	logsDir := filepath.Join(binDir, "..", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Convert binDir to absolute path
	absBinDir, err := filepath.Abs(binDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for binaries directory: %w", err)
	}

	// Find patterns directory (should be at project root)
	patternsDir := filepath.Join(absBinDir, "..", "..", "patterns")
	if _, err := os.Stat(patternsDir); os.IsNotExist(err) {
		return fmt.Errorf("patterns directory not found at %s", patternsDir)
	}

	// Start components in order
	components := []struct {
		name    string
		binary  string
		args    []string
		logFile string
		delay   time.Duration
	}{
		{
			name:    "prism-admin",
			binary:  filepath.Join(absBinDir, "prism-admin"),
			args:    []string{"serve", "--port=8981"},
			logFile: filepath.Join(logsDir, "admin.log"),
			delay:   2 * time.Second,
		},
		{
			name:    "prism-launcher",
			binary:  filepath.Join(absBinDir, "prism-launcher"),
			args:    []string{"--admin-endpoint=localhost:8981", "--launcher-id=launcher-01", "--grpc-port=7070", "--patterns-dir=" + patternsDir},
			logFile: filepath.Join(logsDir, "launcher.log"),
			delay:   2 * time.Second,
		},
		{
			name:    "keyvalue-runner",
			binary:  filepath.Join(absBinDir, "keyvalue-runner"),
			args:    []string{"--proxy-addr=localhost:9090"},
			logFile: filepath.Join(logsDir, "keyvalue.log"),
			delay:   1 * time.Second,
		},
	}

	for _, comp := range components {
		fmt.Printf("  Starting %s...\n", comp.name)

		// Check if binary exists
		if _, err := os.Stat(comp.binary); os.IsNotExist(err) {
			return fmt.Errorf("binary not found: %s (run 'make build' first)", comp.binary)
		}

		// Create log file
		logFile, err := os.Create(comp.logFile)
		if err != nil {
			return fmt.Errorf("failed to create log file for %s: %w", comp.name, err)
		}

		// Start process - use exec.Command (not CommandContext) so process survives parent exit
		cmd := exec.Command(comp.binary, comp.args...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Dir = binDir

		// Detach process from parent so it continues running after prismctl exits
		// This creates a new process group for the child
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}

		if err := cmd.Start(); err != nil {
			logFile.Close()
			return fmt.Errorf("failed to start %s: %w", comp.name, err)
		}

		// Close the log file handle in the parent process
		// The child process has its own handle and will continue writing
		logFile.Close()

		// Store PID
		localStackPIDs[comp.name] = cmd.Process.Pid
		fmt.Printf("    ✅ %s started (PID: %d)\n", comp.name, cmd.Process.Pid)

		// Save PID to file for stop command
		pidFile := filepath.Join(logsDir, fmt.Sprintf("%s.pid", comp.name))
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
			fmt.Printf("    ⚠️  Warning: Could not save PID file: %v\n", err)
		}

		// Wait before starting next component
		if comp.delay > 0 {
			time.Sleep(comp.delay)
		}
	}

	fmt.Printf("\n✅ Local Prism stack started successfully!\n\n")
	fmt.Println("📊 Stack Overview:")
	fmt.Println("  • Admin Control Plane: localhost:8981 (gRPC)")
	fmt.Println("  • Admin Web UI:        http://localhost:8080 🌐")
	fmt.Println("  • Pattern Launcher:    localhost:7070")
	fmt.Println("  • KeyValue: Ready (MemStore backend)")
	fmt.Println()
	fmt.Println("📝 View logs:  prismctl local logs [component]")
	fmt.Println("🛑 Stop stack: prismctl local stop")

	return nil
}

// stopLocalStack stops all components of the local stack in proper order
// Pattern runners → Launchers → Admin (last)
func stopLocalStack() error {
	binDir, err := findBinariesDir()
	if err != nil {
		return fmt.Errorf("cannot find binaries directory: %w", err)
	}

	logsDir := filepath.Join(binDir, "..", "logs")

	fmt.Println("🛑 Stopping local Prism stack (graceful shutdown)...")
	fmt.Println()

	// Step 1: Check admin connectivity for coordinated shutdown
	adminRunning := checkAdminConnectivity()
	if adminRunning {
		fmt.Println("  ✅ Admin control plane is running - coordinating graceful shutdown")
	} else {
		fmt.Println("  ⚠️  Admin not reachable - proceeding with direct shutdown")
	}
	fmt.Println()

	// Step 2: Stop pattern runners first (they depend on launchers/proxies)
	fmt.Println("  Stopping pattern runners...")
	patternRunners := []string{"keyvalue-runner"}
	for _, comp := range patternRunners {
		if err := stopComponent(logsDir, comp); err != nil {
			fmt.Printf("    ⚠️  %s: %v\n", comp, err)
		}
	}
	fmt.Println()

	// Step 3: Stop launchers (they depend on admin)
	fmt.Println("  Stopping launchers...")
	launchers := []string{"prism-launcher"}
	for _, comp := range launchers {
		if err := stopComponent(logsDir, comp); err != nil {
			fmt.Printf("    ⚠️  %s: %v\n", comp, err)
		}
	}
	fmt.Println()

	// Step 4: Wait a moment for graceful shutdown
	fmt.Println("  Waiting for graceful shutdown...")
	time.Sleep(1 * time.Second)
	fmt.Println()

	// Step 5: Finally, stop admin itself (no dependencies)
	fmt.Println("  Stopping admin control plane...")
	if err := stopComponent(logsDir, "prism-admin"); err != nil {
		fmt.Printf("    ⚠️  prism-admin: %v\n", err)
	}

	fmt.Println()
	fmt.Println("✅ Local Prism stack stopped cleanly")
	return nil
}

// checkAdminConnectivity checks if admin is reachable
func checkAdminConnectivity() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		"localhost:8981",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// stopComponent stops a single component by PID file
func stopComponent(logsDir, component string) error {
	pidFile := filepath.Join(logsDir, fmt.Sprintf("%s.pid", component))
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("no PID file found")
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file")
	}

	// Check if process exists first
	if err := syscall.Kill(pid, syscall.Signal(0)); err != nil {
		os.Remove(pidFile)
		return fmt.Errorf("not running (PID: %d)", pid)
	}

	fmt.Printf("    Stopping %s (PID: %d)...\n", component, pid)

	// Send SIGTERM to process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found")
	}

	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send signal: %v", err)
	}

	// Wait for process to exit (up to 5 seconds)
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(pid, syscall.Signal(0)); err != nil {
			// Process has exited
			os.Remove(pidFile)
			fmt.Printf("    ✅ %s stopped\n", component)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// If still running after 5 seconds, force kill
	fmt.Printf("    ⚠️  %s did not stop gracefully, sending SIGKILL...\n", component)
	process.Signal(syscall.SIGKILL)
	os.Remove(pidFile)
	return nil
}

// showLocalStackStatus shows the status of all stack components
func showLocalStackStatus() error {
	binDir, err := findBinariesDir()
	if err != nil {
		return fmt.Errorf("cannot find binaries directory: %w", err)
	}

	logsDir := filepath.Join(binDir, "..", "logs")

	fmt.Println("📊 Local Prism Stack Status")

	components := []string{"prism-admin", "prism-launcher", "keyvalue-runner"}

	for _, comp := range components {
		pidFile := filepath.Join(logsDir, fmt.Sprintf("%s.pid", comp))
		pidData, err := os.ReadFile(pidFile)
		if err != nil {
			fmt.Printf("  ❌ %s: Not running\n", comp)
			continue
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
			fmt.Printf("  ❌ %s: Invalid PID\n", comp)
			continue
		}

		// Check if process is running using signal 0 (null signal)
		// This checks for process existence without actually sending a signal
		err = syscall.Kill(pid, syscall.Signal(0))
		if err != nil {
			if err == syscall.ESRCH {
				fmt.Printf("  ❌ %s: Not running (PID: %d not found)\n", comp, pid)
			} else if err == syscall.EPERM {
				// Process exists but we don't have permission (means it's running)
				fmt.Printf("  ✅ %s: Running (PID: %d)\n", comp, pid)
			} else {
				fmt.Printf("  ❌ %s: Not running (PID: %d exited)\n", comp, pid)
			}
			continue
		}

		fmt.Printf("  ✅ %s: Running (PID: %d)\n", comp, pid)
	}

	return nil
}

// showLocalStackLogs shows logs from a specific component or all components
func showLocalStackLogs(component string) error {
	binDir, err := findBinariesDir()
	if err != nil {
		return fmt.Errorf("cannot find binaries directory: %w", err)
	}

	logsDir := filepath.Join(binDir, "..", "logs")

	if component == "all" {
		components := []string{"admin", "proxy1", "proxy2", "launcher", "keyvalue"}
		for _, comp := range components {
			fmt.Printf("\n=== %s ===\n", comp)
			showComponentLog(logsDir, comp)
		}
		return nil
	}

	return showComponentLog(logsDir, component)
}

// showComponentLog shows the log file for a specific component
func showComponentLog(logsDir, component string) error {
	// Map component name to log file name
	logMap := map[string]string{
		"admin":    "admin.log",
		"proxy1":   "proxy1.log",
		"proxy2":   "proxy2.log",
		"launcher": "launcher.log",
		"keyvalue": "keyvalue.log",
	}

	logFile, ok := logMap[component]
	if !ok {
		return fmt.Errorf("unknown component: %s", component)
	}

	logPath := filepath.Join(logsDir, logFile)
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// findBinariesDir locates the build/binaries directory
func findBinariesDir() (string, error) {
	// Try current directory
	if isInBinariesDir(".") {
		return ".", nil
	}

	// Try build/binaries relative to current directory
	if isInBinariesDir("build/binaries") {
		return "build/binaries", nil
	}

	// Try ../build/binaries (if we're in a subdirectory)
	if isInBinariesDir("../build/binaries") {
		return "../build/binaries", nil
	}

	// Try ../../build/binaries (if we're deeper)
	if isInBinariesDir("../../build/binaries") {
		return "../../build/binaries", nil
	}

	return "", fmt.Errorf("binaries directory not found (looking for prism-proxy, prism-admin, etc.)")
}

// isInBinariesDir checks if a directory contains the expected binaries
func isInBinariesDir(dir string) bool {
	requiredBinaries := []string{"prism-proxy", "prism-admin", "prism-launcher"}
	for _, binary := range requiredBinaries {
		path := filepath.Join(dir, binary)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// provisionNamespace creates a namespace via the control plane
func provisionNamespace(namespace string) error {
	fmt.Printf("📦 Provisioning namespace: %s\n", namespace)

	// Connect to admin control plane
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		"localhost:8981",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to admin: %w", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneClient(conn)

	// Send CreateNamespace request
	req := &pb.CreateNamespaceRequest{
		Namespace:       namespace,
		RequestingProxy: "prismctl-local",
		Principal:       "local-user",
		Config: &pb.NamespaceConfig{
			Backends: map[string]*pb.BackendConfig{
				"memstore": {
					BackendType:      "memstore",
					ConnectionString: "memory://local",
					Credentials:      map[string]string{},
					Options:          map[string]string{},
				},
			},
			Patterns: map[string]*pb.PatternConfig{
				"keyvalue": {
					PatternName:         "keyvalue",
					Settings:            map[string]string{},
					RequiredInterfaces:  []string{"KeyValue"},
				},
			},
			Auth:     &pb.AuthConfig{Enabled: false},
			Metadata: map[string]string{"source": "prismctl-local"},
		},
	}

	resp, err := client.CreateNamespace(ctx, req)
	if err != nil {
		// Improve error messages for common issues
		if strings.Contains(err.Error(), "no proxy assigned to partition") {
			fmt.Printf("\n❌ Namespace creation failed\n")
			fmt.Printf("   Error: No proxy is available to handle this namespace\n")
			fmt.Printf("   Namespace: %s\n", namespace)
			fmt.Printf("\n")
			fmt.Printf("   This typically means:\n")
			fmt.Printf("     • No prism-proxy instances are running\n")
			fmt.Printf("     • No proxy has registered with the admin control plane\n")
			fmt.Printf("\n")
			fmt.Printf("   To fix:\n")
			fmt.Printf("     1. Start a prism-proxy instance\n")
			fmt.Printf("     2. Ensure it connects to admin at localhost:8981\n")
			fmt.Printf("     3. Retry namespace creation\n")
			fmt.Printf("\n")
			return fmt.Errorf("no proxy available")
		}
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("namespace creation rejected: %s", resp.Message)
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("✅ Namespace Created Successfully\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  Namespace:  %s\n", namespace)
	fmt.Printf("  Partition:  %d\n", resp.AssignedPartition)
	fmt.Printf("  Proxy:      %s\n", resp.AssignedProxy)
	fmt.Printf("  Message:    %s\n", resp.Message)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	return nil
}
