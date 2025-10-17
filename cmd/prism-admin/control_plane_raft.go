package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	adminpb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/admin"
	pb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ReadConsistency defines read consistency levels
type ReadConsistency int

const (
	ReadStale        ReadConsistency = 0 // Read from local FSM (may be stale up to 200ms)
	ReadLeaseCheck   ReadConsistency = 1 // Leader confirms lease before read
	ReadLinearizable ReadConsistency = 2 // Leader + quorum check (slowest, strongest)
)

// ControlPlaneServiceRaft wraps ControlPlaneService with Raft integration
type ControlPlaneServiceRaft struct {
	pb.UnimplementedControlPlaneServer

	// Raft components
	raft       *RaftNode
	fsm        *AdminStateMachine
	partitions *PartitionManager

	// Configuration
	readConsistency map[string]ReadConsistency

	// Leader connection pool for forwarding
	leaderConnPool *LeaderConnectionPool

	// Logger
	log *slog.Logger
}

// NewControlPlaneServiceRaft creates a Raft-integrated control plane service
func NewControlPlaneServiceRaft(
	raft *RaftNode,
	fsm *AdminStateMachine,
	partitions *PartitionManager,
	readConsistencyConfig map[string]string,
	log *slog.Logger,
) *ControlPlaneServiceRaft {
	// Parse read consistency configuration
	consistency := make(map[string]ReadConsistency)
	for op, level := range readConsistencyConfig {
		switch level {
		case "stale":
			consistency[op] = ReadStale
		case "lease-based", "lease_based":
			consistency[op] = ReadLeaseCheck
		case "linearizable":
			consistency[op] = ReadLinearizable
		default:
			consistency[op] = ReadStale // Default to stale
		}
	}

	return &ControlPlaneServiceRaft{
		raft:            raft,
		fsm:             fsm,
		partitions:      partitions,
		readConsistency: consistency,
		leaderConnPool:  NewLeaderConnectionPool(),
		log:             log,
	}
}

// ====================================================================
// Proxy RPCs (ADR-055) - Raft-integrated
// ====================================================================

// RegisterProxy registers a proxy instance via Raft consensus
func (s *ControlPlaneServiceRaft) RegisterProxy(
	ctx context.Context,
	req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
	s.log.Info("proxy registration request",
		"proxy_id", req.ProxyId,
		"address", req.Address,
		"region", req.Region)

	// Check if leader
	if !s.raft.IsLeader() {
		// Forward to leader
		return s.forwardRegisterProxy(ctx, req)
	}

	// Build Raft command
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin", // Could be extracted from auth context
		Payload: &adminpb.Command_RegisterProxy{
			RegisterProxy: &adminpb.RegisterProxyCommand{
				ProxyId:      req.ProxyId,
				Address:      req.Address,
				Region:       req.Region,
				Version:      req.Version,
				Capabilities: req.Capabilities,
				Metadata:     req.Metadata,
			},
		},
	}

	// Serialize and propose to Raft
	data, err := proto.Marshal(cmd)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal command: %v", err)
	}

	// Propose to Raft (blocks until committed to quorum)
	proposeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := s.raft.Propose(proposeCtx, data); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to propose to raft: %v", err)
	}

	s.log.Info("proxy registered via raft", "proxy_id", req.ProxyId)

	// Compute partition ranges (NOT stored in FSM - computed on-demand)
	ranges := s.computePartitionRanges(req.ProxyId)

	// Get initial namespace assignments (read from FSM with stale consistency)
	namespaces := s.getNamespacesForRanges(ranges)

	return &pb.ProxyRegistrationAck{
		Success:           true,
		Message:           "Proxy registered successfully",
		InitialNamespaces: namespaces,
		PartitionRanges:   ranges,
	}, nil
}

