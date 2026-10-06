package localgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Copied hook events never become inference requests or verified training labels.
func (g *Gateway) trainingIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, 405, "method_not_allowed", "Use POST")
		return
	}
	if g.training == nil {
		apiError(w, 409, "training_disabled", "Enable training mode with a private training directory")
		return
	}
	if r.Header.Get("Origin") != "" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		apiError(w, 400, "invalid_capture", "Use a local application/json hook request")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, g.cfg.MaxRequestBytes+1))
	if err != nil || int64(len(body)) > g.cfg.MaxRequestBytes {
		apiError(w, 413, "request_too_large", "Capture exceeds configured limit")
		return
	}
	var event map[string]any
	if json.Unmarshal(body, &event) != nil || event == nil {
		apiError(w, 400, "invalid_capture", "Expected a copied event object")
		return
	}
	client, _ := event["client"].(string)
	if client != "claude" && client != "codex" {
		apiError(w, 400, "invalid_client", "Copied event client must be claude or codex")
		return
	}
	source, _ := event["source"].(string)
	if source != "client_hook" {
		apiError(w, 400, "invalid_source", "Only client_hook events are accepted")
		return
	}
	id, _ := event["event_id"].(string)
	if id == "" {
		apiError(w, 400, "invalid_event", "event_id is required")
		return
	}
	model, _ := event["model"].(string)
	pair, _ := event["pair_id"].(string)
	output, _ := json.Marshal(event["outputs"])
	g.training.record(r.Context(), trainingEvent{Provider: "client-direct", Client: client, RequestID: id, PairID: pair, Model: model, Input: event["inputs"], Output: string(output), Reference: event, Quality: map[string]any{"status": "unscored", "source": "client_hook", "protocol_valid": false, "billing_class": "client_native_unknown"}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "received", "event_id": id, "training_eligible": false})
}
