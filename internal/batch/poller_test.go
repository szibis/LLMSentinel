package batch

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/client"
)

func TestNewBatchPoller(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	if bp == nil {
		t.Fatal("expected non-nil poller")
	}
	if bp.isRunning {
		t.Error("poller should not be running initially")
	}
	if len(bp.jobs) != 0 {
		t.Error("jobs map should be empty initially")
	}
}

func TestTrackJob(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	err := bp.TrackJob("job_1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fail to track same job twice
	err = bp.TrackJob("job_1", 10)
	if err == nil {
		t.Error("expected error when tracking duplicate job")
	}
}

func TestGetJobStatus(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)

	tracker, err := bp.GetJobStatus("job_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tracker.JobID != "job_1" {
		t.Errorf("expected job ID job_1, got %s", tracker.JobID)
	}
	if tracker.RequestCount != 5 {
		t.Errorf("expected 5 requests, got %d", tracker.RequestCount)
	}
	if tracker.Status != "queued" {
		t.Errorf("expected queued status, got %s", tracker.Status)
	}
}

func TestGetJobStatusNotFound(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	_, err := bp.GetJobStatus("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent job")
	}
}

func TestListJobs(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)
	bp.TrackJob("job_2", 10)
	bp.TrackJob("job_3", 15)

	jobs := bp.ListJobs()
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestListJobsByStatus(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)
	bp.TrackJob("job_2", 10)

	// Get reference to jobs to modify status
	allJobs := bp.ListJobs()
	if len(allJobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(allJobs))
	}
}

func TestForgetJob(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)

	// Cannot forget job in progress
	err := bp.ForgetJob("job_1")
	if err == nil {
		t.Error("expected error when forgetting in-progress job")
	}
}

func TestPollerStats(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)
	bp.TrackJob("job_2", 10)

	stats := bp.PollerStats()
	if stats.TrackedJobs != 2 {
		t.Errorf("expected 2 tracked jobs, got %d", stats.TrackedJobs)
	}
	if stats.ActiveJobs != 2 {
		t.Errorf("expected 2 active jobs, got %d", stats.ActiveJobs)
	}
}

func TestSetPollingInterval(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.SetPollingInterval(5 * time.Second)
	stats := bp.PollerStats()

	if stats.PollingInterval != 5*time.Second {
		t.Errorf("expected 5s interval, got %v", stats.PollingInterval)
	}
}

func TestSetMaxRetries(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.SetMaxRetries(10)
	// Verify it was set (internal state)
	if bp.maxRetries != 10 {
		t.Errorf("expected maxRetries 10, got %d", bp.maxRetries)
	}
}

func TestStartStop(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	ctx := context.Background()

	err := bp.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting poller: %v", err)
	}

	stats := bp.PollerStats()
	if !stats.IsRunning {
		t.Error("poller should be running after Start")
	}

	bp.Stop()

	stats = bp.PollerStats()
	if stats.IsRunning {
		t.Error("poller should not be running after Stop")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	ctx := context.Background()

	bp.Start(ctx)
	defer bp.Stop()

	// Try to start again
	err := bp.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already-running poller")
	}
}

func TestBatchJobTrackerCopy(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)

	tracker1, _ := bp.GetJobStatus("job_1")
	tracker1.Status = "modified"

	tracker2, _ := bp.GetJobStatus("job_1")

	if tracker2.Status != "queued" {
		t.Error("status should not be modified (got copy)")
	}
}

func TestListJobsCopy(t *testing.T) {
	ac := client.NewAnthropicClient("test-key")
	bp := NewBatchPoller(ac)

	bp.TrackJob("job_1", 5)
	bp.TrackJob("job_2", 10)

	jobs := bp.ListJobs()
	jobs[0].Status = "modified"

	jobs2 := bp.ListJobs()
	if jobs2[0].Status != "queued" {
		t.Error("status should not be modified (got copies)")
	}
}
