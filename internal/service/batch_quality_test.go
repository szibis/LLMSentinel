package service

import (
	"context"
	"encoding/json"
	"github.com/szibis/claude-escalate/internal/batch"
	"github.com/szibis/claude-escalate/internal/client"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBatchOfflineSubmissionCancellationAndResults(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	mode := "success"
	finished := make(chan struct{}, 1)
	http.DefaultTransport = serviceRoundTrip(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := `{"id":"job-test","created_at":"2026-01-01T00:00:00Z","processing_status":"succeeded","output_file_id":"file-test","request_counts":{"total":1,"succeeded":1}}`
		if mode == "error" {
			status = 401
			body = `{"error":"offline failure"}`
		}
		if r.Method == "POST" && r.URL.Path == "/v1/batches" && mode != "error" {
			var req client.BatchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.CustomID != "req-test" || req.Params.MaxTokens != 20 || req.Params.Model != "haiku" {
				t.Errorf("submitted params: %+v", req)
			}
		}
		if strings.Contains(r.URL.Path, "/files/") {
			body = `{"custom_id":"req-test","result":{"type":"succeeded","message":{"id":"message-test"}}}`
			select {
			case finished <- struct{}{}:
			default:
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	ac := client.NewAnthropicClient("offline-key")
	queue := batch.NewBatchQueue()
	poller := batch.NewBatchPoller(ac)
	h := NewBatchHandlers(ac, queue, poller)
	// Marshal the public request type so this test follows the real request JSON tags.
	encoded, err := json.Marshal(SubmitBatchRequest{Requests: []*batch.BatchRequest{{ID: "req-test", Model: "haiku", EstimatedOutput: 20, EstimatedCost: 1, EstimatedBatchSavings: .5}}})
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	w := call(t, h.HandleSubmitBatch, "POST", "/api/batch/submit", body)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	response := decodeResponse(t, w)
	if response["job_id"] != "job-test" || response["estimated_cost"] != float64(1) || response["estimated_savings"] != .5 {
		t.Fatal(response)
	}
	// Duplicate job IDs must fail tracking rather than silently replacing a job.
	w = call(t, h.HandleSubmitBatch, "POST", "/api/batch/submit", body)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "track job") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := call(t, h.HandleSubmitBatch, "POST", "/api/batch/submit", "{"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = call(t, h.HandleBatchResults, "GET", "/api/batch/results/job-test", "")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = call(t, h.HandleBatchResults, "GET", "/api/batch/results/", "")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	poller.SetPollingInterval(time.Millisecond)
	if err := poller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		poller.Stop()
		t.Fatal("offline batch was not polled")
	}
	poller.Stop()
	w = call(t, h.HandleBatchResults, "GET", "/api/batch/results/job-test", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	data := decodeResponse(t, w)
	results := data["results"].([]interface{})
	if len(results) != 1 || results[0].(map[string]interface{})["custom_id"] != "req-test" {
		t.Fatal(data)
	}
	w = call(t, h.HandleCancelBatch, "POST", "/api/batch/cancel/job-test", "")
	if w.Code != 200 || decodeResponse(t, w)["job_id"] != "job-test" {
		t.Fatal(w.Body)
	}
	w = call(t, h.HandleCancelBatch, "POST", "/api/batch/cancel/missing", "")
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	mode = "error"
	w = call(t, h.HandleSubmitBatch, "POST", "/api/batch/submit", body)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "submit batch") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
