package cmd

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Type string // "sqlite" or "postgresql"
	Path string // For SQLite
	URN  string // For PostgreSQL
}

// Storage provides database operations for prism-admin
type Storage struct {
	db   *sql.DB
	cfg  *DatabaseConfig
}

// Models

type Namespace struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    json.RawMessage
}

type Proxy struct {
	ID        int64
	ProxyID   string
	Address   string
	Version   string
	Status    string // "healthy", "unhealthy", "unknown"
	LastSeen  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  json.RawMessage
}

type Pattern struct {
	ID          int64
	PatternID   string
	PatternType string
	ProxyID     string
	Namespace   string
	Status      string // "active", "stopped", "error"
	Config      json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Launcher struct {
	ID             int64
	LauncherID     string
	Address        string
	Region         string
	Version        string
	Status         string // "healthy", "unhealthy", "unknown"
	MaxProcesses   int32
	AvailableSlots int32
	Capabilities   json.RawMessage // JSON array
	LastSeen       *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Metadata       json.RawMessage
}

type AuditLog struct {
	ID           int64
	Timestamp    time.Time
	User         string
	Action       string
	ResourceType string
	ResourceID   string
	Namespace    string
	Method       string
	Path         string
	StatusCode   int
	RequestBody  json.RawMessage
	ResponseBody json.RawMessage
	Error        string
	DurationMs   int64
	ClientIP     string
	UserAgent    string
}

// ParseDatabaseURN parses a database URN string
func ParseDatabaseURN(urn string) (*DatabaseConfig, error) {
	if urn == "" {
		return &DatabaseConfig{
			Type: "sqlite",
			Path: defaultDatabasePath(),
		}, nil
	}

	// Parse sqlite:///path/to/db or sqlite://path/to/db
	if strings.HasPrefix(urn, "sqlite://") {
		path := strings.TrimPrefix(urn, "sqlite://")
		// Handle sqlite:///absolute/path (three slashes)
		if strings.HasPrefix(path, "/") {
			return &DatabaseConfig{Type: "sqlite", Path: path}, nil
		}
		// Handle sqlite://relative/path (two slashes)
		return &DatabaseConfig{Type: "sqlite", Path: path}, nil
	}

	// Parse postgresql://... or postgres://...
	if strings.HasPrefix(urn, "postgres") {
		return &DatabaseConfig{Type: "postgresql", URN: urn}, nil
	}

	return nil, fmt.Errorf("unsupported database URN: %s (supported: sqlite://, postgresql://)", urn)
}

// defaultDatabasePath returns the default SQLite database path
func defaultDatabasePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./prism-admin.db"
	}
	prismDir := filepath.Join(homeDir, ".prism")
	if err := os.MkdirAll(prismDir, 0700); err != nil {
		return "./prism-admin.db"
	}
	return filepath.Join(prismDir, "admin.db")
}

