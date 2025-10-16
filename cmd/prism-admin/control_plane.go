package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"sync"
	"time"

	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControlPlaneService implements the ControlPlane gRPC service
type ControlPlaneService struct {
	pb.UnimplementedControlPlaneServer
	storage    *Storage
	partitions *PartitionManager
	mu         sync.RWMutex
}

// NewControlPlaneService creates a new control plane service
func NewControlPlaneService(storage *Storage) *ControlPlaneService {
	return &ControlPlaneService{
		storage:    storage,
		partitions: NewPartitionManager(),
	}
}

// ====================================================================
// Proxy RPCs (ADR-055)
// ====================================================================

// RegisterProxy registers a proxy instance with admin on startup
func (s *ControlPlaneService) RegisterProxy(
	ctx context.Context,
	req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("[ControlPlane] RegisterProxy: proxy_id=%s, address=%s, region=%s, version=%s\n",
		req.ProxyId, req.Address, req.Region, req.Version)

	// Record proxy in storage
	now := time.Now()
	proxy := &Proxy{
		ProxyID:  req.ProxyId,
		Address:  req.Address,
		Version:  req.Version,
		Status:   "healthy",
		LastSeen: &now,
	}

	if err := s.storage.UpsertProxy(ctx, proxy); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register proxy: %v", err)
	}

	// Assign partition ranges
	ranges := s.partitions.AssignRanges(req.ProxyId)

	// Get initial namespace assignments for this proxy's partitions
	namespaces, err := s.getNamespacesForRanges(ctx, ranges)
	if err != nil {
		fmt.Printf("[ControlPlane] Warning: failed to get namespaces for ranges: %v\n", err)
		namespaces = []*pb.NamespaceAssignment{} // Continue with empty list
	}

	fmt.Printf("[ControlPlane] Proxy registered: %d partition ranges, %d initial namespaces\n",
		len(ranges), len(namespaces))

	return &pb.ProxyRegistrationAck{
		Success:           true,
		Message:           "Proxy registered successfully",
		InitialNamespaces: namespaces,
		PartitionRanges:   ranges,
	}, nil
}

// AssignNamespace pushes namespace configuration from admin to proxy
func (s *ControlPlaneService) AssignNamespace(
	ctx context.Context,
	req *pb.NamespaceAssignment,
) (*pb.NamespaceAssignmentAck, error) {
	fmt.Printf("[ControlPlane] AssignNamespace: namespace=%s, partition=%d, version=%d\n",
		req.Namespace, req.PartitionId, req.Version)

	// TODO: Implement namespace assignment logic
	// This would be called by admin when pushing config to proxy

	return &pb.NamespaceAssignmentAck{
		Success: true,
		Message: "Namespace assigned successfully",
	}, nil
}

// CreateNamespace handles client-initiated namespace creation requests
func (s *ControlPlaneService) CreateNamespace(
	ctx context.Context,
	req *pb.CreateNamespaceRequest,
) (*pb.CreateNamespaceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("[ControlPlane] CreateNamespace: namespace=%s, requesting_proxy=%s, principal=%s\n",
		req.Namespace, req.RequestingProxy, req.Principal)

	// Calculate partition ID for this namespace
	partitionID := s.partitions.HashNamespace(req.Namespace)

	// Find proxy assigned to this partition
	proxyID, err := s.partitions.GetProxyForPartition(partitionID)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"no proxy assigned to partition %d: %v", partitionID, err)
	}

	// Persist namespace in storage
	ns := &Namespace{
		Name:        req.Namespace,
		Description: fmt.Sprintf("Created via %s by %s", req.RequestingProxy, req.Principal),
	}

	if err := s.storage.CreateNamespace(ctx, ns); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create namespace: %v", err)
	}

	fmt.Printf("[ControlPlane] Namespace created: %s → partition %d → proxy %s\n",
		req.Namespace, partitionID, proxyID)

	// TODO: Send NamespaceAssignment to the assigned proxy

	return &pb.CreateNamespaceResponse{
		Success:          true,
		Message:          "Namespace created successfully",
		AssignedPartition: partitionID,
		AssignedProxy:    proxyID,
	}, nil
}

