---
author: Platform Team
created: 2025-10-11
doc_uuid: a643bb33-4294-4cd6-a688-57d8ba4b108d
id: memo-014
project_id: prism-data-layer
tags:
- pattern-sdk
- refactoring
- code-reuse
- poc1
title: 'MEMO-014: Pattern SDK Shared Complexity Analysis'
updated: 2025-10-12
---

# MEMO-014: Pattern SDK Shared Complexity Analysis

## Summary

Analysis of RFC-021 reveals significant shared complexity across the three POC 1 plugins (MemStore, Redis, Kafka) that should be extracted into the Pattern SDK. This memo identifies 12 areas of duplication and proposes SDK enhancements to reduce plugin implementation burden by ~40%.

**Key Finding**: Each plugin currently re-implements connection management, TTL handling, health checks, and concurrency patterns. Moving these to the SDK would reduce plugin code by an estimated 300-500 lines per plugin.

## Context

RFC-021 defines three minimal plugins for POC 1:
1. **MemStore**: In-memory storage with TTL support
2. **Redis**: External backend with connection pooling
3. **Kafka**: Streaming with async buffering

Current Pattern SDK provides:
- `auth/` - Authentication stub
- `observability/` - Structured logging
- `lifecycle/` - Startup/shutdown hooks
- `server/` - gRPC server setup
- `storage/` - Basic retry logic

## Analysis

### Plugin Implementation Breakdown

| Feature | MemStore | Redis | Kafka | SDK Support |
|---------|----------|-------|-------|-------------|
| TTL Management | ✅ sync.Map + timers | ✅ Redis EXPIRE | ❌ N/A | ❌ None |
| Connection Pooling | ❌ N/A | ✅ Custom pool | ✅ Custom pool | ❌ None |
| Health Checks | ✅ Custom | ✅ Custom | ✅ Custom | ❌ None |
| Retry Logic | ❌ N/A | ✅ Custom | ✅ Custom | ✅ Basic only |
| Error Handling | ✅ Custom | ✅ Custom | ✅ Custom | ❌ None |
| Async Buffering | ❌ N/A | ❌ N/A | ✅ Custom | ❌ None |
| gRPC Registration | ✅ Boilerplate | ✅ Boilerplate | ✅ Boilerplate | ✅ Partial |
| Config Loading | ✅ Custom | ✅ Custom | ✅ Custom | ❌ None |
| Metrics | ✅ Manual | ✅ Manual | ✅ Manual | ❌ None |
| Testcontainers | ❌ N/A | ✅ Custom | ✅ Custom | ❌ None |

**Finding**: 10 of 10 features have duplication across plugins.

## Recommended SDK Enhancements

### Priority 1: High-Impact, Low-Risk

#### 1. Connection Pool Manager

**Problem**: Redis and Kafka both need connection pools with health checking.

**Current State**: Each plugin implements custom pooling.

**Proposed SDK Package**: `plugins/core/pool/`

