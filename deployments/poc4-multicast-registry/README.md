# POC 4: Multicast Registry Load Testing Deployment

This deployment sets up the complete infrastructure for load testing the Multicast Registry pattern with containerized backends.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  POC 4 Load Test Deployment                 │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐      ┌──────────────┐                    │
│  │    Redis     │      │     NATS     │                    │
│  │  (Registry)  │      │  (Messaging) │                    │
│  └──────┬───────┘      └──────┬───────┘                    │
│         │                     │                             │
│         └──────────┬──────────┘                             │
│                    │                                        │
│            ┌───────▼───────┐                                │
│            │  prism-loadtest  │                             │
│            │   CLI Tool      │                             │
│            └─────────────────┘                              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. Redis (Registry Backend)
- **Image**: `redis:7-alpine`
- **Port**: 6379
- **Purpose**: Stores identity metadata with TTL support
- **Health Check**: Redis PING command

### 2. NATS (Messaging Backend)
- **Image**: `nats:2-alpine`
- **Ports**:
  - 4222 (client connections)
  - 8222 (HTTP management)
- **Purpose**: Delivers multicast messages to registered identities
- **Health Check**: HTTP healthz endpoint

### 3. Load Test Tool (prism-loadtest)
- **Build**: Custom Go binary from source
- **Purpose**: Executes load tests against multicast registry pattern
- **Supported Commands**:
  - `register`: Load test identity registration
  - `enumerate`: Load test identity enumeration
  - `multicast`: Load test message broadcasting
  - `mixed`: Combined workload testing

## Quick Start

### 1. Start Infrastructure

```bash
cd deployments/poc4-multicast-registry
docker-compose up -d redis nats
```

Wait for health checks to pass:

```bash
docker-compose ps
```

### 2. Build Load Test Tool

```bash
docker-compose build loadtest
```

### 3. Run Load Tests

#### Register Workload (100 req/sec for 60 seconds)

```bash
docker-compose run --rm loadtest register -r 100 -d 60s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

#### Enumerate Workload

First, populate with some identities:

```bash
docker-compose run --rm loadtest register -r 100 -d 30s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

Then run enumerate test:

```bash
docker-compose run --rm loadtest enumerate -r 100 -d 60s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

#### Multicast Workload

With pre-existing identities, run multicast test:

```bash
docker-compose run --rm loadtest multicast -r 50 -d 60s --filter --status online --redis-addr redis:6379 --nats-servers nats://nats:4222
```

#### Mixed Workload (Recommended)

Run combined test with 50% register, 30% enumerate, 20% multicast:

```bash
docker-compose run --rm loadtest mixed -r 100 -d 120s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

### 4. View Results

Load test results are printed to stdout with detailed metrics:
- Total requests
- Success rate
- Latency percentiles (P50, P95, P99)
- Throughput (req/sec)
- Operation-specific stats

### 5. Monitor Backends

#### Redis

```bash
# Connect to Redis CLI
docker exec -it poc4-redis redis-cli

# Check registered identities count
DBSIZE

# View specific identity
HGETALL loadtest:loadtest-user-1

# Monitor operations in real-time
MONITOR
```

#### NATS

```bash
# View NATS server stats
curl http://localhost:8222/varz | jq

# View connections
curl http://localhost:8222/connz | jq
```

### 6. Cleanup

```bash
docker-compose down -v
```

## Load Test Scenarios

### Scenario 1: Registration Performance Baseline

**Goal**: Establish maximum registration throughput

```bash
docker-compose run --rm loadtest register -r 200 -d 60s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

**Expected Results**:
- Throughput: 150-200 req/sec
- P95 Latency: <10ms
- Success Rate: >99%

### Scenario 2: Enumerate Scalability

**Goal**: Test enumerate performance with 1000 registered identities

```bash
# Populate 1000 identities
docker-compose run --rm loadtest register -r 100 -d 10s --redis-addr redis:6379 --nats-servers nats://nats:4222

# Run enumerate test
docker-compose run --rm loadtest enumerate -r 100 -d 60s --filter --status online --redis-addr redis:6379 --nats-servers nats://nats:4222
```

**Expected Results**:
- Enumerate latency: <5ms for 1000 identities
- Throughput: 80-100 req/sec

### Scenario 3: Multicast Fan-Out

**Goal**: Validate multicast delivery to 100+ targets

```bash
# Populate 500 identities
docker-compose run --rm loadtest register -r 100 -d 5s --redis-addr redis:6379 --nats-servers nats://nats:4222

# Run multicast test
docker-compose run --rm loadtest multicast -r 50 -d 60s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

**Expected Results**:
- Multicast latency: <50ms for 100 targets
- Delivery rate: >98%
- Throughput: 40-50 multicasts/sec

### Scenario 4: Production-Like Mixed Workload

**Goal**: Simulate real-world usage with mixed operations

```bash
docker-compose run --rm loadtest mixed -r 100 -d 300s --register-pct 60 --enumerate-pct 30 --multicast-pct 10 --redis-addr redis:6379 --nats-servers nats://nats:4222
```

**Expected Results**:
- Register: ~60 req/sec, P95 <10ms
- Enumerate: ~30 req/sec, P95 <8ms
- Multicast: ~10 req/sec, P95 <30ms
- Overall success rate: >99%

## Troubleshooting

### Redis Connection Failures

```bash
# Check Redis is running
docker-compose ps redis

# View Redis logs
docker-compose logs redis

# Test connectivity
docker-compose run --rm loadtest sh -c "apk add redis && redis-cli -h redis ping"
```

### NATS Connection Failures

```bash
# Check NATS is running
docker-compose ps nats

# View NATS logs
docker-compose logs nats

# Test connectivity
curl http://localhost:8222/varz
```

### Load Test Failures

```bash
# Run with verbose output
docker-compose run --rm loadtest register -r 100 -d 10s -v --redis-addr redis:6379 --nats-servers nats://nats:4222

# Check backend health
docker-compose ps
```

## Performance Tuning

### Increase Rate Limit

For higher throughput testing:

```bash
docker-compose run --rm loadtest mixed -r 500 -d 60s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

### Adjust Duration

For long-running tests:

```bash
docker-compose run --rm loadtest mixed -r 100 -d 600s --redis-addr redis:6379 --nats-servers nats://nats:4222
```

### Custom Workload Mix

```bash
docker-compose run --rm loadtest mixed -r 100 -d 120s --register-pct 80 --enumerate-pct 15 --multicast-pct 5 --redis-addr redis:6379 --nats-servers nats://nats:4222
```

## Related Documentation

- [POC 4 Implementation Tracking](/pocs/poc-004-multicast-registry)
- [MEMO-009: POC 4 Complete Summary](/memos/memo-009)
- [RFC-017: Multicast Registry Pattern](/rfc/rfc-017)
- [Multicast Registry README](/patterns/multicast_registry/README.md)

## Next Steps

After validating load test results:

1. **POC 5**: Authentication & Multi-Tenancy
2. **POC 6**: Observability with OpenTelemetry
3. **Production Deployment**: Kubernetes with Redis Cluster + NATS Cluster
