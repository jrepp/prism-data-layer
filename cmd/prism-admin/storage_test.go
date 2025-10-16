package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageInitialization(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &DatabaseConfig{
		Type: "sqlite",
		Path: dbPath,
	}

	ctx := context.Background()
	storage, err := NewStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created at %s", dbPath)
	}
}

func TestNamespaceOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(tmpDir, "test.db"),
	}

	ctx := context.Background()
	storage, err := NewStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create namespace
	ns := &Namespace{
		Name:        "test-namespace",
		Description: "Test namespace description",
	}

	if err := storage.CreateNamespace(ctx, ns); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	if ns.ID == 0 {
		t.Error("Namespace ID was not set after creation")
	}

	// Get namespace
	retrieved, err := storage.GetNamespace(ctx, "test-namespace")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if retrieved.Name != ns.Name {
		t.Errorf("Expected name %s, got %s", ns.Name, retrieved.Name)
	}

	if retrieved.Description != ns.Description {
		t.Errorf("Expected description %s, got %s", ns.Description, retrieved.Description)
	}

	// List namespaces
	namespaces, err := storage.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	if len(namespaces) != 1 {
		t.Errorf("Expected 1 namespace, got %d", len(namespaces))
	}
}

func TestProxyOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(tmpDir, "test.db"),
	}

	ctx := context.Background()
	storage, err := NewStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	now := time.Now()
	proxy := &Proxy{
		ProxyID:  "proxy-001",
		Address:  "localhost:8980",
		Version:  "0.1.0",
		Status:   "healthy",
		LastSeen: &now,
	}

	// Upsert proxy
	if err := storage.UpsertProxy(ctx, proxy); err != nil {
		t.Fatalf("Failed to upsert proxy: %v", err)
	}

	// Get proxy
	retrieved, err := storage.GetProxy(ctx, "proxy-001")
	if err != nil {
		t.Fatalf("Failed to get proxy: %v", err)
	}

	if retrieved.ProxyID != proxy.ProxyID {
		t.Errorf("Expected proxy_id %s, got %s", proxy.ProxyID, retrieved.ProxyID)
	}

	if retrieved.Status != proxy.Status {
		t.Errorf("Expected status %s, got %s", proxy.Status, retrieved.Status)
	}

	// Update proxy status
	proxy.Status = "unhealthy"
	if err := storage.UpsertProxy(ctx, proxy); err != nil {
		t.Fatalf("Failed to update proxy: %v", err)
	}

	updated, err := storage.GetProxy(ctx, "proxy-001")
	if err != nil {
		t.Fatalf("Failed to get updated proxy: %v", err)
	}

	if updated.Status != "unhealthy" {
		t.Errorf("Expected status unhealthy, got %s", updated.Status)
	}

	// List proxies
	proxies, err := storage.ListProxies(ctx)
	if err != nil {
		t.Fatalf("Failed to list proxies: %v", err)
	}

	if len(proxies) != 1 {
		t.Errorf("Expected 1 proxy, got %d", len(proxies))
	}
}

func TestAuditLogOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(tmpDir, "test.db"),
	}

	ctx := context.Background()
	storage, err := NewStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Log audit entry
	log := &AuditLog{
		Timestamp:    time.Now(),
		User:         "admin",
		Action:       "CREATE_NAMESPACE",
		ResourceType: "namespace",
		ResourceID:   "test-ns",
		Namespace:    "test-ns",
		Method:       "POST",
		Path:         "/api/namespaces",
		StatusCode:   201,
		DurationMs:   15,
		ClientIP:     "127.0.0.1",
		UserAgent:    "prism-admin/0.1.0",
	}

	if err := storage.LogAudit(ctx, log); err != nil {
		t.Fatalf("Failed to log audit entry: %v", err)
	}

	// Query audit logs
	logs, err := storage.QueryAuditLogs(ctx, AuditQueryOptions{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Failed to query audit logs: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 audit log, got %d", len(logs))
	}

	if logs[0].User != "admin" {
		t.Errorf("Expected user admin, got %s", logs[0].User)
	}

	// Query by namespace
	logs, err = storage.QueryAuditLogs(ctx, AuditQueryOptions{
		Namespace: "test-ns",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Failed to query audit logs by namespace: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 audit log for namespace, got %d", len(logs))
	}
}

func TestParseDatabaseURN(t *testing.T) {
	tests := []struct {
		name    string
		urn     string
		wantErr bool
		dbType  string
	}{
		{
			name:   "empty URN uses default",
			urn:    "",
			dbType: "sqlite",
		},
		{
			name:   "sqlite with relative path",
			urn:    "sqlite://test.db",
			dbType: "sqlite",
		},
		{
			name:   "sqlite with absolute path",
			urn:    "sqlite:///tmp/test.db",
			dbType: "sqlite",
		},
		{
			name:   "postgresql URN",
			urn:    "postgresql://user:pass@localhost:5432/prism",
			dbType: "postgresql",
		},
		{
			name:    "unsupported URN",
			urn:     "mysql://localhost",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDatabaseURN(tt.urn)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDatabaseURN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && cfg.Type != tt.dbType {
				t.Errorf("Expected type %s, got %s", tt.dbType, cfg.Type)
			}
		})
	}
}
