---
author: Platform Team
created: 2025-10-11
doc_uuid: 50cf9cc5-7e1f-44ba-a8b0-e52c01c54957
id: rfc-029
project_id: prism-data-layer
status: Proposed
tags:
- load-testing
- performance
- tooling
- evaluation
title: Load Testing Framework Evaluation and Strategy
updated: 2025-10-12
---

# RFC-029: Load Testing Framework Evaluation and Strategy

## Summary

Evaluation of Go load testing frameworks for Prism data access gateway. Analyzes custom tool (prism-loadtest) vs best-of-breed frameworks (ghz, k6, vegeta, fortio) and proposes a **two-tier testing strategy**: custom tool for pattern-level load testing + ghz for end-to-end gRPC integration testing.

**Recommendation**: Keep custom `prism-loadtest` tool and add `ghz` for gRPC integration testing.

**Key Finding**: Prism needs **two types** of load testing:
1. **Pattern-level**: Tests pattern logic directly (current custom tool)
2. **Integration-level**: Tests through Rust proxy via gRPC (needs ghz)

## Motivation

### Problem

Current load testing tool (`cmd/prism-loadtest`) is custom-built:
- ✅ Production-ready (validated by MEMO-010)
- ✅ Direct Pattern SDK integration
- ✅ Custom metrics collection
- ❌ Tests patterns directly (not through proxy)
- ❌ No gRPC integration testing
- ❌ Maintenance burden (custom code)

**Question**: Should we adopt a best-of-breed framework or keep the custom tool?

### Goals

1. **Evaluate** best-of-breed Go load testing frameworks
2. **Compare** frameworks against Prism requirements
3. **Recommend** load testing strategy for POC 1+
4. **Define** testing scope (pattern-level vs integration-level)

## Current State: Custom Load Testing Tool

### Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  prism-loadtest CLI (Custom Tool)                           │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐    │
│  │   register   │  │  enumerate   │  │   multicast   │    │
│  │   command    │  │   command    │  │    command    │    │
│  └──────┬───────┘  └──────┬───────┘  └───────┬───────┘    │
│         │                  │                   │            │
│         └──────────────────┴───────────────────┘            │
│                            │                                │
│              ┌─────────────▼──────────────┐                │
│              │  Coordinator (direct call) │                │
│              └─────────────┬──────────────┘                │
│                            │                                │
│         ┌──────────────────┴────────────────┐              │
│         │                                    │              │
│    ┌────▼─────┐                       ┌─────▼──────┐      │
│    │  Redis   │                       │    NATS    │      │
│    │ Backend  │                       │  Backend   │      │
│    └──────────┘                       └────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics**:
- Tests **pattern logic directly** (bypasses proxy)
- Direct Coordinator instantiation
- No gRPC overhead
- Isolated backend testing (Redis + NATS)

### Implementation Details

| Feature | Implementation | LOC |
|---------|---------------|-----|
| CLI Framework | Cobra | ~100 |
| Rate Limiting | golang.org/x/time/rate | ~50 |
| Metrics Collection | Custom histogram | ~150 |
| Progress Reporting | Custom ticker | ~50 |
| Register Command | Direct Coordinator call | ~100 |
| Enumerate Command | Direct Coordinator call | ~80 |
| Multicast Command | Direct Coordinator call | ~120 |
| Mixed Workload | Weighted random | ~150 |
| **Total** | **~800 LOC** | **~800** |

### Performance Validation (MEMO-010)

| Metric | Target | Actual | Status |
|--------|--------|--------|--------|
| Rate Limiting | 100 req/sec | 101.81 req/sec | ✅ 1.81% error |
| Workload Mix | 50/30/20 | 50.1/30.0/20.0 | ✅ Precise |
| Thread Safety | No data races | Fixed with mutex | ✅ Safe |
| Register P95 | &lt;10ms | 5ms | ✅ 2x faster |
| Enumerate P95 | &lt;20ms | 500µs | ✅ 40x faster |
| Multicast P95 | &lt;100ms | 100ms | ✅ On target |

