package s3

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	testBucket     = "test-bucket"
	testAccessKey  = "minioadmin"
	testSecretKey  = "minioadmin"
)

// setupMinIO starts a MinIO testcontainer and returns the endpoint and cleanup function.
func setupMinIO(t *testing.T, ctx context.Context) (string, func()) {
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     testAccessKey,
			"MINIO_ROOT_PASSWORD": testSecretKey,
		},
		Cmd:        []string{"server", "/data"},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(60 * time.Second),
	}

	minioContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	endpoint, err := minioContainer.Endpoint(ctx, "")
	require.NoError(t, err)

	cleanup := func() {
		if err := minioContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return endpoint, cleanup
}

// createS3Driver creates an S3Driver configured for MinIO.
func createS3Driver(t *testing.T, endpoint string) *S3Driver {
	driver := New()

	config := &plugin.Config{
		Plugin: plugin.PluginConfig{
			Name:    "s3-test",
			Version: "0.1.0",
		},
		Backend: map[string]interface{}{
			"endpoint":          endpoint,
			"region":            "us-east-1",
			"access_key_id":     testAccessKey,
			"secret_access_key": testSecretKey,
			"use_ssl":           false,
			"force_path_style":  true, // Required for MinIO
		},
	}

	ctx := context.Background()
	err := driver.Initialize(ctx, config)
	require.NoError(t, err)

	// Create test bucket
	_, err = driver.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})
	require.NoError(t, err)

	return driver
}

func TestS3Driver_PutAndGet(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	testData := []byte("Hello, S3!")
	testKey := "test-key"

	// Put object
	err := driver.Put(ctx, testBucket, testKey, testData)
	require.NoError(t, err)

	// Get object
	retrieved, err := driver.Get(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.Equal(t, testData, retrieved)
}

func TestS3Driver_Exists(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	testKey := "exists-key"

	// Check non-existent key
	exists, err := driver.Exists(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.False(t, exists)

	// Put object
	err = driver.Put(ctx, testBucket, testKey, []byte("data"))
	require.NoError(t, err)

	// Check existent key
	exists, err = driver.Exists(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestS3Driver_Delete(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	testKey := "delete-key"
	testData := []byte("to be deleted")

	// Put object
	err := driver.Put(ctx, testBucket, testKey, testData)
	require.NoError(t, err)

	// Verify exists
	exists, err := driver.Exists(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete object
	err = driver.Delete(ctx, testBucket, testKey)
	require.NoError(t, err)

	// Verify deleted
	exists, err = driver.Exists(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestS3Driver_GetMetadata(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	testKey := "metadata-key"
	testData := []byte("test data for metadata")

	// Put object
	err := driver.Put(ctx, testBucket, testKey, testData)
	require.NoError(t, err)

	// Get metadata
	metadata, err := driver.GetMetadata(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.Equal(t, int64(len(testData)), metadata.Size)
	assert.NotEmpty(t, metadata.ETag)
	assert.Greater(t, metadata.LastModified, int64(0))
}

func TestS3Driver_SetTTL(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	testKey := "ttl-key"
	testData := []byte("data with TTL")

	// Put object
	err := driver.Put(ctx, testBucket, testKey, testData)
	require.NoError(t, err)

	// Set TTL
	err = driver.SetTTL(ctx, testBucket, testKey, 3600) // 1 hour
	require.NoError(t, err)

	// Verify object still exists (TTL is a tag, actual deletion requires lifecycle rules)
	exists, err := driver.Exists(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestS3Driver_LargeObject(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	// Create 5MB payload
	largeData := make([]byte, 5*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	testKey := "large-object-key"

	// Put large object
	err := driver.Put(ctx, testBucket, testKey, largeData)
	require.NoError(t, err)

	// Get large object
	retrieved, err := driver.Get(ctx, testBucket, testKey)
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(retrieved))
	assert.Equal(t, largeData, retrieved)
}

func TestS3Driver_MultipleObjects(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	// Put multiple objects
	objectCount := 10
	for i := 0; i < objectCount; i++ {
		key := fmt.Sprintf("multi-key-%d", i)
		data := []byte(fmt.Sprintf("data-%d", i))

		err := driver.Put(ctx, testBucket, key, data)
		require.NoError(t, err)
	}

	// Verify all objects exist
	for i := 0; i < objectCount; i++ {
		key := fmt.Sprintf("multi-key-%d", i)

		exists, err := driver.Exists(ctx, testBucket, key)
		require.NoError(t, err)
		assert.True(t, exists)

		data, err := driver.Get(ctx, testBucket, key)
		require.NoError(t, err)
		assert.Equal(t, []byte(fmt.Sprintf("data-%d", i)), data)
	}
}

func TestS3Driver_Health(t *testing.T) {
	ctx := context.Background()
	endpoint, cleanup := setupMinIO(t, ctx)
	defer cleanup()

	driver := createS3Driver(t, endpoint)

	status, err := driver.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, plugin.HealthHealthy, status.Status)
	assert.Contains(t, status.Message, "connected")
}

func TestS3Driver_InterfaceCompliance(t *testing.T) {
	// Compile-time check that S3Driver implements required interfaces
	var _ plugin.Plugin = (*S3Driver)(nil)
	var _ plugin.ObjectStoreInterface = (*S3Driver)(nil)
}
