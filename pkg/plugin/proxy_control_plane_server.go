package plugin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"google.golang.org/grpc"
)

// ProxyControlPlaneServer manages connections from patterns
// Patterns connect TO the proxy (not the other way around)
type ProxyControlPlaneServer struct {
	pb.UnimplementedProxyControlPlaneServer

	port       int
	grpcServer *grpc.Server
	listener   net.Listener

	// Map of pattern instance ID -> stream
	patterns   map[string]*PatternConnection
	patternsMu sync.RWMutex
}

// PatternConnection represents a connected pattern
type PatternConnection struct {
	InstanceID  string
	PatternName string
	Stream      pb.ProxyControlPlane_ManagePatternServer
	Metadata    *pb.PatternMetadata

	// Channel for sending commands to pattern
	commands chan *pb.ProxyCommand

	// For correlation tracking
	nextCorrelationID int
	pendingResponses  map[string]chan *pb.PatternMessage
	responseMu        sync.Mutex
}

// NewProxyControlPlaneServer creates a new proxy control plane server
func NewProxyControlPlaneServer(port int) *ProxyControlPlaneServer {
	return &ProxyControlPlaneServer{
		port:     port,
		patterns: make(map[string]*PatternConnection),
	}
}

// Start begins serving control plane requests
func (s *ProxyControlPlaneServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	// Create gRPC server
	s.grpcServer = grpc.NewServer()

	// Register ProxyControlPlane service
	pb.RegisterProxyControlPlaneServer(s.grpcServer, s)
	slog.Info("[PROXY] Registered ProxyControlPlane service", "port", s.port)

	// Start serving
	go func() {
		slog.Info("[PROXY] Control plane listening", "port", s.port)
		if err := s.grpcServer.Serve(listener); err != nil {
			slog.Error("[PROXY] Control plane serve error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the control plane server
func (s *ProxyControlPlaneServer) Stop(ctx context.Context) error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	return nil
}

// ManagePattern implements the bidirectional streaming RPC
// Patterns call this to connect and receive commands
func (s *ProxyControlPlaneServer) ManagePattern(stream pb.ProxyControlPlane_ManagePatternServer) error {
	slog.Info("[PROXY] Pattern connecting to control plane")

	// Wait for initial registration message
	msg, err := stream.Recv()
	if err != nil {
		slog.Error("[PROXY] Failed to receive registration", "error", err)
		return fmt.Errorf("failed to receive registration: %w", err)
	}

	register := msg.GetRegister()
	if register == nil {
		return fmt.Errorf("first message must be RegisterRequest")
	}

	// Generate instance ID
	instanceID := fmt.Sprintf("%s-%d", register.PatternName, register.ProcessId)

	// Log pattern registration
	slog.Info("[PROXY] Pattern registered",
		"pattern", register.PatternName,
		"instance_id", instanceID,
		"version", register.PatternVersion,
		"interfaces", len(register.Metadata.Interfaces))

	// Create pattern connection
	conn := &PatternConnection{
		InstanceID:       instanceID,
		PatternName:      register.PatternName,
		Stream:           stream,
		Metadata:         register.Metadata,
		commands:         make(chan *pb.ProxyCommand, 10),
		pendingResponses: make(map[string]chan *pb.PatternMessage),
	}

	// Store connection
	s.patternsMu.Lock()
	s.patterns[instanceID] = conn
	s.patternsMu.Unlock()

	// Send registration acknowledgment
	ack := &pb.ProxyCommand{
		CorrelationId: msg.CorrelationId,
		Command: &pb.ProxyCommand_RegisterAck{
			RegisterAck: &pb.RegisterResponse{
				Success:    true,
				InstanceId: instanceID,
			},
		},
	}
	if err := stream.Send(ack); err != nil {
		return fmt.Errorf("failed to send registration ack: %w", err)
	}

	// Handle bidirectional communication
	errChan := make(chan error, 2)

	// Goroutine to receive messages from pattern
	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				errChan <- nil
				return
			}
			if err != nil {
				errChan <- err
				return
			}

			// Route message based on correlation ID
			conn.responseMu.Lock()
			if respChan, ok := conn.pendingResponses[msg.CorrelationId]; ok {
				respChan <- msg
				delete(conn.pendingResponses, msg.CorrelationId)
				conn.responseMu.Unlock()
			} else {
				conn.responseMu.Unlock()
				// Handle unsolicited messages (heartbeats, etc.)
				if heartbeat := msg.GetHeartbeat(); heartbeat != nil {
					slog.Debug("[PROXY] Received heartbeat",
						"instance_id", instanceID,
						"status", heartbeat.Status.String())
				}
			}
		}
	}()

	// Goroutine to send commands to pattern
	go func() {
		for cmd := range conn.commands {
			if err := stream.Send(cmd); err != nil {
				errChan <- fmt.Errorf("failed to send command: %w", err)
				return
			}
		}
	}()

	// Wait for error or completion
	err = <-errChan

	// Cleanup
	s.patternsMu.Lock()
	delete(s.patterns, instanceID)
	s.patternsMu.Unlock()

	close(conn.commands)

	if err != nil {
		slog.Error("[PROXY] Pattern connection closed with error",
			"instance_id", instanceID,
			"error", err)
	} else {
		slog.Info("[PROXY] Pattern connection closed",
			"instance_id", instanceID)
	}

	return err
}

// SendCommand sends a command to a specific pattern and waits for response
func (s *ProxyControlPlaneServer) SendCommand(instanceID string, cmd *pb.ProxyCommand) (*pb.PatternMessage, error) {
	s.patternsMu.RLock()
	conn, ok := s.patterns[instanceID]
	s.patternsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("pattern %s not connected", instanceID)
	}

	// Generate correlation ID
	conn.responseMu.Lock()
	conn.nextCorrelationID++
	correlationID := fmt.Sprintf("%s-%d", instanceID, conn.nextCorrelationID)
	cmd.CorrelationId = correlationID

	// Create response channel
	respChan := make(chan *pb.PatternMessage, 1)
	conn.pendingResponses[correlationID] = respChan
	conn.responseMu.Unlock()

	// Send command
	conn.commands <- cmd

	// Wait for response (with timeout handled by caller's context)
	resp := <-respChan
	return resp, nil
}

// GetPatternInstances returns list of connected patterns
func (s *ProxyControlPlaneServer) GetPatternInstances() []string {
	s.patternsMu.RLock()
	defer s.patternsMu.RUnlock()

	instances := make([]string, 0, len(s.patterns))
	for id := range s.patterns {
		instances = append(instances, id)
	}
	return instances
}

// GetPort returns the actual port the server is listening on
// This is useful when port 0 is used for dynamic port allocation
func (s *ProxyControlPlaneServer) GetPort() int {
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return s.port
}