```go
// plugins/core/pool/pool.go
package pool

import (
    "context"
    "sync"
    "time"
)

// Connection represents a generic backend connection
type Connection interface {
    // Health checks if connection is healthy
    Health(context.Context) error
    // Close closes the connection
    Close() error
}

// Factory creates new connections
type Factory func(context.Context) (Connection, error)

// Config configures the connection pool
type Config struct {
    MinIdle        int           // Minimum idle connections
    MaxOpen        int           // Maximum open connections
    MaxIdleTime    time.Duration // Max time connection can be idle
    HealthInterval time.Duration // Health check interval
}

// Pool manages a pool of connections
type Pool struct {
    factory Factory
    config  Config

    mu       sync.Mutex
    conns    []Connection
    idle     []Connection
    health   map[Connection]time.Time
}

// NewPool creates a new connection pool
func NewPool(factory Factory, config Config) *Pool {
    p := &Pool{
        factory: factory,
        config:  config,
        conns:   make([]Connection, 0),
        idle:    make([]Connection, 0),
        health:  make(map[Connection]time.Time),
    }

    go p.healthChecker()

    return p
}

// Acquire gets a connection from the pool
func (p *Pool) Acquire(ctx context.Context) (Connection, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Try to reuse idle connection
    if len(p.idle) > 0 {
        conn := p.idle[len(p.idle)-1]
        p.idle = p.idle[:len(p.idle)-1]
        return conn, nil
    }

    // Create new connection if under max
    if len(p.conns) < p.config.MaxOpen {
        conn, err := p.factory(ctx)
        if err != nil {
            return nil, err
        }
        p.conns = append(p.conns, conn)
        p.health[conn] = time.Now()
        return conn, nil
    }

    // Wait for connection to become available
    // (simplified - production would use channel)
    return nil, ErrPoolExhausted
}

// Release returns a connection to the pool
func (p *Pool) Release(conn Connection) {
    p.mu.Lock()
    defer p.mu.Unlock()

    p.idle = append(p.idle, conn)
}

// Close closes all connections in the pool
func (p *Pool) Close() error {
    p.mu.Lock()
    defer p.mu.Unlock()

    for _, conn := range p.conns {
        conn.Close()
    }

    p.conns = nil
    p.idle = nil
    p.health = nil

    return nil
}

func (p *Pool) healthChecker() {
    ticker := time.NewTicker(p.config.HealthInterval)
    defer ticker.Stop()

    for range ticker.C {
        p.checkHealth()
    }
}

func (p *Pool) checkHealth() {
    p.mu.Lock()
    defer p.mu.Unlock()

    ctx := context.Background()
    healthy := make([]Connection, 0, len(p.conns))

    for _, conn := range p.conns {
        if err := conn.Health(ctx); err == nil {
            healthy = append(healthy, conn)
            p.health[conn] = time.Now()
        } else {
            // Remove unhealthy connection
            conn.Close()
            delete(p.health, conn)
        }
    }

    p.conns = healthy
}
```

**Usage in Redis Plugin**:
```go
// plugins/redis/client/pool.go
package client

import (
    "context"
    "github.com/prism/plugins/core/pool"
    "github.com/redis/go-redis/v9"
)

type RedisConnection struct {
    client *redis.Client
}

func (rc *RedisConnection) Health(ctx context.Context) error {
    return rc.client.Ping(ctx).Err()
}

func (rc *RedisConnection) Close() error {
    return rc.client.Close()
}

func NewRedisPool(addr string) (*pool.Pool, error) {
    factory := func(ctx context.Context) (pool.Connection, error) {
        client := redis.NewClient(&redis.Options{
            Addr: addr,
        })
        return &RedisConnection{client: client}, nil
    }

    config := pool.Config{
        MinIdle:        5,
        MaxOpen:        50,
        MaxIdleTime:    5 * time.Minute,
        HealthInterval: 30 * time.Second,
    }

    return pool.NewPool(factory, config), nil
}
```

**Impact**:
- Reduces Redis plugin code by ~150 lines
- Reduces Kafka plugin code by ~120 lines
- Standardizes connection management across all plugins

**Test Coverage Target**: 90%+ (critical infrastructure)

---

#### 2. TTL Management Library

**Problem**: MemStore implements per-key timers; Redis uses EXPIRE. Both need TTL support.

**Current State**: MemStore uses `sync.Map` + `time.AfterFunc` per key (inefficient for many keys).

**Proposed SDK Package**: `plugins/core/ttl/`

