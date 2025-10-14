# Consumer Pattern Acceptance Tests

This directory contains acceptance tests for the consumer pattern, demonstrating two testing approaches:

## 1. In-Process Testing (`consumer_test.go`)

**Current implementation** - Tests the consumer library directly in the test process.

```go
// Create consumer directly
consumer := consumer.New(config)
consumer.BindSlots(messageSource, stateStore, nil)
consumer.Start(ctx)
```

**Pros:**
- Fast - no process overhead
- Easy to debug
- Direct access to internal state

**Cons:**
- Doesn't match production architecture
- Doesn't test process lifecycle
- Doesn't test IPC mechanisms

## 2. Process-Based Testing (`process_test.go`, `process_grpc_test.go`)

**Proxy-like architecture** - Tests consumer as a separate process, mimicking how the proxy would interact with patterns.

### File-Based Config Approach (`process_test.go`)

Launches consumer with config file:

```bash
consumer-runner -config /tmp/test-config.yaml
```

This approach:
- ✅ Runs consumer as separate process
- ✅ Tests real process lifecycle
- ❌ Uses config files (not how proxy works)
- ❌ No dynamic configuration

### RPC-Based Config Approach (`process_grpc_test.go`)

**This is the target architecture** - matches how proxy should work:

```go
// 1. Launch pattern with control plane
cmd := exec.Command("consumer-runner", "-control-port", "50051")
cmd.Start()

// 2. Connect to pattern's control plane
client := connectToControlPlane(ctx, 50051)

// 3. Send configuration via gRPC
config := buildConsumerConfig(natsUrl, stateStoreAddr)
client.Configure(ctx, config)

// 4. Start pattern
client.Start(ctx)

// 5. Monitor health
health, _ := client.Health(ctx)

// 6. Stop pattern
client.Stop(ctx)
```

## Architecture: How Proxy Interacts with Patterns

**CORRECT ARCHITECTURE** - Pattern connects TO proxy (not the other way around):

```
┌─────────────────────────┐         ┌──────────────────────┐
│  Prism Proxy            │         │  Consumer Pattern    │
│  (Control Plane Server) │         │   (separate process) │
│                         │         │                      │
│  1. Start Control Plane │         │                      │
│     Listen on :9090     │         │                      │
│                         │         │                      │
│  2. Launch Pattern      │────────▶│  Launch with         │
│     with proxy address  │ fork/   │  -proxy-addr :9090   │
│                         │  exec   │                      │
│                         │         │  3. Connect to Proxy │
│     4. Accept Connection│◀────────┤     (bidirectional   │
│        Register Pattern │  gRPC   │      stream)         │
│                         │  stream │                      │
│  5. Send Initialize()   │────────▶│  Receive command     │
│     command via stream  │         │  Parse config        │
│                         │◀────────┤  Send response       │
│                         │         │  Initialize backends │
│                         │         │                      │
│  6. Send Start()        │────────▶│  Receive command     │
│     command             │         │  Start consuming     │
│                         │◀────────┤  Send response       │
│                         │         │                      │
│  7. Send HealthCheck()  │────────▶│  Receive command     │
│     periodically        │         │  Report status       │
│                         │◀────────┤  Send health status  │
│                         │         │                      │
│  8. Send Stop()         │────────▶│  Receive command     │
│     on shutdown         │         │  Graceful shutdown   │
│                         │◀────────┤  Send response       │
└─────────────────────────┘         └──────────────────────┘
```

**Key Points:**
- Proxy runs **ProxyControlPlaneServer** on a well-known port (e.g., :9090)
- Pattern is launched with `-proxy-addr localhost:9090`
- Pattern **connects TO proxy** using **PatternControlPlaneClient**
- Communication happens via **bidirectional gRPC stream**
- Proxy sends commands, pattern sends responses
- Single persistent connection for entire pattern lifecycle

## What Needs to Be Implemented

### 1. Consumer Control Plane (in `patterns/consumer/`)

Add gRPC server to consumer-runner:

```go
// patterns/consumer/cmd/consumer-runner/control.go

type ControlPlaneServer struct {
    consumer *consumer.Consumer
    // ...
}

func (s *ControlPlaneServer) Configure(ctx context.Context, req *ConfigureRequest) (*ConfigureResponse, error) {
    // Initialize consumer with provided config
    // Set up backends based on config
    return &ConfigureResponse{Success: true}, nil
}

func (s *ControlPlaneServer) Start(ctx context.Context, req *StartRequest) (*StartResponse, error) {
    // Start consumer processing
    return &StartResponse{Success: true}, nil
}

func (s *ControlPlaneServer) Stop(ctx context.Context, req *StopRequest) (*StopResponse, error) {
    // Stop consumer gracefully
    return &StopResponse{Success: true}, nil
}

func (s *ControlPlaneServer) Health(ctx context.Context, req *HealthRequest) (*HealthResponse, error) {
    // Return consumer health status
    return consumer.Health(ctx)
}
```

