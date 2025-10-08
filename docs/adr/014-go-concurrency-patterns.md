---
title: "ADR-014: Go Concurrency Patterns"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['go', 'concurrency', 'performance', 'patterns']
---

## Context

Go tooling may require concurrent operations for:
- Data migration across namespaces
- Load testing multiple backends
- Parallel health checks
- Batch processing operations

We need established concurrency patterns that:
- Utilize goroutines efficiently
- Handle errors gracefully
- Provide deterministic behavior
- Scale with available resources

## Decision

Use **fork-join concurrency model** with worker pools:

1. **Fork**: Spawn goroutines to process work in parallel
2. **Join**: Collect results and handle errors
3. **Worker pool**: Limit concurrency with configurable pool size
4. **Error propagation**: First error cancels remaining work
5. **Context-based cancellation**: Use `context.Context` for cleanup

## Rationale

### Architecture

```
                    ┌─────────────┐
                    │  Work Queue  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Distribute  │
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              │                         │
         FORK PHASE                     │
              │                         │
    ┌─────────▼─────────┐              │
    │   Worker Pool      │              │
    │  (goroutines)      │              │
    │                    │              │
    │  ┌────┐  ┌────┐   │              │
    │  │ W1 │  │ W2 │   │              │
    │  └─┬──┘  └─┬──┘   │              │
    │    │       │      │              │
    │  ┌─▼──┐  ┌─▼──┐   │              │
    │  │ W3 │  │ W4 │   │              │
    │  └────┘  └────┘   │              │
    └────────┬───────────┘              │
             │                          │
       (process work)                   │
             │                          │
    ┌────────▼───────────┐              │
    │   Results Channel  │              │
    └────────┬───────────┘              │
             │                          │
        JOIN PHASE                      │
             │                          │
    ┌────────▼───────────┐              │
    │  Collect Results   │              │
    └────────┬───────────┘              │
             │                          │
    ┌────────▼───────────┐              │
    │   Return to Caller │              │
    └────────────────────┘              │
```

### Implementation Pattern

```go
// Fork-join with worker pool
func ProcessParallel(ctx context.Context, items []string, workers int) ([]Result, error) {
    // FORK: Create channels
    jobs := make(chan string, len(items))
    results := make(chan Result, len(items))
    errs := make(chan error, workers)

    // Context for cancellation
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    // Launch worker pool
    var wg sync.WaitGroup
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go worker(ctx, jobs, results, errs, &wg)
    }

    // Send jobs
    for _, item := range items {
        jobs <- item
    }
    close(jobs)

    // JOIN: Collect results
    go func() {
        wg.Wait()
        close(results)
        close(errs)
    }()

    // Gather results
    collected := make([]Result, 0, len(items))
    for result := range results {
        collected = append(collected, result)
    }

    // Check for errors
    if err := <-errs; err != nil {
        return nil, fmt.Errorf("process parallel: %w", err)
    }

    return collected, nil
}

func worker(ctx context.Context, jobs <-chan string, results chan<- Result, errs chan<- error, wg *sync.WaitGroup) {
    defer wg.Done()

    for item := range jobs {
        select {
        case <-ctx.Done():
            return // Cancelled
        default:
            result, err := processItem(item)
            if err != nil {
                select {
                case errs <- fmt.Errorf("worker: %w", err):
                default: // Error already reported
                }
                return
            }
            results <- result
        }
    }
}
```

### Why Fork-Join

**Pros:**
- Simple mental model (fork work, join results)
- Natural fit for embarrassingly parallel problems
- Easy to reason about and test
- Goroutines are lightweight (can spawn thousands)

**Cons:**
- May buffer all results before returning
- Memory usage proportional to work size

### Alternative: errgroup Pattern

For simpler cases, use `golang.org/x/sync/errgroup`:

```go
import "golang.org/x/sync/errgroup"

func MigrateNamespaces(ctx context.Context, namespaces []string) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, ns := range namespaces {
        ns := ns // Capture for closure
        g.Go(func() error {
            return migrateNamespace(ctx, ns)
        })
    }

    // Wait for all migrations, return first error
    return g.Wait()
}
```

### Alternatives Considered

1. **Sequential Processing**
   - Pros: Simple, deterministic, low memory
   - Cons: Slow for large workloads
   - Rejected: Unacceptable performance for batch operations

2. **Pipeline Pattern (stages)**
   - Pros: Streaming, lower memory
   - Cons: Complex for our use cases
   - Rejected: Fork-join simpler for batch processing

3. **Unlimited Concurrency**
   - Pros: Maximum speed
   - Cons: Resource exhaustion
   - Rejected: Must bound concurrency

## Consequences

### Positive

- **10x-100x speedup** for parallel workloads
- Scales naturally with CPU cores
- Simple implementation with goroutines and channels
- Error handling via context cancellation

### Negative

- Memory usage: May buffer results
- Complexity: Error handling more nuanced than sequential
- Requires tuning worker pool size

### Neutral

- Worker pool size configurable (default: `runtime.NumCPU()`)
- Works well for batch operations up to 10k items

## Implementation Notes

### Worker Pool Sizing

```go
// Default: match CPU cores
func DefaultWorkers() int {
    return runtime.NumCPU()
}

// Allow override via flag
var workers = flag.Int("workers", DefaultWorkers(), "concurrent workers")
```

### Error Propagation

First error cancels all workers via `context.Context`:

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()

// First error triggers cancellation
case errs <- err:
    cancel() // Stop all workers
```

### Testing Concurrency

```go
func TestProcessParallel_Concurrent(t *testing.T) {
    items := []string{"a", "b", "c", "d", "e"}

    // Test with different worker counts
    for _, workers := range []int{1, 2, 4, 8} {
        t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
            results, err := ProcessParallel(context.Background(), items, workers)
            if err != nil {
                t.Fatal(err)
            }

            if len(results) != len(items) {
                t.Errorf("got %d results, want %d", len(results), len(items))
            }
        })
    }
}
```

### Benchmarking

```go
func BenchmarkProcessSequential(b *testing.B) {
    items := generateItems(1000)
    for i := 0; i < b.N; i++ {
        processSequential(items)
    }
}

func BenchmarkProcessParallel(b *testing.B) {
    items := generateItems(1000)
    for i := 0; i < b.N; i++ {
        ProcessParallel(context.Background(), items, runtime.NumCPU())
    }
}
```

## References

- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines)
- [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency)
- [errgroup documentation](https://pkg.go.dev/golang.org/x/sync/errgroup)
- ADR-012: Go for Tooling
- ADR-013: Go Error Handling Strategy
- org-stream-producer ADR-006: Concurrency Model

## Revision History

- 2025-10-07: Initial draft and acceptance (adapted from org-stream-producer)