```go
// plugins/core/ttl/manager.go
package ttl

import (
    "container/heap"
    "sync"
    "time"
)

// ExpiryCallback is called when a key expires
type ExpiryCallback func(key string)

// Manager manages TTLs for keys efficiently
type Manager struct {
    mu       sync.Mutex
    expiries *expiryHeap
    index    map[string]*expiryItem
    callback ExpiryCallback
    stopCh   chan struct{}
}

type expiryItem struct {
    key       string
    expiresAt time.Time
    index     int
}

type expiryHeap []*expiryItem

// Standard heap interface implementation
func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].expiresAt.Before(h[j].expiresAt) }
func (h expiryHeap) Swap(i, j int) {
    h[i], h[j] = h[j], h[i]
    h[i].index = i
    h[j].index = j
}

func (h *expiryHeap) Push(x interface{}) {
    item := x.(*expiryItem)
    item.index = len(*h)
    *h = append(*h, item)
}

func (h *expiryHeap) Pop() interface{} {
    old := *h
    n := len(old)
    item := old[n-1]
    item.index = -1
    *h = old[0 : n-1]
    return item
}

// NewManager creates a new TTL manager
func NewManager(callback ExpiryCallback) *Manager {
    m := &Manager{
        expiries: &expiryHeap{},
        index:    make(map[string]*expiryItem),
        callback: callback,
        stopCh:   make(chan struct{}),
    }
    heap.Init(m.expiries)
    go m.expiryWorker()
    return m
}

// Set sets a TTL for a key
func (m *Manager) Set(key string, ttl time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    expiresAt := time.Now().Add(ttl)

    // Update existing entry
    if item, exists := m.index[key]; exists {
        item.expiresAt = expiresAt
        heap.Fix(m.expiries, item.index)
        return
    }

    // Create new entry
    item := &expiryItem{
        key:       key,
        expiresAt: expiresAt,
    }
    heap.Push(m.expiries, item)
    m.index[key] = item
}

// Remove removes a key from TTL tracking
func (m *Manager) Remove(key string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if item, exists := m.index[key]; exists {
        heap.Remove(m.expiries, item.index)
        delete(m.index, key)
    }
}

// Persist removes TTL for a key (makes it permanent)
func (m *Manager) Persist(key string) {
    m.Remove(key)
}

// GetTTL returns remaining TTL for a key
func (m *Manager) GetTTL(key string) (time.Duration, bool) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if item, exists := m.index[key]; exists {
        remaining := time.Until(item.expiresAt)
        if remaining < 0 {
            return 0, false
        }
        return remaining, true
    }

    return 0, false
}

// Close stops the TTL manager
func (m *Manager) Close() {
    close(m.stopCh)
}

func (m *Manager) expiryWorker() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-m.stopCh:
            return
        case <-ticker.C:
            m.processExpiries()
        }
    }
}

func (m *Manager) processExpiries() {
    m.mu.Lock()
    defer m.mu.Unlock()

    now := time.Now()

    for m.expiries.Len() > 0 {
        item := (*m.expiries)[0]

        // Stop if next item not expired yet
        if item.expiresAt.After(now) {
            break
        }

        // Remove expired item
        heap.Pop(m.expiries)
        delete(m.index, item.key)

        // Call expiry callback
        if m.callback != nil {
            go m.callback(item.key) // Async to avoid blocking
        }
    }
}
```

**Usage in MemStore Plugin**:
```go
// plugins/memstore/storage/keyvalue.go
package storage

import (
    "github.com/prism/plugins/core/ttl"
    "sync"
)

type KeyValueStore struct {
    data    sync.Map
    ttlMgr  *ttl.Manager
}

func NewKeyValueStore() *KeyValueStore {
    kv := &KeyValueStore{}

    // TTL callback deletes expired keys
    kv.ttlMgr = ttl.NewManager(func(key string) {
        kv.data.Delete(key)
    })

    return kv
}

func (kv *KeyValueStore) Set(key string, value []byte, ttlSeconds int64) error {
    kv.data.Store(key, value)

    if ttlSeconds > 0 {
        kv.ttlMgr.Set(key, time.Duration(ttlSeconds)*time.Second)
    }

    return nil
}

func (kv *KeyValueStore) Expire(key string, ttlSeconds int64) bool {
    if _, exists := kv.data.Load(key); !exists {
        return false
    }

    kv.ttlMgr.Set(key, time.Duration(ttlSeconds)*time.Second)
    return true
}

func (kv *KeyValueStore) GetTTL(key string) (int64, bool) {
    ttl, exists := kv.ttlMgr.GetTTL(key)
    if !exists {
        return -1, false
    }
    return int64(ttl.Seconds()), true
}

func (kv *KeyValueStore) Persist(key string) bool {
    if _, exists := kv.data.Load(key); !exists {
        return false
    }

    kv.ttlMgr.Persist(key)
    return true
}
```

