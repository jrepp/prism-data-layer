package consumer_test

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"github.com/jrepp/prism-data-layer/pkg/plugin"
)

// ControlClient wraps a gRPC client for the pattern control plane
type ControlClient struct {
	conn   *grpc.ClientConn
	client pb.LifecycleInterfaceClient
	addr   string
}

// NewControlClient connects to a pattern's control plane
func NewControlClient(ctx context.Context, addr string) (*ControlClient, error) {
	// Retry connection for up to 5 seconds
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to control plane at %s: %w", addr, err)
	}

	client := pb.NewLifecycleInterfaceClient(conn)

	return &ControlClient{
		conn:   conn,
		client: client,
		addr:   addr,
	}, nil
}

// Close closes the connection to the control plane
func (c *ControlClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Initialize sends configuration to the pattern
func (c *ControlClient) Initialize(ctx context.Context, name, version string, config map[string]interface{}) error {
	// Convert config map to protobuf Struct
	configStruct, err := structpb.NewStruct(config)
	if err != nil {
		return fmt.Errorf("failed to convert config to protobuf struct: %w", err)
	}

	req := &pb.InitializeRequest{
		Name:    name,
		Version: version,
		Config:  configStruct,
	}

	resp, err := c.client.Initialize(ctx, req)
	if err != nil {
		return fmt.Errorf("initialize RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("initialize failed: %s", resp.Error)
	}

	return nil
}

// Start tells the pattern to start processing
func (c *ControlClient) Start(ctx context.Context) error {
	req := &pb.StartRequest{}

	resp, err := c.client.Start(ctx, req)
	if err != nil {
		return fmt.Errorf("start RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("start failed: %s", resp.Error)
	}

	return nil
}

// Stop tells the pattern to stop processing
func (c *ControlClient) Stop(ctx context.Context, timeoutSeconds int32) error {
	req := &pb.StopRequest{
		TimeoutSeconds: timeoutSeconds,
	}

	resp, err := c.client.Stop(ctx, req)
	if err != nil {
		return fmt.Errorf("stop RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("stop failed: %s", resp.Error)
	}

	return nil
}

// Health checks the pattern's health status
func (c *ControlClient) Health(ctx context.Context) (*plugin.HealthStatus, error) {
	req := &pb.HealthCheckRequest{}

	resp, err := c.client.HealthCheck(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("health check RPC failed: %w", err)
	}

	// Convert protobuf health status to plugin.HealthStatus
	var status plugin.HealthState
	switch resp.Status {
	case pb.HealthStatus_HEALTH_STATUS_HEALTHY:
		status = plugin.HealthHealthy
	case pb.HealthStatus_HEALTH_STATUS_DEGRADED:
		status = plugin.HealthDegraded
	case pb.HealthStatus_HEALTH_STATUS_UNHEALTHY:
		status = plugin.HealthUnhealthy
	default:
		status = plugin.HealthUnknown
	}

	return &plugin.HealthStatus{
		Status:  status,
		Message: resp.Message,
		Details: resp.Details,
	}, nil
}

// BuildConsumerConfig creates a consumer configuration map for Initialize RPC
func BuildConsumerConfig(natsUrl, memstoreAddr, topic, group string) map[string]interface{} {
	config := map[string]interface{}{
		"slots": map[string]interface{}{
			"message_source": map[string]interface{}{
				"driver": "nats",
				"config": map[string]interface{}{
					"url":              natsUrl,
					"max_reconnects":   10,
					"reconnect_wait":   "2s",
					"timeout":          "5s",
					"enable_jetstream": false,
				},
			},
		},
		"behavior": map[string]interface{}{
			"consumer_group":  group,
			"topic":           topic,
			"max_retries":     3,
			"auto_commit":     false,
			"batch_size":      1,
			"commit_interval": "1s",
		},
	}

	// Add state store if address provided
	if memstoreAddr != "" {
		slots := config["slots"].(map[string]interface{})
		slots["state_store"] = map[string]interface{}{
			"driver": "memstore",
			"config": map[string]interface{}{
				"address": memstoreAddr,
			},
		}
		// Enable auto-commit when stateful
		behavior := config["behavior"].(map[string]interface{})
		behavior["auto_commit"] = true
	}

	return config
}
