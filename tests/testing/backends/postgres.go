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
	Host     string
	Port     int
	Database string
	User     string
	Password string
	cleanup  func()
}

// SetupPostgres starts a PostgreSQL container and returns connection info
func SetupPostgres(t *testing.T, ctx context.Context) *PostgresBackend {
	t.Helper()

	const (
		dbName   = "prism_test"
		user     = "prism"
		password = "prism_test_password"
	)

	// Start PostgreSQL container
	postgresContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err, "Failed to start PostgreSQL container")

	// Get connection details
	host, err := postgresContainer.Host(ctx)
	require.NoError(t, err, "Failed to get PostgreSQL host")

	port, err := postgresContainer.MappedPort(ctx, "5432")
	require.NoError(t, err, "Failed to get PostgreSQL port")

	t.Logf("PostgreSQL available at %s:%s (database: %s, user: %s)", host, port.Port(), dbName, user)

	return &PostgresBackend{
		Host:     host,
		Port:     port.Int(),
		Database: dbName,
		User:     user,
		Password: password,
		cleanup: func() {
			if err := postgresContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate PostgreSQL container: %v", err)
			}
		},
	}
}

// ConnectionString returns the PostgreSQL connection string in standard format
func (b *PostgresBackend) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		b.Host, b.Port, b.User, b.Password, b.Database)
}

// DSN returns the PostgreSQL Data Source Name (DSN) format
func (b *PostgresBackend) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		b.User, b.Password, b.Host, b.Port, b.Database)
}

// Cleanup terminates the PostgreSQL container
func (b *PostgresBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}