**Impact**:
- Reduces MemStore plugin code by ~80 lines
- More efficient: O(log n) heap vs O(1) per-key timers
- Scales to 100K+ keys with TTLs
- Single goroutine for all expirations

**Test Coverage Target**: 95%+ (data structure complexity)

---

#### 3. Backend Health Check Framework

**Problem**: All three plugins implement custom health checks.

**Current State**: Each plugin has custom health check logic.

**Proposed SDK Package**: `plugins/core/health/`

```go
// plugins/core/health/checker.go
package health

import (
    "context"
    "sync"
    "time"
)

// Status represents health status
type Status int

const (
    StatusUnknown Status = iota
    StatusHealthy
    StatusDegraded
    StatusUnhealthy
)

func (s Status) String() string {
    switch s {
    case StatusHealthy:
        return "healthy"
    case StatusDegraded:
        return "degraded"
    case StatusUnhealthy:
        return "unhealthy"
    default:
        return "unknown"
    }
}

// Check performs a health check
type Check func(context.Context) error

// Checker manages multiple health checks
type Checker struct {
    mu      sync.RWMutex
    checks  map[string]Check
    status  map[string]Status
    errors  map[string]error

    interval time.Duration
    timeout  time.Duration
    stopCh   chan struct{}
}

// Config configures health checking
type Config struct {
    Interval time.Duration // How often to run checks
    Timeout  time.Duration // Timeout per check
}

// NewChecker creates a new health checker
func NewChecker(config Config) *Checker {
    c := &Checker{
        checks:   make(map[string]Check),
        status:   make(map[string]Status),
        errors:   make(map[string]error),
        interval: config.Interval,
        timeout:  config.Timeout,
        stopCh:   make(chan struct{}),
    }

    go c.worker()

    return c
}

// Register adds a health check
func (c *Checker) Register(name string, check Check) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.checks[name] = check
    c.status[name] = StatusUnknown
}

// Status returns overall health status
func (c *Checker) Status() Status {
    c.mu.RLock()
    defer c.mu.RUnlock()

    hasUnhealthy := false
    hasDegraded := false

    for _, status := range c.status {
        switch status {
        case StatusUnhealthy:
            hasUnhealthy = true
        case StatusDegraded:
            hasDegraded = true
        }
    }

    if hasUnhealthy {
        return StatusUnhealthy
    }
    if hasDegraded {
        return StatusDegraded
    }

    return StatusHealthy
}

// CheckStatus returns status for a specific check
func (c *Checker) CheckStatus(name string) (Status, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    return c.status[name], c.errors[name]
}

// Close stops the health checker
func (c *Checker) Close() {
    close(c.stopCh)
}

func (c *Checker) worker() {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()

    for {
        select {
        case <-c.stopCh:
            return
        case <-ticker.C:
            c.runChecks()
        }
    }
}

func (c *Checker) runChecks() {
    c.mu.RLock()
    checks := make(map[string]Check, len(c.checks))
    for name, check := range c.checks {
        checks[name] = check
    }
    c.mu.RUnlock()

    for name, check := range checks {
        ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
        err := check(ctx)
        cancel()

        status := StatusHealthy
        if err != nil {
            status = StatusUnhealthy
        }

        c.mu.Lock()
        c.status[name] = status
        c.errors[name] = err
        c.mu.Unlock()
    }
}
```