**Verdict**: Custom tool is **production-ready** ✅

### Strengths

1. **Direct Integration**: Tests pattern logic without gRPC overhead
2. **Custom Metrics**: Tailored to Prism patterns (multicast delivery stats)
3. **Proven**: Validated by POC 4 (MEMO-010)
4. **Fast Iteration**: No external dependencies
5. **Workload Flexibility**: Mixed workloads with precise distribution

### Weaknesses

1. **No gRPC Testing**: Doesn't test through Rust proxy
2. **Maintenance Burden**: ~800 LOC to maintain
3. **Not Best-of-Breed**: Missing features like:
   - Advanced load profiles (ramp-up, spike, soak)
   - Distributed load generation (multiple clients)
   - Real-time dashboards
   - Standard output formats (JSON, CSV)

## Framework Evaluation

### Selection Criteria

| Criterion | Weight | Description |
|-----------|--------|-------------|
| **gRPC Support** | High | Critical for Prism architecture |
| **Custom Metrics** | High | Need pattern-specific metrics (multicast delivery) |
| **Learning Curve** | Medium | Team productivity |
| **Maintenance** | Medium | Long-term support |
| **Integration** | High | Works with existing patterns |
| **Performance** | Medium | Tool shouldn't bottleneck tests |

### Framework 1: ghz (gRPC Load Testing)

**URL**: https://github.com/bojand/ghz

**Description**: gRPC benchmarking and load testing tool written in Go.

#### Architecture

```text
┌──────────────────────────────────────────────────────────┐
│  ghz CLI                                                 │
│                                                          │
│  ┌────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ Proto File │  │  Rate Config │  │ Load Profile │   │
│  └─────┬──────┘  └──────┬───────┘  └──────┬───────┘   │
│        │                 │                  │           │
│        └─────────────────┴──────────────────┘           │
│                          │                              │
│                          ▼                              │
│              ┌───────────────────────┐                 │
│              │  gRPC Client (tonic) │                 │
│              └───────────┬───────────┘                 │
│                          │                              │
│                          ▼                              │
│              ┌───────────────────────┐                 │
│              │   Rust Proxy :8980   │                 │
│              └───────────┬───────────┘                 │
│                          │                              │
│                          ▼                              │
│              ┌───────────────────────┐                 │
│              │  Pattern (MemStore/   │                 │
│              │  Redis/Kafka)         │                 │
│              └───────────────────────┘                 │
└──────────────────────────────────────────────────────────┘
```

#### Features

| Feature | Support | Notes |
|---------|---------|-------|
| gRPC Support | ✅ Native | Built specifically for gRPC |
| Protocol Buffers | ✅ Required | Requires .proto files |
| Rate Limiting | ✅ Built-in | Constant, step, line profiles |
| Concurrency | ✅ Built-in | Configurable workers |
| Output Formats | ✅ JSON, CSV, HTML | Standard formats |
| Real-time Stats | ✅ Built-in | Progress bar |
| Custom Metrics | ❌ Limited | Standard gRPC metrics only |
| Direct Integration | ❌ No | Must go through gRPC |

#### Example Usage

```bash
# Basic gRPC load test
ghz --proto ./proto/interfaces/keyvalue_basic.proto \
    --call prism.KeyValueBasicInterface.Set \
    --insecure \
    --total 6000 \
    --concurrency 100 \
    --rps 100 \
    --duration 60s \
    --data '{"namespace":"default","key":"test-key","value":"dGVzdC12YWx1ZQ=="}' \
    localhost:8980

# Output:
# Summary:
#   Count:        6000
#   Total:        60.00 s
#   Slowest:      15.23 ms
#   Fastest:      0.45 ms
#   Average:      2.31 ms
#   Requests/sec: 100.00
#
# Response time histogram:
#   0.450 [1]      |
#   1.928 [2341]   |∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎
#   3.406 [2892]   |∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎∎
#   4.884 [582]    |∎∎∎∎∎∎∎
#   6.362 [145]    |∎∎
#   7.840 [32]     |
#   9.318 [5]      |
#   10.796 [1]     |
#   12.274 [0]     |
#   13.752 [0]     |
#   15.230 [1]     |
#
# Status code distribution:
#   [OK]   6000 responses
```

