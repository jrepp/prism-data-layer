package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jrepp/prism-data-layer/pkg/plugin"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// Driver implements TableWriterInterface and TableReaderInterface for SQLite storage
type Driver struct {
	mu       sync.RWMutex
	db       *sql.DB
	dbPath   string
	table    string
	config   *plugin.Config
	started  bool
	stopChan chan struct{}
}

// New creates a new SQLite driver instance
func New() plugin.Plugin {
	return &Driver{
		stopChan: make(chan struct{}),
	}
}

// Initialize implements plugin.Plugin
func (d *Driver) Initialize(ctx context.Context, config *plugin.Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config = config

	// Extract configuration
	dbPath, ok := config.Backend["database_path"].(string)
	if !ok {
		dbPath = "./mailbox.db" // Default path
	}
	d.dbPath = dbPath

	tableName, ok := config.Backend["table_name"].(string)
	if !ok {
		tableName = "mailbox" // Default table name
	}
	d.table = tableName

	// Open database
	db, err := sql.Open("sqlite", d.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	d.db = db

	// Configure SQLite for optimal performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",           // Write-Ahead Logging
		"PRAGMA synchronous=NORMAL",         // Balance safety and performance
		"PRAGMA cache_size=10000",           // 10k pages cache
		"PRAGMA temp_store=MEMORY",          // In-memory temp tables
		"PRAGMA mmap_size=30000000000",      // 30GB memory-mapped I/O
		"PRAGMA foreign_keys=ON",            // Enable foreign keys
		"PRAGMA busy_timeout=5000",          // 5s timeout for locks
	}

	for _, pragma := range pragmas {
		if _, err := d.db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}

	// Create table schema
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			-- Indexed headers (extracted from metadata)
			message_id TEXT NOT NULL UNIQUE,
			timestamp INTEGER NOT NULL,
			topic TEXT NOT NULL,
			content_type TEXT,
			schema_id TEXT,
			encryption TEXT,
			correlation_id TEXT,
			principal TEXT,
			namespace TEXT,

			-- Custom headers (JSON for flexibility)
			custom_headers TEXT,

			-- Body (opaque blob, may be encrypted)
			body BLOB NOT NULL,

			-- Metadata
			created_at INTEGER NOT NULL
		)
	`, d.table)

	if _, err := d.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Create indexes for common queries
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_timestamp ON %s(timestamp)", d.table, d.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_topic ON %s(topic)", d.table, d.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_principal ON %s(principal)", d.table, d.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_correlation_id ON %s(correlation_id)", d.table, d.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_namespace ON %s(namespace)", d.table, d.table),
	}

	for _, indexSQL := range indexes {
		if _, err := d.db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// Start implements plugin.Plugin
func (d *Driver) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return fmt.Errorf("driver already started")
	}

	d.started = true

	// Start retention cleanup goroutine if configured
	retentionDays, ok := d.config.Backend["retention_days"].(float64)
	if ok && retentionDays > 0 {
		go d.retentionCleanupLoop(int(retentionDays))
	}

	return nil
}

// Drain prepares the plugin for shutdown
func (d *Driver) Drain(ctx context.Context, timeoutSeconds int32, reason string) (*plugin.DrainMetrics, error) {
	// SQLite driver waits for in-flight queries to complete
	// The database connection pool handles this automatically
	return &plugin.DrainMetrics{
		DrainedOperations: 0,
		AbortedOperations: 0,
	}, nil
}

// Stop implements plugin.Plugin
func (d *Driver) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started {
		return nil
	}

	close(d.stopChan)
	d.started = false

	if d.db != nil {
		return d.db.Close()
	}

	return nil
}

// Health implements plugin.Plugin
func (d *Driver) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "sqlite driver healthy",
		Details: map[string]string{
			"database": d.dbPath,
			"table":    d.table,
		},
	}

	// Verify database connectivity
	if d.db != nil {
		if err := d.db.PingContext(ctx); err != nil {
			status.Status = plugin.HealthDegraded
			status.Message = fmt.Sprintf("database ping failed: %v", err)
			return status, nil
		}
	} else {
		status.Status = plugin.HealthDegraded
		status.Message = "database not initialized"
	}

	return status, nil
}

// Name implements plugin.Plugin
func (d *Driver) Name() string {
	return "sqlite"
}

// Version implements plugin.Plugin
func (d *Driver) Version() string {
	return "0.1.0"
}

// GetInterfaceDeclarations implements plugin.Plugin
func (d *Driver) GetInterfaceDeclarations() []*plugin.InterfaceDeclaration {
	return []*plugin.InterfaceDeclaration{
		{
			Name:    "TableWriter",
			Version: "1.0",
		},
		{
			Name:    "TableReader",
			Version: "1.0",
		},
	}
}

// WriteEvent implements TableWriterInterface
func (d *Driver) WriteEvent(ctx context.Context, event *plugin.MailboxEvent) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.started {
		return fmt.Errorf("driver not started")
	}

	// Serialize custom headers to JSON
	customHeadersJSON, err := json.Marshal(event.CustomHeaders)
	if err != nil {
		return fmt.Errorf("failed to marshal custom headers: %w", err)
	}

	insertSQL := fmt.Sprintf(`
		INSERT INTO %s (
			message_id, timestamp, topic, content_type, schema_id,
			encryption, correlation_id, principal, namespace,
			custom_headers, body, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.table)

	_, err = d.db.ExecContext(ctx, insertSQL,
		event.MessageID,
		event.Timestamp,
		event.Topic,
		event.ContentType,
		event.SchemaID,
		event.Encryption,
		event.CorrelationID,
		event.Principal,
		event.Namespace,
		string(customHeadersJSON),
		event.Body,
		time.Now().UnixMilli(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// QueryEvents implements TableWriterInterface
func (d *Driver) QueryEvents(ctx context.Context, filter *plugin.EventFilter) ([]*plugin.MailboxEvent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.started {
		return nil, fmt.Errorf("driver not started")
	}

	// Build query with filters
	query := fmt.Sprintf("SELECT message_id, timestamp, topic, content_type, schema_id, encryption, correlation_id, principal, namespace, custom_headers, body FROM %s WHERE 1=1", d.table)
	args := []interface{}{}

	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}

	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}

	if len(filter.Topics) > 0 {
		query += " AND topic IN ("
		for i := range filter.Topics {
			if i > 0 {
				query += ", "
			}
			query += "?"
			args = append(args, filter.Topics[i])
		}
		query += ")"
	}

	if len(filter.Principals) > 0 {
		query += " AND principal IN ("
		for i := range filter.Principals {
			if i > 0 {
				query += ", "
			}
			query += "?"
			args = append(args, filter.Principals[i])
		}
		query += ")"
	}

	if filter.CorrelationID != nil {
		query += " AND correlation_id = ?"
		args = append(args, *filter.CorrelationID)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*plugin.MailboxEvent
	for rows.Next() {
		var event plugin.MailboxEvent
		var customHeadersJSON string

		err := rows.Scan(
			&event.MessageID,
			&event.Timestamp,
			&event.Topic,
			&event.ContentType,
			&event.SchemaID,
			&event.Encryption,
			&event.CorrelationID,
			&event.Principal,
			&event.Namespace,
			&customHeadersJSON,
			&event.Body,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Deserialize custom headers
		if customHeadersJSON != "" {
			if err := json.Unmarshal([]byte(customHeadersJSON), &event.CustomHeaders); err != nil {
				return nil, fmt.Errorf("failed to unmarshal custom headers: %w", err)
			}
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return events, nil
}

// GetEvent implements TableReaderInterface
func (d *Driver) GetEvent(ctx context.Context, messageID string) (*plugin.MailboxEvent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.started {
		return nil, fmt.Errorf("driver not started")
	}

	query := fmt.Sprintf("SELECT message_id, timestamp, topic, content_type, schema_id, encryption, correlation_id, principal, namespace, custom_headers, body FROM %s WHERE message_id = ?", d.table)

	var event plugin.MailboxEvent
	var customHeadersJSON string

	err := d.db.QueryRowContext(ctx, query, messageID).Scan(
		&event.MessageID,
		&event.Timestamp,
		&event.Topic,
		&event.ContentType,
		&event.SchemaID,
		&event.Encryption,
		&event.CorrelationID,
		&event.Principal,
		&event.Namespace,
		&customHeadersJSON,
		&event.Body,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", messageID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query event: %w", err)
	}

	// Deserialize custom headers
	if customHeadersJSON != "" {
		if err := json.Unmarshal([]byte(customHeadersJSON), &event.CustomHeaders); err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom headers: %w", err)
		}
	}

	return &event, nil
}

// DeleteOldEvents implements TableWriterInterface
func (d *Driver) DeleteOldEvents(ctx context.Context, olderThan int64) (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.started {
		return 0, fmt.Errorf("driver not started")
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", d.table)
	result, err := d.db.ExecContext(ctx, deleteSQL, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old events: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// GetTableStats implements TableWriterInterface
func (d *Driver) GetTableStats(ctx context.Context) (*plugin.TableStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.started {
		return nil, fmt.Errorf("driver not started")
	}

	stats := &plugin.TableStats{}

	// Get total events
	countSQL := fmt.Sprintf("SELECT COUNT(*), MIN(timestamp), MAX(timestamp) FROM %s", d.table)
	err := d.db.QueryRowContext(ctx, countSQL).Scan(&stats.TotalEvents, &stats.OldestEvent, &stats.NewestEvent)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get event count: %w", err)
	}

	// Get total size (approximate)
	sizeSQL := fmt.Sprintf("SELECT SUM(LENGTH(body)) FROM %s", d.table)
	err = d.db.QueryRowContext(ctx, sizeSQL).Scan(&stats.TotalSizeBytes)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total size: %w", err)
	}

	return stats, nil
}

// retentionCleanupLoop runs periodic cleanup of old events
func (d *Driver) retentionCleanupLoop(retentionDays int) {
	ticker := time.NewTicker(24 * time.Hour) // Run daily
	defer ticker.Stop()

	for {
		select {
		case <-d.stopChan:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			deleted, err := d.DeleteOldEvents(ctx, cutoff)
			cancel()

			if err != nil {
				// Log error (in production, use structured logging)
				fmt.Printf("retention cleanup error: %v\n", err)
			} else if deleted > 0 {
				fmt.Printf("retention cleanup: deleted %d events older than %d days\n", deleted, retentionDays)
			}
		}
	}
}
