package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Test loading with defaults (no config file)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.OIDC.Issuer == "" {
		t.Error("OIDC.Issuer should have default value")
	}

	if cfg.Proxy.URL == "" {
		t.Error("Proxy.URL should have default value")
	}

	if cfg.Token.Path == "" {
		t.Error("Token.Path should have default value")
	}
}

func TestEnsurePrismDir(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	if err := EnsurePrismDir(); err != nil {
		t.Fatalf("EnsurePrismDir() error = %v, want nil", err)
	}

	prismDir := filepath.Join(tmpDir, ".prism")
	if _, err := os.Stat(prismDir); os.IsNotExist(err) {
		t.Errorf("EnsurePrismDir() did not create directory at %s", prismDir)
	}
}

func TestDefaultScopes(t *testing.T) {
	scopes := DefaultScopes()
	expected := []string{"openid", "profile", "email", "offline_access"}

	if len(scopes) != len(expected) {
		t.Errorf("DefaultScopes() returned %d scopes, want %d", len(scopes), len(expected))
	}

	for i, scope := range expected {
		if scopes[i] != scope {
			t.Errorf("DefaultScopes()[%d] = %q, want %q", i, scopes[i], scope)
		}
	}
}