#### Pros

- ✅ **gRPC Native**: Tests through actual Rust proxy (integration testing)
- ✅ **Production-Grade**: Used by many companies
- ✅ **Standard Output**: JSON, CSV, HTML reports
- ✅ **Load Profiles**: Ramp-up, step, spike patterns
- ✅ **Minimal Code**: Zero code for basic tests

#### Cons

- ❌ **No Direct Integration**: Can't test pattern logic directly
- ❌ **Limited Custom Metrics**: Can't track multicast delivery stats
- ❌ **Proto Dependency**: Requires .proto files (manageable)
- ❌ **Less Flexible**: Hard to implement mixed workloads

#### Score

| Criterion | Score (1-5) | Reasoning |
|-----------|-------------|-----------|
| gRPC Support | 5 | Native gRPC support |
| Custom Metrics | 2 | Limited to standard gRPC metrics |
| Learning Curve | 4 | Simple CLI, easy to learn |
| Maintenance | 5 | External project, no code to maintain |
| Integration | 3 | Tests through proxy (good), not patterns (bad) |
| Performance | 5 | High-performance tool |
| **Total** | **24/30** | **Strong for integration testing** |

---

### Framework 2: k6

**URL**: https://k6.io/

**Description**: Modern load testing tool with Go runtime and JavaScript scripting.

#### Architecture

```text
┌──────────────────────────────────────────────────────────┐
│  k6 (Go Runtime)                                         │
│                                                          │
│  ┌────────────────────────────────────────────────┐    │
│  │  JavaScript Test Script (user-written)        │    │
│  │                                                │    │
│  │  export default function() {                  │    │
│  │    const client = new grpc.Client();          │    │
│  │    client.connect("localhost:8980");          │    │
│  │    client.invoke("Set", { key: "foo" });      │    │
│  │  }                                             │    │
│  └────────────────────────┬───────────────────────┘    │
│                            │                            │
│               ┌────────────▼─────────────┐             │
│               │  k6 Go Runtime (goja)   │             │
│               └────────────┬─────────────┘             │
│                            │                            │
│                            ▼                            │
│               ┌────────────────────────┐               │
│               │  Rust Proxy :8980     │               │
│               └────────────────────────┘               │
└──────────────────────────────────────────────────────────┘
```

#### Features

| Feature | Support | Notes |
|---------|---------|-------|
| gRPC Support | ✅ Plugin | Via k6-grpc extension |
| Protocol Buffers | ✅ Required | Requires .proto reflection or files |
| Rate Limiting | ✅ Built-in | Virtual users + iterations |
| Concurrency | ✅ Built-in | Virtual user model |
| Output Formats | ✅ JSON, CSV, Cloud | k6 Cloud integration |
| Real-time Stats | ✅ Built-in | Beautiful terminal UI |
| Custom Metrics | ✅ Good | Custom metrics via JS |
| Direct Integration | ❌ No | JavaScript only |

#### Example Usage

```javascript
// loadtest.js
import grpc from 'k6/net/grpc';
import { check } from 'k6';

const client = new grpc.Client();
client.load(['proto'], 'keyvalue_basic.proto');

export default function () {
  client.connect('localhost:8980', { plaintext: true });

  const response = client.invoke('prism.KeyValueBasicInterface/Set', {
    namespace: 'default',
    key: 'test-key',
    value: Buffer.from('test-value').toString('base64'),
  });

  check(response, {
    'status is OK': (r) => r.status === grpc.StatusOK,
  });

  client.close();
}

export const options = {
  vus: 100, // 100 virtual users
  duration: '60s',
  thresholds: {
    'grpc_req_duration': ['p(95)&lt;10'], // P95 &lt; 10ms
  },
};
```

