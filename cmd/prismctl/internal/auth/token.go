// Package auth provides OIDC authentication for prismctl
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Token represents an OIDC token with metadata
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// IsExpired checks if the access token is expired
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// NeedsRefresh checks if token should be refreshed (within 5 minutes of expiry)
func (t *Token) NeedsRefresh() bool {
	return time.Now().After(t.ExpiresAt.Add(-5 * time.Minute))
}

// TokenManager manages token storage and retrieval
type TokenManager struct {
	tokenPath string
}

// NewTokenManager creates a new token manager
func NewTokenManager(tokenPath string) *TokenManager {
	return &TokenManager{tokenPath: tokenPath}
}

// Save saves the token to disk with secure permissions
func (tm *TokenManager) Save(token *Token) error {
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	if err := os.WriteFile(tm.tokenPath, data, 0600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}

	return nil
}

// Load loads the token from disk
func (tm *TokenManager) Load() (*Token, error) {
	data, err := os.ReadFile(tm.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}

	return &token, nil
}

// Delete removes the stored token
func (tm *TokenManager) Delete() error {
	if err := os.Remove(tm.tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete token file: %w", err)
	}
	return nil
}

// Exists checks if a token file exists
func (tm *TokenManager) Exists() bool {
	_, err := os.Stat(tm.tokenPath)
	return err == nil
}
