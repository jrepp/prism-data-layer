package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/jrepp/prism-data-layer/prismctl/internal/config"
)

func TestClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("Expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	cfg := &config.ProxyConfig{
		URL:     server.URL,
		Timeout: 10,
	}

	client := NewClient(cfg, nil)
	result, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v, want nil", err)
	}

	if status, ok := result["status"].(string); !ok || status != "healthy" {
		t.Errorf("Health() status = %v, want healthy", status)
	}
}

func TestClient_WithAuth(t *testing.T) {
	const expectedToken = "test-access-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + expectedToken
		if auth != expected {
			t.Errorf("Authorization header = %q, want %q", auth, expected)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"success"}`))
	}))
	defer server.Close()

	cfg := &config.ProxyConfig{
		URL:     server.URL,
		Timeout: 10,
	}

	token := &auth.Token{
		AccessToken: expectedToken,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		TokenType:   "Bearer",
	}

	client := NewClient(cfg, token)
	_, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() with auth error = %v, want nil", err)
	}
}
