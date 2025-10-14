package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/interface-suites/keyvalue_basic"
	"github.com/jrepp/prism-data-layer/tests/interface-suites/keyvalue_ttl"
)

// PatternRunner discovers interfaces from a pattern executable and runs test suites
type PatternRunner struct {
	patternExe string
	verbose    bool
}

func main() {
	var (
		patternExe = flag.String("pattern", "", "Path to compiled pattern executable")
		verbose    = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *patternExe == "" {
		log.Fatal("Usage: pattern-runner -pattern <path-to-pattern-executable>")
	}

	runner := &PatternRunner{
		patternExe: *patternExe,
		verbose:    *verbose,
	}

	if err := runner.Run(); err != nil {
		log.Fatal(err)
	}
}

// Run discovers interfaces and executes test suites
func (r *PatternRunner) Run() error {
	fmt.Printf("Pattern Runner: Testing %s\n", r.patternExe)

	// Start the pattern executable
	ctx := context.Background()
	patternClient, err := r.startPattern(ctx)
	if err != nil {
		return fmt.Errorf("failed to start pattern: %w", err)
	}
	defer patternClient.Stop()

	// Discover supported interfaces via InterfaceSupport interface
	interfaces, err := r.discoverInterfaces(patternClient)
	if err != nil {
		return fmt.Errorf("failed to discover interfaces: %w", err)
	}

	fmt.Printf("Discovered %d interfaces: %v\n", len(interfaces), interfaces)

	// Run test suite for each supported interface
	for _, iface := range interfaces {
		if err := r.runSuiteForInterface(iface, patternClient); err != nil {
			return fmt.Errorf("test suite failed for %s: %w", iface, err)
		}
	}

	fmt.Println("✅ All test suites passed")
	return nil
}

// startPattern launches the pattern executable and establishes connection
func (r *PatternRunner) startPattern(ctx context.Context) (*PatternClient, error) {
	// Start pattern as subprocess
	cmd := exec.CommandContext(ctx, r.patternExe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start pattern: %w", err)
	}

	// TODO: Connect to pattern's gRPC server
	// For now, return a placeholder client
	client := &PatternClient{
		cmd: cmd,
	}

	return client, nil
}

// discoverInterfaces queries the pattern for supported interfaces
func (r *PatternRunner) discoverInterfaces(client *PatternClient) ([]string, error) {
	// TODO: Use InterfaceSupport.ListInterfaces() RPC
	// For now, return hardcoded interfaces
	return []string{"KeyValueBasicInterface", "KeyValueTTLInterface"}, nil
}

// runSuiteForInterface runs the appropriate test suite for an interface
func (r *PatternRunner) runSuiteForInterface(iface string, client *PatternClient) error {
	fmt.Printf("\n🧪 Running test suite for %s\n", iface)

	// Create a test instance for running suites programmatically
	t := &testing.T{}

	switch iface {
	case "KeyValueBasicInterface":
		suite := &keyvalue_basic.Suite{
			CreateBackend: func(t *testing.T) plugin.KeyValueBasicInterface {
				return client
			},
			CleanupBackend: func(t *testing.T, backend plugin.KeyValueBasicInterface) {
				// No-op, client cleanup happens at runner level
			},
		}
		suite.Run(t)

	case "KeyValueTTLInterface":
		suite := &keyvalue_ttl.Suite{
			CreateBackend: func(t *testing.T) plugin.KeyValueBasicInterface {
				return client
			},
			CleanupBackend: func(t *testing.T, backend plugin.KeyValueBasicInterface) {
				// No-op
			},
		}
		suite.Run(t)

	default:
		return fmt.Errorf("unsupported interface: %s", iface)
	}

	// Check if any tests failed
	if t.Failed() {
		return fmt.Errorf("test suite failed for %s", iface)
	}

	fmt.Printf("✅ %s tests passed\n", iface)
	return nil
}

// PatternClient wraps a connection to a running pattern executable
type PatternClient struct {
	cmd *exec.Cmd
	// TODO: Add gRPC client connection
}

// Implement plugin.KeyValueBasicInterface by forwarding to pattern via gRPC
func (c *PatternClient) Set(key string, value []byte, ttlSeconds int64) error {
	// TODO: Forward to pattern via gRPC
	return fmt.Errorf("not implemented")
}

func (c *PatternClient) Get(key string) ([]byte, bool, error) {
	// TODO: Forward to pattern via gRPC
	return nil, false, fmt.Errorf("not implemented")
}

func (c *PatternClient) Delete(key string) error {
	// TODO: Forward to pattern via gRPC
	return fmt.Errorf("not implemented")
}

func (c *PatternClient) Exists(key string) (bool, error) {
	// TODO: Forward to pattern via gRPC
	return false, fmt.Errorf("not implemented")
}

func (c *PatternClient) Stop() error {
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
