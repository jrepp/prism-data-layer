package launcher

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AdminClient handles communication with prism-admin control plane
type AdminClient struct {
	client     pb.ControlPlaneClient
	conn       *grpc.ClientConn
	launcherID string
	address    string
	region     string
	maxProcs   int32
}

// AdminClientConfig configures the admin client
type AdminClientConfig struct {
	AdminEndpoint string
	LauncherID    string
	Address       string
	Region        string
	MaxProcesses  int32
}

// NewAdminClient creates a new admin client
func NewAdminClient(cfg *AdminClientConfig) (*AdminClient, error) {
	log.Printf("[AdminClient] Connecting to admin at %s...", cfg.AdminEndpoint)

	conn, err := grpc.NewClient(
		cfg.AdminEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial admin: %w", err)
	}

	client := pb.NewControlPlaneClient(conn)

	return &AdminClient{
		client:     client,
		conn:       conn,
		launcherID: cfg.LauncherID,
		address:    cfg.Address,
		region:     cfg.Region,
		maxProcs:   cfg.MaxProcesses,
	}, nil
}

// Register registers the launcher with admin on startup
func (c *AdminClient) Register(ctx context.Context) (*pb.LauncherRegistrationAck, error) {
	log.Printf("[AdminClient] Registering launcher %s with admin...", c.launcherID)

	req := &pb.LauncherRegistration{
		LauncherId:   c.launcherID,
		Address:      c.address,
		Region:       c.region,
		Version:      "0.1.0",
		Capabilities: []string{"pattern", "proxy", "backend", "utility"},
		MaxProcesses: c.maxProcs,
		ProcessTypes: []string{"pattern"},
		Metadata:     map[string]string{},
	}

	resp, err := c.client.RegisterLauncher(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("registration rejected: %s", resp.Message)
	}

	log.Printf("[AdminClient] Registration successful: %s", resp.Message)
	log.Printf("[AdminClient] Assigned capacity: %d, initial processes: %d",
		resp.AssignedCapacity, len(resp.InitialProcesses))

	return resp, nil
}

// StartHeartbeatLoop starts sending periodic heartbeats to admin
func (c *AdminClient) StartHeartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[AdminClient] Starting heartbeat loop (interval: %v)", interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[AdminClient] Heartbeat loop stopping")
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(ctx); err != nil {
				log.Printf("[AdminClient] Heartbeat failed: %v", err)
			}
		}
	}
}

// sendHeartbeat sends a heartbeat to admin
func (c *AdminClient) sendHeartbeat(ctx context.Context) error {
	req := &pb.LauncherHeartbeatRequest{
		LauncherId:    c.launcherID,
		ProcessHealth: map[string]*pb.ProcessHealth{},
		Resources: &pb.LauncherResourceUsage{
			ProcessCount:   0,
			MaxProcesses:   c.maxProcs,
			TotalMemoryMb:  0,
			CpuPercent:     0.0,
			AvailableSlots: c.maxProcs,
		},
		Timestamp: time.Now().Unix(),
	}

	resp, err := c.client.LauncherHeartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("heartbeat RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("heartbeat rejected: %s", resp.Message)
	}

	log.Printf("[AdminClient] Heartbeat acknowledged (server_time=%d)", resp.ServerTimestamp)
	return nil
}

// ReportLifecycleEvent sends a lifecycle event to admin (implements EventPublisher)
func (c *AdminClient) ReportLifecycleEvent(ctx context.Context, eventType, message string, metadata map[string]string) error {
	req := &pb.LifecycleEventRequest{
		ComponentId:   c.launcherID,
		ComponentType: "launcher",
		EventType:     eventType,
		Message:       message,
		Timestamp:     time.Now().Unix(),
		Metadata:      metadata,
	}

	resp, err := c.client.ReportLifecycleEvent(ctx, req)
	if err != nil {
		return fmt.Errorf("lifecycle event RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("lifecycle event rejected: %s", resp.Message)
	}

	log.Printf("[AdminClient] Lifecycle event sent: %s - %s", eventType, message)
	return nil
}

// Close closes the admin client connection
func (c *AdminClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