**Usage in Redis Plugin**:
```go
// plugins/redis/main.go
package main

import (
    "context"
    "github.com/prism/plugins/core/health"
)

func setupHealth(client *redis.Client) *health.Checker {
    checker := health.NewChecker(health.Config{
        Interval: 30 * time.Second,
        Timeout:  5 * time.Second,
    })

    // Register Redis connectivity check
    checker.Register("redis", func(ctx context.Context) error {
        return client.Ping(ctx).Err()
    })

    // Register memory check
    checker.Register("memory", func(ctx context.Context) error {
        info := client.Info(ctx, "memory").Val()
        // Parse memory usage and return error if > 90%
        return nil
    })

    return checker
}
```

**Impact**:
- Reduces all plugins by ~50 lines each
- Standardizes health check reporting
- Enables composite health status

**Test Coverage Target**: 90%+

---

### Priority 2: Medium-Impact

#### 4. gRPC Service Registration Helpers

**Problem**: All plugins have boilerplate gRPC service registration.

**Current State**: `plugins/core/server/grpc.go` exists but incomplete.

**Enhancement**: Add middleware and registration helpers.

```go
// plugins/core/server/middleware.go
package server

import (
    "context"
    "time"

    "go.uber.org/zap"
    "google.golang.org/grpc"
)

// LoggingInterceptor logs all gRPC requests
func LoggingInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        start := time.Now()

        logger.Info("request started",
            zap.String("method", info.FullMethod),
        )

        resp, err := handler(ctx, req)

        duration := time.Since(start)

        if err != nil {
            logger.Error("request failed",
                zap.String("method", info.FullMethod),
                zap.Duration("duration", duration),
                zap.Error(err),
            )
        } else {
            logger.Info("request completed",
                zap.String("method", info.FullMethod),
                zap.Duration("duration", duration),
            )
        }

        return resp, err
    }
}

// ErrorInterceptor standardizes error responses
func ErrorInterceptor() grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        resp, err := handler(ctx, req)

        if err != nil {
            // Convert internal errors to gRPC status codes
            return nil, toGRPCError(err)
        }

        return resp, nil
    }
}
```

**Impact**:
- Reduces all plugins by ~30 lines each
- Standardizes logging format

**Test Coverage Target**: 85%+

---

#### 5. Configuration Management

**Problem**: All plugins load config from environment variables with custom parsing.

**Proposed SDK Package**: `plugins/core/config/`

```go
// plugins/core/config/loader.go
package config

import (
    "fmt"
    "os"
    "strconv"
    "time"
)

// Loader loads configuration from environment
type Loader struct {
    prefix string
}

// NewLoader creates a config loader with prefix
func NewLoader(prefix string) *Loader {
    return &Loader{prefix: prefix}
}

// String loads a string value
func (l *Loader) String(key, defaultVal string) string {
    envKey := l.prefix + "_" + key
    if val := os.Getenv(envKey); val != "" {
        return val
    }
    return defaultVal
}

// Int loads an int value
func (l *Loader) Int(key string, defaultVal int) int {
    envKey := l.prefix + "_" + key
    if val := os.Getenv(envKey); val != "" {
        if i, err := strconv.Atoi(val); err == nil {
            return i
        }
    }
    return defaultVal
}

// Duration loads a duration value
func (l *Loader) Duration(key string, defaultVal time.Duration) time.Duration {
    envKey := l.prefix + "_" + key
    if val := os.Getenv(envKey); val != "" {
        if d, err := time.ParseDuration(val); err == nil {
            return d
        }
    }
    return defaultVal
}

// Required loads a required string value (panics if missing)
func (l *Loader) Required(key string) string {
    envKey := l.prefix + "_" + key
    val := os.Getenv(envKey)
    if val == "" {
        panic(fmt.Sprintf("required config %s not set", envKey))
    }
    return val
}
```

