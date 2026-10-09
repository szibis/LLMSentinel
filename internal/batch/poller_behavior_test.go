package batch

import (
	"context"
	"errors"
	"github.com/szibis/claude-escalate/internal/client"
	"sync"
	"testing"
	"time"
)

type fakeBatchAPI struct {
	status                           string
	statusErr, resultsErr, cancelErr error
	results                          []client.BatchResult
}

func (f *fakeBatchAPI) GetBatchStatus(context.Context, string) (*client.BatchJob, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	j := &client.BatchJob{ProcessingStatus: f.status}
	j.RequestCounts.Total = 2
	j.RequestCounts.Succeeded = 1
	j.RequestCounts.Errored = 1
	return j, nil
}
func (f *fakeBatchAPI) GetBatchResults(context.Context, string) ([]client.BatchResult, error) {
	return f.results, f.resultsErr
}
func (f *fakeBatchAPI) CancelBatch(context.Context, string) (*client.BatchJob, error) {
	return &client.BatchJob{ProcessingStatus: "expired"}, f.cancelErr
}
func TestPollerOfflineJobLifecycleAndErrors(t *testing.T) {
	api := &fakeBatchAPI{status: "succeeded", results: []client.BatchResult{{CustomID: "one"}}}
	p := NewBatchPoller(api)
	ctx := context.Background()
	if _, err := p.GetJobResults("missing"); err == nil {
		t.Fatal("missing results accepted")
	}
	if err := p.CancelJob(ctx, "missing"); err == nil {
		t.Fatal("missing cancel accepted")
	}
	if err := p.ForgetJob("missing"); err == nil {
		t.Fatal("missing forget accepted")
	}
	if err := p.TrackJob("job", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetJobResults("job"); err == nil {
		t.Fatal("unfinished results accepted")
	}
	if err := p.ForgetJob("job"); err == nil {
		t.Fatal("unfinished forget accepted")
	}
	p.pollJob(ctx, "missing")
	p.pollAllJobs(ctx)
	results, err := p.GetJobResults("job")
	if err != nil || len(results) != 1 || results[0].CustomID != "one" {
		t.Fatalf("results=%+v %v", results, err)
	}
	if len(p.ListJobsByStatus("succeeded")) != 1 || len(p.ListJobsByStatus("queued")) != 0 {
		t.Fatal("status filter incorrect")
	}
	stats := p.PollerStats()
	if stats.TotalJobsComplete != 1 || stats.TotalJobsPolled != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if err := p.ForgetJob("job"); err != nil {
		t.Fatal(err)
	}
	if err := p.TrackJob("failed", 2); err != nil {
		t.Fatal(err)
	}
	api.status = "failed"
	api.resultsErr = errors.New("results unavailable")
	p.pollAllJobs(ctx)
	tracker, err := p.GetJobStatus("failed")
	if err != nil || tracker.ErrorMessage == "" || tracker.Status != "failed" {
		t.Fatalf("failed tracker=%+v %v", tracker, err)
	}
	if p.PollerStats().TotalJobsFailed != 1 {
		t.Fatal("failure not counted")
	}
	api.statusErr = errors.New("status unavailable")
	p.pollAllJobs(ctx)
	tracker, _ = p.GetJobStatus("failed")
	if tracker.ErrorMessage == "" {
		t.Fatal("status error lost")
	}
	api.cancelErr = errors.New("cancel unavailable")
	if err := p.CancelJob(ctx, "failed"); err == nil {
		t.Fatal("cancel error lost")
	}
	api.cancelErr = nil
	if err := p.CancelJob(ctx, "failed"); err != nil {
		t.Fatal(err)
	}
	if len(p.ListJobsByStatus("expired")) != 1 {
		t.Fatal("cancel status not updated")
	}
	p.Stop()
}

func TestPollerConcurrentStatusSnapshots(t *testing.T) {
	api := &fakeBatchAPI{status: "in_progress"}
	p := NewBatchPoller(api)
	if err := p.TrackJob("job", 2); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.pollJob(context.Background(), "job")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.GetJobStatus("job")
			p.ListJobs()
			p.PollerStats()
		}
	}()
	wg.Wait()
}

func TestPollerRestartAndCancellation(t *testing.T) {
	bp := NewBatchPoller(nil)
	for i := 0; i < 2; i++ {
		if err := bp.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		bp.Stop()
		if bp.PollerStats().IsRunning {
			t.Fatal("still running")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := bp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for bp.PollerStats().IsRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if bp.PollerStats().IsRunning {
		t.Fatal("cancellation retained running state")
	}
	bp.Stop()
}
