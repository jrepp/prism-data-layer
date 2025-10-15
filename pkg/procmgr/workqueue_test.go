package procmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkQueue_EnqueueDequeue tests basic enqueue and dequeue
func TestWorkQueue_EnqueueDequeue(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue without delay
	wq.Enqueue("test-1", 0)

	// Should be immediately available
	id, ok := wq.Dequeue()
	require.True(t, ok, "Item should be available")
	assert.Equal(t, ProcessID("test-1"), id)

	// Queue should be empty
	id, ok = wq.Dequeue()
	assert.False(t, ok, "Queue should be empty")
	assert.Equal(t, ProcessID(""), id)
}

// TestWorkQueue_DelayedDequeue tests dequeue with delay
func TestWorkQueue_DelayedDequeue(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue with 100ms delay
	wq.Enqueue("test-1", 100*time.Millisecond)

	// Should not be available immediately
	id, ok := wq.Dequeue()
	assert.False(t, ok, "Item should not be ready yet")
	assert.Equal(t, ProcessID(""), id)

	// Wait for delay
	time.Sleep(150 * time.Millisecond)

	// Should be available now
	id, ok = wq.Dequeue()
	require.True(t, ok, "Item should be ready")
	assert.Equal(t, ProcessID("test-1"), id)
}

// TestWorkQueue_MultipleItems tests ordering with multiple items
func TestWorkQueue_MultipleItems(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue in reverse time order
	wq.Enqueue("test-3", 300*time.Millisecond)
	wq.Enqueue("test-1", 100*time.Millisecond)
	wq.Enqueue("test-2", 200*time.Millisecond)

	assert.Equal(t, 3, wq.Len(), "Queue should have 3 items")

	// Wait for first item
	time.Sleep(150 * time.Millisecond)

	id, ok := wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-1"), id, "Should dequeue in time order")

	// Second item not ready yet
	id, ok = wq.Dequeue()
	assert.False(t, ok, "Second item not ready")

	// Wait for second item
	time.Sleep(100 * time.Millisecond)

	id, ok = wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-2"), id)

	// Wait for third item
	time.Sleep(100 * time.Millisecond)

	id, ok = wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-3"), id)

	// Queue empty
	assert.Equal(t, 0, wq.Len())
}

// TestWorkQueue_UpdateEarlierTime tests updating item with earlier time
func TestWorkQueue_UpdateEarlierTime(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue with long delay
	wq.Enqueue("test-1", 500*time.Millisecond)

	// Update with shorter delay
	wq.Enqueue("test-1", 100*time.Millisecond)

	// Should only have one item
	assert.Equal(t, 1, wq.Len())

	// Wait for shorter delay
	time.Sleep(150 * time.Millisecond)

	// Should be ready
	id, ok := wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-1"), id)
}

// TestWorkQueue_UpdateLaterTime tests updating item with later time (should not update)
func TestWorkQueue_UpdateLaterTime(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue with short delay
	wq.Enqueue("test-1", 100*time.Millisecond)

	// Try to update with longer delay (should be ignored)
	wq.Enqueue("test-1", 500*time.Millisecond)

	// Should still have one item
	assert.Equal(t, 1, wq.Len())

	// Wait for original delay
	time.Sleep(150 * time.Millisecond)

	// Should be ready (not waiting for 500ms)
	id, ok := wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-1"), id)
}

// TestWorkQueue_Wait tests notification channel
func TestWorkQueue_Wait(t *testing.T) {
	wq := NewWorkQueue()

	// Get wait channel
	waitCh := wq.Wait()

	// Enqueue item
	go func() {
		time.Sleep(50 * time.Millisecond)
		wq.Enqueue("test-1", 0)
	}()

	// Should receive notification
	select {
	case <-waitCh:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Should receive notification")
	}

	// Item should be available
	id, ok := wq.Dequeue()
	require.True(t, ok)
	assert.Equal(t, ProcessID("test-1"), id)
}

