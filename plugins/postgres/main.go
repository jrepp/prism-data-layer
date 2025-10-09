package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jrepp/prism-data-layer/plugins/core"
)

const (
	version = "0.1.0"
)

// PostgresPlugin implements the Prism backend plugin for PostgreSQL
// Based on RFC-011 (Vault-managed credentials) and ADR-005 (KeyValue backend)
type PostgresPlugin struct {
	pool   *pgxpool.Pool
	config *PostgresConfig
}

// PostgresConfig holds Postgres-specific configuration
// Follows ADR-025 (environment-based configuration)
type PostgresConfig struct {
	DatabaseURL  string
	PoolSize     int
	VaultEnabled bool
	VaultPath    string
}

// Name returns the plugin identifier
func (p *PostgresPlugin) Name() string {
	return "postgres"
}

// Version returns the plugin version
func (p *PostgresPlugin) Version() string {
	return version
}

// Initialize prepares the PostgreSQL connection pool
// Based on RFC-011: Supports both direct connection and Vault-managed credentials
func (p *PostgresPlugin) Initialize(ctx context.Context, config *core.Config) error {
	slog.Info("initializing postgres plugin", "version", version)

	// Extract backend-specific config
	var pgConfig PostgresConfig
	if err := config.GetBackendConfig(&pgConfig); err != nil {
		return fmt.Errorf("failed to parse postgres config: %w", err)
	}
	p.config = &pgConfig

	// Get database URL from environment or config
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = pgConfig.DatabaseURL
	}
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not configured")
	}

	// If Vault enabled, fetch credentials (RFC-011 pattern)
	if pgConfig.VaultEnabled {
		slog.Info("vault-managed credentials enabled", "path", pgConfig.VaultPath)
		// TODO: Implement Vault credential fetching
		// creds, err := p.fetchVaultCredentials(ctx, pgConfig.VaultPath)
		// if err != nil {
		//     return fmt.Errorf("failed to fetch vault credentials: %w", err)
		// }
		// dbURL = p.buildConnectionString(creds)
	}

	// Configure connection pool (ADR-005 pattern)
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	if pgConfig.PoolSize > 0 {
		poolConfig.MaxConns = int32(pgConfig.PoolSize)
	} else {
		poolConfig.MaxConns = 10 // Default
	}

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.pool = pool

	slog.Info("postgres plugin initialized",
		"max_conns", poolConfig.MaxConns,
		"vault_enabled", pgConfig.VaultEnabled)

	return nil
}

// Start begins serving requests
// ADR-025: Plugins should be long-running processes
func (p *PostgresPlugin) Start(ctx context.Context) error {
	slog.Info("postgres plugin started")

	// Keep running until context is cancelled
	<-ctx.Done()

	slog.Info("postgres plugin stopping")
	return nil
}

// Stop gracefully shuts down the plugin
func (p *PostgresPlugin) Stop(ctx context.Context) error {
	slog.Info("stopping postgres plugin")

	if p.pool != nil {
		p.pool.Close()
		slog.Info("closed database connection pool")
	}

	return nil
}

// Health reports the plugin health status
// ADR-025: Required health check interface
func (p *PostgresPlugin) Health(ctx context.Context) (*core.HealthStatus, error) {
	if p.pool == nil {
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: "database pool not initialized",
		}, nil
	}

	// Check database connectivity with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := p.pool.Ping(pingCtx); err != nil {
		return &core.HealthStatus{
			Status:  core.HealthUnhealthy,
			Message: fmt.Sprintf("database ping failed: %v", err),
			Details: map[string]string{
				"error": err.Error(),
			},
		}, nil
	}

	// Check pool stats
	stats := p.pool.Stat()
	if stats.AcquiredConns() >= int32(float64(stats.MaxConns())*0.9) {
		return &core.HealthStatus{
			Status:  core.HealthDegraded,
			Message: "connection pool near capacity",
			Details: map[string]string{
				"acquired": fmt.Sprintf("%d", stats.AcquiredConns()),
				"max":      fmt.Sprintf("%d", stats.MaxConns()),
				"idle":     fmt.Sprintf("%d", stats.IdleConns()),
			},
		}, nil
	}

	return &core.HealthStatus{
		Status:  core.HealthHealthy,
		Message: "database healthy",
		Details: map[string]string{
			"acquired": fmt.Sprintf("%d", stats.AcquiredConns()),
			"max":      fmt.Sprintf("%d", stats.MaxConns()),
			"idle":     fmt.Sprintf("%d", stats.IdleConns()),
		},
	}, nil
}

// KeyValue Operations (ADR-005: KeyValue backend interface)
// These would be exposed via gRPC service in production

// Put stores key-value items in PostgreSQL
func (p *PostgresPlugin) Put(ctx context.Context, namespace, id string, items map[string][]byte) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert or update items
	for key, value := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO keyvalue (namespace, id, key, value, updated_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (namespace, id, key)
			DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`, namespace, id, key, value)
		if err != nil {
			return fmt.Errorf("failed to put item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Debug("put items", "namespace", namespace, "id", id, "count", len(items))
	return nil
}

// Get retrieves key-value items from PostgreSQL
func (p *PostgresPlugin) Get(ctx context.Context, namespace, id string, keys []string) (map[string][]byte, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT key, value
		FROM keyvalue
		WHERE namespace = $1 AND id = $2 AND key = ANY($3)
	`, namespace, id, keys)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}
	defer rows.Close()

	items := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		items[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	slog.Debug("got items", "namespace", namespace, "id", id, "requested", len(keys), "found", len(items))
	return items, nil
}

// Delete removes key-value items from PostgreSQL
func (p *PostgresPlugin) Delete(ctx context.Context, namespace, id string, keys []string) error {
	result, err := p.pool.Exec(ctx, `
		DELETE FROM keyvalue
		WHERE namespace = $1 AND id = $2 AND key = ANY($3)
	`, namespace, id, keys)
	if err != nil {
		return fmt.Errorf("failed to delete items: %w", err)
	}

	slog.Debug("deleted items", "namespace", namespace, "id", id, "count", result.RowsAffected())
	return nil
}

func main() {
	// Get config path from environment
	configPath := os.Getenv("PRISM_PLUGIN_CONFIG")
	if configPath == "" {
		configPath = "/etc/prism/plugin.yaml"
	}

	// Bootstrap the plugin using core package
	// ADR-025: Standard plugin lifecycle management
	plugin := &PostgresPlugin{}
	if err := core.Bootstrap(plugin, configPath); err != nil {
		slog.Error("plugin failed", "error", err)
		os.Exit(1)
	}
}
