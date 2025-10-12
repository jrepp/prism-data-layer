package framework

import (
	"fmt"
	"sort"
	"sync"
)

var (
	// Global backend registry
	registry = make(map[string]Backend)
	mu       sync.RWMutex
)

// RegisterBackend registers a backend with the test framework
// This is typically called from init() functions in backend implementation files
func RegisterBackend(backend Backend) error {
	mu.Lock()
	defer mu.Unlock()

	if backend.Name == "" {
		return fmt.Errorf("backend name cannot be empty")
	}

	if backend.SetupFunc == nil {
		return fmt.Errorf("backend %s must provide SetupFunc", backend.Name)
	}

	if len(backend.SupportedPatterns) == 0 {
		return fmt.Errorf("backend %s must support at least one pattern", backend.Name)
	}

	if _, exists := registry[backend.Name]; exists {
		return fmt.Errorf("backend %s is already registered", backend.Name)
	}

	registry[backend.Name] = backend
	return nil
}

// MustRegisterBackend registers a backend and panics on error
// Convenient for use in init() functions
func MustRegisterBackend(backend Backend) {
	if err := RegisterBackend(backend); err != nil {
		panic(fmt.Sprintf("failed to register backend: %v", err))
	}
}

// GetBackend retrieves a registered backend by name
func GetBackend(name string) (Backend, bool) {
	mu.RLock()
	defer mu.RUnlock()

	backend, ok := registry[name]
	return backend, ok
}

// GetAllBackends returns all registered backends sorted by name
func GetAllBackends() []Backend {
	mu.RLock()
	defer mu.RUnlock()

	backends := make([]Backend, 0, len(registry))
	for _, backend := range registry {
		backends = append(backends, backend)
	}

	// Sort by name for consistent ordering
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})

	return backends
}

// GetBackendsForPattern returns all backends that support a specific pattern
func GetBackendsForPattern(pattern Pattern) []Backend {
	mu.RLock()
	defer mu.RUnlock()

	var backends []Backend
	for _, backend := range registry {
		if backend.SupportsPattern(pattern) {
			backends = append(backends, backend)
		}
	}

	// Sort by name for consistent ordering
	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Name < backends[j].Name
	})

	return backends
}

// GetBackendNames returns the names of all registered backends
func GetBackendNames() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// GetSupportedPatterns returns all patterns supported by at least one backend
func GetSupportedPatterns() []Pattern {
	mu.RLock()
	defer mu.RUnlock()

	patternSet := make(map[Pattern]bool)
	for _, backend := range registry {
		for _, pattern := range backend.SupportedPatterns {
			patternSet[pattern] = true
		}
	}

	patterns := make([]Pattern, 0, len(patternSet))
	for pattern := range patternSet {
		patterns = append(patterns, pattern)
	}

	// Sort patterns for consistent ordering
	sort.Slice(patterns, func(i, j int) bool {
		return string(patterns[i]) < string(patterns[j])
	})

	return patterns
}

// SupportsPattern checks if a backend supports a specific pattern
func (b Backend) SupportsPattern(pattern Pattern) bool {
	for _, p := range b.SupportedPatterns {
		if p == pattern {
			return true
		}
	}
	return false
}

// HasCapability checks if a backend has a specific capability
func (b Backend) HasCapability(capability string) bool {
	return b.Capabilities.HasCapability(capability)
}

// ClearRegistry removes all registered backends (for testing only)
func ClearRegistry() {
	mu.Lock()
	defer mu.Unlock()

	registry = make(map[string]Backend)
}

// BackendCount returns the number of registered backends
func BackendCount() int {
	mu.RLock()
	defer mu.RUnlock()

	return len(registry)
}

// PatternBackendMatrix returns a matrix of patterns and backends
// Useful for generating compliance reports
func PatternBackendMatrix() map[Pattern][]string {
	mu.RLock()
	defer mu.RUnlock()

	matrix := make(map[Pattern][]string)

	for _, backend := range registry {
		for _, pattern := range backend.SupportedPatterns {
			matrix[pattern] = append(matrix[pattern], backend.Name)
		}
	}

	// Sort backend lists
	for pattern := range matrix {
		sort.Strings(matrix[pattern])
	}

	return matrix
}
