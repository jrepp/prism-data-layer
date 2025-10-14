package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// PostgresPlugin implements the Prism backend plugin for PostgreSQL
// Implements interfaces from MEMO-006:
// - keyvalue_basic
// - keyvalue_scan
// - keyvalue_transactional
// - keyvalue_batch
type PostgresPlugin struct {
	pool   *pgxpool.Pool
	config *PostgresConfig
}

// PostgresConfig holds Postgres-specific configuration
type PostgresConfig struct {
	DatabaseURL    string `yaml:"database_url"`
	PoolSize       int    `yaml:"pool_size"`
	VaultEnabled   bool   `yaml:"vault_enabled"`
	VaultPath      string `yaml:"vault_path"`
	DefaultTimeout int    `yaml:"default_timeout_seconds"`
}

// New creates a new PostgreSQL plugin instance
func New() *PostgresPlugin {
	return &PostgresPlugin{}
}

// Name returns the plugin identifier
func (p *PostgresPlugin) Name() string {
	return "postgres"
}

// Version returns the plugin version
func (p *PostgresPlugin) Version() string {
	return "0.1.0"
}

// Initialize prepares the PostgreSQL connection pool and creates schema
func (p *PostgresPlugin) Initialize(ctx context.Context, config *plugin.Config) error {
	slog.Info("initializing postgres plugin", "version", p.Version())

	// Extract backend-specific config
	var pgConfig PostgresConfig

	// Try structured config first
	_ = config.GetBackendConfig(&pgConfig)

	// Try various field names for connection string
	if pgConfig.DatabaseURL == "" {
		if connStr, ok := config.Backend["connection_string"].(string); ok {
			pgConfig.DatabaseURL = connStr
		} else if dbURL, ok := config.Backend["database_url"].(string); ok {
			pgConfig.DatabaseURL = dbURL
		}
	}

	// Apply defaults
	if pgConfig.PoolSize == 0 {
		if poolSize, ok := config.Backend["pool_size"].(int); ok {
			pgConfig.PoolSize = poolSize
		} else {
			pgConfig.PoolSize = 10
		}
	}
	if pgConfig.DefaultTimeout == 0 {
		pgConfig.DefaultTimeout = 30
	}

	p.config = &pgConfig

	// Get database URL
	dbURL := pgConfig.DatabaseURL
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not configured (tried connection_string and database_url fields)")
	}

	// Configure connection pool
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	poolConfig.MaxConns = int32(pgConfig.PoolSize)
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

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

	// Create keyvalue schema if not exists
	if err := p.createSchema(ctx); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	slog.Info("postgres plugin initialized",
		"max_conns", poolConfig.MaxConns,
		"vault_enabled", pgConfig.VaultEnabled)

	return nil
}

// createSchema creates the keyvalue table schema
func (p *PostgresPlugin) createSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS keyvalue (
			namespace VARCHAR(255) NOT NULL,
			key VARCHAR(1024) NOT NULL,
			value BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (namespace, key)
		);

		CREATE INDEX IF NOT EXISTS idx_keyvalue_namespace ON keyvalue(namespace);
		CREATE INDEX IF NOT EXISTS idx_keyvalue_namespace_key_prefix ON keyvalue(namespace, key text_pattern_ops);
	`

	_, err := p.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Start begins serving requests
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
func (p *PostgresPlugin) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	if p.pool == nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthUnhealthy,
			Message: "database pool not initialized",
		}, nil
	}

	// Check database connectivity with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := p.pool.Ping(pingCtx); err != nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthUnhealthy,
			Message: fmt.Sprintf("database ping failed: %v", err),
			Details: map[string]string{
				"error": err.Error(),
			},
		}, nil
	}

	// Check pool stats
	stats := p.pool.Stat()
	if stats.AcquiredConns() >= int32(float64(stats.MaxConns())*0.9) {
		return &plugin.HealthStatus{
			Status:  plugin.HealthDegraded,
			Message: "connection pool near capacity",
			Details: map[string]string{
				"acquired": fmt.Sprintf("%d", stats.AcquiredConns()),
				"max":      fmt.Sprintf("%d", stats.MaxConns()),
				"idle":     fmt.Sprintf("%d", stats.IdleConns()),
			},
		}, nil
	}

	return &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "database healthy",
		Details: map[string]string{
			"acquired": fmt.Sprintf("%d", stats.AcquiredConns()),
			"max":      fmt.Sprintf("%d", stats.MaxConns()),
			"idle":     fmt.Sprintf("%d", stats.IdleConns()),
		},
	}, nil
}

// =============================================================================
// KeyValueBasicInterface Implementation (MEMO-006: keyvalue_basic)
// =============================================================================

// Set stores a key-value pair in PostgreSQL
// Implements keyvalue_basic.Set operation
func (p *PostgresPlugin) Set(key string, value []byte, ttlSeconds int64) error {
	ctx := context.Background()

	// Note: PostgreSQL doesn't natively support TTL like Redis
	// TTL functionality would require a scheduled job or trigger
	// For now, we ignore ttlSeconds parameter (keyvalue_basic doesn't require TTL support)

	_, err := p.pool.Exec(ctx, `
		INSERT INTO keyvalue (namespace, key, value, updated_at)
		VALUES ('default', $1, $2, NOW())
		ON CONFLICT (namespace, key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, value)

	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}

	return nil
}