// TestWorkQueue_ConcurrentEnqueue tests concurrent enqueue operations
func TestWorkQueue_ConcurrentEnqueue(t *testing.T) {
	wq := NewWorkQueue()

	// Enqueue 100 items concurrently
	for i := 0; i < 100; i++ {
		go func(n int) {
			wq.Enqueue(ProcessID("test-"+string(rune(n))), 0)
		}(i)
	}

	// Wait for all enqueues
	time.Sleep(100 * time.Millisecond)

	// Should have 100 items
	assert.Equal(t, 100, wq.Len())

	// Dequeue all
	count := 0
	for {
		_, ok := wq.Dequeue()
		if !ok {
			break
		}
		count++
	}
	assert.Equal(t, 100, count)
}

// TestJitter tests jitter function
func TestJitter(t *testing.T) {
	baseDelay := 1 * time.Second

	// No jitter
	result := Jitter(baseDelay, 0.0)
	assert.Equal(t, baseDelay, result)

	// 50% jitter - should be between 0.5s and 1.5s
	for i := 0; i < 100; i++ {
		result := Jitter(baseDelay, 0.5)
		assert.GreaterOrEqual(t, result, 500*time.Millisecond)
		assert.LessOrEqual(t, result, 1500*time.Millisecond)
	}

	// 100% jitter - should be between 0s and 2s
	for i := 0; i < 100; i++ {
		result := Jitter(baseDelay, 1.0)
		assert.GreaterOrEqual(t, result, 0*time.Millisecond)
		assert.LessOrEqual(t, result, 2*time.Second)
	}
}

// TestExponentialBackoff tests exponential backoff calculation
func TestExponentialBackoff(t *testing.T) {
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second

	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 750 * time.Millisecond, 1250 * time.Millisecond},   // 1s ± 25%
		{1, 1500 * time.Millisecond, 2500 * time.Millisecond},  // 2s ± 25%
		{2, 3000 * time.Millisecond, 5000 * time.Millisecond},  // 4s ± 25%
		{3, 6000 * time.Millisecond, 10000 * time.Millisecond}, // 8s ± 25%
		{4, 12 * time.Second, 20 * time.Second},                // 16s ± 25%
		{5, 24 * time.Second, 40 * time.Second},                // 32s ± 25%
		{6, 45 * time.Second, 60 * time.Second},                // 64s capped at 60s ± 25%
		{10, 45 * time.Second, 60 * time.Second},               // Way over, capped at 60s
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, baseDelay, maxDelay)
		assert.GreaterOrEqual(t, result, tt.minExpected,
			"Attempt %d should be >= %v, got %v", tt.attempt, tt.minExpected, result)
		assert.LessOrEqual(t, result, tt.maxExpected,
			"Attempt %d should be <= %v, got %v", tt.attempt, tt.maxExpected, result)
	}
}

// TestExponentialBackoff_NegativeAttempt tests handling of negative attempt
func TestExponentialBackoff_NegativeAttempt(t *testing.T) {
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second

	result := ExponentialBackoff(-5, baseDelay, maxDelay)

	// Should treat as attempt 0
	assert.GreaterOrEqual(t, result, 750*time.Millisecond)
	assert.LessOrEqual(t, result, 1250*time.Millisecond)
}

// BenchmarkWorkQueue_Enqueue benchmarks enqueue operation
func BenchmarkWorkQueue_Enqueue(b *testing.B) {
	wq := NewWorkQueue()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wq.Enqueue(ProcessID("test"), 1*time.Second)
	}
}

// BenchmarkWorkQueue_Dequeue benchmarks dequeue operation
func BenchmarkWorkQueue_Dequeue(b *testing.B) {
	wq := NewWorkQueue()

	// Pre-populate queue
	for i := 0; i < b.N; i++ {
		wq.Enqueue(ProcessID("test"), 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wq.Dequeue()
	}
}

// BenchmarkExponentialBackoff benchmarks backoff calculation
func BenchmarkExponentialBackoff(b *testing.B) {
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExponentialBackoff(i%10, baseDelay, maxDelay)
	}
}
