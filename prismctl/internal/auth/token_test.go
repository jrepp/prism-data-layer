package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "expired token",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "valid token",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &Token{ExpiresAt: tt.expiresAt}
			if got := token.IsExpired(); got != tt.want {
				t.Errorf("Token.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToken_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "needs refresh - within 5 minutes",
			expiresAt: time.Now().Add(3 * time.Minute),
			want:      true,
		},
		{
			name:      "no refresh needed",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &Token{ExpiresAt: tt.expiresAt}
			if got := token.NeedsRefresh(); got != tt.want {
				t.Errorf("Token.NeedsRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Create a test token
	token := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		IDToken:      "test-id-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		TokenType:    "Bearer",
	}

	// Save token
	if err := tm.Save(token); err != nil {
		t.Fatalf("TokenManager.Save() error = %v", err)
	}

	// Check file permissions
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Token file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Load token
	loaded, err := tm.Load()
	if err != nil {
		t.Fatalf("TokenManager.Load() error = %v", err)
	}

	if loaded.AccessToken != token.AccessToken {
		t.Errorf("Loaded AccessToken = %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("Loaded RefreshToken = %q, want %q", loaded.RefreshToken, token.RefreshToken)
	}
}

func TestTokenManager_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Create a token
	token := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		TokenType:   "Bearer",
	}

	if err := tm.Save(token); err != nil {
		t.Fatalf("TokenManager.Save() error = %v", err)
	}

	// Delete token
	if err := tm.Delete(); err != nil {
		t.Fatalf("TokenManager.Delete() error = %v", err)
	}

	// Verify deletion
	if tm.Exists() {
		t.Error("Token file still exists after Delete()")
	}

	// Delete non-existent token (should not error)
	if err := tm.Delete(); err != nil {
		t.Errorf("TokenManager.Delete() on non-existent file error = %v, want nil", err)
	}
}

func TestTokenManager_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Should not exist initially
	if tm.Exists() {
		t.Error("TokenManager.Exists() = true, want false (before creation)")
	}

	// Create token
	token := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		TokenType:   "Bearer",
	}

	if err := tm.Save(token); err != nil {
		t.Fatalf("TokenManager.Save() error = %v", err)
	}

	// Should exist now
	if !tm.Exists() {
		t.Error("TokenManager.Exists() = false, want true (after creation)")
	}
}
