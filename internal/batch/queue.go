package batch

import (
	"fmt"
	"sync"
	"time"
)

// BatchQueue manages requests pending batch submission to Anthropic API
type BatchQueue struct {
	mu                   sync.RWMutex
	requests             []*BatchRequest
	maxQueueSize         int
	maxBatchWaitTime     time.Duration
	minBatchSize         int
	lastFlushTime        time.Time
	idleTimeoutDuration  time.Duration // Auto-flush if idle this long
	totalProcessed       int64
	totalSaved           float64
	createdAt            time.Time
}

// NewBatchQueue creates a new batch queue with default settings
func NewBatchQueue() *BatchQueue {
	return &BatchQueue{
		requests:            make([]*BatchRequest, 0, 100),
		maxQueueSize:        100,
		maxBatchWaitTime:    5 * time.Minute,
		minBatchSize:        3,
		idleTimeoutDuration: 30 * time.Second,
		lastFlushTime:       time.Now(),
		createdAt:           time.Now(),
	}
}

// Enqueue adds a request to the queue
func (bq *BatchQueue) Enqueue(req *BatchRequest) error {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if len(bq.requests) >= bq.maxQueueSize {
		return fmt.Errorf("batch queue full (max %d requests)", bq.maxQueueSize)
	}

	bq.requests = append(bq.requests, req)
	return nil
}

// Dequeue removes and returns the oldest request
func (bq *BatchQueue) Dequeue() *BatchRequest {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if len(bq.requests) == 0 {
		return nil
	}

	req := bq.requests[0]
	bq.requests = bq.requests[1:]
	return req
}

// Size returns current queue size
func (bq *BatchQueue) Size() int {
	bq.mu.RLock()
	defer bq.mu.RUnlock()
	return len(bq.requests)
}

// IsEmpty returns true if queue has no requests
func (bq *BatchQueue) IsEmpty() bool {
	bq.mu.RLock()
	defer bq.mu.RUnlock()
	return len(bq.requests) == 0
}

// IsFull returns true if queue has reached max size
func (bq *BatchQueue) IsFull() bool {
	bq.mu.RLock()
	defer bq.mu.RUnlock()
	return len(bq.requests) >= bq.maxQueueSize
}

// IsReady returns true if batch should be flushed
// Conditions: size reached minBatchSize OR idle timeout exceeded OR queue full
func (bq *BatchQueue) IsReady() bool {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	size := len(bq.requests)
	if size == 0 {
		return false
	}

	// Flush if full (force submission)
	if size >= bq.maxQueueSize {
		return true
	}

	// Flush if minimum batch size reached
	if size >= bq.minBatchSize {
		return true
	}

	// Flush if idle timeout exceeded
	idleTime := time.Since(bq.lastFlushTime)
	if idleTime > bq.idleTimeoutDuration {
		return true
	}

	return false
}

// Flush returns all queued requests and clears the queue
func (bq *BatchQueue) Flush() []*BatchRequest {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if len(bq.requests) == 0 {
		return []*BatchRequest{}
	}

	// Sort by priority (high first, descending)
	bq.sortByPriority()

	requests := make([]*BatchRequest, len(bq.requests))
	copy(requests, bq.requests)
	bq.requests = bq.requests[:0]
	bq.lastFlushTime = time.Now()

	bq.totalProcessed += int64(len(requests))

	return requests
}

// sortByPriority sorts requests by priority (high values first)
func (bq *BatchQueue) sortByPriority() {
	for i := 0; i < len(bq.requests); i++ {
		for j := i + 1; j < len(bq.requests); j++ {
			if bq.requests[j].Priority > bq.requests[i].Priority {
				bq.requests[i], bq.requests[j] = bq.requests[j], bq.requests[i]
			}
		}
	}
}

// Peek returns the oldest request without removing it
func (bq *BatchQueue) Peek() *BatchRequest {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	if len(bq.requests) == 0 {
		return nil
	}
	return bq.requests[0]
}

// EstimatedWaitTime returns estimated wait for the oldest request
func (bq *BatchQueue) EstimatedWaitTime() time.Duration {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	size := len(bq.requests)
	if size == 0 {
		return 0
	}

	// Estimate based on position and processing time
	avgProcessTime := 10 * time.Second
	waitTime := time.Duration(size) * avgProcessTime

	if waitTime > bq.maxBatchWaitTime {
		waitTime = bq.maxBatchWaitTime
	}

	return waitTime
}

// QueueStats returns statistics about the queue
func (bq *BatchQueue) QueueStats() BatchQueueStats {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	stats := BatchQueueStats{
		Size:            len(bq.requests),
		MaxSize:         bq.maxQueueSize,
		MinSize:         bq.minBatchSize,
		TotalProcessed:  bq.totalProcessed,
		EstimatedSaved:  bq.totalSaved,
		CreatedAt:       bq.createdAt,
		LastFlushTime:   bq.lastFlushTime,
		IdleTime:        time.Since(bq.lastFlushTime),
		OldestRequestAge: 0,
		AverageAge:      0,
	}

	if len(bq.requests) > 0 {
		now := time.Now()
		totalAge := time.Duration(0)

		for i, req := range bq.requests {
			age := now.Sub(req.CreatedAt)
			if i == 0 || age > stats.OldestRequestAge {
				stats.OldestRequestAge = age
			}
			totalAge += age
		}

		stats.AverageAge = totalAge / time.Duration(len(bq.requests))
	}

	return stats
}

// BatchQueueStats contains queue metrics
type BatchQueueStats struct {
	Size              int
	MaxSize           int
	MinSize           int
	TotalProcessed    int64
	EstimatedSaved    float64
	CreatedAt         time.Time
	LastFlushTime     time.Time
	IdleTime          time.Duration
	OldestRequestAge  time.Duration
	AverageAge        time.Duration
}

// SetMinBatchSize sets minimum batch size before flushing
func (bq *BatchQueue) SetMinBatchSize(size int) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if size > 0 {
		bq.minBatchSize = size
	}
}

// SetMaxQueueSize sets maximum queue capacity
func (bq *BatchQueue) SetMaxQueueSize(size int) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if size > 0 {
		bq.maxQueueSize = size
	}
}

// SetIdleTimeout sets auto-flush timeout
func (bq *BatchQueue) SetIdleTimeout(duration time.Duration) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if duration > 0 {
		bq.idleTimeoutDuration = duration
	}
}

// SetMaxBatchWaitTime sets maximum acceptable wait time for batching
func (bq *BatchQueue) SetMaxBatchWaitTime(duration time.Duration) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if duration > 0 {
		bq.maxBatchWaitTime = duration
	}
}

// RecordSavings tracks cost savings from batch processing
func (bq *BatchQueue) RecordSavings(amount float64) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if amount > 0 {
		bq.totalSaved += amount
	}
}

// Clear removes all requests from the queue
func (bq *BatchQueue) Clear() {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	bq.requests = bq.requests[:0]
	bq.lastFlushTime = time.Now()
}
