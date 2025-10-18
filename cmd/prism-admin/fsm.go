package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	adminpb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/admin"
	"google.golang.org/protobuf/proto"
)

// AdminStateMachine implements raft.FSM for admin cluster state management.
// It processes commands from the Raft log and maintains the complete admin state.
//
// Thread-safety: All operations are protected by mu (RWMutex).
// Reads use RLock, writes use Lock.
//
// State Synchronization: All FSM state changes are automatically persisted to
// the local database for durability and querying. This ensures replicas maintain
// a consistent view of the distributed state.
//
// Reference: RFC-038 Admin Leader Election and High Availability with Raft
type AdminStateMachine struct {
	mu sync.RWMutex

	// All admin cluster state (versioned for schema evolution)
	state *adminpb.AdminState

	// Snapshot metadata
	lastAppliedIndex uint64
	lastAppliedTerm  uint64

	// Storage for persisting state to local database
	storage *Storage

	// Metrics
	metrics *AdminMetrics

	// Logger for FSM operations
	log *slog.Logger
}

// NewAdminStateMachine creates a new admin state machine with initial state
func NewAdminStateMachine(log *slog.Logger, metrics *AdminMetrics, storage *Storage) *AdminStateMachine {
	return &AdminStateMachine{
		state: &adminpb.AdminState{
			Version:    1, // Schema version
			Namespaces: make(map[string]*adminpb.NamespaceEntry),
			Proxies:    make(map[string]*adminpb.ProxyEntry),
			Launchers:  make(map[string]*adminpb.LauncherEntry),
			Patterns:   make(map[string]*adminpb.PatternEntry),
		},
		storage: storage,
		metrics: metrics,
		log:     log,
	}
}

// ====================================================================
// Raft FSM Interface Implementation
// ====================================================================

// Apply applies a Raft log entry to the FSM.
// This is called by Raft after log entry is committed to quorum.
// Must be deterministic and idempotent.
func (fsm *AdminStateMachine) Apply(log *raft.Log) interface{} {
	start := time.Now()

	// Deserialize command from Raft log entry
	var cmd adminpb.Command
	if err := proto.Unmarshal(log.Data, &cmd); err != nil {
		fsm.log.Error("failed to unmarshal command",
			"index", log.Index,
			"term", log.Term,
			"error", err)
		return err
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// Update last applied metadata
	fsm.lastAppliedIndex = log.Index
	fsm.lastAppliedTerm = log.Term
	fsm.state.LastAppliedIndex = int64(log.Index)
	fsm.state.LastAppliedTerm = int64(log.Term)
	fsm.state.StateUpdatedAt = time.Now().Unix()

	// Dispatch command to appropriate handler
	var err error
	commandType := cmd.Type.String()
	switch cmd.Type {
	case adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE:
		err = fsm.applyCreateNamespace(cmd.GetCreateNamespace())
	case adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY:
		err = fsm.applyRegisterProxy(cmd.GetRegisterProxy())
	case adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER:
		err = fsm.applyRegisterLauncher(cmd.GetRegisterLauncher())
	case adminpb.CommandType_COMMAND_TYPE_ASSIGN_PATTERN:
		err = fsm.applyAssignPattern(cmd.GetAssignPattern())
	case adminpb.CommandType_COMMAND_TYPE_UPDATE_PROXY_STATUS:
		err = fsm.applyUpdateProxyStatus(cmd.GetUpdateProxyStatus())
	case adminpb.CommandType_COMMAND_TYPE_UPDATE_LAUNCHER_STATUS:
		err = fsm.applyUpdateLauncherStatus(cmd.GetUpdateLauncherStatus())
	default:
		err = fmt.Errorf("unknown command type: %v", cmd.Type)
	}

	// Record metrics
	duration := time.Since(start)
	if fsm.metrics != nil {
		fsm.metrics.RecordFSMCommand(commandType, err == nil, duration)
	}

	if err != nil {
		fsm.log.Error("command apply failed",
			"type", cmd.Type,
			"index", log.Index,
			"error", err)
		return err
	}

	fsm.log.Debug("command applied successfully",
		"type", cmd.Type,
		"index", log.Index,
		"term", log.Term,
		"duration_ms", duration.Milliseconds())

	return nil
}

// Snapshot returns FSM snapshot for log compaction.
// Raft calls this periodically to create snapshots of the FSM state.
// The snapshot should capture the entire state atomically.
func (fsm *AdminStateMachine) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	// Create atomic snapshot of entire state
	snapshot := &AdminSnapshot{
		LastAppliedIndex: fsm.lastAppliedIndex,
		LastAppliedTerm:  fsm.lastAppliedTerm,
		SnapshotTime:     time.Now(),
		State:            proto.Clone(fsm.state).(*adminpb.AdminState), // Deep copy
	}

	// Record metrics
	if fsm.metrics != nil {
		fsm.metrics.RecordFSMSnapshot()
	}

	fsm.log.Info("created FSM snapshot",
		"index", fsm.lastAppliedIndex,
		"term", fsm.lastAppliedTerm,
		"namespaces", len(fsm.state.Namespaces),
		"proxies", len(fsm.state.Proxies),
		"launchers", len(fsm.state.Launchers),
		"patterns", len(fsm.state.Patterns))

	return snapshot, nil
}

