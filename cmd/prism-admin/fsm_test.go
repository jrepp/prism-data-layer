package main

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	adminpb "github.com/jrepp/prism-data-layer/pkg/plugin/gen/prism/admin"
	"google.golang.org/protobuf/proto"
)

// testFSM creates a test FSM without metrics/storage to avoid registration conflicts
func testFSM() *AdminStateMachine {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewAdminStateMachine(log, nil, nil) // nil metrics and storage for tests
}

// TestNewAdminStateMachine verifies initial state machine creation
func TestNewAdminStateMachine(t *testing.T) {
	fsm := testFSM()

	if fsm == nil {
		t.Fatal("expected non-nil FSM")
	}

	if fsm.state == nil {
		t.Fatal("expected non-nil state")
	}

	if fsm.state.Version != 1 {
		t.Errorf("expected version 1, got %d", fsm.state.Version)
	}

	if len(fsm.state.Namespaces) != 0 {
		t.Errorf("expected empty namespaces, got %d", len(fsm.state.Namespaces))
	}

	if len(fsm.state.Proxies) != 0 {
		t.Errorf("expected empty proxies, got %d", len(fsm.state.Proxies))
	}

	if len(fsm.state.Launchers) != 0 {
		t.Errorf("expected empty launchers, got %d", len(fsm.state.Launchers))
	}

	if len(fsm.state.Patterns) != 0 {
		t.Errorf("expected empty patterns, got %d", len(fsm.state.Patterns))
	}
}

// TestApplyCreateNamespace tests namespace creation via FSM
func TestApplyCreateNamespace(t *testing.T) {
	fsm := testFSM()

	// Create command
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
		Timestamp: time.Now().Unix(),
		Issuer:    "test-user",
		Payload: &adminpb.Command_CreateNamespace{
			CreateNamespace: &adminpb.CreateNamespaceCommand{
				Namespace:     "test-ns",
				PartitionId:   42,
				AssignedProxy: "proxy-01",
				Config:        nil, // Config not essential for this test
				Principal:     "test-user",
			},
		},
	}

	// Marshal command
	data, err := proto.Marshal(cmd)
	if err != nil {
		t.Fatalf("failed to marshal command: %v", err)
	}

	// Apply to FSM
	logEntry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  data,
	}

	result := fsm.Apply(logEntry)
	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify namespace was created
	ns, exists := fsm.GetNamespace("test-ns")
	if !exists {
		t.Fatal("expected namespace to exist")
	}

	if ns.Name != "test-ns" {
		t.Errorf("expected name 'test-ns', got '%s'", ns.Name)
	}

	if ns.PartitionId != 42 {
		t.Errorf("expected partition 42, got %d", ns.PartitionId)
	}

	if ns.AssignedProxy != "proxy-01" {
		t.Errorf("expected proxy 'proxy-01', got '%s'", ns.AssignedProxy)
	}

	if ns.CreatedBy != "test-user" {
		t.Errorf("expected created_by 'test-user', got '%s'", ns.CreatedBy)
	}

	// Verify FSM metadata updated
	if fsm.lastAppliedIndex != 1 {
		t.Errorf("expected last applied index 1, got %d", fsm.lastAppliedIndex)
	}

	if fsm.lastAppliedTerm != 1 {
		t.Errorf("expected last applied term 1, got %d", fsm.lastAppliedTerm)
	}
}

