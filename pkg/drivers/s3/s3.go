package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
)

// S3Driver implements an S3-compatible object storage plugin
// Compatible with AWS S3, MinIO, and other S3-compatible services
type S3Driver struct {
	name    string
	version string
	client  *s3.Client
	config  *Config
	stopCh  chan struct{}
}

// Config holds S3-specific configuration
type Config struct {
	Endpoint        string `yaml:"endpoint"`         // S3 endpoint (e.g., s3.amazonaws.com or localhost:9000 for MinIO)
	Region          string `yaml:"region"`           // AWS region
	AccessKeyID     string `yaml:"access_key_id"`    // Access key ID
	SecretAccessKey string `yaml:"secret_access_key"` // Secret access key
	UseSSL          bool   `yaml:"use_ssl"`          // Use HTTPS (default: true)
	ForcePathStyle  bool   `yaml:"force_path_style"` // Use path-style addressing (required for MinIO)
}

// New creates a new S3Driver plugin
func New() *S3Driver {
	return &S3Driver{
		name:    "s3",
		version: "0.1.0",
		stopCh:  make(chan struct{}),
	}
}

// Name returns the plugin name
func (d *S3Driver) Name() string {
	return d.name
}

// Version returns the plugin version
func (d *S3Driver) Version() string {
	return d.version
}

// Initialize prepares the plugin with configuration
func (d *S3Driver) Initialize(ctx context.Context, cfg *plugin.Config) error {
	// Extract backend-specific config
	var backendConfig Config
	if err := cfg.GetBackendConfig(&backendConfig); err != nil {
		return fmt.Errorf("failed to parse backend config: %w", err)
	}

	// Apply defaults
	if backendConfig.Region == "" {
		backendConfig.Region = "us-east-1"
	}
	if backendConfig.Endpoint == "" {
		backendConfig.Endpoint = "s3.amazonaws.com"
	}

	d.config = &backendConfig

	// Create AWS config
	var opts []func(*config.LoadOptions) error

	// Add credentials if provided
	if backendConfig.AccessKeyID != "" && backendConfig.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				backendConfig.AccessKeyID,
				backendConfig.SecretAccessKey,
				"",
			),
		))
	}

	// Set region
	opts = append(opts, config.WithRegion(backendConfig.Region))

	awsConfig, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	d.client = s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		if backendConfig.Endpoint != "" && backendConfig.Endpoint != "s3.amazonaws.com" {
			scheme := "https"
			if !backendConfig.UseSSL {
				scheme = "http"
			}
			o.BaseEndpoint = aws.String(fmt.Sprintf("%s://%s", scheme, backendConfig.Endpoint))
		}
		o.UsePathStyle = backendConfig.ForcePathStyle
	})

	slog.Info("s3 driver initialized",
		"endpoint", backendConfig.Endpoint,
		"region", backendConfig.Region,
		"use_ssl", backendConfig.UseSSL,
		"force_path_style", backendConfig.ForcePathStyle)

	return nil
}

// Start begins serving requests
func (d *S3Driver) Start(ctx context.Context) error {
	// Block until context is cancelled
	<-ctx.Done()
	close(d.stopCh)
	return nil
}

// Stop gracefully shuts down the plugin
func (d *S3Driver) Stop(ctx context.Context) error {
	slog.Info("s3 driver stopped")
	return nil
}

// Health returns the plugin health status
func (d *S3Driver) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	// Try to list buckets to verify connectivity
	_, err := d.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return &plugin.HealthStatus{
			Status:  plugin.HealthUnhealthy,
			Message: fmt.Sprintf("failed to connect to S3: %v", err),
			Details: map[string]string{
				"endpoint": d.config.Endpoint,
				"region":   d.config.Region,
			},
		}, nil
	}

	return &plugin.HealthStatus{
		Status:  plugin.HealthHealthy,
		Message: "connected to S3",
		Details: map[string]string{
			"endpoint": d.config.Endpoint,
			"region":   d.config.Region,
		},
	}, nil
}

// Put stores an object
func (d *S3Driver) Put(ctx context.Context, bucket, key string, data []byte) error {
	_, err := d.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}

	slog.Debug("object stored",
		"bucket", bucket,
		"key", key,
		"size", len(data))

	return nil
}

// Get retrieves an object
func (d *S3Driver) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	result, err := d.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	slog.Debug("object retrieved",
		"bucket", bucket,
		"key", key,
		"size", len(data))

	return data, nil
}

// Delete removes an object
func (d *S3Driver) Delete(ctx context.Context, bucket, key string) error {
	_, err := d.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	slog.Debug("object deleted",
		"bucket", bucket,
		"key", key)

	return nil
}

// SetTTL sets object expiration using S3 object tagging and lifecycle rules
// Note: This requires bucket lifecycle rules to be configured for tag-based expiration
func (d *S3Driver) SetTTL(ctx context.Context, bucket, key string, ttlSeconds int) error {
	// Calculate expiration date
	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)

	// Set object tagging for lifecycle rules
	_, err := d.client.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Tagging: &types.Tagging{
			TagSet: []types.Tag{
				{
					Key:   aws.String("prism-ttl"),
					Value: aws.String(expiresAt.Format(time.RFC3339)),
				},
				{
					Key:   aws.String("prism-ttl-seconds"),
					Value: aws.String(fmt.Sprintf("%d", ttlSeconds)),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to set object TTL: %w", err)
	}

	slog.Debug("object TTL set",
		"bucket", bucket,
		"key", key,
		"ttl_seconds", ttlSeconds,
		"expires_at", expiresAt)

	return nil
}

// Exists checks if object exists
func (d *S3Driver) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// GetMetadata retrieves object metadata without downloading
func (d *S3Driver) GetMetadata(ctx context.Context, bucket, key string) (*plugin.ObjectMetadata, error) {
	result, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	metadata := &plugin.ObjectMetadata{
		Size: aws.ToInt64(result.ContentLength),
	}

	if result.ContentType != nil {
		metadata.ContentType = *result.ContentType
	}

	if result.LastModified != nil {
		metadata.LastModified = result.LastModified.Unix()
	}

	if result.ETag != nil {
		metadata.ETag = *result.ETag
	}

	return metadata, nil
}

// Compile-time interface compliance checks
var (
	_ plugin.Plugin              = (*S3Driver)(nil) // Core plugin interface
	_ plugin.ObjectStoreInterface = (*S3Driver)(nil) // Object store operations
)

// GetClient returns the underlying S3 client for advanced operations (e.g., bucket management in tests)
func (d *S3Driver) GetClient() *s3.Client {
	return d.client
}

// GetInterfaceDeclarations returns the interfaces this driver implements
func (d *S3Driver) GetInterfaceDeclarations() []*pb.InterfaceDeclaration {
	return []*pb.InterfaceDeclaration{
		{
			Name:      "ObjectStoreInterface",
			ProtoFile: "prism/interfaces/objectstore/objectstore.proto",
			Version:   "v1",
		},
	}
}