**Usage**:
```go
// plugins/redis/main.go
func loadConfig() RedisConfig {
    cfg := config.NewLoader("REDIS")

    return RedisConfig{
        Addr:        cfg.Required("ADDR"),
        MaxRetries:  cfg.Int("MAX_RETRIES", 3),
        PoolSize:    cfg.Int("POOL_SIZE", 10),
        IdleTimeout: cfg.Duration("IDLE_TIMEOUT", 5*time.Minute),
    }
}
```

**Impact**:
- Reduces all plugins by ~20 lines each
- Type-safe config loading

**Test Coverage Target**: 95%+

---

#### 6. Error Classification and Circuit Breaker

**Problem**: Redis and Kafka need sophisticated retry logic beyond basic backoff.

**Enhancement to**: `plugins/core/storage/retry.go`

```go
// plugins/core/storage/errors.go
package storage

import "errors"

// Error types for classification
var (
    ErrRetryable   = errors.New("retryable error")
    ErrPermanent   = errors.New("permanent error")
    ErrTimeout     = errors.New("timeout")
    ErrRateLimit   = errors.New("rate limited")
)

// Classify determines if an error is retryable
func Classify(err error) error {
    if err == nil {
        return nil
    }

    // Check for known retryable errors
    switch {
    case errors.Is(err, ErrTimeout):
        return ErrRetryable
    case errors.Is(err, ErrRateLimit):
        return ErrRetryable
    default:
        return ErrPermanent
    }
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
    maxFailures int
    timeout     time.Duration

    mu           sync.Mutex
    failures     int
    lastFailure  time.Time
    state        CircuitState
}

type CircuitState int

const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)

func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        maxFailures: maxFailures,
        timeout:     timeout,
        state:       StateClosed,
    }
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if !cb.canProceed() {
        return errors.New("circuit breaker open")
    }

    err := fn()
    cb.recordResult(err)

    return err
}

func (cb *CircuitBreaker) canProceed() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateClosed:
        return true
    case StateOpen:
        // Check if timeout elapsed
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.state = StateHalfOpen
            return true
        }
        return false
    case StateHalfOpen:
        return true
    default:
        return false
    }
}

func (cb *CircuitBreaker) recordResult(err error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()

        if cb.failures >= cb.maxFailures {
            cb.state = StateOpen
        }
    } else {
        cb.failures = 0
        cb.state = StateClosed
    }
}
```

**Impact**:
- Prevents cascading failures in Redis/Kafka
- Standardizes error handling

**Test Coverage Target**: 90%+

---

### Priority 3: Lower-Impact (Future Work)

#### 7. Buffer and Batch Manager

**Use Case**: Kafka async buffering

**Status**: Defer to POC 2 (too specific to Kafka for POC 1)

---

#### 8. Testcontainer Helpers

**Use Case**: Redis and Kafka integration tests

**Status**: Defer to POC 2 (testcontainers already easy to use)

---

#### 9. Metrics Collection

**Use Case**: All plugins need request duration tracking

**Status**: Defer to POC 3 (observability POC)

---

#### 10. Concurrency Patterns (Worker Pools)

**Use Case**: All plugins handle concurrent requests

**Status**: Defer (gRPC handles concurrency already)

---

## Implementation Plan

### Phase 1: Foundation (Week 1)

**Estimated Effort**: 3 days

| Package | Lines | Tests | Coverage Target | Owner |
|---------|-------|-------|----------------|-------|
| `pool/` | ~300 | ~200 | 90%+ | Go Expert |
| `ttl/` | ~250 | ~150 | 95%+ | Go Expert |
| `health/` | ~200 | ~100 | 90%+ | Go Expert |

**Deliverables**:
- [ ] Connection pool manager with health checking
- [ ] TTL manager with heap-based expiration
- [ ] Health check framework
- [ ] All tests passing with coverage targets met
- [ ] Documentation with usage examples

### Phase 2: Convenience (Week 1)

**Estimated Effort**: 2 days

