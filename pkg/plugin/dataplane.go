package core

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces/keyvalue"
	"google.golang.org/grpc"
)

// DataPlaneServer provides gRPC endpoints for data plane operations
// It wraps backend implementations and exposes them as gRPC services
type DataPlaneServer struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
	backend    KeyValueBasicInterface
}

// NewDataPlaneServer creates a new data plane server
func NewDataPlaneServer(backend KeyValueBasicInterface, port int) *DataPlaneServer {
	return &DataPlaneServer{
		backend: backend,
		port:    port,
	}
}

// Start begins serving gRPC data plane requests
func (s *DataPlaneServer) Start(ctx context.Context) error {
	// Create listener
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Get actual port if dynamic allocation (port 0)
	if s.port == 0 {
		s.port = s.listener.Addr().(*net.TCPAddr).Port
	}

	// Create gRPC server
	s.grpcServer = grpc.NewServer()

	// Register KeyValue service
	kvService := &keyValueServiceImpl{backend: s.backend}
	pb.RegisterKeyValueBasicInterfaceServer(s.grpcServer, kvService)

	slog.Info("data plane server starting",
		"address", listener.Addr().String(),
		"port", s.port)

	// Start serving in a goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			slog.Error("data plane server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the data plane server
func (s *DataPlaneServer) Stop(ctx context.Context) error {
	if s.grpcServer != nil {
		slog.Info("stopping data plane server")
		s.grpcServer.GracefulStop()
	}
	return nil
}

// Port returns the port the server is listening on
func (s *DataPlaneServer) Port() int {
	return s.port
}

// Addr returns the address the server is listening on
func (s *DataPlaneServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf(":%d", s.port)
}

// keyValueServiceImpl implements the gRPC KeyValueBasicInterface service
type keyValueServiceImpl struct {
	pb.UnimplementedKeyValueBasicInterfaceServer
	backend KeyValueBasicInterface
}

// Set implements the Set RPC
func (s *keyValueServiceImpl) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	slog.Debug("data plane: Set",
		"key", req.Key,
		"value_size", len(req.Value))

	err := s.backend.Set(req.Key, req.Value, 0) // ttlSeconds defaults to 0 (no expiration)
	if err != nil {
		slog.Error("data plane: Set failed", "key", req.Key, "error", err)
		return &pb.SetResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &pb.SetResponse{
		Success: true,
	}, nil
}

// Get implements the Get RPC
func (s *keyValueServiceImpl) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	slog.Debug("data plane: Get", "key", req.Key)

	value, found, err := s.backend.Get(req.Key)
	if err != nil {
		slog.Error("data plane: Get failed", "key", req.Key, "error", err)
		return &pb.GetResponse{
			Found: false,
			Error: err.Error(),
		}, nil
	}

	if !found {
		slog.Debug("data plane: Get - key not found", "key", req.Key)
		return &pb.GetResponse{
			Found: false,
		}, nil
	}

	slog.Debug("data plane: Get - found",
		"key", req.Key,
		"value_size", len(value))
	return &pb.GetResponse{
		Found: true,
		Value: value,
	}, nil
}

// Delete implements the Delete RPC
func (s *keyValueServiceImpl) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	slog.Debug("data plane: Delete", "key", req.Key)

	err := s.backend.Delete(req.Key)
	if err != nil {
		slog.Error("data plane: Delete failed", "key", req.Key, "error", err)
		return &pb.DeleteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &pb.DeleteResponse{
		Success: true,
	}, nil
}

// Exists implements the Exists RPC
func (s *keyValueServiceImpl) Exists(ctx context.Context, req *pb.ExistsRequest) (*pb.ExistsResponse, error) {
	slog.Debug("data plane: Exists", "key", req.Key)

	exists, err := s.backend.Exists(req.Key)
	if err != nil {
		slog.Error("data plane: Exists failed", "key", req.Key, "error", err)
		return &pb.ExistsResponse{
			Exists: false,
			Error:  err.Error(),
		}, nil
	}

	slog.Debug("data plane: Exists - result",
		"key", req.Key,
		"exists", exists)
	return &pb.ExistsResponse{
		Exists: exists,
	}, nil
}