// TestApplyCreateNamespaceIdempotent verifies namespace creation is idempotent
func TestApplyCreateNamespaceIdempotent(t *testing.T) {
	fsm := testFSM()

	// Create command
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
		Timestamp: time.Now().Unix(),
		Issuer:    "test-user",
		Payload: &adminpb.Command_CreateNamespace{
			CreateNamespace: &adminpb.CreateNamespaceCommand{
				Namespace:     "test-ns",
				PartitionId:   42,
				AssignedProxy: "proxy-01",
				Config:        nil, // Config not essential for this test
				Principal:     "test-user",
			},
		},
	}

	data, _ := proto.Marshal(cmd)
	logEntry1 := &raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data}

	// Apply first time
	fsm.Apply(logEntry1)

	// Update command with new proxy assignment
	cmd.GetCreateNamespace().AssignedProxy = "proxy-02"
	data, _ = proto.Marshal(cmd)
	logEntry2 := &raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: data}

	// Apply second time (idempotent update)
	fsm.Apply(logEntry2)

	// Verify namespace was updated (not duplicated)
	ns, _ := fsm.GetNamespace("test-ns")
	if ns.AssignedProxy != "proxy-02" {
		t.Errorf("expected proxy 'proxy-02', got '%s'", ns.AssignedProxy)
	}

	// Verify only one namespace exists
	if len(fsm.state.Namespaces) != 1 {
		t.Errorf("expected 1 namespace, got %d", len(fsm.state.Namespaces))
	}
}

// TestApplyRegisterProxy tests proxy registration
func TestApplyRegisterProxy(t *testing.T) {
	fsm := testFSM()



	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterProxy{
			RegisterProxy: &adminpb.RegisterProxyCommand{
				ProxyId:      "proxy-01",
				Address:      "proxy-01:8080",
				Region:       "us-west-2",
				Version:      "1.0.0",
				Capabilities: []string{"kafka", "redis"},
				Metadata:     map[string]string{"az": "us-west-2a"},
			},
		},
	}

	data, _ := proto.Marshal(cmd)
	logEntry := &raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data}

	result := fsm.Apply(logEntry)
	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify proxy was registered
	proxy, exists := fsm.GetProxy("proxy-01")
	if !exists {
		t.Fatal("expected proxy to exist")
	}

	if proxy.ProxyId != "proxy-01" {
		t.Errorf("expected proxy_id 'proxy-01', got '%s'", proxy.ProxyId)
	}

	if proxy.Address != "proxy-01:8080" {
		t.Errorf("expected address 'proxy-01:8080', got '%s'", proxy.Address)
	}

	if proxy.Region != "us-west-2" {
		t.Errorf("expected region 'us-west-2', got '%s'", proxy.Region)
	}

	if proxy.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", proxy.Status)
	}

	if len(proxy.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(proxy.Capabilities))
	}
}

// TestApplyRegisterProxyIdempotent verifies proxy registration is idempotent
func TestApplyRegisterProxyIdempotent(t *testing.T) {
	fsm := testFSM()



	// First registration
	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterProxy{
			RegisterProxy: &adminpb.RegisterProxyCommand{
				ProxyId: "proxy-01",
				Address: "proxy-01:8080",
				Version: "1.0.0",
			},
		},
	}

	data, _ := proto.Marshal(cmd)
	fsm.Apply(&raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data})

	// Second registration (update version)
	cmd.GetRegisterProxy().Version = "1.1.0"
	data, _ = proto.Marshal(cmd)
	fsm.Apply(&raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: data})

	// Verify proxy was updated (not duplicated)
	proxy, _ := fsm.GetProxy("proxy-01")
	if proxy.Version != "1.1.0" {
		t.Errorf("expected version '1.1.0', got '%s'", proxy.Version)
	}

	// Verify only one proxy exists
	proxies := fsm.GetAllProxies()
	if len(proxies) != 1 {
		t.Errorf("expected 1 proxy, got %d", len(proxies))
	}
}