// Heartbeat receives periodic health updates from proxies
func (s *ControlPlaneService) Heartbeat(
	ctx context.Context,
	req *pb.ProxyHeartbeat,
) (*pb.HeartbeatAck, error) {
	fmt.Printf("[ControlPlane] Heartbeat from proxy %s: %d namespaces, cpu=%.1f%%, mem=%dMB\n",
		req.ProxyId,
		len(req.NamespaceHealth),
		req.Resources.CpuPercent,
		req.Resources.MemoryMb)

	// Update proxy last_seen timestamp
	now := time.Now()
	proxy := &Proxy{
		ProxyID:  req.ProxyId,
		LastSeen: &now,
		Status:   "healthy",
	}

	if err := s.storage.UpsertProxy(ctx, proxy); err != nil {
		fmt.Printf("[ControlPlane] Warning: failed to update proxy heartbeat: %v\n", err)
	}

	// TODO: Update namespace health metrics in storage

	return &pb.HeartbeatAck{
		Success:         true,
		Message:         "Heartbeat received",
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

// RevokeNamespace removes namespace assignment from proxy
func (s *ControlPlaneService) RevokeNamespace(
	ctx context.Context,
	req *pb.NamespaceRevocation,
) (*pb.NamespaceRevocationAck, error) {
	fmt.Printf("[ControlPlane] RevokeNamespace: proxy=%s, namespace=%s, graceful_timeout=%ds\n",
		req.ProxyId, req.Namespace, req.GracefulTimeoutSeconds)

	// TODO: Implement namespace revocation logic

	return &pb.NamespaceRevocationAck{
		Success:   true,
		Message:   "Namespace revoked successfully",
		RevokedAt: time.Now().Unix(),
	}, nil
}

// ====================================================================
// Launcher RPCs (ADR-056, ADR-057)
// ====================================================================

// RegisterLauncher registers a launcher instance with admin on startup
func (s *ControlPlaneService) RegisterLauncher(
	ctx context.Context,
	req *pb.LauncherRegistration,
) (*pb.LauncherRegistrationAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("[ControlPlane] RegisterLauncher: launcher_id=%s, address=%s, region=%s, max_processes=%d, capabilities=%v\n",
		req.LauncherId, req.Address, req.Region, req.MaxProcesses, req.Capabilities)

	// Persist launcher in storage
	now := time.Now()
	capabilitiesJSON, _ := json.Marshal(req.Capabilities)
	launcher := &Launcher{
		LauncherID:     req.LauncherId,
		Address:        req.Address,
		Region:         req.Region,
		Version:        req.Version,
		Status:         "healthy",
		MaxProcesses:   req.MaxProcesses,
		AvailableSlots: req.MaxProcesses, // Initially all slots available
		Capabilities:   capabilitiesJSON,
		LastSeen:       &now,
	}

	if err := s.storage.UpsertLauncher(ctx, launcher); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register launcher: %v", err)
	}

	// TODO: Get initial process assignments for this launcher

	fmt.Printf("[ControlPlane] Launcher registered successfully\n")

	return &pb.LauncherRegistrationAck{
		Success:          true,
		Message:          "Launcher registered successfully",
		InitialProcesses: []*pb.ProcessAssignment{}, // No initial processes for now
		AssignedCapacity: 0,
	}, nil
}

// AssignProcess pushes process assignment from admin to launcher
func (s *ControlPlaneService) AssignProcess(
	ctx context.Context,
	req *pb.ProcessAssignment,
) (*pb.ProcessAssignmentAck, error) {
	fmt.Printf("[ControlPlane] AssignProcess: process_id=%s, type=%s, namespace=%s\n",
		req.ProcessId, req.ProcessType, req.Namespace)

	// TODO: Implement process assignment logic

	return &pb.ProcessAssignmentAck{
		Success: true,
		Message: "Process assigned successfully",
	}, nil
}

// LauncherHeartbeat receives periodic health updates from launchers
func (s *ControlPlaneService) LauncherHeartbeat(
	ctx context.Context,
	req *pb.LauncherHeartbeatRequest,
) (*pb.HeartbeatAck, error) {
	fmt.Printf("[ControlPlane] LauncherHeartbeat from %s: %d processes, available_slots=%d, cpu=%.1f%%, mem=%dMB\n",
		req.LauncherId,
		req.Resources.ProcessCount,
		req.Resources.AvailableSlots,
		req.Resources.CpuPercent,
		req.Resources.TotalMemoryMb)

	// Update launcher last_seen timestamp and resource info in storage
	now := time.Now()
	launcher := &Launcher{
		LauncherID:     req.LauncherId,
		LastSeen:       &now,
		Status:         "healthy",
		AvailableSlots: req.Resources.AvailableSlots,
	}

	if err := s.storage.UpsertLauncher(ctx, launcher); err != nil {
		fmt.Printf("[ControlPlane] Warning: failed to update launcher heartbeat: %v\n", err)
	}

	// TODO: Update process health metrics in storage

	return &pb.HeartbeatAck{
		Success:         true,
		Message:         "Heartbeat received",
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

// RevokeProcess removes process assignment from launcher
func (s *ControlPlaneService) RevokeProcess(
	ctx context.Context,
	req *pb.ProcessRevocation,
) (*pb.ProcessRevocationAck, error) {
	fmt.Printf("[ControlPlane] RevokeProcess: launcher=%s, process_id=%s, graceful_timeout=%ds\n",
		req.LauncherId, req.ProcessId, req.GracefulTimeoutSeconds)

	// TODO: Implement process revocation logic

	return &pb.ProcessRevocationAck{
		Success:   true,
		Message:   "Process revoked successfully",
		StoppedAt: time.Now().Unix(),
		ExitCode:  0,
	}, nil
}

// ====================================================================
// Helper Methods
// ====================================================================

// getNamespacesForRanges retrieves namespace assignments for given partition ranges
func (s *ControlPlaneService) getNamespacesForRanges(
	ctx context.Context,
	ranges []*pb.PartitionRange,
) ([]*pb.NamespaceAssignment, error) {
	// Get all namespaces from storage
	namespaces, err := s.storage.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	// Filter namespaces that belong to these partition ranges
	var assignments []*pb.NamespaceAssignment
	for _, ns := range namespaces {
		partitionID := s.partitions.HashNamespace(ns.Name)

		// Check if partition falls within any of our ranges
		for _, r := range ranges {
			if partitionID >= r.Start && partitionID <= r.End {
				// TODO: Load actual namespace config from storage
				assignments = append(assignments, &pb.NamespaceAssignment{
					Namespace:   ns.Name,
					PartitionId: partitionID,
					Config: &pb.NamespaceConfig{
						Backends: map[string]*pb.BackendConfig{},
						Patterns: map[string]*pb.PatternConfig{},
						Metadata: map[string]string{},
					},
					Version: 1,
				})
				break
			}
		}
	}

	return assignments, nil
}

// ====================================================================
// Partition Manager
// ====================================================================

// PartitionManager handles partition distribution across proxies
type PartitionManager struct {
	mu           sync.RWMutex
	proxies      map[string][]*pb.PartitionRange // proxy_id → partition ranges
	partitionMap map[int32]string                // partition_id → proxy_id
}

// NewPartitionManager creates a new partition manager
func NewPartitionManager() *PartitionManager {
	return &PartitionManager{
		proxies:      make(map[string][]*pb.PartitionRange),
		partitionMap: make(map[int32]string),
	}
}

// HashNamespace calculates partition ID for a namespace using consistent hashing
func (pm *PartitionManager) HashNamespace(namespace string) int32 {
	hash := crc32.ChecksumIEEE([]byte(namespace))
	return int32(hash % 256) // 256 partitions (0-255)
}

// AssignRanges assigns partition ranges to a proxy using round-robin distribution
func (pm *PartitionManager) AssignRanges(proxyID string) []*pb.PartitionRange {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if proxy already has assignments
	if existing, ok := pm.proxies[proxyID]; ok {
		return existing
	}

	// Calculate range size based on number of proxies
	proxyCount := len(pm.proxies) + 1 // +1 for new proxy
	rangeSize := 256 / proxyCount

	// Calculate start/end for this proxy
	proxyIndex := len(pm.proxies)
	start := int32(proxyIndex * rangeSize)
	end := int32(start + int32(rangeSize) - 1)

	// Last proxy gets remaining partitions
	if end > 255 {
		end = 255
	}
	if proxyIndex == proxyCount-1 {
		end = 255
	}

	ranges := []*pb.PartitionRange{{Start: start, End: end}}
	pm.proxies[proxyID] = ranges

	// Update partition map
	for i := start; i <= end; i++ {
		pm.partitionMap[i] = proxyID
	}

	fmt.Printf("[PartitionManager] Assigned partitions [%d-%d] to proxy %s\n", start, end, proxyID)

	return ranges
}

// GetProxyForPartition returns the proxy ID assigned to a partition
func (pm *PartitionManager) GetProxyForPartition(partitionID int32) (string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proxyID, ok := pm.partitionMap[partitionID]
	if !ok {
		return "", fmt.Errorf("no proxy assigned to partition %d", partitionID)
	}

	return proxyID, nil
}
