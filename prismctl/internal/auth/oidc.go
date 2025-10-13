package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/config"
	"golang.org/x/oauth2"
)

// OIDCAuthenticator handles OIDC authentication flows
type OIDCAuthenticator struct {
	config                      *config.OIDCConfig
	issuer                      string
	tokenEndpoint               string
	deviceAuthorizationEndpoint string
	authorizationEndpoint       string
	userinfoEndpoint            string
}

// NewOIDCAuthenticator creates a new OIDC authenticator
func NewOIDCAuthenticator(cfg *config.OIDCConfig) (*OIDCAuthenticator, error) {
	auth := &OIDCAuthenticator{
		config: cfg,
		issuer: strings.TrimSuffix(cfg.Issuer, "/"),
	}

	if err := auth.discoverEndpoints(); err != nil {
		return nil, fmt.Errorf("discover OIDC endpoints: %w", err)
	}

	return auth, nil
}

// discoverEndpoints discovers OIDC endpoints from the issuer
func (a *OIDCAuthenticator) discoverEndpoints() error {
	discoveryURL := a.issuer + "/.well-known/openid-configuration"

	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("fetch discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery request failed with status %d", resp.StatusCode)
	}

	var discovery struct {
		TokenEndpoint               string `json:"token_endpoint"`
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
		AuthorizationEndpoint       string `json:"authorization_endpoint"`
		UserinfoEndpoint            string `json:"userinfo_endpoint"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return fmt.Errorf("decode discovery document: %w", err)
	}

	a.tokenEndpoint = discovery.TokenEndpoint
	a.deviceAuthorizationEndpoint = discovery.DeviceAuthorizationEndpoint
	a.authorizationEndpoint = discovery.AuthorizationEndpoint
	a.userinfoEndpoint = discovery.UserinfoEndpoint

	return nil
}

// DeviceCodeResponse represents the response from device authorization endpoint
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// LoginDeviceCode performs device code authentication flow
func (a *OIDCAuthenticator) LoginDeviceCode(ctx context.Context) (*DeviceCodeResponse, *Token, error) {
	if a.deviceAuthorizationEndpoint == "" {
		return nil, nil, fmt.Errorf("device code flow not supported by OIDC provider")
	}

	// Step 1: Request device code
	scopes := a.config.Scopes
	if len(scopes) == 0 {
		scopes = config.DefaultScopes()
	}

	data := url.Values{}
	data.Set("client_id", a.config.ClientID)
	data.Set("scope", strings.Join(scopes, " "))

	resp, err := http.PostForm(a.deviceAuthorizationEndpoint, data)
	if err != nil {
		return nil, nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("device authorization failed with status %d: %s", resp.StatusCode, string(body))
	}

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, nil, fmt.Errorf("decode device code response: %w", err)
	}

	// Step 2: Poll for token (caller will handle this to show progress)
	return &deviceResp, nil, nil
}

// PollForToken polls the token endpoint until authentication completes
func (a *OIDCAuthenticator) PollForToken(ctx context.Context, deviceCode string, interval int) (*Token, error) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("device_code", deviceCode)
	data.Set("client_id", a.config.ClientID)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			resp, err := http.PostForm(a.tokenEndpoint, data)
			if err != nil {
				return nil, fmt.Errorf("poll for token: %w", err)
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				// Success! Parse token
				var tokenResp oauth2.Token
				if err := json.Unmarshal(body, &tokenResp); err != nil {
					return nil, fmt.Errorf("decode token response: %w", err)
				}
				return createToken(&tokenResp), nil
			}

			// Handle error responses
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(body, &errorResp); err != nil {
				return nil, fmt.Errorf("decode error response: %w", err)
			}

			switch errorResp.Error {
			case "authorization_pending":
				// Keep polling
				continue
			case "slow_down":
				// Increase interval
				ticker.Reset(time.Duration(interval+5) * time.Second)
				continue
			case "expired_token":
				return nil, fmt.Errorf("device code expired")
			case "access_denied":
				return nil, fmt.Errorf("authentication denied by user")
			default:
				return nil, fmt.Errorf("authentication error: %s", errorResp.Error)
			}
		}
	}
}

// LoginPassword performs password grant authentication (testing only)
func (a *OIDCAuthenticator) LoginPassword(ctx context.Context, username, password string) (*Token, error) {
	scopes := a.config.Scopes
	if len(scopes) == 0 {
		scopes = config.DefaultScopes()
	}

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("username", username)
	data.Set("password", password)
	data.Set("client_id", a.config.ClientID)
	if a.config.ClientSecret != "" {
		data.Set("client_secret", a.config.ClientSecret)
	}
	data.Set("scope", strings.Join(scopes, " "))

	resp, err := http.PostForm(a.tokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("password grant request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth2.Token
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return createToken(&tokenResp), nil
}

// RefreshToken refreshes an expired access token
func (a *OIDCAuthenticator) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", a.config.ClientID)
	if a.config.ClientSecret != "" {
		data.Set("client_secret", a.config.ClientSecret)
	}

	resp, err := http.PostForm(a.tokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth2.Token
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return createToken(&tokenResp), nil
}

// GetUserinfo retrieves user information using the access token
func (a *OIDCAuthenticator) GetUserinfo(ctx context.Context, token *Token) (map[string]interface{}, error) {
	if a.userinfoEndpoint == "" {
		return nil, fmt.Errorf("userinfo endpoint not available")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", a.userinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userinfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userinfo); err != nil {
		return nil, fmt.Errorf("decode userinfo response: %w", err)
	}

	return userinfo, nil
}

// createToken converts oauth2.Token to our Token type
func createToken(oauthToken *oauth2.Token) *Token {
	expiresAt := oauthToken.Expiry
	if expiresAt.IsZero() {
		// Default to 1 hour if not specified
		expiresAt = time.Now().Add(1 * time.Hour)
	}

	token := &Token{
		AccessToken: oauthToken.AccessToken,
		ExpiresAt:   expiresAt,
		TokenType:   oauthToken.TokenType,
	}

	if oauthToken.RefreshToken != "" {
		token.RefreshToken = oauthToken.RefreshToken
	}

	// Extract ID token from extra fields
	if idToken, ok := oauthToken.Extra("id_token").(string); ok {
		token.IDToken = idToken
	}

	return token
}