// Get retrieves a value by key from PostgreSQL
// Implements keyvalue_basic.Get operation
func (p *PostgresPlugin) Get(key string) ([]byte, bool, error) {
	ctx := context.Background()

	var value []byte
	err := p.pool.QueryRow(ctx, `
		SELECT value
		FROM keyvalue
		WHERE namespace = 'default' AND key = $1
	`, key).Scan(&value)

	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get key %s: %w", key, err)
	}

	return value, true, nil
}

// Delete removes a key from PostgreSQL
// Implements keyvalue_basic.Delete operation
func (p *PostgresPlugin) Delete(key string) error {
	ctx := context.Background()

	_, err := p.pool.Exec(ctx, `
		DELETE FROM keyvalue
		WHERE namespace = 'default' AND key = $1
	`, key)

	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	return nil
}

// Exists checks if a key exists in PostgreSQL
// Implements keyvalue_basic.Exists operation
func (p *PostgresPlugin) Exists(key string) (bool, error) {
	ctx := context.Background()

	var exists bool
	err := p.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM keyvalue
			WHERE namespace = 'default' AND key = $1
		)
	`, key).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check existence of key %s: %w", key, err)
	}

	return exists, nil
}

// =============================================================================
// KeyValueScanInterface Implementation (MEMO-006: keyvalue_scan)
// =============================================================================

// Scan retrieves keys matching a prefix
// Implements keyvalue_scan.Scan operation
func (p *PostgresPlugin) Scan(prefix string, limit int) ([]string, error) {
	ctx := context.Background()

	query := `
		SELECT key
		FROM keyvalue
		WHERE namespace = 'default' AND key LIKE $1
		ORDER BY key
	`
	args := []interface{}{prefix + "%"}

	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys with prefix %s: %w", prefix, err)
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return keys, nil
}

// ScanWithValues retrieves key-value pairs matching a prefix
// Implements keyvalue_scan.ScanWithValues operation
func (p *PostgresPlugin) ScanWithValues(prefix string, limit int) (map[string][]byte, error) {
	ctx := context.Background()

	query := `
		SELECT key, value
		FROM keyvalue
		WHERE namespace = 'default' AND key LIKE $1
		ORDER BY key
	`
	args := []interface{}{prefix + "%"}

	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to scan keys with values for prefix %s: %w", prefix, err)
	}
	defer rows.Close()

	results := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// =============================================================================
// InterfaceSupport Implementation
// =============================================================================

// SupportsInterface returns true if PostgreSQL implements the named interface
func (p *PostgresPlugin) SupportsInterface(interfaceName string) bool {
	supported := map[string]bool{
		"Plugin":                    true,
		"KeyValueBasicInterface":    true,
		"KeyValueScanInterface":     true,
		"InterfaceSupport":          true,
	}
	return supported[interfaceName]
}

// ListInterfaces returns all interfaces that PostgreSQL implements
func (p *PostgresPlugin) ListInterfaces() []string {
	return []string{
		"Plugin",
		"KeyValueBasicInterface",
		"KeyValueScanInterface",
		"InterfaceSupport",
	}
}

// Compile-time interface compliance checks
var (
	_ plugin.Plugin                 = (*PostgresPlugin)(nil)
	_ plugin.KeyValueBasicInterface = (*PostgresPlugin)(nil)
	_ plugin.KeyValueScanInterface  = (*PostgresPlugin)(nil)
	_ plugin.InterfaceSupport       = (*PostgresPlugin)(nil)
)