```bash
# Run load test
k6 run loadtest.js

# Output:
#          /\      |‾‾| /‾‾/   /‾‾/
#     /\  /  \     |  |/  /   /  /
#    /  \/    \    |     (   /   ‾‾\
#   /          \   |  |\  \ |  (‾)  |
#  / __________ \  |__| \__\ \_____/ .io
#
#   execution: local
#      script: loadtest.js
#      output: -
#
#   scenarios: (100.00%) 1 scenario, 100 max VUs, 1m30s max duration
#
#   ✓ status is OK
#
#   grpc_req_duration.........: avg=2.31ms  p(95)=4.5ms  p(99)=8.2ms
#   grpc_reqs.................: 6000    100.00/s
#   vus.......................: 100     min=100  max=100
```

#### Pros

- ✅ **Modern UX**: Beautiful terminal UI
- ✅ **Custom Metrics**: JavaScript flexibility
- ✅ **Load Profiles**: Sophisticated VU ramping
- ✅ **Cloud Integration**: k6 Cloud for distributed testing
- ✅ **Ecosystem**: Large community, extensions

#### Cons

- ❌ **JavaScript Required**: Team must learn JS for load tests
- ❌ **gRPC Via Plugin**: Not native gRPC support
- ❌ **Complexity**: Overkill for simple tests
- ❌ **No Direct Integration**: Can't test patterns directly

#### Score

| Criterion | Score (1-5) | Reasoning |
|-----------|-------------|-----------|
| gRPC Support | 3 | Via plugin, not native |
| Custom Metrics | 4 | Good JS flexibility |
| Learning Curve | 2 | Requires JavaScript knowledge |
| Maintenance | 5 | External project |
| Integration | 2 | Must learn k6 + JS ecosystem |
| Performance | 4 | High-performance (Go runtime) |
| **Total** | **20/30** | **Powerful but complex** |

---

### Framework 3: vegeta (HTTP Library)

**URL**: https://github.com/tsenart/vegeta

**Description**: HTTP load testing library and CLI tool.

#### Features

| Feature | Support | Notes |
|---------|---------|-------|
| gRPC Support | ❌ No | HTTP only |
| HTTP/2 | ✅ Yes | Can test gRPC-Web |
| Rate Limiting | ✅ Built-in | Constant rate |
| Concurrency | ✅ Built-in | Worker pool |
| Output Formats | ✅ JSON, CSV, binary | Standard formats |
| Library Mode | ✅ Yes | Can embed in Go code |
| Custom Metrics | ✅ Good | Go library flexibility |

#### Pros

- ✅ **Library**: Can embed in custom tools
- ✅ **Mature**: Well-tested, stable
- ✅ **Go Native**: Easy integration

#### Cons

- ❌ **No gRPC**: HTTP only (dealbreaker for Prism)

#### Score

| Criterion | Score (1-5) | Reasoning |
|-----------|-------------|-----------|
| gRPC Support | 0 | No gRPC support |
| **Total** | **Disqualified** | **No gRPC = not viable** |

---

### Framework 4: fortio (Istio's Tool)

**URL**: https://github.com/fortio/fortio

**Description**: Load testing tool from Istio project (HTTP + gRPC).

#### Features

| Feature | Support | Notes |
|---------|---------|-------|
| gRPC Support | ✅ Yes | Native gRPC support |
| HTTP Support | ✅ Yes | Also supports HTTP/HTTP2 |
| Rate Limiting | ✅ Built-in | QPS control |
| Concurrency | ✅ Built-in | Configurable connections |
| Output Formats | ✅ JSON, HTML | Includes web UI |
| Web UI | ✅ Built-in | Real-time dashboard |
| Custom Metrics | ❌ Limited | Standard metrics only |

#### Example Usage

