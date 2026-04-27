package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/szibis/claude-escalate/internal/batch"
	"github.com/szibis/claude-escalate/internal/client"
)

// BatchHandlers manages HTTP endpoints for batch operations
type BatchHandlers struct {
	anthropicClient *client.AnthropicClient
	batchQueue      *batch.BatchQueue
	batchPoller     *batch.BatchPoller
}

// NewBatchHandlers creates a new batch handlers instance
func NewBatchHandlers(ac *client.AnthropicClient, queue *batch.BatchQueue, poller *batch.BatchPoller) *BatchHandlers {
	return &BatchHandlers{
		anthropicClient: ac,
		batchQueue:      queue,
		batchPoller:     poller,
	}
}

// SubmitBatchRequest represents a request to submit batch jobs
type SubmitBatchRequest struct {
	Requests []*batch.BatchRequest `json:"requests"`
}

// SubmitBatchResponse represents response from batch submission
type SubmitBatchResponse struct {
	JobID           string `json:"job_id"`
	RequestCount    int    `json:"request_count"`
	SubmittedAt     string `json:"submitted_at"`
	EstimatedCost   float64 `json:"estimated_cost"`
	EstimatedSavings float64 `json:"estimated_savings"`
	Message         string `json:"message"`
}

// HandleSubmitBatch submits queued requests as a batch job
func (bh *BatchHandlers) HandleSubmitBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse optional request body (for explicit request submission)
	var submitReq SubmitBatchRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&submitReq); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
	}

	// If no requests provided, flush queue
	var requests []*batch.BatchRequest
	if len(submitReq.Requests) > 0 {
		requests = submitReq.Requests
	} else {
		requests = bh.batchQueue.Flush()
	}

	if len(requests) == 0 {
		http.Error(w, "no requests to submit", http.StatusBadRequest)
		return
	}

	// Convert to API format and submit
	apiRequests := make([]client.BatchRequest, len(requests))
	totalEstimatedCost := 0.0
	totalEstimatedSavings := 0.0

	for i, req := range requests {
		apiRequests[i] = client.BatchRequest{
			CustomID: req.ID,
			Params: client.MessageRequest{
				Model:     req.Model,
				MaxTokens: req.EstimatedOutput,
			},
		}
		totalEstimatedCost += req.EstimatedCost
		totalEstimatedSavings += req.EstimatedBatchSavings
	}

	// Submit batch to Anthropic API
	ctx := r.Context()
	job, err := bh.anthropicClient.SubmitBatch(ctx, apiRequests)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to submit batch: %v", err), http.StatusInternalServerError)
		return
	}

	// Track job for polling
	if err := bh.batchPoller.TrackJob(job.ID, len(requests)); err != nil {
		http.Error(w, fmt.Sprintf("failed to track job: %v", err), http.StatusInternalServerError)
		return
	}

	response := SubmitBatchResponse{
		JobID:            job.ID,
		RequestCount:     len(requests),
		SubmittedAt:      job.CreatedAt.String(),
		EstimatedCost:    totalEstimatedCost,
		EstimatedSavings: totalEstimatedSavings,
		Message:          "batch submitted successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// BatchStatusResponse represents status of a batch job
type BatchStatusResponse struct {
	JobID           string `json:"job_id"`
	Status          string `json:"status"`
	RequestCount    int    `json:"request_count"`
	ProcessingCount int    `json:"processing_count"`
	SuccessCount    int    `json:"success_count"`
	FailureCount    int    `json:"failure_count"`
	SubmittedAt     string `json:"submitted_at"`
	LastPolledAt    string `json:"last_polled_at"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

// HandleBatchStatus returns status of a batch job
func (bh *BatchHandlers) HandleBatchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path: /api/batch/status/{job_id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/batch/status/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "job ID required", http.StatusBadRequest)
		return
	}

	jobID := parts[0]

	// Get job status from poller
	tracker, err := bh.batchPoller.GetJobStatus(jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("job not found: %v", err), http.StatusNotFound)
		return
	}

	response := BatchStatusResponse{
		JobID:           tracker.JobID,
		Status:          tracker.Status,
		RequestCount:    tracker.RequestCount,
		ProcessingCount: tracker.ProcessingCount,
		SuccessCount:    tracker.SuccessCount,
		FailureCount:    tracker.FailureCount,
		SubmittedAt:     tracker.SubmittedAt.String(),
		LastPolledAt:    tracker.LastPolledAt.String(),
		ErrorMessage:    tracker.ErrorMessage,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// BatchResultsResponse represents results from a completed batch
type BatchResultsResponse struct {
	JobID    string                 `json:"job_id"`
	Status   string                 `json:"status"`
	Results  []map[string]interface{} `json:"results"`
	Message  string                 `json:"message"`
}

// HandleBatchResults returns results from a completed batch job
func (bh *BatchHandlers) HandleBatchResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path: /api/batch/results/{job_id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/batch/results/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "job ID required", http.StatusBadRequest)
		return
	}

	jobID := parts[0]

	// Get job status first
	tracker, err := bh.batchPoller.GetJobStatus(jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("job not found: %v", err), http.StatusNotFound)
		return
	}

	// Get results
	results, err := bh.batchPoller.GetJobResults(jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot retrieve results: %v", err), http.StatusBadRequest)
		return
	}

	// Convert results to JSON-serializable format
	resultsJSON := make([]map[string]interface{}, len(results))
	for i, result := range results {
		resultsJSON[i] = map[string]interface{}{
			"custom_id": result.CustomID,
			"result":    result.Result,
			"error":     result.Error,
		}
	}

	response := BatchResultsResponse{
		JobID:   jobID,
		Status:  tracker.Status,
		Results: resultsJSON,
		Message: fmt.Sprintf("%d results retrieved", len(results)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleCancelBatch cancels a batch job
func (bh *BatchHandlers) HandleCancelBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path: /api/batch/cancel/{job_id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/batch/cancel/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "job ID required", http.StatusBadRequest)
		return
	}

	jobID := parts[0]

	// Cancel via poller
	ctx := r.Context()
	if err := bh.batchPoller.CancelJob(ctx, jobID); err != nil {
		http.Error(w, fmt.Sprintf("failed to cancel job: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"job_id":  jobID,
		"message": "batch cancellation requested",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleQueueStatus returns current queue statistics
func (bh *BatchHandlers) HandleQueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := bh.batchQueue.QueueStats()

	response := map[string]interface{}{
		"size":                stats.Size,
		"max_size":            stats.MaxSize,
		"min_size":            stats.MinSize,
		"total_processed":     stats.TotalProcessed,
		"total_estimated_saved": stats.EstimatedSaved,
		"idle_time":           stats.IdleTime.String(),
		"oldest_request_age":  stats.OldestRequestAge.String(),
		"average_request_age": stats.AverageAge.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandlePollerStats returns polling statistics
func (bh *BatchHandlers) HandlePollerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := bh.batchPoller.PollerStats()

	response := map[string]interface{}{
		"is_running":          stats.IsRunning,
		"total_jobs_polled":   stats.TotalJobsPolled,
		"total_jobs_complete": stats.TotalJobsComplete,
		"total_jobs_failed":   stats.TotalJobsFailed,
		"active_jobs":         stats.ActiveJobs,
		"in_progress_jobs":    stats.InProgressJobs,
		"completed_jobs":      stats.CompletedJobs,
		"failed_jobs":         stats.FailedJobs,
		"polling_interval":    stats.PollingInterval.String(),
		"tracked_jobs":        stats.TrackedJobs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterBatchRoutes registers all batch API routes
func RegisterBatchRoutes(mux *http.ServeMux, handlers *BatchHandlers) {
	mux.HandleFunc("/api/batch/submit", handlers.HandleSubmitBatch)
	mux.HandleFunc("/api/batch/status/", handlers.HandleBatchStatus)
	mux.HandleFunc("/api/batch/results/", handlers.HandleBatchResults)
	mux.HandleFunc("/api/batch/cancel/", handlers.HandleCancelBatch)
	mux.HandleFunc("/api/batch/queue", handlers.HandleQueueStatus)
	mux.HandleFunc("/api/batch/poller", handlers.HandlePollerStats)
}