// NewStorage creates a new Storage instance
func NewStorage(ctx context.Context, cfg *DatabaseConfig) (*Storage, error) {
	var db *sql.DB
	var err error

	switch cfg.Type {
	case "sqlite":
		// Ensure directory exists
		dir := filepath.Dir(cfg.Path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		db, err = sql.Open("sqlite", cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}

		// Configure SQLite for better performance
		_, err = db.Exec(`
			PRAGMA journal_mode=WAL;
			PRAGMA synchronous=NORMAL;
			PRAGMA foreign_keys=ON;
			PRAGMA busy_timeout=5000;
		`)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to configure sqlite: %w", err)
		}

	case "postgresql":
		return nil, fmt.Errorf("postgresql support not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	s := &Storage{
		db:  db,
		cfg: cfg,
	}

	// Run migrations
	if err := s.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return s, nil
}

// runMigrations applies database migrations
func (s *Storage) runMigrations() error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	dbDriver, err := sqlite3.WithInstance(s.db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite3", dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Namespace operations

func (s *Storage) CreateNamespace(ctx context.Context, ns *Namespace) error {
	metadataJSON, _ := json.Marshal(ns.Metadata)

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO namespaces (name, description, metadata)
		VALUES (?, ?, ?)
	`, ns.Name, ns.Description, string(metadataJSON))

	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	id, _ := result.LastInsertId()
	ns.ID = id
	return nil
}

func (s *Storage) GetNamespace(ctx context.Context, name string) (*Namespace, error) {
	var ns Namespace
	var metadataStr string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, created_at, updated_at, metadata
		FROM namespaces WHERE name = ?
	`, name).Scan(&ns.ID, &ns.Name, &ns.Description, &ns.CreatedAt, &ns.UpdatedAt, &metadataStr)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("namespace not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	if metadataStr != "" {
		ns.Metadata = json.RawMessage(metadataStr)
	}

	return &ns, nil
}

func (s *Storage) ListNamespaces(ctx context.Context) ([]*Namespace, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, created_at, updated_at, metadata
		FROM namespaces
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	defer rows.Close()

	var namespaces []*Namespace
	for rows.Next() {
		var ns Namespace
		var metadataStr string

		if err := rows.Scan(&ns.ID, &ns.Name, &ns.Description, &ns.CreatedAt, &ns.UpdatedAt, &metadataStr); err != nil {
			return nil, fmt.Errorf("failed to scan namespace: %w", err)
		}

		if metadataStr != "" {
			ns.Metadata = json.RawMessage(metadataStr)
		}

		namespaces = append(namespaces, &ns)
	}

	return namespaces, rows.Err()
}

// Proxy operations

func (s *Storage) UpsertProxy(ctx context.Context, p *Proxy) error {
	metadataJSON, _ := json.Marshal(p.Metadata)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proxies (proxy_id, address, version, status, last_seen, metadata, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(proxy_id) DO UPDATE SET
			address = excluded.address,
			version = excluded.version,
			status = excluded.status,
			last_seen = excluded.last_seen,
			metadata = excluded.metadata,
			updated_at = CURRENT_TIMESTAMP
	`, p.ProxyID, p.Address, p.Version, p.Status, p.LastSeen, string(metadataJSON))

	return err
}

func (s *Storage) GetProxy(ctx context.Context, proxyID string) (*Proxy, error) {
	var p Proxy
	var metadataStr string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, proxy_id, address, version, status, last_seen, created_at, updated_at, metadata
		FROM proxies WHERE proxy_id = ?
	`, proxyID).Scan(&p.ID, &p.ProxyID, &p.Address, &p.Version, &p.Status, &p.LastSeen,
		&p.CreatedAt, &p.UpdatedAt, &metadataStr)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("proxy not found: %s", proxyID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}

	if metadataStr != "" {
		p.Metadata = json.RawMessage(metadataStr)
	}

	return &p, nil
}

func (s *Storage) ListProxies(ctx context.Context) ([]*Proxy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, proxy_id, address, version, status, last_seen, created_at, updated_at, metadata
		FROM proxies
		ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list proxies: %w", err)
	}
	defer rows.Close()

	var proxies []*Proxy
	for rows.Next() {
		var p Proxy
		var metadataStr string

		if err := rows.Scan(&p.ID, &p.ProxyID, &p.Address, &p.Version, &p.Status, &p.LastSeen,
			&p.CreatedAt, &p.UpdatedAt, &metadataStr); err != nil {
			return nil, fmt.Errorf("failed to scan proxy: %w", err)
		}

		if metadataStr != "" {
			p.Metadata = json.RawMessage(metadataStr)
		}

		proxies = append(proxies, &p)
	}

	return proxies, rows.Err()
}

// Launcher operations

func (s *Storage) UpsertLauncher(ctx context.Context, l *Launcher) error {
	metadataJSON, _ := json.Marshal(l.Metadata)
	capabilitiesJSON, _ := json.Marshal(l.Capabilities)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO launchers (launcher_id, address, region, version, status, max_processes, available_slots, capabilities, last_seen, metadata, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(launcher_id) DO UPDATE SET
			address = excluded.address,
			region = excluded.region,
			version = excluded.version,
			status = excluded.status,
			max_processes = excluded.max_processes,
			available_slots = excluded.available_slots,
			capabilities = excluded.capabilities,
			last_seen = excluded.last_seen,
			metadata = excluded.metadata,
			updated_at = CURRENT_TIMESTAMP
	`, l.LauncherID, l.Address, l.Region, l.Version, l.Status, l.MaxProcesses, l.AvailableSlots, string(capabilitiesJSON), l.LastSeen, string(metadataJSON))

	return err
}

func (s *Storage) GetLauncher(ctx context.Context, launcherID string) (*Launcher, error) {
	var l Launcher
	var metadataStr, capabilitiesStr string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, launcher_id, address, region, version, status, max_processes, available_slots, capabilities, last_seen, created_at, updated_at, metadata
		FROM launchers WHERE launcher_id = ?
	`, launcherID).Scan(&l.ID, &l.LauncherID, &l.Address, &l.Region, &l.Version, &l.Status, &l.MaxProcesses, &l.AvailableSlots,
		&capabilitiesStr, &l.LastSeen, &l.CreatedAt, &l.UpdatedAt, &metadataStr)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("launcher not found: %s", launcherID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get launcher: %w", err)
	}

	if metadataStr != "" {
		l.Metadata = json.RawMessage(metadataStr)
	}
	if capabilitiesStr != "" {
		l.Capabilities = json.RawMessage(capabilitiesStr)
	}

	return &l, nil
}

