package backends

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

// NATSBackend provides NATS testcontainer setup and cleanup
type NATSBackend struct {
	ConnectionString string
	cleanup          func()
}

// SetupNATS starts a NATS container and returns connection info
func SetupNATS(t *testing.T, ctx context.Context) *NATSBackend {
	t.Helper()

	// Start NATS container
	natsContainer, err := tcnats.Run(ctx,
		"nats:2.10-alpine",
	)
	require.NoError(t, err, "Failed to start NATS container")

	// Get connection string
	connStr, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get NATS connection string")

	return &NATSBackend{
		ConnectionString: connStr,
		cleanup: func() {
			if err := natsContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate NATS container: %v", err)
			}
		},
	}
}

// Cleanup terminates the NATS container
func (b *NATSBackend) Cleanup() {
	if b.cleanup != nil {
		b.cleanup()
	}
}
