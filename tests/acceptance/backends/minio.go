package backends

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jrepp/prism-data-layer/pkg/drivers/s3"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/jrepp/prism-data-layer/tests/acceptance/framework"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	minioAccessKey = "minioadmin"
	minioSecretKey = "minioadmin"
	testBucket     = "prism-test-bucket"
)

func init() {
	// Register MinIO backend with the acceptance test framework
	framework.MustRegisterBackend(framework.Backend{
		Name:      "MinIO",
		SetupFunc: setupMinIO,

		SupportedPatterns: []framework.Pattern{
			framework.PatternObjectStore,
		},

		Capabilities: framework.Capabilities{
			SupportsObjectStore: true,
			MaxObjectSize:       5 * 1024 * 1024 * 1024, // 5GB
			SupportsTTL:         true,                   // Via object tagging + lifecycle rules
			Custom: map[string]interface{}{
				"S3Compatible": true,
			},
		},
	})
}

// setupMinIO creates a MinIO backend for testing using testcontainers
func setupMinIO(t *testing.T, ctx context.Context) (interface{}, func()) {
	t.Helper()

	// Start MinIO testcontainer
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     minioAccessKey,
			"MINIO_ROOT_PASSWORD": minioSecretKey,
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(60 * time.Second),
	}

	minioContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start MinIO container")

	// Get endpoint
	endpoint, err := minioContainer.Endpoint(ctx, "")
	require.NoError(t, err, "Failed to get MinIO endpoint")

	// Create S3 driver
	driver := s3.New()

	// Configure driver with testcontainer connection
	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "minio-test",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"endpoint":          endpoint,
			"region":            "us-east-1",
			"access_key_id":     minioAccessKey,
			"secret_access_key": minioSecretKey,
			"use_ssl":           false,
			"force_path_style":  true, // Required for MinIO
		},
	}

	// Initialize driver
	err = driver.Initialize(ctx, config)
	require.NoError(t, err, "Failed to initialize MinIO driver")

	// Start driver
	err = driver.Start(ctx)
	require.NoError(t, err, "Failed to start MinIO driver")

	// Wait for driver to be healthy
	err = waitForMinIOHealthy(driver, 10*time.Second)
	require.NoError(t, err, "MinIO driver did not become healthy")

	// Create test bucket
	_, err = driver.GetClient().CreateBucket(ctx, &awss3.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})
	require.NoError(t, err, "Failed to create test bucket")

	// Cleanup function stops driver and terminates container
	cleanup := func() {
		driver.Stop(ctx)
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate MinIO container: %v", err)
		}
	}

	return driver, cleanup
}

// waitForMinIOHealthy polls the MinIO driver's health endpoint
func waitForMinIOHealthy(driver plugin.Plugin, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			health, err := driver.Health(ctx)
			if err == nil && health.Status == plugin.HealthHealthy {
				return nil
			}
		}
	}
}
