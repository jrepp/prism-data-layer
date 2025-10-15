package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistry_Discover(t *testing.T) {
	// Use the test-pattern directory from project root
	testDir := "../../patterns"

	registry := NewRegistry(testDir)
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}

	// Discover patterns
	err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Check that at least the test pattern was discovered
	count := registry.Count()
	if count == 0 {
		t.Fatal("No patterns discovered")
	}

	t.Logf("Discovered %d patterns", count)

	// List all patterns
	patterns := registry.ListPatterns()
	if len(patterns) != count {
		t.Errorf("ListPatterns returned %d patterns, expected %d", len(patterns), count)
	}
}

func TestRegistry_GetPattern(t *testing.T) {
	testDir := "../../patterns"

	registry := NewRegistry(testDir)
	err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Try to get the test-pattern
	manifest, ok := registry.GetPattern("test-pattern")
	if !ok {
		t.Fatal("test-pattern not found")
	}

	if manifest.Name != "test-pattern" {
		t.Errorf("Expected pattern name 'test-pattern', got '%s'", manifest.Name)
	}

	if manifest.Version == "" {
		t.Error("Pattern version is empty")
	}

	t.Logf("Found test-pattern: version=%s, isolation=%s", manifest.Version, manifest.IsolationLevel)
}

func TestRegistry_NonExistentDirectory(t *testing.T) {
	registry := NewRegistry("/nonexistent/directory")
	err := registry.Discover()
	if err == nil {
		t.Fatal("Expected error for nonexistent directory, got nil")
	}
}

func TestLoadManifest_Valid(t *testing.T) {
	manifestPath := "../../patterns/test-pattern/manifest.yaml"

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if manifest.Name != "test-pattern" {
		t.Errorf("Expected name 'test-pattern', got '%s'", manifest.Name)
	}

	if manifest.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", manifest.Version)
	}

	if manifest.IsolationLevel != "namespace" {
		t.Errorf("Expected isolation 'namespace', got '%s'", manifest.IsolationLevel)
	}

	if manifest.HealthCheck.Port != 9090 {
		t.Errorf("Expected health port 9090, got %d", manifest.HealthCheck.Port)
	}

	if manifest.HealthCheck.Path != "/health" {
		t.Errorf("Expected health path '/health', got '%s'", manifest.HealthCheck.Path)
	}

	// Check executable path resolution
	execPath := manifest.ExecutablePath()
	if execPath == "" {
		t.Error("ExecutablePath returned empty string")
	}

	if !filepath.IsAbs(execPath) {
		t.Errorf("ExecutablePath should return absolute path, got: %s", execPath)
	}

	t.Logf("Executable path: %s", execPath)

	// Check if executable exists
	_, err = os.Stat(execPath)
	if err != nil {
		t.Errorf("Executable not found: %v", err)
	}
}

func TestLoadManifest_InvalidIsolation(t *testing.T) {
	// Create temporary manifest with invalid isolation level
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	content := `
name: test
version: 1.0.0
executable: /bin/echo
isolation_level: invalid
healthcheck:
  port: 9090
  path: /health
  interval: 30s
`

	err := os.WriteFile(manifestPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	_, err = LoadManifest(manifestPath)
	if err == nil {
		t.Fatal("Expected error for invalid isolation level, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestLoadManifest_MissingExecutable(t *testing.T) {
	// Create temporary manifest with nonexistent executable
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	content := `
name: test
version: 1.0.0
executable: /nonexistent/binary
isolation_level: namespace
healthcheck:
  port: 9090
  path: /health
  interval: 30s
`

	err := os.WriteFile(manifestPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	_, err = LoadManifest(manifestPath)
	if err == nil {
		t.Fatal("Expected error for missing executable, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

func TestManifest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		manifest *Manifest
		wantErr bool
	}{
		{
			name: "valid manifest",
			manifest: &Manifest{
				Name:           "test",
				Version:        "1.0.0",
				Executable:     "/bin/echo",
				IsolationLevel: "namespace",
				HealthCheck: HealthCheckConfig{
					Port:     9090,
					Path:     "/health",
					Interval: 30,
				},
				manifestPath: "/tmp/manifest.yaml",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				Version:        "1.0.0",
				Executable:     "/bin/echo",
				IsolationLevel: "namespace",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			manifest: &Manifest{
				Name:           "test",
				Executable:     "/bin/echo",
				IsolationLevel: "namespace",
			},
			wantErr: true,
		},
		{
			name: "invalid isolation",
			manifest: &Manifest{
				Name:           "test",
				Version:        "1.0.0",
				Executable:     "/bin/echo",
				IsolationLevel: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_Reload(t *testing.T) {
	testDir := "../../patterns"

	registry := NewRegistry(testDir)

	// Initial discovery
	err := registry.Discover()
	if err != nil {
		t.Fatalf("Initial Discover failed: %v", err)
	}

	initialCount := registry.Count()

	// Reload
	err = registry.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	reloadCount := registry.Count()

	if reloadCount != initialCount {
		t.Errorf("After reload, count changed from %d to %d", initialCount, reloadCount)
	}

	t.Logf("Reload successful: %d patterns", reloadCount)
}
