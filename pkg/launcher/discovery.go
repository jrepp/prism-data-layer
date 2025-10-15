package launcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Registry maintains a collection of discovered patterns
type Registry struct {
	mu        sync.RWMutex
	patterns  map[string]*Manifest // pattern name -> manifest
	directory string               // root directory for pattern discovery
}

// NewRegistry creates a new pattern registry
func NewRegistry(directory string) *Registry {
	return &Registry{
		patterns:  make(map[string]*Manifest),
		directory: directory,
	}
}

// Discover scans the patterns directory and loads all manifests
func (r *Registry) Discover() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf("Discovering patterns in directory: %s", r.directory)

	// Check if directory exists
	if _, err := os.Stat(r.directory); err != nil {
		return fmt.Errorf("patterns directory not found: %s: %w", r.directory, err)
	}

	// Read directory entries
	entries, err := os.ReadDir(r.directory)
	if err != nil {
		return fmt.Errorf("read patterns directory: %w", err)
	}

	discovered := 0
	failed := 0

	// Scan each subdirectory for manifest.yaml
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		patternDir := filepath.Join(r.directory, entry.Name())
		manifestPath := filepath.Join(patternDir, "manifest.yaml")

		// Check if manifest.yaml exists
		if _, err := os.Stat(manifestPath); err != nil {
			log.Printf("Pattern directory %s has no manifest.yaml, skipping", entry.Name())
			continue
		}

		// Load manifest
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			log.Printf("Failed to load manifest for %s: %v", entry.Name(), err)
			failed++
			continue
		}

		// Register pattern
		r.patterns[manifest.Name] = manifest
		discovered++

		log.Printf("Discovered pattern: %s (version: %s, isolation: %s)",
			manifest.Name, manifest.Version, manifest.IsolationLevel)
	}

	log.Printf("Pattern discovery complete: %d discovered, %d failed", discovered, failed)

	if discovered == 0 {
		return fmt.Errorf("no patterns discovered in directory: %s", r.directory)
	}

	return nil
}

// GetPattern returns a manifest by name
func (r *Registry) GetPattern(name string) (*Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	manifest, ok := r.patterns[name]
	return manifest, ok
}

// ListPatterns returns all registered patterns
func (r *Registry) ListPatterns() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	patterns := make([]*Manifest, 0, len(r.patterns))
	for _, manifest := range r.patterns {
		patterns = append(patterns, manifest)
	}

	return patterns
}

// Count returns the number of registered patterns
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.patterns)
}

// Reload re-discovers all patterns (useful for hot-reload)
func (r *Registry) Reload() error {
	// Clear existing patterns
	r.mu.Lock()
	r.patterns = make(map[string]*Manifest)
	r.mu.Unlock()

	// Re-discover
	return r.Discover()
}