func (s *Storage) ListLaunchers(ctx context.Context) ([]*Launcher, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, launcher_id, address, region, version, status, max_processes, available_slots, capabilities, last_seen, created_at, updated_at, metadata
		FROM launchers
		ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list launchers: %w", err)
	}
	defer rows.Close()

	var launchers []*Launcher
	for rows.Next() {
		var l Launcher
		var metadataStr, capabilitiesStr string

		if err := rows.Scan(&l.ID, &l.LauncherID, &l.Address, &l.Region, &l.Version, &l.Status, &l.MaxProcesses, &l.AvailableSlots,
			&capabilitiesStr, &l.LastSeen, &l.CreatedAt, &l.UpdatedAt, &metadataStr); err != nil {
			return nil, fmt.Errorf("failed to scan launcher: %w", err)
		}

		if metadataStr != "" {
			l.Metadata = json.RawMessage(metadataStr)
		}
		if capabilitiesStr != "" {
			l.Capabilities = json.RawMessage(capabilitiesStr)
		}

		launchers = append(launchers, &l)
	}

	return launchers, rows.Err()
}

// Pattern operations

func (s *Storage) CreatePattern(ctx context.Context, p *Pattern) error {
	configJSON, _ := json.Marshal(p.Config)

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO patterns (pattern_id, pattern_type, proxy_id, namespace, status, config)
		VALUES (?, ?, ?, ?, ?, ?)
	`, p.PatternID, p.PatternType, p.ProxyID, p.Namespace, p.Status, string(configJSON))

	if err != nil {
		return fmt.Errorf("failed to create pattern: %w", err)
	}

	id, _ := result.LastInsertId()
	p.ID = id
	return nil
}

func (s *Storage) ListPatternsByNamespace(ctx context.Context, namespace string) ([]*Pattern, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, pattern_id, pattern_type, proxy_id, namespace, status, config, created_at, updated_at
		FROM patterns
		WHERE namespace = ?
		ORDER BY created_at DESC
	`, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list patterns: %w", err)
	}
	defer rows.Close()

	var patterns []*Pattern
	for rows.Next() {
		var p Pattern
		var configStr string

		if err := rows.Scan(&p.ID, &p.PatternID, &p.PatternType, &p.ProxyID, &p.Namespace,
			&p.Status, &configStr, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan pattern: %w", err)
		}

		if configStr != "" {
			p.Config = json.RawMessage(configStr)
		}

		patterns = append(patterns, &p)
	}

	return patterns, rows.Err()
}

// Audit log operations

func (s *Storage) LogAudit(ctx context.Context, log *AuditLog) error {
	requestJSON, _ := json.Marshal(log.RequestBody)
	responseJSON, _ := json.Marshal(log.ResponseBody)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			timestamp, user, action, resource_type, resource_id, namespace,
			method, path, status_code, request_body, response_body, error,
			duration_ms, client_ip, user_agent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.Timestamp, log.User, log.Action, log.ResourceType, log.ResourceID, log.Namespace,
		log.Method, log.Path, log.StatusCode, string(requestJSON), string(responseJSON), log.Error,
		log.DurationMs, log.ClientIP, log.UserAgent)

	return err
}

func (s *Storage) QueryAuditLogs(ctx context.Context, opts AuditQueryOptions) ([]*AuditLog, error) {
	query := `
		SELECT id, timestamp, user, action, resource_type, resource_id, namespace,
			method, path, status_code, request_body, response_body, error,
			duration_ms, client_ip, user_agent
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, opts.Namespace)
	}
	if opts.User != "" {
		query += " AND user = ?"
		args = append(args, opts.User)
	}
	if !opts.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, opts.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		var log AuditLog
		var requestStr, responseStr string

		if err := rows.Scan(&log.ID, &log.Timestamp, &log.User, &log.Action, &log.ResourceType,
			&log.ResourceID, &log.Namespace, &log.Method, &log.Path, &log.StatusCode,
			&requestStr, &responseStr, &log.Error, &log.DurationMs, &log.ClientIP, &log.UserAgent); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		if requestStr != "" {
			log.RequestBody = json.RawMessage(requestStr)
		}
		if responseStr != "" {
			log.ResponseBody = json.RawMessage(responseStr)
		}

		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// AuditQueryOptions specifies filters for querying audit logs
type AuditQueryOptions struct {
	Namespace string
	User      string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}
