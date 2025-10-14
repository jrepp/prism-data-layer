package framework

import (
	"context"
	"fmt"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/interfaces"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// PatternExecutable represents a running pattern executable that can be queried
type PatternExecutable struct {
	// Name of the pattern (e.g., "kafka", "redis", "memstore")
	Name string

	// Address of the gRPC endpoint (e.g., "localhost:50051")
	Address string

	// gRPC connection
	conn *grpc.ClientConn

	// Lifecycle client for introspection
	lifecycleClient pb.LifecycleInterfaceClient

	// Metadata discovered from the pattern
	Metadata *pb.PatternMetadata

	// Cleanup function to stop the pattern and close connections
	Cleanup func()
}

// DiscoverPatternInterfaces connects to a pattern executable and discovers its supported interfaces
// This is the core of the dynamic test selection system
func DiscoverPatternInterfaces(ctx context.Context, name string, address string, config map[string]interface{}) (*PatternExecutable, error) {
	// Connect to the pattern's gRPC endpoint
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to pattern %s at %s: %w", name, address, err)
	}

	// Create lifecycle client
	lifecycleClient := pb.NewLifecycleInterfaceClient(conn)

	// Convert config to protobuf Struct
	configStruct, err := structpb.NewStruct(config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to convert config to protobuf struct: %w", err)
	}

	// Initialize the pattern and get metadata
	initResp, err := lifecycleClient.Initialize(ctx, &pb.InitializeRequest{
		Name:    name,
		Version: "0.1.0",
		Config:  configStruct,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize pattern %s: %w", name, err)
	}

	if !initResp.Success {
		conn.Close()
		return nil, fmt.Errorf("pattern %s initialization failed: %s", name, initResp.Error)
	}

	// Start the pattern
	startResp, err := lifecycleClient.Start(ctx, &pb.StartRequest{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to start pattern %s: %w", name, err)
	}

	if !startResp.Success {
		conn.Close()
		return nil, fmt.Errorf("pattern %s start failed: %s", name, startResp.Error)
	}

	executable := &PatternExecutable{
		Name:            name,
		Address:         address,
		conn:            conn,
		lifecycleClient: lifecycleClient,
		Metadata:        initResp.Metadata,
		Cleanup: func() {
			// Stop the pattern gracefully
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, _ = lifecycleClient.Stop(stopCtx, &pb.StopRequest{
				TimeoutSeconds: 5,
			})

			conn.Close()
		},
	}

	return executable, nil
}

// GetSupportedInterfaces returns the list of interface names supported by this pattern
func (p *PatternExecutable) GetSupportedInterfaces() []string {
	if p.Metadata == nil {
		return nil
	}

	interfaces := make([]string, 0, len(p.Metadata.Interfaces))
	for _, iface := range p.Metadata.Interfaces {
		interfaces = append(interfaces, iface.Name)
	}

	return interfaces
}

// SupportsInterface checks if the pattern implements a specific interface
func (p *PatternExecutable) SupportsInterface(interfaceName string) bool {
	for _, iface := range p.GetSupportedInterfaces() {
		if iface == interfaceName {
			return true
		}
	}
	return false
}

// GetInterfaceDeclaration returns the full declaration for a specific interface
func (p *PatternExecutable) GetInterfaceDeclaration(interfaceName string) (*pb.InterfaceDeclaration, bool) {
	if p.Metadata == nil {
		return nil, false
	}

	for _, iface := range p.Metadata.Interfaces {
		if iface.Name == interfaceName {
			return iface, true
		}
	}

	return nil, false
}

// HealthCheck performs a health check on the pattern
func (p *PatternExecutable) HealthCheck(ctx context.Context) (*pb.HealthCheckResponse, error) {
	return p.lifecycleClient.HealthCheck(ctx, &pb.HealthCheckRequest{})
}

// Connection returns the gRPC connection for creating interface-specific clients
func (p *PatternExecutable) Connection() *grpc.ClientConn {
	return p.conn
}

// InterfaceToPattern maps interface names to Pattern constants for backward compatibility
// with the existing test framework
var InterfaceToPattern = map[string]Pattern{
	"KeyValueBasicInterface": PatternKeyValueBasic,
	"KeyValueTTLInterface":   PatternKeyValueTTL,
	"KeyValueScanInterface":  PatternKeyValueScan,
	"KeyValueAtomicInterface": PatternKeyValueAtomic,
	"PubSubBasicInterface":   PatternPubSubBasic,
	"PubSubOrderingInterface": PatternPubSubOrdering,
	"PubSubFanoutInterface":  PatternPubSubFanout,
	"QueueBasicInterface":    PatternQueueBasic,
}

// PatternToInterface maps Pattern constants back to interface names
var PatternToInterface = map[Pattern]string{
	PatternKeyValueBasic:  "KeyValueBasicInterface",
	PatternKeyValueTTL:    "KeyValueTTLInterface",
	PatternKeyValueScan:   "KeyValueScanInterface",
	PatternKeyValueAtomic: "KeyValueAtomicInterface",
	PatternPubSubBasic:    "PubSubBasicInterface",
	PatternPubSubOrdering: "PubSubOrderingInterface",
	PatternPubSubFanout:   "PubSubFanoutInterface",
	PatternQueueBasic:     "QueueBasicInterface",
}

// GetSupportedPatterns returns the Pattern constants for interfaces this executable supports
// This provides backward compatibility with existing test code
func (p *PatternExecutable) GetSupportedPatterns() []Pattern {
	interfaces := p.GetSupportedInterfaces()
	patterns := make([]Pattern, 0, len(interfaces))

	for _, iface := range interfaces {
		if pattern, ok := InterfaceToPattern[iface]; ok {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}
