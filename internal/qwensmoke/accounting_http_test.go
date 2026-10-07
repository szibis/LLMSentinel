package qwensmoke

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtocolProofChecksAggregateNativeWindowAfterCorrection(t *testing.T) {
	completed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/status" {
			snapshot := accountingSnapshot(10, 100, 50, nil)
			if completed {
				snapshot = accountingSnapshot(12, 140, 58, document{"native_generation_metadata": true, "finish_reason": "stop", "prompt_tokens": float64(20), "generation_tokens": float64(4)})
			}
			json.NewEncoder(w).Encode(snapshot)
			return
		}
		completed = true
		d := messageFixture()
		d["stop_reason"] = "tool_use"
		d["content"] = []any{document{"type": "tool_use", "id": "call_test", "name": "record_marker", "input": document{"marker": marker}}}
		d["usage"] = document{"input_tokens": float64(40), "output_tokens": float64(8)}
		json.NewEncoder(w).Encode(d)
	}))
	defer server.Close()
	result := document{"checks": []any{}}
	if err := runProtocolProof(context.Background(), server.URL, "small", "haiku", "messages", false, true, nil, result, server.URL); err != nil {
		t.Fatal(err)
	}
	check := mapping(result["checks"].([]any)[0])
	native := mapping(check["native_accounting"])
	if native["attempts"] != float64(2) || native["prompt_tokens"] != float64(40) || check["passed"] != true {
		t.Fatalf("aggregate native evidence absent: %v", check)
	}
}
