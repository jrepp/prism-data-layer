// Package client provides HTTP client for Prism proxy admin APIs
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jrepp/prism-data-layer/prismctl/internal/auth"
	"github.com/jrepp/prism-data-layer/prismctl/internal/config"
)

// Client is an HTTP client for Prism proxy admin APIs
type Client struct {
	baseURL string
	token   *auth.Token
	client  *http.Client
}

// NewClient creates a new Prism API client
func NewClient(cfg *config.ProxyConfig, token *auth.Token) *Client {
	return &Client{
		baseURL: cfg.URL,
		token:   token,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

// Health checks proxy health
func (c *Client) Health(ctx context.Context) (map[string]interface{}, error) {
	return c.doRequest(ctx, "GET", "/health", nil)
}

// Ready checks if proxy is ready
func (c *Client) Ready(ctx context.Context) (map[string]interface{}, error) {
	return c.doRequest(ctx, "GET", "/ready", nil)
}

// Metrics retrieves Prometheus metrics
func (c *Client) Metrics(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, "GET", "/metrics", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return string(body), nil
}

// ListNamespaces lists all namespaces
func (c *Client) ListNamespaces(ctx context.Context) ([]map[string]interface{}, error) {
	result, err := c.doRequest(ctx, "GET", "/api/v1/namespaces", nil)
	if err != nil {
		return nil, err
	}

	// Result should be an array
	if arr, ok := result["namespaces"].([]interface{}); ok {
		namespaces := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			if ns, ok := item.(map[string]interface{}); ok {
				namespaces[i] = ns
			}
		}
		return namespaces, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// GetNamespace retrieves namespace details
func (c *Client) GetNamespace(ctx context.Context, name string) (map[string]interface{}, error) {
	return c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/namespaces/%s", name), nil)
}

// ListSessions lists active sessions
func (c *Client) ListSessions(ctx context.Context, namespace string) ([]map[string]interface{}, error) {
	path := "/api/v1/sessions"
	if namespace != "" {
		path += "?namespace=" + namespace
	}

	result, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	// Result should be an array
	if arr, ok := result["sessions"].([]interface{}); ok {
		sessions := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			if session, ok := item.(map[string]interface{}); ok {
				sessions[i] = session
			}
		}
		return sessions, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// PublishMessage publishes a message to a topic in a namespace
func (c *Client) PublishMessage(ctx context.Context, namespace, topic string, payload []byte, metadata map[string]string) (string, error) {
	body := map[string]interface{}{
		"topic":    topic,
		"payload":  string(payload),
		"metadata": metadata,
	}

	result, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/namespaces/%s/publish", namespace), body)
	if err != nil {
		return "", err
	}

	// Extract message ID from response
	if messageID, ok := result["message_id"].(string); ok {
		return messageID, nil
	}

	return "", fmt.Errorf("message_id not found in response")
}

// QueryMailbox queries messages from a mailbox namespace
func (c *Client) QueryMailbox(ctx context.Context, namespace string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	result, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/namespaces/%s/mailbox/query", namespace), filter)
	if err != nil {
		return nil, err
	}

	// Result should be an array
	if arr, ok := result["events"].([]interface{}); ok {
		events := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			if event, ok := item.(map[string]interface{}); ok {
				events[i] = event
			}
		}
		return events, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// GetMailboxEvent retrieves a single event by message ID from a mailbox
func (c *Client) GetMailboxEvent(ctx context.Context, namespace, messageID string) (map[string]interface{}, error) {
	return c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/namespaces/%s/mailbox/events/%s", namespace, messageID), nil)
}

// doRequest performs an HTTP request and decodes JSON response
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (map[string]interface{}, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// newRequest creates a new HTTP request with auth headers
func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token.AccessToken))
	}

	return req, nil
}