```bash
# gRPC load test
fortio load \
  -grpc \
  -n 6000 \
  -c 100 \
  -qps 100 \
  -t 60s \
  localhost:8980

# Output similar to ghz
```

#### Pros

- ✅ **gRPC + HTTP**: Dual protocol support
- ✅ **Web UI**: Real-time dashboard
- ✅ **Istio Integration**: If we use Istio later

#### Cons

- ❌ **Limited Flexibility**: Can't do complex workflows
- ❌ **Basic Metrics**: Standard metrics only
- ❌ **Less Popular**: Smaller community than ghz

#### Score

| Criterion | Score (1-5) | Reasoning |
|-----------|-------------|-----------|
| gRPC Support | 5 | Native gRPC support |
| Custom Metrics | 2 | Limited to standard metrics |
| Learning Curve | 4 | Simple CLI |
| Maintenance | 4 | Istio project (stable) |
| Integration | 3 | Tests through proxy only |
| Performance | 4 | Good performance |
| **Total** | **22/30** | **Solid alternative to ghz** |

---

### Framework 5: hey / bombardier (HTTP Tools)

**Description**: Simple HTTP benchmarking tools.

**Verdict**: ❌ **Disqualified** - No gRPC support

---

## Framework Comparison Matrix

| Framework | gRPC | Custom Metrics | Learning Curve | Maintenance | Integration | Performance | **Total** |
|-----------|------|----------------|---------------|-------------|-------------|-------------|-----------|
| **ghz** | 5 | 2 | 4 | 5 | 3 | 5 | **24/30** ✅ |
| **k6** | 3 | 4 | 2 | 5 | 2 | 4 | **20/30** |
| **fortio** | 5 | 2 | 4 | 4 | 3 | 4 | **22/30** |
| vegeta | 0 | - | - | - | - | - | Disqualified |
| hey/bombardier | 0 | - | - | - | - | - | Disqualified |
| **Custom Tool** | 0 | 5 | 5 | 2 | 5 | 5 | **22/30** |

**Key Observations**:
1. **ghz** scores highest for integration testing (24/30)
2. **Custom tool** excellent for pattern-level testing (22/30)
3. **k6** powerful but complex (20/30)
4. **fortio** solid alternative to ghz (22/30)

## Testing Strategy: Two-Tier Approach

### Insight: Two Types of Load Testing

Prism needs **two distinct types** of load testing:

#### 1. Pattern-Level Load Testing (Unit Load Testing)

**Goal**: Test pattern logic in isolation

**Tool**: Custom `prism-loadtest` ✅

**Architecture**:
```text
prism-loadtest → Coordinator → Redis/NATS
```

**Benefits**:
- ✅ No proxy overhead (fastest possible)
- ✅ Custom metrics (multicast delivery stats)
- ✅ Direct debugging
- ✅ Isolated testing

**Use Cases**:
- Pattern development (POC 1-4)
- Backend benchmarking (Redis vs SQLite)
- Algorithm optimization (TTL cleanup, fan-out)

#### 2. Integration-Level Load Testing (End-to-End)

**Goal**: Test through Rust proxy via gRPC

**Tool**: `ghz` ✅

**Architecture**:
```text
ghz → Rust Proxy (gRPC) → Pattern (gRPC) → Redis/NATS
```

**Benefits**:
- ✅ Tests real production path
- ✅ Includes gRPC overhead
- ✅ Validates proxy performance
- ✅ Catches serialization issues

**Use Cases**:
- Integration testing (POC 1+)
- Proxy performance validation
- End-to-end latency measurement
- Production load simulation