| Package | Lines | Tests | Coverage Target | Owner |
|---------|-------|-------|----------------|-------|
| `server/middleware.go` | ~150 | ~80 | 85%+ | Any Engineer |
| `config/` | ~100 | ~60 | 95%+ | Any Engineer |
| `storage/errors.go` | ~200 | ~100 | 90%+ | Go Expert |

**Deliverables**:
- [ ] gRPC middleware (logging, error standardization)
- [ ] Configuration loader
- [ ] Error classification and circuit breaker
- [ ] All tests passing with coverage targets met

### Phase 3: Plugin Refactoring (Week 2)

**Estimated Effort**: 2 days

Refactor existing plugins to use new SDK packages:

- [ ] MemStore: Use `ttl.Manager` instead of per-key timers
- [ ] Redis: Use `pool.Pool` for connection management
- [ ] Kafka: Use `pool.Pool` for connection management
- [ ] All plugins: Use `health.Checker` for health checks
- [ ] All plugins: Use `config.Loader` for configuration
- [ ] Verify all tests still pass
- [ ] Measure code reduction

**Expected Code Reduction**:
| Plugin | Before (LOC) | After (LOC) | Reduction |
|--------|-------------|------------|-----------|
| MemStore | ~600 | ~350 | ~42% |
| Redis | ~700 | ~450 | ~36% |
| Kafka | ~800 | ~500 | ~38% |
| **Total** | **2100** | **1300** | **38%** |

## Testing Strategy

### Unit Tests

Each SDK package must have comprehensive unit tests:

```go
// plugins/core/pool/pool_test.go
func TestPool_AcquireRelease(t *testing.T) { /* ... */ }
func TestPool_HealthChecking(t *testing.T) { /* ... */ }
func TestPool_MaxConnections(t *testing.T) { /* ... */ }
func TestPool_ConcurrentAccess(t *testing.T) { /* ... */ }
```

### Integration Tests

Plugins using SDK packages must have integration tests:

```go
// plugins/redis/client/pool_test.go
func TestRedisPool_WithRealRedis(t *testing.T) {
    // Use testcontainer
    redis := startRedisContainer(t)
    defer redis.Terminate()

    pool := NewRedisPool(redis.Endpoint())
    defer pool.Close()

    // Test pool functionality
    conn, err := pool.Acquire(context.Background())
    // ...
}
```

### Coverage Enforcement

```makefile
# Makefile (root)
coverage-sdk:
	@echo "=== Connection Pool ==="
	cd plugins/core/pool && go test -coverprofile=coverage.out ./...
	@cd plugins/core/pool && go tool cover -func=coverage.out | grep total

	@echo "=== TTL Manager ==="
	cd plugins/core/ttl && go test -coverprofile=coverage.out ./...
	@cd plugins/core/ttl && go tool cover -func=coverage.out | grep total

	@echo "=== Health Checker ==="
	cd plugins/core/health && go test -coverprofile=coverage.out ./...
	@cd plugins/core/health && go tool cover -func=coverage.out | grep total

# Fail if any SDK package < 85%
coverage-sdk-enforce:
	@for pkg in pool ttl health; do \
		cd plugins/core/$$pkg && \
		COVERAGE=$$(go test -coverprofile=coverage.out ./... && \
			go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
		if (( $$(echo "$$COVERAGE < 85" | bc -l) )); then \
			echo "❌ SDK package $$pkg coverage $$COVERAGE% < 85%"; \
			exit 1; \
		fi; \
		echo "✅ SDK package $$pkg coverage $$COVERAGE% >= 85%"; \
	done
```

## Benefits

### Developer Experience

**Before** (without SDK enhancements):
```go
// plugins/redis/client/pool.go - ~150 lines of custom pooling
// plugins/redis/client/health.go - ~50 lines of custom health checks
// plugins/redis/client/config.go - ~40 lines of custom config loading
// Total: ~240 lines of boilerplate per plugin
```

