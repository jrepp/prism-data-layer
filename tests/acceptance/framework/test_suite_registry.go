package framework

import (
	"fmt"
	"sync"
)

// TestSuite represents a collection of tests for a specific interface
type TestSuite struct {
	// InterfaceName is the gRPC interface name (e.g., "KeyValueBasicInterface")
	InterfaceName string

	// Pattern is the backward-compatible pattern enum
	Pattern Pattern

	// Tests are the actual test cases
	Tests []PatternTest

	// Description provides context about what this suite tests
	Description string
}

var (
	// Global test suite registry
	testSuites = make(map[string]*TestSuite)
	suitesMu   sync.RWMutex
)

// RegisterTestSuite registers a test suite for an interface
// This is typically called from init() functions in test files
func RegisterTestSuite(suite TestSuite) error {
	suitesMu.Lock()
	defer suitesMu.Unlock()

	if suite.InterfaceName == "" {
		return fmt.Errorf("test suite must specify InterfaceName")
	}

	if len(suite.Tests) == 0 {
		return fmt.Errorf("test suite for %s must contain at least one test", suite.InterfaceName)
	}

	if _, exists := testSuites[suite.InterfaceName]; exists {
		return fmt.Errorf("test suite for %s is already registered", suite.InterfaceName)
	}

	testSuites[suite.InterfaceName] = &suite
	return nil
}

// MustRegisterTestSuite registers a test suite and panics on error
func MustRegisterTestSuite(suite TestSuite) {
	if err := RegisterTestSuite(suite); err != nil {
		panic(fmt.Sprintf("failed to register test suite: %v", err))
	}
}

// GetTestSuite retrieves a test suite by interface name
func GetTestSuite(interfaceName string) (*TestSuite, bool) {
	suitesMu.RLock()
	defer suitesMu.RUnlock()

	suite, ok := testSuites[interfaceName]
	return suite, ok
}

// GetAllTestSuites returns all registered test suites
func GetAllTestSuites() []*TestSuite {
	suitesMu.RLock()
	defer suitesMu.RUnlock()

	suites := make([]*TestSuite, 0, len(testSuites))
	for _, suite := range testSuites {
		suites = append(suites, suite)
	}

	return suites
}

// GetTestSuitesForInterfaces returns test suites for a list of interface names
func GetTestSuitesForInterfaces(interfaces []string) []*TestSuite {
	suitesMu.RLock()
	defer suitesMu.RUnlock()

	suites := make([]*TestSuite, 0, len(interfaces))
	for _, iface := range interfaces {
		if suite, ok := testSuites[iface]; ok {
			suites = append(suites, suite)
		}
	}

	return suites
}

// GetTestSuiteNames returns the names of all registered test suites
func GetTestSuiteNames() []string {
	suitesMu.RLock()
	defer suitesMu.RUnlock()

	names := make([]string, 0, len(testSuites))
	for name := range testSuites {
		names = append(names, name)
	}

	return names
}

// ClearTestSuites removes all registered test suites (for testing only)
func ClearTestSuites() {
	suitesMu.Lock()
	defer suitesMu.Unlock()

	testSuites = make(map[string]*TestSuite)
}

// TestSuiteCount returns the number of registered test suites
func TestSuiteCount() int {
	suitesMu.RLock()
	defer suitesMu.RUnlock()

	return len(testSuites)
}