// CreateNamespace handles client-initiated namespace creation via Raft
func (s *ControlPlaneServiceRaft) CreateNamespace(
	ctx context.Context,
	req *pb.CreateNamespaceRequest,
) (*pb.CreateNamespaceResponse, error) {
	s.log.Info("namespace creation request",
		"namespace", req.Namespace,
		"requesting_proxy", req.RequestingProxy,
		"principal", req.Principal)

	// Check if leader
	if !s.raft.IsLeader() {
		return s.forwardCreateNamespace(ctx, req)
	}

	// Calculate partition ID (deterministic consistent hashing)
	partitionID := s.partitions.HashNamespace(req.Namespace)

	// Find proxy assigned to this partition (computed from proxy set in FSM)
	proxyID, err := s.getProxyForPartition(partitionID)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"no proxy assigned to partition %d: %v", partitionID, err)
	}

	// Build Raft command
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
		Timestamp: time.Now().Unix(),
		Issuer:    req.Principal,
		Payload: &adminpb.Command_CreateNamespace{
			CreateNamespace: &adminpb.CreateNamespaceCommand{
				Namespace:     req.Namespace,
				PartitionId:   partitionID,
				AssignedProxy: proxyID,
				Config:        req.Config,
				Principal:     req.Principal,
			},
		},
	}

	// Serialize and propose
	data, err := proto.Marshal(cmd)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal command: %v", err)
	}

	proposeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := s.raft.Propose(proposeCtx, data); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to propose to raft: %v", err)
	}

	s.log.Info("namespace created via raft",
		"namespace", req.Namespace,
		"partition", partitionID,
		"proxy", proxyID)

	// TODO: Push NamespaceAssignment to the assigned proxy

	return &pb.CreateNamespaceResponse{
		Success:           true,
		Message:           "Namespace created successfully",
		AssignedPartition: partitionID,
		AssignedProxy:     proxyID,
	}, nil
}

// Heartbeat receives periodic health updates from proxies
// This is a write operation (updates last_seen), so goes through Raft
func (s *ControlPlaneServiceRaft) Heartbeat(
	ctx context.Context,
	req *pb.ProxyHeartbeat,
) (*pb.HeartbeatAck, error) {
	// Heartbeats use stale reads by default (high volume, low latency)
	// We still update state via Raft but don't block waiting for quorum

	// Check if leader
	if !s.raft.IsLeader() {
		// Forward to leader (async, don't block)
		go s.forwardHeartbeat(ctx, req)
		// Return immediately with success
		return &pb.HeartbeatAck{
			Success:         true,
			Message:         "Heartbeat forwarded to leader",
			ServerTimestamp: time.Now().Unix(),
		}, nil
	}

	// Build command for proxy status update
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_UPDATE_PROXY_STATUS,
		Timestamp: req.Timestamp,
		Issuer:    req.ProxyId,
		Payload: &adminpb.Command_UpdateProxyStatus{
			UpdateProxyStatus: &adminpb.UpdateProxyStatusCommand{
				ProxyId:   req.ProxyId,
				Status:    "healthy",
				LastSeen:  req.Timestamp,
				Resources: req.Resources,
			},
		},
	}

	// Serialize and propose (async, don't block)
	data, err := proto.Marshal(cmd)
	if err != nil {
		s.log.Warn("failed to marshal heartbeat command", "error", err)
		return &pb.HeartbeatAck{
			Success:         true,
			Message:         "Heartbeat received (serialization error)",
			ServerTimestamp: time.Now().Unix(),
		}, nil
	}

	// Propose asynchronously
	go func() {
		proposeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.raft.Propose(proposeCtx, data); err != nil {
			s.log.Warn("failed to propose heartbeat", "proxy_id", req.ProxyId, "error", err)
		}
	}()

	return &pb.HeartbeatAck{
		Success:         true,
		Message:         "Heartbeat received",
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

// AssignNamespace pushes namespace configuration to proxy
func (s *ControlPlaneServiceRaft) AssignNamespace(
	ctx context.Context,
	req *pb.NamespaceAssignment,
) (*pb.NamespaceAssignmentAck, error) {
	// This is typically called internally after CreateNamespace
	// For now, just acknowledge (actual push happens via separate mechanism)
	return &pb.NamespaceAssignmentAck{
		Success: true,
		Message: "Namespace assignment acknowledged",
	}, nil
}

// RevokeNamespace removes namespace from proxy
func (s *ControlPlaneServiceRaft) RevokeNamespace(
	ctx context.Context,
	req *pb.NamespaceRevocation,
) (*pb.NamespaceRevocationAck, error) {
	// TODO: Implement namespace revocation via Raft
	return &pb.NamespaceRevocationAck{
		Success:   true,
		Message:   "Namespace revocation not yet implemented",
		RevokedAt: time.Now().Unix(),
	}, nil
}

// ====================================================================
// Launcher RPCs (ADR-056, ADR-057) - Raft-integrated
// ====================================================================

// RegisterLauncher registers a launcher instance via Raft
func (s *ControlPlaneServiceRaft) RegisterLauncher(
	ctx context.Context,
	req *pb.LauncherRegistration,
) (*pb.LauncherRegistrationAck, error) {
	s.log.Info("launcher registration request",
		"launcher_id", req.LauncherId,
		"address", req.Address,
		"max_processes", req.MaxProcesses)

	// Check if leader
	if !s.raft.IsLeader() {
		return s.forwardRegisterLauncher(ctx, req)
	}

	// Build Raft command
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterLauncher{
			RegisterLauncher: &adminpb.RegisterLauncherCommand{
				LauncherId:   req.LauncherId,
				Address:      req.Address,
				Region:       req.Region,
				Version:      req.Version,
				Capabilities: req.Capabilities,
				ProcessTypes: req.ProcessTypes,
				MaxProcesses: req.MaxProcesses,
				Metadata:     req.Metadata,
			},
		},
	}

	// Serialize and propose
	data, err := proto.Marshal(cmd)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal command: %v", err)
	}

	proposeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := s.raft.Propose(proposeCtx, data); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to propose to raft: %v", err)
	}

	s.log.Info("launcher registered via raft", "launcher_id", req.LauncherId)

	// TODO: Get initial process assignments

	return &pb.LauncherRegistrationAck{
		Success:          true,
		Message:          "Launcher registered successfully",
		InitialProcesses: []*pb.ProcessAssignment{},
		AssignedCapacity: 0,
	}, nil
}