// Restore restores FSM from snapshot.
// Called by Raft when loading snapshot from disk or receiving from leader.
func (fsm *AdminStateMachine) Restore(snapshot io.ReadCloser) error {
	defer snapshot.Close()

	// Decode snapshot using gob (binary serialization)
	var snap AdminSnapshot
	if err := gob.NewDecoder(snapshot).Decode(&snap); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// Atomic restore of entire state
	fsm.state = snap.State
	fsm.lastAppliedIndex = snap.LastAppliedIndex
	fsm.lastAppliedTerm = snap.LastAppliedTerm

	// Record metrics
	if fsm.metrics != nil {
		fsm.metrics.RecordFSMRestore()
	}

	fsm.log.Info("restored FSM from snapshot",
		"index", fsm.lastAppliedIndex,
		"term", fsm.lastAppliedTerm,
		"snapshot_time", snap.SnapshotTime,
		"namespaces", len(fsm.state.Namespaces),
		"proxies", len(fsm.state.Proxies),
		"launchers", len(fsm.state.Launchers),
		"patterns", len(fsm.state.Patterns))

	return nil
}

// ====================================================================
// Command Handlers (all protected by fsm.mu Lock)
// ====================================================================

// applyCreateNamespace creates a namespace entry in FSM state.
// Idempotent: If namespace exists, update config and assigned proxy.
// Also persists to local database for durability.
func (fsm *AdminStateMachine) applyCreateNamespace(cmd *adminpb.CreateNamespaceCommand) error {
	now := time.Now().Unix()

	// Check if namespace already exists
	if existing, ok := fsm.state.Namespaces[cmd.Namespace]; ok {
		// Update existing namespace (idempotent retry)
		existing.Config = cmd.Config
		existing.AssignedProxy = cmd.AssignedProxy
		existing.UpdatedAt = now
		fsm.log.Debug("updated existing namespace",
			"namespace", cmd.Namespace,
			"partition", cmd.PartitionId,
			"proxy", cmd.AssignedProxy)
		return nil
	}

	// Create new namespace entry in FSM
	fsm.state.Namespaces[cmd.Namespace] = &adminpb.NamespaceEntry{
		Name:          cmd.Namespace,
		Description:   fmt.Sprintf("Created by %s", cmd.Principal),
		PartitionId:   cmd.PartitionId,
		AssignedProxy: cmd.AssignedProxy,
		Config:        cmd.Config,
		CreatedBy:     cmd.Principal,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Sync to local database
	if fsm.storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Convert to storage model
		ns := &Namespace{
			Name:        cmd.Namespace,
			Description: fmt.Sprintf("Created by %s (partition %d, proxy %s)", cmd.Principal, cmd.PartitionId, cmd.AssignedProxy),
			Metadata:    nil, // TODO: Convert protobuf config to JSON if needed
		}

		if err := fsm.storage.CreateNamespace(ctx, ns); err != nil {
			// Log but don't fail - FSM state is source of truth
			fsm.log.Warn("failed to sync namespace to storage",
				"namespace", cmd.Namespace,
				"error", err)
		}
	}

	fsm.log.Info("created namespace",
		"namespace", cmd.Namespace,
		"partition", cmd.PartitionId,
		"proxy", cmd.AssignedProxy,
		"principal", cmd.Principal)

	return nil
}