// TestApplyRegisterLauncher tests launcher registration
func TestApplyRegisterLauncher(t *testing.T) {
	fsm := testFSM()



	cmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterLauncher{
			RegisterLauncher: &adminpb.RegisterLauncherCommand{
				LauncherId:   "launcher-01",
				Address:      "launcher-01:9090",
				Region:       "us-west-2",
				Version:      "1.0.0",
				ProcessTypes: []string{"postgres", "redis"},
				MaxProcesses: 10,
			},
		},
	}

	data, _ := proto.Marshal(cmd)
	logEntry := &raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data}

	result := fsm.Apply(logEntry)
	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify launcher exists in FSM state
	fsm.mu.RLock()
	launcher, exists := fsm.state.Launchers["launcher-01"]
	fsm.mu.RUnlock()

	if !exists {
		t.Fatal("expected launcher to exist")
	}

	if launcher.LauncherId != "launcher-01" {
		t.Errorf("expected launcher_id 'launcher-01', got '%s'", launcher.LauncherId)
	}

	if launcher.MaxProcesses != 10 {
		t.Errorf("expected max_processes 10, got %d", launcher.MaxProcesses)
	}

	if launcher.AvailableSlots != 10 {
		t.Errorf("expected available_slots 10, got %d", launcher.AvailableSlots)
	}

	if launcher.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", launcher.Status)
	}
}

// TestApplyAssignPattern tests pattern assignment
func TestApplyAssignPattern(t *testing.T) {
	fsm := testFSM()



	// First register a launcher
	registerCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterLauncher{
			RegisterLauncher: &adminpb.RegisterLauncherCommand{
				LauncherId:   "launcher-01",
				MaxProcesses: 10,
			},
		},
	}
	data, _ := proto.Marshal(registerCmd)
	fsm.Apply(&raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data})

	// Now assign a pattern
	assignCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_ASSIGN_PATTERN,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_AssignPattern{
			AssignPattern: &adminpb.AssignPatternCommand{
				PatternId:   "pattern-01",
				PatternType: "postgres-consumer",
				LauncherId:  "launcher-01",
				Namespace:   "test-ns",
				Config:      nil, // Config not essential for this test
			},
		},
	}

	data, _ = proto.Marshal(assignCmd)
	result := fsm.Apply(&raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: data})

	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify pattern was assigned
	fsm.mu.RLock()
	pattern, exists := fsm.state.Patterns["pattern-01"]
	fsm.mu.RUnlock()

	if !exists {
		t.Fatal("expected pattern to exist")
	}

	if pattern.PatternId != "pattern-01" {
		t.Errorf("expected pattern_id 'pattern-01', got '%s'", pattern.PatternId)
	}

	if pattern.LauncherId != "launcher-01" {
		t.Errorf("expected launcher_id 'launcher-01', got '%s'", pattern.LauncherId)
	}

	if pattern.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", pattern.Status)
	}

	// Verify launcher available slots decreased
	fsm.mu.RLock()
	launcher := fsm.state.Launchers["launcher-01"]
	fsm.mu.RUnlock()

	if launcher.AvailableSlots != 9 {
		t.Errorf("expected available_slots 9, got %d", launcher.AvailableSlots)
	}

	// Verify pattern count
	count := fsm.GetPatternCountForLauncher("launcher-01")
	if count != 1 {
		t.Errorf("expected pattern count 1, got %d", count)
	}
}

// TestApplyUpdateProxyStatus tests proxy status updates
func TestApplyUpdateProxyStatus(t *testing.T) {
	fsm := testFSM()



	// Register proxy first
	registerCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterProxy{
			RegisterProxy: &adminpb.RegisterProxyCommand{
				ProxyId: "proxy-01",
				Address: "proxy-01:8080",
			},
		},
	}
	data, _ := proto.Marshal(registerCmd)
	fsm.Apply(&raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data})

	// Update status
	now := time.Now().Unix()
	statusCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_UPDATE_PROXY_STATUS,
		Timestamp: now,
		Issuer:    "proxy-01",
		Payload: &adminpb.Command_UpdateProxyStatus{
			UpdateProxyStatus: &adminpb.UpdateProxyStatusCommand{
				ProxyId:  "proxy-01",
				Status:   "degraded",
				LastSeen: now,
			},
		},
	}

	data, _ = proto.Marshal(statusCmd)
	result := fsm.Apply(&raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: data})

	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify status was updated
	proxy, _ := fsm.GetProxy("proxy-01")
	if proxy.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", proxy.Status)
	}

	if proxy.LastSeen != now {
		t.Errorf("expected last_seen %d, got %d", now, proxy.LastSeen)
	}
}

