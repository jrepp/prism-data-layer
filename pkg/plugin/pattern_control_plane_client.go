package plugin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PatternControlPlaneClient connects a pattern back to the proxy
// This is used by pattern executables to receive commands from proxy
type PatternControlPlaneClient struct {
	proxyAddr string
	conn      *grpc.ClientConn
	client    pb.ProxyControlPlaneClient
	stream    pb.ProxyControlPlane_ManagePatternClient

	// Pattern info
	patternName    string
	patternVersion string
	plugin         Plugin
	instanceID     string

	// Command handling
	commandHandlers map[string]func(*pb.ProxyCommand) (*pb.PatternMessage, error)
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// NewPatternControlPlaneClient creates a client that connects to proxy
func NewPatternControlPlaneClient(proxyAddr, patternName, patternVersion string, plugin Plugin) *PatternControlPlaneClient {
	return &PatternControlPlaneClient{
		proxyAddr:       proxyAddr,
		patternName:     patternName,
		patternVersion:  patternVersion,
		plugin:          plugin,
		commandHandlers: make(map[string]func(*pb.ProxyCommand) (*pb.PatternMessage, error)),
		stopChan:        make(chan struct{}),
	}
}

// Connect establishes connection to proxy and registers pattern
func (c *PatternControlPlaneClient) Connect(ctx context.Context) error {
	// Connect to proxy
	conn, err := grpc.DialContext(ctx, c.proxyAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to proxy at %s: %w", c.proxyAddr, err)
	}
	c.conn = conn
	c.client = pb.NewProxyControlPlaneClient(conn)

	// Open bidirectional stream
	stream, err := c.client.ManagePattern(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	c.stream = stream

	// Send registration with protocol specification (RFC-030)
	registerMsg := &pb.PatternMessage{
		CorrelationId: "register-1",
		Message: &pb.PatternMessage_Register{
			Register: &pb.RegisterRequest{
				PatternName:    c.patternName,
				PatternVersion: c.patternVersion,
				ProcessId:      int32(os.Getpid()),
				Metadata: &pb.PatternMetadata{
					Name:       c.plugin.Name(),
					Version:    c.plugin.Version(),
					Interfaces: c.plugin.GetInterfaceDeclarations(),
				},
				// Consumer protocol specification (RFC-030)
				ConsumerProtocol: &pb.ConsumerProtocol{
					Topics: []string{}, // Will be populated from config
					SchemaExpectations: make(map[string]*pb.SchemaExpectation),
					Metadata: &pb.ConsumerMetadata{
						Team:                   "default-team",      // TODO: get from config
						Purpose:                "pattern execution", // TODO: get from config
						DataUsage:              pb.DataUsage_DATA_USAGE_OPERATIONAL,
						PiiAccess:              pb.PIIAccess_PII_ACCESS_NOT_NEEDED, // TODO: get from config
						RetentionDays:          30,                                 // TODO: get from config
						ComplianceFrameworks:   []string{},                         // TODO: get from config
						ApprovedBy:             "",                                 // TODO: get from config
						ApprovalDate:           "",                                 // TODO: get from config
						AccessPattern:          pb.AccessPattern_ACCESS_PATTERN_READ_ONLY,
						RateLimit:              &pb.RateLimit{MaxMessagesPerSecond: 1000, MaxConsumers: 5},
					},
				},
			},
		},
	}

	if err := stream.Send(registerMsg); err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}

	slog.Info("[PATTERN-CLIENT] Sent registration to proxy",
		"pattern", c.patternName,
		"proxy", c.proxyAddr)

	// Wait for registration ack
	cmd, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive registration ack: %w", err)
	}

	ack := cmd.GetRegisterAck()
	if ack == nil || !ack.Success {
		return fmt.Errorf("registration failed: %s", ack.GetError())
	}

	c.instanceID = ack.InstanceId
	slog.Info("[PATTERN-CLIENT] Registered with proxy",
		"instance_id", c.instanceID)

	// Start command processing loop
	c.wg.Add(1)
	go c.processCommands()

	return nil
}

// processCommands handles incoming commands from proxy
func (c *PatternControlPlaneClient) processCommands() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		cmd, err := c.stream.Recv()
		if err == io.EOF {
			slog.Info("[PATTERN-CLIENT] Proxy closed connection")
			return
		}
		if err != nil {
			slog.Error("[PATTERN-CLIENT] Failed to receive command", "error", err)
			return
		}

		// Handle command
		resp, err := c.handleCommand(cmd)
		if err != nil {
			slog.Error("[PATTERN-CLIENT] Failed to handle command", "error", err)
			// Send error response
			resp = &pb.PatternMessage{
				CorrelationId: cmd.CorrelationId,
				// TODO: add error message type
			}
		}

		// Send response
		if err := c.stream.Send(resp); err != nil {
			slog.Error("[PATTERN-CLIENT] Failed to send response", "error", err)
			return
		}
	}
}

// handleCommand processes a single command from proxy
func (c *PatternControlPlaneClient) handleCommand(cmd *pb.ProxyCommand) (*pb.PatternMessage, error) {
	correlationID := cmd.CorrelationId

	switch cmd.Command.(type) {
	case *pb.ProxyCommand_Initialize:
		return c.handleInitialize(correlationID, cmd.GetInitialize())

	case *pb.ProxyCommand_Start:
		return c.handleStart(correlationID, cmd.GetStart())

	case *pb.ProxyCommand_Stop:
		return c.handleStop(correlationID, cmd.GetStop())

	case *pb.ProxyCommand_HealthCheck:
		return c.handleHealthCheck(correlationID, cmd.GetHealthCheck())

	case *pb.ProxyCommand_Shutdown:
		return c.handleShutdown(correlationID, cmd.GetShutdown())

	default:
		return nil, fmt.Errorf("unknown command type")
	}
}