// applyRegisterProxy registers or updates a proxy entry.
// Idempotent: Always updates proxy with latest registration info.
// Also persists to local database for durability.
func (fsm *AdminStateMachine) applyRegisterProxy(cmd *adminpb.RegisterProxyCommand) error {
	now := time.Now().Unix()
	nowTime := time.Unix(now, 0)

	// Check if proxy already exists
	isUpdate := false
	if existing, ok := fsm.state.Proxies[cmd.ProxyId]; ok {
		// Update existing proxy (heartbeat or re-registration)
		existing.Address = cmd.Address
		existing.Version = cmd.Version
		existing.Capabilities = cmd.Capabilities
		existing.Metadata = cmd.Metadata
		existing.Status = "healthy"
		existing.LastSeen = now
		isUpdate = true
		fsm.log.Debug("updated existing proxy",
			"proxy_id", cmd.ProxyId,
			"address", cmd.Address,
			"version", cmd.Version)
	} else {
		// Create new proxy entry
		fsm.state.Proxies[cmd.ProxyId] = &adminpb.ProxyEntry{
			ProxyId:      cmd.ProxyId,
			Address:      cmd.Address,
			Region:       cmd.Region,
			Version:      cmd.Version,
			Capabilities: cmd.Capabilities,
			Metadata:     cmd.Metadata,
			Status:       "healthy",
			LastSeen:     now,
			RegisteredAt: now,
		}

		fsm.log.Info("registered proxy",
			"proxy_id", cmd.ProxyId,
			"address", cmd.Address,
			"region", cmd.Region,
			"version", cmd.Version)
	}

	// Sync to local database
	if fsm.storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		proxy := &Proxy{
			ProxyID:  cmd.ProxyId,
			Address:  cmd.Address,
			Version:  cmd.Version,
			Status:   "healthy",
			LastSeen: &nowTime,
			Metadata: nil, // TODO: Convert protobuf metadata to JSON
		}

		if err := fsm.storage.UpsertProxy(ctx, proxy); err != nil {
			fsm.log.Warn("failed to sync proxy to storage",
				"proxy_id", cmd.ProxyId,
				"error", err)
		}
	}

	if !isUpdate {
		fsm.log.Info("registered proxy",
			"proxy_id", cmd.ProxyId,
			"address", cmd.Address,
			"region", cmd.Region)
	}

	return nil
}