### 2. Control Plane Proto Definitions

```protobuf
// proto/pattern/control_plane.proto

service PatternControlPlane {
  rpc Configure(ConfigureRequest) returns (ConfigureResponse);
  rpc Start(StartRequest) returns (StartResponse);
  rpc Stop(StopRequest) returns (StopResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}

message ConfigureRequest {
  string name = 1;
  map<string, SlotConfig> slots = 2;
  map<string, google.protobuf.Any> behavior = 3;
}

message SlotConfig {
  string backend = 1;
  map<string, google.protobuf.Any> config = 2;
}
```

### 3. Test Control Client

```go
// tests/acceptance/patterns/consumer/control_client.go

type ConsumerControlClient struct {
    conn *grpc.ClientConn
    client pb.PatternControlPlaneClient
}

func ConnectToControlPlane(ctx context.Context, port int) (*ConsumerControlClient, error) {
    conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port),
        grpc.WithInsecure())
    if err != nil {
        return nil, err
    }

    return &ConsumerControlClient{
        conn: conn,
        client: pb.NewPatternControlPlaneClient(conn),
    }, nil
}
```

## Testing Strategy

### Unit Tests (in `patterns/consumer/`)
- Test consumer logic directly
- Mock backends
- Fast, focused

### Integration Tests (in `tests/acceptance/patterns/consumer/`)
- **In-Process** (`consumer_test.go`): Test with real backends (NATS, MemStore)
- **Process-File** (`process_test.go`): Test as subprocess with config files
- **Process-RPC** (`process_grpc_test.go`): Test as subprocess with dynamic config via gRPC ✨ **TARGET**

### Acceptance Tests (via framework)
- Run against all registered backend combinations
- Verify pattern compliance
- Generate compatibility matrix

## Current Status

- ✅ In-process tests working
- ✅ Process-based tests (file config) working
- ✅ **Proxy control plane architecture implemented and tested!**
- ✅ Control plane proto definitions (`proxy_control_plane.proto`)
- ✅ Proxy control plane server (`ProxyControlPlaneServer`)
- ✅ Pattern control plane client (`PatternControlPlaneClient`)
- ✅ Consumer-runner supports `-proxy-addr` flag
- ✅ Bidirectional streaming working
- ✅ End-to-end test passing (`TestConsumerProxyArchitecture`)
- ✅ **RFC-030 compliance: Consumer specifies protocol on registration**
  - Schema expectations (version, compatibility mode, allowed fields)
  - Consumer metadata (team, purpose, data usage, PII access)
  - Compliance frameworks and retention policies
  - Rate limits and access patterns

## Running Tests

```bash
# In-process tests
cd tests/acceptance
go test ./patterns/consumer/ -run TestConsumerPattern -v

# Process-based tests (file config - legacy)
go test ./patterns/consumer/ -run TestConsumerProcessBased -v

# Proxy architecture tests (NEW - correct architecture!)
go test ./patterns/consumer/ -run TestConsumerProxyArchitecture -v
```

## Benefits of Proxy Control Plane Architecture

1. **Correct Direction**: Pattern connects TO proxy (not proxy TO pattern)
2. **Single Connection**: Bidirectional stream for entire lifecycle
3. **Centralized Control**: Proxy manages all patterns from one place
4. **Realistic**: Matches production proxy architecture exactly
5. **Isolation**: Each pattern is isolated process with no exposed ports
6. **Process Lifecycle**: Full control over startup, health, shutdown
7. **Dynamic Config**: No config files needed - pure RPC
8. **Multi-Pattern**: Same framework works for all patterns (consumer, multicast, etc.)

## Next Steps

1. ✅ ~~Define control plane proto~~ - DONE
2. ✅ ~~Implement proxy control plane server~~ - DONE
3. ✅ ~~Implement pattern control plane client~~ - DONE
4. ✅ ~~Update consumer-runner~~ - DONE
5. ✅ ~~Create end-to-end test~~ - DONE
6. Extend to other patterns (multicast_registry, queue, etc.)
7. Add pattern reconnection logic
8. Add heartbeat monitoring
9. Integrate with real proxy implementation