### Recommended Architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                     Load Testing Strategy                       │
│                                                                 │
│  ┌───────────────────────┐         ┌──────────────────────┐   │
│  │  Pattern-Level Tests  │         │ Integration Tests    │   │
│  │  (prism-loadtest)     │         │ (ghz)                │   │
│  └───────────┬───────────┘         └──────────┬───────────┘   │
│              │                                 │               │
│              │                                 │               │
│              ▼                                 ▼               │
│  ┌───────────────────────┐         ┌──────────────────────┐   │
│  │  Coordinator (direct) │         │  Rust Proxy (gRPC)   │   │
│  └───────────┬───────────┘         └──────────┬───────────┘   │
│              │                                 │               │
│              └─────────────┬───────────────────┘               │
│                            │                                   │
│                            ▼                                   │
│              ┌─────────────────────────┐                      │
│              │  Redis + NATS Backends  │                      │
│              └─────────────────────────┘                      │
└─────────────────────────────────────────────────────────────────┘
```

## Recommended Strategy

### Phase 1: POC 1 (Current State)

**Keep Custom Tool** (`prism-loadtest`)

✅ **Rationale**:
- Already production-ready (MEMO-010)
- Pattern-level testing sufficient for POC 1
- No proxy implementation yet

**Actions**:
- [ ] Continue using prism-loadtest
- [ ] Add advanced load profiles (ramp-up, spike)
- [ ] Add JSON output format

### Phase 2: POC 2+ (Add Integration Testing)

**Add ghz for Integration Testing**

✅ **Rationale**:
- Rust proxy will be ready
- Need end-to-end testing
- ghz scores highest for integration (24/30)

**Actions**:
- [ ] Install ghz
- [ ] Create ghz test scenarios for each pattern
- [ ] Integrate into CI/CD
- [ ] Compare results (pattern-level vs integration)

**Example ghz Test Suite**:
```bash
# tests/load/ghz/keyvalue.sh
#!/bin/bash

# Test MemStore pattern via proxy
ghz --proto proto/interfaces/keyvalue_basic.proto \
    --call prism.KeyValueBasicInterface.Set \
    --insecure \
    --rps 100 \
    --duration 60s \
    --data '{"namespace":"default","key":"test-{{.RequestNumber}}","value":"dGVzdC12YWx1ZQ=="}' \
    --output json \
    --output-path results/memstore-set.json \
    localhost:8980

# Test Redis pattern via proxy
ghz --proto proto/interfaces/keyvalue_basic.proto \
    --call prism.KeyValueBasicInterface.Get \
    --insecure \
    --rps 100 \
    --duration 60s \
    --data '{"namespace":"default","key":"test-{{.RequestNumber}}"}' \
    --output json \
    --output-path results/redis-get.json \
    localhost:8981