// applyRegisterLauncher registers or updates a launcher entry.
// Idempotent: Always updates launcher with latest registration info.
// Also persists to local database for durability.
func (fsm *AdminStateMachine) applyRegisterLauncher(cmd *adminpb.RegisterLauncherCommand) error {
	now := time.Now().Unix()
	nowTime := time.Unix(now, 0)
	isUpdate := false

	// Check if launcher already exists
	if existing, ok := fsm.state.Launchers[cmd.LauncherId]; ok {
		// Update existing launcher
		existing.Address = cmd.Address
		existing.Version = cmd.Version
		existing.Capabilities = cmd.Capabilities
		existing.ProcessTypes = cmd.ProcessTypes
		existing.MaxProcesses = cmd.MaxProcesses
		existing.Metadata = cmd.Metadata
		existing.Status = "healthy"
		existing.LastSeen = now
		existing.AvailableSlots = cmd.MaxProcesses // Reset on re-registration
		isUpdate = true
		fsm.log.Debug("updated existing launcher",
			"launcher_id", cmd.LauncherId,
			"address", cmd.Address,
			"version", cmd.Version)
	} else {
		// Create new launcher entry
		fsm.state.Launchers[cmd.LauncherId] = &adminpb.LauncherEntry{
			LauncherId:     cmd.LauncherId,
			Address:        cmd.Address,
			Region:         cmd.Region,
			Version:        cmd.Version,
			Capabilities:   cmd.Capabilities,
			ProcessTypes:   cmd.ProcessTypes,
			MaxProcesses:   cmd.MaxProcesses,
			Metadata:       cmd.Metadata,
			Status:         "healthy",
			LastSeen:       now,
			RegisteredAt:   now,
			AvailableSlots: cmd.MaxProcesses,
		}

		fsm.log.Info("registered launcher",
			"launcher_id", cmd.LauncherId,
			"address", cmd.Address,
			"region", cmd.Region,
			"max_processes", cmd.MaxProcesses)
	}

	// Sync to local database
	if fsm.storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		launcher := &Launcher{
			LauncherID:     cmd.LauncherId,
			Address:        cmd.Address,
			Region:         cmd.Region,
			Version:        cmd.Version,
			Status:         "healthy",
			MaxProcesses:   cmd.MaxProcesses,
			AvailableSlots: cmd.MaxProcesses,
			Capabilities:   nil, // TODO: Convert protobuf capabilities to JSON
			LastSeen:       &nowTime,
			Metadata:       nil, // TODO: Convert protobuf metadata to JSON
		}

		if err := fsm.storage.UpsertLauncher(ctx, launcher); err != nil {
			fsm.log.Warn("failed to sync launcher to storage",
				"launcher_id", cmd.LauncherId,
				"error", err)
		}
	}

	if !isUpdate {
		fsm.log.Info("registered launcher",
			"launcher_id", cmd.LauncherId,
			"address", cmd.Address,
			"region", cmd.Region)
	}

	return nil
}

// applyAssignPattern assigns a pattern to a launcher.
// Idempotent: If pattern already assigned, update config.
func (fsm *AdminStateMachine) applyAssignPattern(cmd *adminpb.AssignPatternCommand) error {
	now := time.Now().Unix()

	// Check if pattern already exists
	if existing, ok := fsm.state.Patterns[cmd.PatternId]; ok {
		// Update existing pattern assignment
		existing.LauncherId = cmd.LauncherId
		existing.Config = cmd.Config
		existing.UpdatedAt = now
		fsm.log.Debug("updated existing pattern",
			"pattern_id", cmd.PatternId,
			"launcher_id", cmd.LauncherId)
		return nil
	}

	// Create new pattern entry
	fsm.state.Patterns[cmd.PatternId] = &adminpb.PatternEntry{
		PatternId:   cmd.PatternId,
		PatternType: cmd.PatternType,
		LauncherId:  cmd.LauncherId,
		Namespace:   cmd.Namespace,
		Config:      cmd.Config,
		Status:      "running", // Initial status
		AssignedAt:  now,
		UpdatedAt:   now,
	}

	// Decrement launcher available slots
	if launcher, ok := fsm.state.Launchers[cmd.LauncherId]; ok {
		if launcher.AvailableSlots > 0 {
			launcher.AvailableSlots--
		}
	}

	fsm.log.Info("assigned pattern",
		"pattern_id", cmd.PatternId,
		"pattern_type", cmd.PatternType,
		"launcher_id", cmd.LauncherId,
		"namespace", cmd.Namespace)

	return nil
}

// applyUpdateProxyStatus updates proxy health status from heartbeat.
func (fsm *AdminStateMachine) applyUpdateProxyStatus(cmd *adminpb.UpdateProxyStatusCommand) error {
	proxy, ok := fsm.state.Proxies[cmd.ProxyId]
	if !ok {
		// Proxy not registered - this can happen if heartbeat arrives before registration
		fsm.log.Warn("received status update for unregistered proxy",
			"proxy_id", cmd.ProxyId)
		return fmt.Errorf("proxy not registered: %s", cmd.ProxyId)
	}

	proxy.Status = cmd.Status
	proxy.LastSeen = cmd.LastSeen
	if cmd.Resources != nil {
		proxy.Resources = cmd.Resources
	}

	fsm.log.Debug("updated proxy status",
		"proxy_id", cmd.ProxyId,
		"status", cmd.Status,
		"last_seen", cmd.LastSeen)

	return nil
}

