package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
)

// BatchPoller manages polling of submitted batch jobs
type BatchPoller struct {
	mu                sync.RWMutex
	anthropicClient   *client.AnthropicClient
	jobs              map[string]*BatchJobTracker
	pollingInterval   time.Duration
	maxRetries        int
	retryBackoff      time.Duration
	stopChan          chan struct{}
	wg                sync.WaitGroup
	isRunning         bool
	totalJobsPolled   int64
	totalJobsComplete int64
	totalJobsFailed   int64
}

// BatchJobTracker tracks status and results of a submitted batch
type BatchJobTracker struct {
	JobID           string
	SubmittedAt     time.Time
	LastPolledAt    time.Time
	Status          string // queued, in_progress, succeeded, failed, expired
	Results         []*client.BatchResult
	ErrorMessage    string
	RequestCount    int
	SuccessCount    int
	FailureCount    int
	ProcessingCount int
}

// NewBatchPoller creates a new batch poller
func NewBatchPoller(anthropicClient *client.AnthropicClient) *BatchPoller {
	return &BatchPoller{
		anthropicClient: anthropicClient,
		jobs:            make(map[string]*BatchJobTracker),
		pollingInterval: 10 * time.Second,
		maxRetries:      5,
		retryBackoff:    2 * time.Second,
		stopChan:        make(chan struct{}),
		isRunning:       false,
	}
}

// Start begins polling for batch job completion in background
func (bp *BatchPoller) Start(ctx context.Context) error {
	bp.mu.Lock()
	if bp.isRunning {
		bp.mu.Unlock()
		return fmt.Errorf("poller already running")
	}
	bp.isRunning = true
	bp.mu.Unlock()

	bp.wg.Add(1)
	go bp.pollingLoop(ctx)

	return nil
}

// Stop halts the polling loop
func (bp *BatchPoller) Stop() {
	bp.mu.Lock()
	if !bp.isRunning {
		bp.mu.Unlock()
		return
	}
	bp.isRunning = false
	bp.mu.Unlock()

	close(bp.stopChan)
	bp.wg.Wait()
}

// pollingLoop continuously checks batch job status
func (bp *BatchPoller) pollingLoop(ctx context.Context) {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bp.stopChan:
			return
		case <-ticker.C:
			bp.pollAllJobs(ctx)
		}
	}
}

// pollAllJobs checks status of all tracked jobs
func (bp *BatchPoller) pollAllJobs(ctx context.Context) {
	bp.mu.RLock()
	jobIDs := make([]string, 0, len(bp.jobs))
	for jobID := range bp.jobs {
		jobIDs = append(jobIDs, jobID)
	}
	bp.mu.RUnlock()

	for _, jobID := range jobIDs {
		bp.pollJob(ctx, jobID)
	}
}

// pollJob checks status of a single batch job
func (bp *BatchPoller) pollJob(ctx context.Context, jobID string) {
	bp.mu.Lock()
	tracker, exists := bp.jobs[jobID]
	if !exists {
		bp.mu.Unlock()
		return
	}
	bp.mu.Unlock()

	// Get current job status from Anthropic API
	job, err := bp.anthropicClient.GetBatchStatus(ctx, jobID)
	if err != nil {
		tracker.ErrorMessage = fmt.Sprintf("polling error: %v", err)
		return
	}

	tracker.LastPolledAt = time.Now()
	tracker.Status = job.ProcessingStatus
	tracker.RequestCount = job.RequestCounts.Total
	tracker.ProcessingCount = job.RequestCounts.Processing
	tracker.SuccessCount = job.RequestCounts.Succeeded
	tracker.FailureCount = job.RequestCounts.Errored

	// If job is done, retrieve results
	if job.ProcessingStatus == "succeeded" || job.ProcessingStatus == "failed" {
		bp.retrieveResults(ctx, jobID)
		bp.mu.Lock()
		bp.totalJobsComplete++
		if job.ProcessingStatus == "failed" {
			bp.totalJobsFailed++
		}
		bp.mu.Unlock()
	}

	bp.mu.Lock()
	bp.totalJobsPolled++
	bp.mu.Unlock()
}

// retrieveResults fetches and processes batch results
func (bp *BatchPoller) retrieveResults(ctx context.Context, jobID string) {
	results, err := bp.anthropicClient.GetBatchResults(ctx, jobID)
	if err != nil {
		bp.mu.Lock()
		tracker := bp.jobs[jobID]
		if tracker != nil {
			tracker.ErrorMessage = fmt.Sprintf("failed to retrieve results: %v", err)
		}
		bp.mu.Unlock()
		return
	}

	// Convert []BatchResult to []*BatchResult
	resultsPtr := make([]*client.BatchResult, len(results))
	for i := range results {
		resultsPtr[i] = &results[i]
	}

	bp.mu.Lock()
	tracker := bp.jobs[jobID]
	if tracker != nil {
		tracker.Results = resultsPtr
	}
	bp.mu.Unlock()
}

