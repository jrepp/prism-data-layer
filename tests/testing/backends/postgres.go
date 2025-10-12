package backends

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// PostgresBackend provides PostgreSQL testcontainer setup and cleanup
type PostgresBackend struct {
	ConnectionString string
	cleanup          func()
}

// SetupPostgres starts a PostgreSQL container and returns connection info
func SetupPostgres(t *testing.T, ctx context.Context) *PostgresBackend {
	t.Helper()

	// Start PostgreSQL container
	postgresContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("prism_test"),
		tcpostgres.WithUsername("prism"),
		tcpostgres.WithPassword("prism_test_password"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	require.NoError(t, err, "Failed to start PostgreSQL container")

	// Get connection string
	connStr, err := postgresContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get PostgreSQL connection string")

	// PostgreSQL container is already ready after Run() completes
	// The tcpostgres.Run() function handles waiting automatically

	return &PostgresBackend{
		ConnectionString: connStr,
		cleanup: func() {
			if err := postgresContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate PostgreSQL container: %v", err)
			}
		},
	}
}

// SetupPostgresWithSchema starts PostgreSQL and creates the keyvalue schema
func SetupPostgresWithSchema(t *testing.T, ctx context.Context) *PostgresBackend {
	t.Helper()

	backend := SetupPostgres(t, ctx)

	// Connect and create schema
	// Note: Schema creation will be done by the PostgreSQL driver's Initialize method
	// This is just the backend setup

	return backend
}

// Cleanup terminates the PostgreSQL container
func (b *PostgresBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

// CreateKeyValueSchema returns the SQL DDL for creating keyvalue tables
// This is used by the PostgreSQL driver during initialization
func CreateKeyValueSchema() string {
	return `
		-- KeyValue storage table
		CREATE TABLE IF NOT EXISTS keyvalue (
			namespace VARCHAR(255) NOT NULL,
			key VARCHAR(1024) NOT NULL,
			value BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (namespace, key)
		);

		-- Index for scanning by namespace
		CREATE INDEX IF NOT EXISTS idx_keyvalue_namespace ON keyvalue(namespace);

		-- Index for prefix scanning
		CREATE INDEX IF NOT EXISTS idx_keyvalue_namespace_key_prefix ON keyvalue(namespace, key text_pattern_ops);
	`
}

// CreateQueueSchema returns the SQL DDL for queue tables
func CreateQueueSchema() string {
	return `
		-- Queue table with visibility timeout support
		CREATE TABLE IF NOT EXISTS queue (
			id BIGSERIAL PRIMARY KEY,
			namespace VARCHAR(255) NOT NULL,
			queue_name VARCHAR(255) NOT NULL,
			payload BYTEA NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			visible_at TIMESTAMP NOT NULL DEFAULT NOW(),
			dequeue_count INT NOT NULL DEFAULT 0,
			max_dequeues INT NOT NULL DEFAULT 3,
			dead_letter BOOLEAN NOT NULL DEFAULT FALSE,
			CONSTRAINT check_max_dequeues CHECK (max_dequeues > 0)
		);

		-- Index for efficient dequeue operations
		CREATE INDEX IF NOT EXISTS idx_queue_dequeue
		ON queue(namespace, queue_name, visible_at)
		WHERE dead_letter = FALSE;

		-- Index for dead letter queue
		CREATE INDEX IF NOT EXISTS idx_queue_dead_letter
		ON queue(namespace, queue_name)
		WHERE dead_letter = TRUE;

		-- Dead letter queue table (separate for clarity)
		CREATE TABLE IF NOT EXISTS queue_dead_letter (
			id BIGSERIAL PRIMARY KEY,
			namespace VARCHAR(255) NOT NULL,
			queue_name VARCHAR(255) NOT NULL,
			payload BYTEA NOT NULL,
			original_id BIGINT,
			failed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			failure_reason TEXT,
			dequeue_count INT NOT NULL
		);
	`
}

// CreateDocumentSchema returns the SQL DDL for document storage (JSONB)
func CreateDocumentSchema() string {
	return `
		-- Document storage using JSONB
		CREATE TABLE IF NOT EXISTS documents (
			namespace VARCHAR(255) NOT NULL,
			collection VARCHAR(255) NOT NULL,
			doc_id VARCHAR(255) NOT NULL,
			document JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (namespace, collection, doc_id)
		);

		-- GIN index for JSONB queries
		CREATE INDEX IF NOT EXISTS idx_documents_jsonb ON documents USING GIN (document);

		-- Index for collection queries
		CREATE INDEX IF NOT EXISTS idx_documents_collection ON documents(namespace, collection);
	`
}

// CreateTimeSeriesSchema returns the SQL DDL for timeseries data (TimescaleDB compatible)
func CreateTimeSeriesSchema() string {
	return `
		-- TimeSeries table (compatible with TimescaleDB if extension is enabled)
		CREATE TABLE IF NOT EXISTS timeseries (
			namespace VARCHAR(255) NOT NULL,
			metric VARCHAR(255) NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			value DOUBLE PRECISION NOT NULL,
			tags JSONB,
			PRIMARY KEY (namespace, metric, timestamp)
		);

		-- Index for time-range queries
		CREATE INDEX IF NOT EXISTS idx_timeseries_time ON timeseries(namespace, metric, timestamp DESC);

		-- GIN index for tag queries
		CREATE INDEX IF NOT EXISTS idx_timeseries_tags ON timeseries USING GIN (tags);

		-- Note: To enable TimescaleDB hypertables, run:
		-- SELECT create_hypertable('timeseries', 'timestamp', if_not_exists => TRUE);
	`
}

// CreateGraphSchema returns the SQL DDL for graph storage
func CreateGraphSchema() string {
	return `
		-- Graph nodes table
		CREATE TABLE IF NOT EXISTS graph_nodes (
			namespace VARCHAR(255) NOT NULL,
			graph_name VARCHAR(255) NOT NULL,
			node_id VARCHAR(255) NOT NULL,
			properties JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (namespace, graph_name, node_id)
		);

		-- Graph edges table
		CREATE TABLE IF NOT EXISTS graph_edges (
			namespace VARCHAR(255) NOT NULL,
			graph_name VARCHAR(255) NOT NULL,
			edge_id VARCHAR(255) NOT NULL,
			from_node VARCHAR(255) NOT NULL,
			to_node VARCHAR(255) NOT NULL,
			properties JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (namespace, graph_name, edge_id)
		);

		-- Indexes for traversal queries
		CREATE INDEX IF NOT EXISTS idx_graph_edges_from ON graph_edges(namespace, graph_name, from_node);
		CREATE INDEX IF NOT EXISTS idx_graph_edges_to ON graph_edges(namespace, graph_name, to_node);

		-- GIN indexes for property queries
		CREATE INDEX IF NOT EXISTS idx_graph_nodes_properties ON graph_nodes USING GIN (properties);
		CREATE INDEX IF NOT EXISTS idx_graph_edges_properties ON graph_edges USING GIN (properties);
	`
}

// CreateAllSchemas creates all supported PostgreSQL schemas
func CreateAllSchemas() string {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
		CreateKeyValueSchema(),
		CreateQueueSchema(),
		CreateDocumentSchema(),
		CreateTimeSeriesSchema(),
		CreateGraphSchema(),
	)
}
