package procmgr

import (
	"container/heap"
	"math"
	"math/rand"
	"sync"
	"time"
)

// WorkQueue manages process work items with backoff and priority
type WorkQueue interface {
	// Enqueue adds a process to the work queue with optional delay
	Enqueue(id ProcessID, delay time.Duration)

	// Dequeue removes and returns the next ready process
	// Returns (id, true) if item available, ("", false) if queue empty
	Dequeue() (ProcessID, bool)

	// Len returns the number of items in the queue
	Len() int

	// Wait blocks until an item is ready or context is cancelled
	Wait() <-chan struct{}
}

// workQueue implements WorkQueue using a priority queue (min-heap)
type workQueue struct {
	mu       sync.Mutex
	items    *workItemHeap
	notifyCh chan struct{}
	rand     *rand.Rand
}

// workItem represents a queued process
type workItem struct {
	id       ProcessID
	readyAt  time.Time
	priority int // Lower priority = process sooner (for heap)
	index    int // Index in heap (for heap.Interface)
}

// workItemHeap implements heap.Interface
type workItemHeap []*workItem

func (h workItemHeap) Len() int { return len(h) }

func (h workItemHeap) Less(i, j int) bool {
	// Sort by readyAt time (earliest first)
	return h[i].readyAt.Before(h[j].readyAt)
}

func (h workItemHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *workItemHeap) Push(x interface{}) {
	item := x.(*workItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *workItemHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// NewWorkQueue creates a new work queue
func NewWorkQueue() WorkQueue {
	items := &workItemHeap{}
	heap.Init(items)

	return &workQueue{
		items:    items,
		notifyCh: make(chan struct{}, 1),
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Enqueue adds a process to the work queue with optional delay
func (wq *workQueue) Enqueue(id ProcessID, delay time.Duration) {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	readyAt := time.Now().Add(delay)

	// Check if process already in queue - update readyAt if earlier
	for _, item := range *wq.items {
		if item.id == id {
			// Update only if new time is earlier
			if readyAt.Before(item.readyAt) {
				item.readyAt = readyAt
				heap.Fix(wq.items, item.index)
			}
			wq.notify()
			return
		}
	}

	// Add new item
	item := &workItem{
		id:      id,
		readyAt: readyAt,
	}
	heap.Push(wq.items, item)
	wq.notify()
}

// Dequeue removes and returns the next ready process
func (wq *workQueue) Dequeue() (ProcessID, bool) {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	if wq.items.Len() == 0 {
		return "", false
	}

	// Peek at first item
	item := (*wq.items)[0]

	// Check if ready
	if time.Now().Before(item.readyAt) {
		return "", false
	}

	// Remove and return
	heap.Pop(wq.items)
	return item.id, true
}

// Len returns the number of items in the queue
func (wq *workQueue) Len() int {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	return wq.items.Len()
}

// Wait returns a channel that signals when items may be ready
func (wq *workQueue) Wait() <-chan struct{} {
	return wq.notifyCh
}

// notify signals that queue state changed
func (wq *workQueue) notify() {
	select {
	case wq.notifyCh <- struct{}{}:
	default:
		// Already has pending notification
	}
}

// Jitter adds random jitter to a duration to prevent thundering herd
// jitterFraction is between 0.0 (no jitter) and 1.0 (up to 100% jitter)
func Jitter(duration time.Duration, jitterFraction float64) time.Duration {
	if jitterFraction <= 0 {
		return duration
	}
	if jitterFraction > 1.0 {
		jitterFraction = 1.0
	}

	// Random value between [0, jitterFraction]
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitter := r.Float64() * jitterFraction

	// Apply jitter: duration * (1 ± jitter)
	multiplier := 1.0 + (jitter * 2.0) - jitterFraction
	return time.Duration(float64(duration) * multiplier)
}

// ExponentialBackoff calculates exponential backoff duration
// attempt: number of failed attempts (0-indexed)
// baseDelay: initial delay (e.g., 1 second)
// maxDelay: maximum delay cap (e.g., 60 seconds)
// Returns duration with jitter applied
func ExponentialBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate exponential delay: baseDelay * 2^attempt
	multiplier := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(baseDelay) * multiplier)

	// Cap at maxDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±25%)
	return Jitter(delay, 0.25)
}
