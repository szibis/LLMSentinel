package labdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDashboardShowsLiveDataAndPreservesUnknownEndpoints(t *testing.T) {
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:19090" {
			t.Fatalf("unexpected outbound endpoint: %s", r.URL)
		}
		body, status := `{"mode":"serving","controls":{"capture_enabled":false}}`, 200
		if r.URL.Path == "/sentinel/activity" {
			body, status = `{"error":"unavailable"}`, 503
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	handler := newHandler(client, func() (json.RawMessage, error) {
		return json.RawMessage(`{"gateway":true,"runtimes":{"large":{"model_loaded":true,"stats":{"requests":12,"tokens_generated":1116}}}}`), nil
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/status", nil))
	var data struct {
		Telemetry map[string]any `json:"telemetry"`
		Gateway   map[string]any `json:"gateway"`
		Activity  any            `json:"activity"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &data) != nil || data.Gateway["mode"] != "serving" || data.Telemetry["gateway"] != true || data.Activity != nil {
		t.Fatalf("lost actual data or invented unavailable activity: %s", response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("live status must not be browser cached")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/dashboard", nil))
	for _, want := range []string{"Sentinel", "/api/status", "textContent", "Decode", "Unknown"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("missing UI %q", want)
		}
	}
}

func TestDashboardRejectsMutationsAndUnknownRoutes(t *testing.T) {
	handler := newHandler(&http.Client{}, func() (json.RawMessage, error) { t.Fatal("unexpected poll"); return nil, nil })
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"POST", "/api/status", 405}, {"GET", "/other", 404}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.path, response.Code)
		}
	}
}

func TestDashboardRejectsRemoteBind(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:8077", "example.com:8077", "127.0.0.1:0", "127.0.0.1:65536"} {
		if validListen(listen) {
			t.Errorf("accepted %s", listen)
		}
	}
	if !validListen("127.0.0.1:8077") {
		t.Fatal("rejected loopback")
	}
}

func TestFastRefreshCachesHeavyTelemetryButPollsActivity(t *testing.T) {
	var snapshots, polls atomic.Int32
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		polls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	handler := newHandler(client, func() (json.RawMessage, error) {
		snapshots.Add(1)
		return json.RawMessage(`{}`), nil
	})
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/status", nil))
		var data map[string]json.RawMessage
		if json.Unmarshal(response.Body.Bytes(), &data) != nil || string(data["history"]) != "[]" || string(data["attempts"]) != "[]" {
			t.Fatalf("history must preserve empty lists: %s", response.Body.String())
		}
	}
	if snapshots.Load() != 1 || polls.Load() != 6 {
		t.Fatalf("heavy telemetry polled too fast or activity cached: %d snapshots, %d endpoint polls", snapshots.Load(), polls.Load())
	}
}
