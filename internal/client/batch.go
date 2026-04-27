package client

import (
	"context"
	"fmt"
	"time"
)

// BatchRequestPayload wraps a request for batch submission
type BatchRequestPayload struct {
	CustomID string         `json:"custom_id"`
	Params   MessageRequest `json:"params"`
}

// BatchJobStatus represents the state of a batch job
type BatchJobStatus string

const (
	BatchStatusQueued      BatchJobStatus = "queued"
	BatchStatusInProgress  BatchJobStatus = "in_progress"
	BatchStatusSucceeded   BatchJobStatus = "succeeded"
	BatchStatusFailed      BatchJobStatus = "failed"
	BatchStatusExpired     BatchJobStatus = "expired"
	BatchStatusCanceled    BatchJobStatus = "canceled"
)

// BatchJobTracker keeps track of submitted batch jobs
type BatchJobTracker struct {
	JobID          string
	SubmittedAt    time.Time
	ExpectedAt     time.Time
	LastPolledAt   time.Time
	RequestCount   int
	SucceededCount int
	ErroredCount   int
	CanceledCount  int
	Status         BatchJobStatus
	OutputFileID   string
	ErrorFileID    string
	Metadata       map[string]interface{}
}

// BatchQueue manages queuing and submission of batch requests
type BatchQueue struct {
	requests    []BatchRequest
	maxSize     int
	minSize     int
	timeout     time.Duration
	createdAt   time.Time
	lastFlushAt time.Time
}

// NewBatchQueue creates a new batch queue
func NewBatchQueue(minSize, maxSize int, timeout time.Duration) *BatchQueue {
	return &BatchQueue{
		requests:   make([]BatchRequest, 0, maxSize),
		minSize:    minSize,
		maxSize:    maxSize,
		timeout:    timeout,
		createdAt:  time.Now(),
		lastFlushAt: time.Now(),
	}
}

// Enqueue adds a request to the queue
func (q *BatchQueue) Enqueue(req BatchRequest) bool {
	if len(q.requests) >= q.maxSize {
		return false // Queue is full
	}
	q.requests = append(q.requests, req)
	return true
}

// IsReady checks if the queue should be flushed
func (q *BatchQueue) IsReady() bool {
	// Flush if we've reached max size
	if len(q.requests) >= q.maxSize {
		return true
	}

	// Flush if we've reached min size and timeout elapsed
	if len(q.requests) >= q.minSize && time.Since(q.createdAt) > q.timeout {
		return true
	}

	return false
}

// Size returns the number of requests in the queue
func (q *BatchQueue) Size() int {
	return len(q.requests)
}

// Requests returns a copy of all queued requests
func (q *BatchQueue) Requests() []BatchRequest {
	result := make([]BatchRequest, len(q.requests))
	copy(result, q.requests)
	return result
}

// Clear empties the queue
func (q *BatchQueue) Clear() {
	q.requests = q.requests[:0]
	q.lastFlushAt = time.Now()
}

// BatchPoller handles polling for batch job completion
type BatchPoller struct {
	client      *AnthropicClient
	pollInterval time.Duration
	maxWait     time.Duration
}

// NewBatchPoller creates a new batch poller
func NewBatchPoller(client *AnthropicClient, pollInterval, maxWait time.Duration) *BatchPoller {
	return &BatchPoller{
		client:       client,
		pollInterval: pollInterval,
		maxWait:      maxWait,
	}
}

// Poll continuously checks batch job status until completion or timeout
func (bp *BatchPoller) Poll(ctx context.Context, jobID string) (*BatchJob, error) {
	deadline := time.Now().Add(bp.maxWait)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("polling timeout exceeded for job %s", jobID)
		}

		job, err := bp.client.GetBatchStatus(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("failed to get batch status: %w", err)
		}

		// Check if job is complete
		if job.ProcessingStatus == "succeeded" || job.ProcessingStatus == "failed" || job.ProcessingStatus == "canceled" || job.ProcessingStatus == "expired" {
			return job, nil
		}

		// Wait before polling again
		select {
		case <-time.After(bp.pollInterval):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// BatchResultProcessor handles processing batch results
type BatchResultProcessor struct {
	client *AnthropicClient
}

// NewBatchResultProcessor creates a new result processor
func NewBatchResultProcessor(client *AnthropicClient) *BatchResultProcessor {
	return &BatchResultProcessor{
		client: client,
	}
}

// ProcessResults retrieves and processes batch results
func (brp *BatchResultProcessor) ProcessResults(ctx context.Context, jobID string) (map[string]*MessageResponse, error) {
	results, err := brp.client.GetBatchResults(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch results: %w", err)
	}

	processedResults := make(map[string]*MessageResponse)
	for _, result := range results {
		if result.Error != nil {
			// Log error but continue processing other results
			fmt.Printf("Error for request %s: %v\n", result.CustomID, result.Error)
			continue
		}
		processedResults[result.CustomID] = &result.Result
	}

	return processedResults, nil
}

// BatchCostTracker calculates cost savings from batch operations
type BatchCostTracker struct {
	calculator *CostCalculator
}

// NewBatchCostTracker creates a new batch cost tracker
func NewBatchCostTracker(calc *CostCalculator) *BatchCostTracker {
	return &BatchCostTracker{
		calculator: calc,
	}
}

// CompareCosts compares the cost of batch vs regular API
func (bct *BatchCostTracker) CompareCosts(inputTokens, outputTokens int64, requestCount int) map[string]interface{} {
	regularCost := bct.calculator.CalculateCost(inputTokens, outputTokens, false)
	batchCost := bct.calculator.CalculateCost(inputTokens, outputTokens, true)
	savings := regularCost - batchCost

	savingsPercent := 0.0
	if regularCost > 0 {
		savingsPercent = (savings / regularCost) * 100
	}

	perRequestSavings := 0.0
	if requestCount > 0 {
		perRequestSavings = savings / float64(requestCount)
	}

	return map[string]interface{}{
		"regular_api_cost": regularCost,
		"batch_api_cost":   batchCost,
		"total_savings":    savings,
		"savings_percent":  savingsPercent,
		"requests":         requestCount,
		"per_request_savings": perRequestSavings,
	}
}