// applyUpdateLauncherStatus updates launcher health status from heartbeat.
func (fsm *AdminStateMachine) applyUpdateLauncherStatus(cmd *adminpb.UpdateLauncherStatusCommand) error {
	launcher, ok := fsm.state.Launchers[cmd.LauncherId]
	if !ok {
		// Launcher not registered
		fsm.log.Warn("received status update for unregistered launcher",
			"launcher_id", cmd.LauncherId)
		return fmt.Errorf("launcher not registered: %s", cmd.LauncherId)
	}

	launcher.Status = cmd.Status
	launcher.LastSeen = cmd.LastSeen
	launcher.AvailableSlots = cmd.AvailableSlots
	if cmd.Resources != nil {
		launcher.Resources = cmd.Resources
	}

	fsm.log.Debug("updated launcher status",
		"launcher_id", cmd.LauncherId,
		"status", cmd.Status,
		"available_slots", cmd.AvailableSlots)

	return nil
}

// ====================================================================
// Query Methods (read-only, use RLock)
// ====================================================================

// GetNamespace retrieves a namespace by name
func (fsm *AdminStateMachine) GetNamespace(name string) (*adminpb.NamespaceEntry, bool) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	ns, ok := fsm.state.Namespaces[name]
	return ns, ok
}

// GetProxy retrieves a proxy by ID
func (fsm *AdminStateMachine) GetProxy(proxyID string) (*adminpb.ProxyEntry, bool) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	proxy, ok := fsm.state.Proxies[proxyID]
	return proxy, ok
}

// GetAllProxies returns all registered proxies
func (fsm *AdminStateMachine) GetAllProxies() []*adminpb.ProxyEntry {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	proxies := make([]*adminpb.ProxyEntry, 0, len(fsm.state.Proxies))
	for _, p := range fsm.state.Proxies {
		proxies = append(proxies, p)
	}
	return proxies
}

// GetHealthyLaunchers returns all healthy launchers
func (fsm *AdminStateMachine) GetHealthyLaunchers() []*adminpb.LauncherEntry {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	launchers := make([]*adminpb.LauncherEntry, 0)
	for _, l := range fsm.state.Launchers {
		if l.Status == "healthy" && l.AvailableSlots > 0 {
			launchers = append(launchers, l)
		}
	}
	return launchers
}

// GetPatternCountForLauncher returns number of patterns assigned to launcher
func (fsm *AdminStateMachine) GetPatternCountForLauncher(launcherID string) int32 {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	count := int32(0)
	for _, p := range fsm.state.Patterns {
		if p.LauncherId == launcherID {
			count++
		}
	}
	return count
}

// LastAppliedIndex returns the last applied Raft log index
func (fsm *AdminStateMachine) LastAppliedIndex() uint64 {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.lastAppliedIndex
}

// LastAppliedTerm returns the last applied Raft term
func (fsm *AdminStateMachine) LastAppliedTerm() uint64 {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.lastAppliedTerm
}

// ====================================================================
// AdminSnapshot (implements raft.FSMSnapshot)
// ====================================================================

// AdminSnapshot captures complete FSM state at a point in time
// Fields are exported for gob encoding/decoding
type AdminSnapshot struct {
	LastAppliedIndex uint64
	LastAppliedTerm  uint64
	SnapshotTime     time.Time
	State            *adminpb.AdminState
}

// Persist writes snapshot to sink.
// Called by Raft to save snapshot to disk.
func (s *AdminSnapshot) Persist(sink raft.SnapshotSink) error {
	// Encode snapshot using gob (efficient binary serialization)
	encoder := gob.NewEncoder(sink)
	if err := encoder.Encode(s); err != nil {
		sink.Cancel()
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}

	// Close the sink (commits the snapshot)
	return sink.Close()
}

// Release is called when snapshot is no longer needed.
// We don't need to do anything since we don't hold external resources.
func (s *AdminSnapshot) Release() {
	// No-op: snapshot doesn't hold any external resources
}