// TrackJob registers a new batch job for polling
func (bp *BatchPoller) TrackJob(jobID string, requestCount int) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if _, exists := bp.jobs[jobID]; exists {
		return fmt.Errorf("job %s already tracked", jobID)
	}

	bp.jobs[jobID] = &BatchJobTracker{
		JobID:        jobID,
		SubmittedAt:  time.Now(),
		Status:       "queued",
		RequestCount: requestCount,
	}

	return nil
}

// GetJobStatus returns current status of a tracked job
func (bp *BatchPoller) GetJobStatus(jobID string) (*BatchJobTracker, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	tracker, exists := bp.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	// Return a copy to avoid race conditions
	copy := *tracker
	return &copy, nil
}

// GetJobResults returns results from a completed job
func (bp *BatchPoller) GetJobResults(jobID string) ([]*client.BatchResult, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	tracker, exists := bp.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	if tracker.Status != "succeeded" && tracker.Status != "failed" {
		return nil, fmt.Errorf("job %s not completed (status: %s)", jobID, tracker.Status)
	}

	// Return copy of results
	resultsCopy := make([]*client.BatchResult, len(tracker.Results))
	copy(resultsCopy, tracker.Results)
	return resultsCopy, nil
}

// ListJobs returns all tracked jobs
func (bp *BatchPoller) ListJobs() []*BatchJobTracker {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	jobs := make([]*BatchJobTracker, 0, len(bp.jobs))
	for _, tracker := range bp.jobs {
		copy := *tracker
		jobs = append(jobs, &copy)
	}
	return jobs
}

// ListJobsByStatus returns jobs matching a status
func (bp *BatchPoller) ListJobsByStatus(status string) []*BatchJobTracker {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	jobs := make([]*BatchJobTracker, 0)
	for _, tracker := range bp.jobs {
		if tracker.Status == status {
			copy := *tracker
			jobs = append(jobs, &copy)
		}
	}
	return jobs
}

// CancelJob cancels a batch job
func (bp *BatchPoller) CancelJob(ctx context.Context, jobID string) error {
	bp.mu.RLock()
	_, exists := bp.jobs[jobID]
	bp.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not tracked", jobID)
	}

	// Cancel via Anthropic API
	job, err := bp.anthropicClient.CancelBatch(ctx, jobID)
	if err != nil {
		return err
	}

	bp.mu.Lock()
	tracker := bp.jobs[jobID]
	if tracker != nil {
		tracker.Status = job.ProcessingStatus
	}
	bp.mu.Unlock()

	return nil
}

// ForgetJob removes a completed job from tracking
func (bp *BatchPoller) ForgetJob(jobID string) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	tracker, exists := bp.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	// Only forget if completed
	if tracker.Status != "succeeded" && tracker.Status != "failed" && tracker.Status != "expired" {
		return fmt.Errorf("job %s still in progress (cannot forget)", jobID)
	}

	delete(bp.jobs, jobID)
	return nil
}

// PollerStats returns statistics about polling activity
func (bp *BatchPoller) PollerStats() PollerStatistics {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	activeJobs := len(bp.jobs)
	inProgressJobs := 0
	completedJobs := 0
	failedJobs := 0

	for _, tracker := range bp.jobs {
		switch tracker.Status {
		case "succeeded":
			completedJobs++
		case "failed", "expired":
			failedJobs++
		default:
			inProgressJobs++
		}
	}

	return PollerStatistics{
		IsRunning:         bp.isRunning,
		TotalJobsPolled:   bp.totalJobsPolled,
		TotalJobsComplete: bp.totalJobsComplete,
		TotalJobsFailed:   bp.totalJobsFailed,
		ActiveJobs:        int64(activeJobs),
		InProgressJobs:    int64(inProgressJobs),
		CompletedJobs:     int64(completedJobs),
		FailedJobs:        int64(failedJobs),
		PollingInterval:   bp.pollingInterval,
		TrackedJobs:       activeJobs,
	}
}

// PollerStatistics contains polling metrics
type PollerStatistics struct {
	IsRunning         bool
	TotalJobsPolled   int64
	TotalJobsComplete int64
	TotalJobsFailed   int64
	ActiveJobs        int64
	InProgressJobs    int64
	CompletedJobs     int64
	FailedJobs        int64
	PollingInterval   time.Duration
	TrackedJobs       int
}

// SetPollingInterval sets how often to poll jobs
func (bp *BatchPoller) SetPollingInterval(interval time.Duration) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if interval > 0 {
		bp.pollingInterval = interval
	}
}

// SetMaxRetries sets maximum retry attempts for failed polls
func (bp *BatchPoller) SetMaxRetries(retries int) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if retries > 0 {
		bp.maxRetries = retries
	}
}