```

### Phase 3: Future (Optional Enhancements)

**Consider k6 for Advanced Scenarios**

⚠️ **Use Cases**:
- Multi-protocol testing (gRPC + HTTP + WebSocket)
- Complex user journeys (multi-step workflows)
- Cloud-scale distributed load testing

**Decision Criteria**:
- If we need distributed load testing → Adopt k6
- If simple gRPC testing sufficient → Stick with ghz

## Implementation Plan

### Week 1: Enhance Custom Tool

**Tasks**:
1. Add ramp-up load profile
   ```go
   // cmd/prism-loadtest/cmd/root.go
   rootCmd.PersistentFlags().String("profile", "constant", "Load profile: constant, ramp-up, spike")
   rootCmd.PersistentFlags().Int("ramp-duration", 30, "Ramp-up duration (seconds)")
   rootCmd.PersistentFlags().Int("max-rate", 1000, "Max rate for ramp-up (req/sec)")
   ```

2. Add JSON output format
   ```go
   // cmd/prism-loadtest/cmd/output.go
   type JSONReport struct {
       Duration        string  `json:"duration"`
       TotalRequests   int64   `json:"total_requests"`
       Successful      int64   `json:"successful"`
       Failed          int64   `json:"failed"`
       Throughput      float64 `json:"throughput"`
       LatencyMin      string  `json:"latency_min"`
       LatencyMax      string  `json:"latency_max"`
       LatencyAvg      string  `json:"latency_avg"`
       LatencyP50      string  `json:"latency_p50"`
       LatencyP95      string  `json:"latency_p95"`
       LatencyP99      string  `json:"latency_p99"`
   }
   ```

3. Add spike load profile
   ```bash
   # 0-30s: 100 req/sec
   # 30-35s: 1000 req/sec (spike)
   # 35-60s: 100 req/sec
   ./prism-loadtest mixed --profile spike --spike-duration 5s --spike-rate 1000 -r 100 -d 60s
   ```

**Estimated Effort**: 2 days

### Week 2: Add ghz Integration Testing

**Tasks**:
1. Install ghz
   ```bash
   go install github.com/bojand/ghz/cmd/ghz@latest
   ```

2. Create ghz test suite
   ```bash
   mkdir -p tests/load/ghz
   # Create test scripts for each pattern
   ```

3. Add to CI/CD
   ```yaml
   # .github/workflows/load-test.yml
   - name: Run integration load tests
     run: |
       # Start proxy
       cd proxy && cargo run --release &
       PROXY_PID=$!

       # Wait for proxy
       sleep 5

       # Run ghz tests
       ghz --proto proto/interfaces/keyvalue_basic.proto \
           --call prism.KeyValueBasicInterface.Set \
           --insecure \
           --rps 100 \
           --duration 30s \
           --data '{"namespace":"default","key":"test-key","value":"dGVzdC12YWx1ZQ=="}' \
           localhost:8980

       # Stop proxy
       kill $PROXY_PID
   ```

4. Compare results
   ```bash
   # Pattern-level (no gRPC overhead)
   ./prism-loadtest register -r 100 -d 60s
   # Expected: P95 = 5ms

   # Integration-level (with gRPC overhead)
   ghz --proto proto/interfaces/keyvalue_basic.proto \
       --call prism.KeyValueBasicInterface.Set \
       --rps 100 \
       --duration 60s \
       localhost:8980
   # Expected: P95 = 8-10ms (gRPC adds ~3-5ms)
   ```

**Estimated Effort**: 3 days

## Decision Matrix

| Scenario | Tool | Reasoning |
|----------|------|-----------|
| **Pattern Development** | prism-loadtest | Direct integration, custom metrics |
| **Backend Benchmarking** | prism-loadtest | No proxy overhead |
| **Integration Testing** | ghz | Tests through proxy |
| **CI/CD Regression** | ghz | End-to-end validation |
| **Production Validation** | ghz | Real production path |
| **Algorithm Optimization** | prism-loadtest | Isolated, fastest feedback |
| **Complex Workflows** | k6 (future) | Multi-step scenarios |

## Benefits

### Two-Tier Strategy

1. **Best of Both Worlds**:
   - Pattern-level: Fast iteration, custom metrics
   - Integration-level: Production accuracy

2. **Minimal Code Changes**:
   - Keep existing prism-loadtest (validated)
   - Add ghz (zero code, just scripts)

3. **Comprehensive Coverage**:
   - Unit load testing: Pattern logic
   - Integration load testing: End-to-end

4. **Clear Separation**:
   - Developers: Use prism-loadtest for pattern work
   - QA/DevOps: Use ghz for integration/production validation

### Cost-Benefit Analysis

| Approach | Code Maintenance | Features | Coverage | **Score** |
|----------|------------------|----------|----------|-----------|
| **Custom Only** | High (~800 LOC) | Custom metrics ✅ | Pattern-level only ❌ | 2/3 |
| **ghz Only** | None | Standard metrics ❌ | Integration-level only ❌ | 1/3 |
| **Two-Tier** | Medium (~800 LOC) | Custom metrics ✅ | Both levels ✅ | **3/3** ✅ |
| k6 Only | None | Advanced ✅ | Integration-level only ❌ | 2/3 |

**Winner**: Two-Tier Strategy (3/3)

## Risks and Mitigations

### Risk 1: Maintaining Two Tools

**Risk**: Overhead of maintaining prism-loadtest + ghz scripts

**Mitigation**:
- prism-loadtest: Only ~800 LOC, well-tested
- ghz: No code to maintain (just shell scripts)
- Clear separation: developers use custom, QA uses ghz

### Risk 2: Results Divergence

**Risk**: Pattern-level vs integration-level results differ significantly

**Mitigation**:
- Expected: gRPC adds ~3-5ms latency (acceptable)
- Document baseline overhead
- Alert if divergence &gt; 10ms (indicates proxy issue)

### Risk 3: Tool Proliferation

**Risk**: Team confused about which tool to use

**Mitigation**:
- Clear documentation (this RFC)
- Decision matrix (see above)
- CI/CD examples

## Alternatives Considered

### Alternative 1: ghz Only

**Pros**:
- No custom code maintenance
- Standard tool

**Cons**:
- ❌ Can't test patterns directly
- ❌ No custom metrics (multicast delivery)
- ❌ Slower iteration (must start proxy)

**Decision**: Rejected - need pattern-level testing

### Alternative 2: k6 Only

**Pros**:
- Modern, powerful
- Large ecosystem

**Cons**:
- ❌ Requires JavaScript
- ❌ Overkill for simple tests
- ❌ No direct pattern integration

**Decision**: Rejected - too complex for current needs

### Alternative 3: Custom Only + vegeta Library

**Idea**: Embed vegeta library in prism-loadtest for HTTP testing

**Pros**:
- Single tool

**Cons**:
- ❌ vegeta doesn't support gRPC
- ❌ More maintenance burden

**Decision**: Rejected - vegeta doesn't support gRPC

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Tool Adoption** | 100% team uses tools | Survey after POC 2 |
| **Pattern-Level Coverage** | All patterns have load tests | Test inventory |
| **Integration Coverage** | All gRPC endpoints tested | ghz test suite |
| **Performance Baseline** | &lt;5ms P95 pattern-level | prism-loadtest results |
| **Performance Integration** | &lt;10ms P95 end-to-end | ghz results |
| **CI/CD Integration** | Load tests in CI pipeline | GitHub Actions |

## Next Steps

### Immediate (POC 1)

1. **Enhance prism-loadtest**:
   - [ ] Add ramp-up profile
   - [ ] Add JSON output
   - [ ] Add spike profile

2. **Document usage**:
   - [ ] Update README with examples
   - [ ] Create decision matrix doc

### Short-Term (POC 2)

3. **Install ghz**:
   - [ ] Add to Dockerfile
   - [ ] Create ghz test suite

4. **CI/CD Integration**:
   - [ ] Add ghz tests to GitHub Actions
   - [ ] Set up performance regression alerts

### Long-Term (POC 5+)

5. **Evaluate k6** (if needed):
   - [ ] Assess need for distributed load testing
   - [ ] If yes: Create k6 test suite

## Related Documents

- **[MEMO-010: Load Test Results](/memos/memo-010)** - Validation of custom tool
- **[RFC-021: POC 1 Implementation](/rfc/rfc-021)** - Three plugins implementation
- **[RFC-018: POC Strategy](/rfc/rfc-018)** - Overall POC roadmap
- **[CLAUDE.md](https://github.com/jrepp/prism-data-layer/blob/main/CLAUDE.md)** - Development workflow

## Conclusion

**Recommendation**: Adopt **two-tier load testing strategy**:

1. **Keep prism-loadtest** for pattern-level testing
   - Validated by MEMO-010
   - Custom metrics
   - Fast iteration

2. **Add ghz** for integration testing
   - Tests through Rust proxy
   - Standard gRPC tool
   - Zero code maintenance

This strategy provides:
- ✅ Comprehensive coverage (pattern + integration)
- ✅ Minimal maintenance (800 LOC + scripts)
- ✅ Clear separation (dev vs QA tools)
- ✅ Best-of-breed solutions for each use case

**Action**: Proceed with Phase 1 (enhance custom tool) and Phase 2 (add ghz).

## Revision History

- 2025-10-11: Initial evaluation of Go load testing frameworks and recommendation for two-tier strategy