// handleInitialize processes Initialize command
func (c *PatternControlPlaneClient) handleInitialize(correlationID string, req *pb.InitializeRequest) (*pb.PatternMessage, error) {
	slog.Info("[PATTERN-CLIENT] Handling Initialize command", "name", req.Name)

	// Parse config from protobuf struct
	config, err := ParseConfigFromStruct(req.Name, req.Version, req.Config)
	if err != nil {
		return &pb.PatternMessage{
			CorrelationId: correlationID,
			Message: &pb.PatternMessage_InitializeResponse{
				InitializeResponse: &pb.InitializeResponse{
					Success: false,
					Error:   fmt.Sprintf("failed to parse config: %v", err),
				},
			},
		}, nil
	}

	// Call plugin Initialize
	if err := c.plugin.Initialize(context.Background(), config); err != nil {
		return &pb.PatternMessage{
			CorrelationId: correlationID,
			Message: &pb.PatternMessage_InitializeResponse{
				InitializeResponse: &pb.InitializeResponse{
					Success: false,
					Error:   err.Error(),
				},
			},
		}, nil
	}

	return &pb.PatternMessage{
		CorrelationId: correlationID,
		Message: &pb.PatternMessage_InitializeResponse{
			InitializeResponse: &pb.InitializeResponse{
				Success: true,
				Metadata: &pb.PatternMetadata{
					Name:       c.plugin.Name(),
					Version:    c.plugin.Version(),
					Interfaces: c.plugin.GetInterfaceDeclarations(),
				},
			},
		},
	}, nil
}

// handleStart processes Start command
func (c *PatternControlPlaneClient) handleStart(correlationID string, req *pb.StartRequest) (*pb.PatternMessage, error) {
	slog.Info("[PATTERN-CLIENT] Handling Start command")

	if err := c.plugin.Start(context.Background()); err != nil {
		return &pb.PatternMessage{
			CorrelationId: correlationID,
			Message: &pb.PatternMessage_StartResponse{
				StartResponse: &pb.StartResponse{
					Success: false,
					Error:   err.Error(),
				},
			},
		}, nil
	}

	return &pb.PatternMessage{
		CorrelationId: correlationID,
		Message: &pb.PatternMessage_StartResponse{
			StartResponse: &pb.StartResponse{
				Success: true,
			},
		},
	}, nil
}

// handleStop processes Stop command
func (c *PatternControlPlaneClient) handleStop(correlationID string, req *pb.StopRequest) (*pb.PatternMessage, error) {
	slog.Info("[PATTERN-CLIENT] Handling Stop command", "timeout", req.TimeoutSeconds)

	if err := c.plugin.Stop(context.Background()); err != nil {
		return &pb.PatternMessage{
			CorrelationId: correlationID,
			Message: &pb.PatternMessage_StopResponse{
				StopResponse: &pb.StopResponse{
					Success: false,
					Error:   err.Error(),
				},
			},
		}, nil
	}

	return &pb.PatternMessage{
		CorrelationId: correlationID,
		Message: &pb.PatternMessage_StopResponse{
			StopResponse: &pb.StopResponse{
				Success: true,
			},
		},
	}, nil
}

// handleHealthCheck processes HealthCheck command
func (c *PatternControlPlaneClient) handleHealthCheck(correlationID string, req *pb.HealthCheckRequest) (*pb.PatternMessage, error) {
	health, err := c.plugin.Health(context.Background())
	if err != nil {
		return &pb.PatternMessage{
			CorrelationId: correlationID,
			Message: &pb.PatternMessage_HealthResponse{
				HealthResponse: &pb.HealthCheckResponse{
					Status:  pb.HealthStatus_HEALTH_STATUS_UNHEALTHY,
					Message: err.Error(),
				},
			},
		}, nil
	}

	// Convert health status
	var pbStatus pb.HealthStatus
	switch health.Status {
	case HealthHealthy:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_HEALTHY
	case HealthDegraded:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_DEGRADED
	case HealthUnhealthy:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_UNHEALTHY
	default:
		pbStatus = pb.HealthStatus_HEALTH_STATUS_UNSPECIFIED
	}

	return &pb.PatternMessage{
		CorrelationId: correlationID,
		Message: &pb.PatternMessage_HealthResponse{
			HealthResponse: &pb.HealthCheckResponse{
				Status:  pbStatus,
				Message: health.Message,
				Details: health.Details,
			},
		},
	}, nil
}

// handleShutdown processes Shutdown command
func (c *PatternControlPlaneClient) handleShutdown(correlationID string, req *pb.ShutdownRequest) (*pb.PatternMessage, error) {
	slog.Info("[PATTERN-CLIENT] Handling Shutdown command", "reason", req.Reason)

	// Stop plugin
	if err := c.plugin.Stop(context.Background()); err != nil {
		slog.Error("[PATTERN-CLIENT] Failed to stop plugin during shutdown", "error", err)
	}

	// Signal stop
	close(c.stopChan)

	return &pb.PatternMessage{
		CorrelationId: correlationID,
		Message: &pb.PatternMessage_StopResponse{
			StopResponse: &pb.StopResponse{
				Success: true,
			},
		},
	}, nil
}

// Close closes the connection to proxy
func (c *PatternControlPlaneClient) Close() error {
	close(c.stopChan)
	c.wg.Wait()

	if c.stream != nil {
		c.stream.CloseSend()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