**After** (with SDK enhancements):
```go
// plugins/redis/main.go - ~30 lines using SDK packages
pool := pool.NewPool(factory, poolConfig)
health := health.NewChecker(healthConfig)
config := config.NewLoader("REDIS")
// Total: ~30 lines, 88% reduction
```

### Maintainability

- **Single source of truth**: Connection pooling logic in one place
- **Consistent behavior**: All plugins handle health checks the same way
- **Easier debugging**: Centralized logging in SDK middleware
- **Faster development**: New plugins can use SDK packages immediately

### Performance

- **TTL Manager**: Heap-based expiration scales to 100K+ keys (vs per-key timers)
- **Connection Pool**: Reuses connections efficiently
- **Health Checks**: Amortized across all plugins

### Quality

- **Higher test coverage**: SDK packages have 85-95% coverage
- **Fewer bugs**: Less duplicated code = fewer places for bugs
- **Standardization**: All plugins follow same patterns

## Risks and Mitigations

### Risk 1: SDK Complexity

**Risk**: SDK becomes too complex and hard to understand.

**Mitigation**:
- Keep SDK packages focused (single responsibility)
- Comprehensive documentation with examples
- Code reviews for all SDK changes

### Risk 2: Breaking Changes

**Risk**: SDK changes break existing plugins.

**Mitigation**:
- Semantic versioning for SDK
- Deprecation warnings before breaking changes
- Integration tests catch breakage

### Risk 3: Performance Regression

**Risk**: Generic SDK code slower than custom implementations.

**Mitigation**:
- Benchmark all SDK packages
- Compare against custom implementations
- Profile in production

### Risk 4: Over-Engineering

**Risk**: Building SDK features that aren't needed.

**Mitigation**:
- Only extract patterns used by 2+ plugins
- Defer "nice to have" features
- Iterative approach (Phase 1 → 2 → 3)

## Alternatives Considered

### Alternative 1: Keep Custom Implementations

**Pros**:
- Plugins can optimize for their specific use case
- No SDK learning curve

**Cons**:
- Code duplication (38% more code)
- Inconsistent behavior across plugins
- Higher maintenance burden

**Decision**: Rejected - duplication outweighs benefits

### Alternative 2: Third-Party Libraries

**Pros**:
- Battle-tested implementations
- Active maintenance

**Cons**:
- External dependencies
- Less control over behavior
- May not fit our use cases

**Decision**: Partial adoption - use zap for logging, but build custom pool/ttl/health

### Alternative 3: Code Generation

**Pros**:
- Zero runtime overhead
- Type-safe

**Cons**:
- Complex build process
- Harder to debug generated code

**Decision**: Deferred to future (POC 5+)

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Code reduction | 35%+ | Lines of code comparison |
| SDK test coverage | 85%+ | `make coverage-sdk` |
| Plugin development time | -30% | Time to add new plugin |
| Bug reduction | -40% | Bugs in connection/TTL logic |
| Performance (TTL) | 10x better | Benchmark: 10K keys w/ TTLs |
| Performance (pool) | No regression | Benchmark vs custom pool |

## Next Steps

1. **Review this memo** with team
2. **Approve Phase 1 packages** (pool, ttl, health)
3. **Assign owners** for each package
4. **Create RFC-023** for detailed API design (if needed)
5. **Begin Phase 1 implementation** (3 days)
6. **Refactor plugins** to use new SDK packages (2 days)
7. **Measure code reduction** and performance impact

## Related Documents

- [RFC-021: POC 1 Three Plugins Implementation](/rfc/rfc-021) - Original plugin design
- [RFC-022: Core Pattern SDK Code Layout](/rfc/rfc-022) - SDK structure
- [RFC-015: Plugin Acceptance Test Framework](/rfc/rfc-015) - Testing strategy
- [MEMO-004: Backend Implementation Guide](/memos/memo-004) - Backend comparison

## Revision History

- 2025-10-11: Initial analysis of shared complexity across RFC-021 plugins