// LauncherHeartbeat receives periodic health updates from launchers
func (s *ControlPlaneServiceRaft) LauncherHeartbeat(
	ctx context.Context,
	req *pb.LauncherHeartbeatRequest,
) (*pb.HeartbeatAck, error) {
	// Similar to proxy heartbeat - async Raft update
	if !s.raft.IsLeader() {
		go s.forwardLauncherHeartbeat(ctx, req)
		return &pb.HeartbeatAck{
			Success:         true,
			Message:         "Heartbeat forwarded to leader",
			ServerTimestamp: time.Now().Unix(),
		}, nil
	}

	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_UPDATE_LAUNCHER_STATUS,
		Timestamp: req.Timestamp,
		Issuer:    req.LauncherId,
		Payload: &adminpb.Command_UpdateLauncherStatus{
			UpdateLauncherStatus: &adminpb.UpdateLauncherStatusCommand{
				LauncherId:     req.LauncherId,
				Status:         "healthy",
				LastSeen:       req.Timestamp,
				AvailableSlots: req.Resources.AvailableSlots,
				Resources:      req.Resources,
			},
		},
	}

	data, err := proto.Marshal(cmd)
	if err != nil {
		s.log.Warn("failed to marshal launcher heartbeat", "error", err)
		return &pb.HeartbeatAck{
			Success:         true,
			Message:         "Heartbeat received",
			ServerTimestamp: time.Now().Unix(),
		}, nil
	}

	go func() {
		proposeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.raft.Propose(proposeCtx, data); err != nil {
			s.log.Warn("failed to propose launcher heartbeat",
				"launcher_id", req.LauncherId,
				"error", err)
		}
	}()

	return &pb.HeartbeatAck{
		Success:         true,
		Message:         "Heartbeat received",
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

// AssignProcess pushes process assignment to launcher
func (s *ControlPlaneServiceRaft) AssignProcess(
	ctx context.Context,
	req *pb.ProcessAssignment,
) (*pb.ProcessAssignmentAck, error) {
	// TODO: Implement process assignment via Raft
	return &pb.ProcessAssignmentAck{
		Success: true,
		Message: "Process assignment not yet implemented",
	}, nil
}

// RevokeProcess removes process from launcher
func (s *ControlPlaneServiceRaft) RevokeProcess(
	ctx context.Context,
	req *pb.ProcessRevocation,
) (*pb.ProcessRevocationAck, error) {
	// TODO: Implement process revocation via Raft
	return &pb.ProcessRevocationAck{
		Success:   true,
		Message:   "Process revocation not yet implemented",
		StoppedAt: time.Now().Unix(),
		ExitCode:  0,
	}, nil
}

// ====================================================================
// Helper Methods
// ====================================================================

// computePartitionRanges computes partition ranges for a proxy
// This is NOT stored in Raft FSM - computed on-demand from proxy set
func (s *ControlPlaneServiceRaft) computePartitionRanges(proxyID string) []*pb.PartitionRange {
	// Get all proxies from FSM
	proxies := s.fsm.GetAllProxies()

	// Use PartitionManager to compute ranges
	return s.partitions.ComputeRangesFromProxySet(proxyID, proxies)
}

// getProxyForPartition finds the proxy assigned to a partition
func (s *ControlPlaneServiceRaft) getProxyForPartition(partitionID int32) (string, error) {
	proxies := s.fsm.GetAllProxies()
	return s.partitions.GetProxyForPartitionFromSet(partitionID, proxies)
}

// getNamespacesForRanges retrieves namespaces that fall within partition ranges
func (s *ControlPlaneServiceRaft) getNamespacesForRanges(ranges []*pb.PartitionRange) []*pb.NamespaceAssignment {
	// This is a read operation - uses FSM directly (stale read)
	var assignments []*pb.NamespaceAssignment

	// Iterate through all namespaces in FSM
	for _, nsEntry := range s.fsm.state.Namespaces {
		// Check if namespace's partition falls within ranges
		for _, r := range ranges {
			if nsEntry.PartitionId >= r.Start && nsEntry.PartitionId <= r.End {
				assignments = append(assignments, &pb.NamespaceAssignment{
					Namespace:   nsEntry.Name,
					PartitionId: nsEntry.PartitionId,
					Config:      nsEntry.Config,
					Version:     1, // TODO: Track version
				})
				break
			}
		}
	}

	return assignments
}

// ====================================================================
// Follower Forwarding Methods
// ====================================================================

func (s *ControlPlaneServiceRaft) forwardRegisterProxy(
	ctx context.Context,
	req *pb.ProxyRegistration,
) (*pb.ProxyRegistrationAck, error) {
	leaderAddr := s.raft.GetLeaderAddr()
	if leaderAddr == "" {
		s.raft.metrics.RecordForwardedRequest("RegisterProxy", false)
		return nil, status.Error(codes.Unavailable, "no leader elected")
	}

	conn, err := s.leaderConnPool.GetConnection(leaderAddr)
	if err != nil {
		s.raft.metrics.RecordForwardedRequest("RegisterProxy", false)
		return nil, status.Errorf(codes.Unavailable, "failed to connect to leader: %v", err)
	}

	client := pb.NewControlPlaneClient(conn)
	resp, err := client.RegisterProxy(ctx, req)
	s.raft.metrics.RecordForwardedRequest("RegisterProxy", err == nil)
	return resp, err
}

func (s *ControlPlaneServiceRaft) forwardCreateNamespace(
	ctx context.Context,
	req *pb.CreateNamespaceRequest,
) (*pb.CreateNamespaceResponse, error) {
	leaderAddr := s.raft.GetLeaderAddr()
	if leaderAddr == "" {
		s.raft.metrics.RecordForwardedRequest("CreateNamespace", false)
		return nil, status.Error(codes.Unavailable, "no leader elected")
	}

	conn, err := s.leaderConnPool.GetConnection(leaderAddr)
	if err != nil {
		s.raft.metrics.RecordForwardedRequest("CreateNamespace", false)
		return nil, status.Errorf(codes.Unavailable, "failed to connect to leader: %v", err)
	}

	client := pb.NewControlPlaneClient(conn)
	resp, err := client.CreateNamespace(ctx, req)
	s.raft.metrics.RecordForwardedRequest("CreateNamespace", err == nil)
	return resp, err
}

func (s *ControlPlaneServiceRaft) forwardRegisterLauncher(
	ctx context.Context,
	req *pb.LauncherRegistration,
) (*pb.LauncherRegistrationAck, error) {
	leaderAddr := s.raft.GetLeaderAddr()
	if leaderAddr == "" {
		s.raft.metrics.RecordForwardedRequest("RegisterLauncher", false)
		return nil, status.Error(codes.Unavailable, "no leader elected")
	}

	conn, err := s.leaderConnPool.GetConnection(leaderAddr)
	if err != nil {
		s.raft.metrics.RecordForwardedRequest("RegisterLauncher", false)
		return nil, status.Errorf(codes.Unavailable, "failed to connect to leader: %v", err)
	}

	client := pb.NewControlPlaneClient(conn)
	resp, err := client.RegisterLauncher(ctx, req)
	s.raft.metrics.RecordForwardedRequest("RegisterLauncher", err == nil)
	return resp, err
}

func (s *ControlPlaneServiceRaft) forwardHeartbeat(ctx context.Context, req *pb.ProxyHeartbeat) {
	leaderAddr := s.raft.GetLeaderAddr()
	if leaderAddr == "" {
		s.raft.metrics.RecordForwardedRequest("Heartbeat", false)
		return
	}

	conn, err := s.leaderConnPool.GetConnection(leaderAddr)
	if err != nil {
		s.log.Warn("failed to forward heartbeat", "error", err)
		s.raft.metrics.RecordForwardedRequest("Heartbeat", false)
		return
	}

	client := pb.NewControlPlaneClient(conn)
	_, err = client.Heartbeat(ctx, req)
	s.raft.metrics.RecordForwardedRequest("Heartbeat", err == nil)
}

func (s *ControlPlaneServiceRaft) forwardLauncherHeartbeat(ctx context.Context, req *pb.LauncherHeartbeatRequest) {
	leaderAddr := s.raft.GetLeaderAddr()
	if leaderAddr == "" {
		s.raft.metrics.RecordForwardedRequest("LauncherHeartbeat", false)
		return
	}

	conn, err := s.leaderConnPool.GetConnection(leaderAddr)
	if err != nil {
		s.log.Warn("failed to forward launcher heartbeat", "error", err)
		s.raft.metrics.RecordForwardedRequest("LauncherHeartbeat", false)
		return
	}

	client := pb.NewControlPlaneClient(conn)
	_, err = client.LauncherHeartbeat(ctx, req)
	s.raft.metrics.RecordForwardedRequest("LauncherHeartbeat", err == nil)
}

// ====================================================================
// LeaderConnectionPool - connection pooling for leader forwarding
// ====================================================================

type LeaderConnectionPool struct {
	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn
}

func NewLeaderConnectionPool() *LeaderConnectionPool {
	return &LeaderConnectionPool{
		conns: make(map[string]*grpc.ClientConn),
	}
}

func (p *LeaderConnectionPool) GetConnection(addr string) (*grpc.ClientConn, error) {
	p.mu.RLock()
	if conn, ok := p.conns[addr]; ok {
		p.mu.RUnlock()
		return conn, nil
	}
	p.mu.RUnlock()

	// Create new connection
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, ok := p.conns[addr]; ok {
		return conn, nil
	}

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second))
	if err != nil {
		return nil, err
	}

	p.conns[addr] = conn
	return conn, nil
}

func (p *LeaderConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[string]*grpc.ClientConn)
}