// TestApplyUpdateLauncherStatus tests launcher status updates
func TestApplyUpdateLauncherStatus(t *testing.T) {
	fsm := testFSM()



	// Register launcher first
	registerCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
		Timestamp: time.Now().Unix(),
		Issuer:    "admin",
		Payload: &adminpb.Command_RegisterLauncher{
			RegisterLauncher: &adminpb.RegisterLauncherCommand{
				LauncherId:   "launcher-01",
				MaxProcesses: 10,
			},
		},
	}
	data, _ := proto.Marshal(registerCmd)
	fsm.Apply(&raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: data})

	// Update status
	now := time.Now().Unix()
	statusCmd := &adminpb.Command{
		Type:      adminpb.CommandType_COMMAND_TYPE_UPDATE_LAUNCHER_STATUS,
		Timestamp: now,
		Issuer:    "launcher-01",
		Payload: &adminpb.Command_UpdateLauncherStatus{
			UpdateLauncherStatus: &adminpb.UpdateLauncherStatusCommand{
				LauncherId:     "launcher-01",
				Status:         "degraded",
				LastSeen:       now,
				AvailableSlots: 8,
			},
		},
	}

	data, _ = proto.Marshal(statusCmd)
	result := fsm.Apply(&raft.Log{Index: 2, Term: 1, Type: raft.LogCommand, Data: data})

	if result != nil {
		t.Errorf("expected nil result, got: %v", result)
	}

	// Verify status was updated
	fsm.mu.RLock()
	launcher := fsm.state.Launchers["launcher-01"]
	fsm.mu.RUnlock()

	if launcher.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", launcher.Status)
	}

	if launcher.AvailableSlots != 8 {
		t.Errorf("expected available_slots 8, got %d", launcher.AvailableSlots)
	}
}

// TestSnapshotAndRestore tests FSM snapshot and restore
func TestSnapshotAndRestore(t *testing.T) {
	fsm1 := testFSM()

	// Populate FSM with data
	commands := []*adminpb.Command{
		{
			Type:      adminpb.CommandType_COMMAND_TYPE_CREATE_NAMESPACE,
			Timestamp: time.Now().Unix(),
			Issuer:    "test",
			Payload: &adminpb.Command_CreateNamespace{
				CreateNamespace: &adminpb.CreateNamespaceCommand{
					Namespace:     "ns-1",
					PartitionId:   1,
					AssignedProxy: "proxy-01",
				},
			},
		},
		{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_PROXY,
			Timestamp: time.Now().Unix(),
			Issuer:    "admin",
			Payload: &adminpb.Command_RegisterProxy{
				RegisterProxy: &adminpb.RegisterProxyCommand{
					ProxyId: "proxy-01",
					Address: "proxy-01:8080",
				},
			},
		},
		{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
			Timestamp: time.Now().Unix(),
			Issuer:    "admin",
			Payload: &adminpb.Command_RegisterLauncher{
				RegisterLauncher: &adminpb.RegisterLauncherCommand{
					LauncherId:   "launcher-01",
					MaxProcesses: 10,
				},
			},
		},
	}

	// Apply all commands
	for i, cmd := range commands {
		data, _ := proto.Marshal(cmd)
		fsm1.Apply(&raft.Log{
			Index: uint64(i + 1),
			Term:  1,
			Type:  raft.LogCommand,
			Data:  data,
		})
	}

	// Create snapshot
	snapshot, err := fsm1.Snapshot()
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Persist snapshot to temp file
	tmpFile, err := os.CreateTemp("", "fsm-snapshot-*.bin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	sink := &testSnapshotSink{file: tmpFile}
	if err := snapshot.Persist(sink); err != nil {
		t.Fatalf("failed to persist snapshot: %v", err)
	}

	// Create new FSM and restore from snapshot
	fsm2 := testFSM()

	// Open snapshot file for reading
	snapshotFile, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open snapshot file: %v", err)
	}
	defer snapshotFile.Close()

	if err := fsm2.Restore(snapshotFile); err != nil {
		t.Fatalf("failed to restore snapshot: %v", err)
	}

	// Verify restored state matches original
	if fsm2.lastAppliedIndex != fsm1.lastAppliedIndex {
		t.Errorf("last applied index mismatch: expected %d, got %d",
			fsm1.lastAppliedIndex, fsm2.lastAppliedIndex)
	}

	if fsm2.lastAppliedTerm != fsm1.lastAppliedTerm {
		t.Errorf("last applied term mismatch: expected %d, got %d",
			fsm1.lastAppliedTerm, fsm2.lastAppliedTerm)
	}

	// Verify namespace
	ns, exists := fsm2.GetNamespace("ns-1")
	if !exists {
		t.Error("namespace not found after restore")
	}
	if ns.PartitionId != 1 {
		t.Errorf("namespace partition mismatch: expected 1, got %d", ns.PartitionId)
	}

	// Verify proxy
	proxy, exists := fsm2.GetProxy("proxy-01")
	if !exists {
		t.Error("proxy not found after restore")
	}
	if proxy.Address != "proxy-01:8080" {
		t.Errorf("proxy address mismatch: expected 'proxy-01:8080', got '%s'", proxy.Address)
	}

	// Verify launcher
	fsm2.mu.RLock()
	launcher, exists := fsm2.state.Launchers["launcher-01"]
	fsm2.mu.RUnlock()
	if !exists {
		t.Error("launcher not found after restore")
	}
	if launcher.MaxProcesses != 10 {
		t.Errorf("launcher max_processes mismatch: expected 10, got %d", launcher.MaxProcesses)
	}
}

// TestGetHealthyLaunchers verifies healthy launcher filtering
func TestGetHealthyLaunchers(t *testing.T) {
	fsm := testFSM()



	// Register launchers with different states
	launchers := []struct {
		id        string
		available int32
	}{
		{"launcher-01", 5},  // Healthy with slots
		{"launcher-02", 0},  // Healthy but no slots
		{"launcher-03", 10}, // Healthy with slots
	}

	for i, l := range launchers {
		cmd := &adminpb.Command{
			Type:      adminpb.CommandType_COMMAND_TYPE_REGISTER_LAUNCHER,
			Timestamp: time.Now().Unix(),
			Issuer:    "admin",
			Payload: &adminpb.Command_RegisterLauncher{
				RegisterLauncher: &adminpb.RegisterLauncherCommand{
					LauncherId:   l.id,
					MaxProcesses: l.available,
				},
			},
		}
		data, _ := proto.Marshal(cmd)
		fsm.Apply(&raft.Log{Index: uint64(i + 1), Term: 1, Type: raft.LogCommand, Data: data})
	}

	// Set launcher-02 to have no available slots
	fsm.mu.Lock()
	fsm.state.Launchers["launcher-02"].AvailableSlots = 0
	fsm.mu.Unlock()

	// Get healthy launchers
	healthy := fsm.GetHealthyLaunchers()

	// Should only return launchers with available slots
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy launchers, got %d", len(healthy))
	}

	// Verify correct launchers returned
	ids := make(map[string]bool)
	for _, l := range healthy {
		ids[l.LauncherId] = true
	}

	if !ids["launcher-01"] || !ids["launcher-03"] {
		t.Error("expected launcher-01 and launcher-03 to be healthy")
	}

	if ids["launcher-02"] {
		t.Error("launcher-02 should not be healthy (no available slots)")
	}
}

// testSnapshotSink implements raft.SnapshotSink for testing
type testSnapshotSink struct {
	file *os.File
}

func (s *testSnapshotSink) Write(p []byte) (n int, err error) {
	return s.file.Write(p)
}

func (s *testSnapshotSink) Close() error {
	return s.file.Close()
}

func (s *testSnapshotSink) ID() string {
	return "test-snapshot"
}

func (s *testSnapshotSink) Cancel() error {
	s.file.Close()
	return os.Remove(s.file.Name())
}
