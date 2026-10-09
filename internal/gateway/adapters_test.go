package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/szibis/claude-escalate/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIExecutionAndCancellation(t *testing.T) {
	a := NewCLIAdapter()
	ctx := context.Background()
	if a.Name() != "cli" || a.Type() != ToolTypeCLI || !a.GetSignature().Parameters["command"].Required {
		t.Fatal("metadata")
	}
	for _, c := range []struct {
		p        map[string]interface{}
		ok       bool
		contains string
	}{
		{nil, false, "command parameter required"},
		{map[string]interface{}{"command": "rm anything"}, false, "whitelist"},
		{map[string]interface{}{"command": "echo sentinel"}, true, "sentinel"},
		{map[string]interface{}{"command": "cat /sentinel-nonexistent-file"}, false, ""},
		{map[string]interface{}{"command": "echo diagnostic >&2"}, false, "whitelist"},
	} {
		r, err := a.Execute(ctx, &ToolRequest{ID: "id", Params: c.p})
		if err != nil || r.Success != c.ok || r.ID != "id" {
			t.Fatalf("response: %+v %v", r, err)
		}
		if c.ok {
			if !strings.Contains(r.Data.(map[string]string)["output"], c.contains) {
				t.Fatal(r)
			}
		} else if !strings.Contains(r.Error, c.contains) {
			t.Fatal(r)
		}
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	r, err := a.Execute(cancelCtx, &ToolRequest{Params: map[string]interface{}{"command": "echo canceled"}})
	if err != nil || r.Success || r.Error == "" {
		t.Fatalf("cancel: %+v %v", r, err)
	}
	if err := a.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Health(cancelCtx); err == nil {
		t.Fatal("cancel health")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type badBody struct{}

func (badBody) Read([]byte) (int, error) { return 0, errors.New("broken body") }
func (badBody) Close() error             { return nil }

func TestRESTLocalHTTPAndFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Trace") != "trace" {
			t.Errorf("request: %s %+v", r.Method, r.Header)
		}
		w.Header().Set("X-Reply", "yes")
		if r.URL.Path == "/fail" {
			w.WriteHeader(503)
		}
		io.WriteString(w, "payload")
	}))
	defer server.Close()
	a := NewRESTAdapter()
	ctx := context.Background()
	if a.Name() != "rest" || a.Type() != ToolTypeREST || a.GetSignature().Name != "rest" {
		t.Fatal("metadata")
	}
	for _, path := range []string{"/ok", "/fail"} {
		r, err := a.Execute(ctx, &ToolRequest{ID: "r", Params: map[string]interface{}{"url": server.URL + path, "method": "post", "headers": map[string]interface{}{"X-Trace": "trace", "ignored": 1}}})
		if err != nil || r.Success != (path == "/ok") {
			t.Fatalf("%+v %v", r, err)
		}
		d := r.Data.(map[string]interface{})
		if d["body"] != "payload" || d["headers"].(http.Header).Get("X-Reply") != "yes" {
			t.Fatal(d)
		}
	}
	for _, p := range []map[string]interface{}{nil, {"url": ":bad"}, {"url": server.URL, "method": "bad method"}} {
		r, err := a.Execute(ctx, &ToolRequest{Params: p})
		if err != nil || r.Success || r.Error == "" {
			t.Fatalf("%+v %v", r, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	r, _ := a.Execute(canceled, &ToolRequest{Params: map[string]interface{}{"url": server.URL}})
	if r.Success || !strings.Contains(r.Error, "request failed") {
		t.Fatal(r)
	}
	a.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: badBody{}, Header: make(http.Header), Request: r}, nil
	})
	r, _ = a.Execute(ctx, &ToolRequest{Params: map[string]interface{}{"url": server.URL}})
	if r.Success || !strings.Contains(r.Error, "failed to read") {
		t.Fatal(r)
	}
	if err := a.Health(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMCPLifecycleAndProtocolContract(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		protocol   string
		p          map[string]interface{}
		key, value string
	}{
		{"scrapling", map[string]interface{}{"url": "https://example.invalid", "css_selector": "main"}, "url", "https://example.invalid"},
		{"lsp", map[string]interface{}{"file": "main.go", "action": "hover"}, "action", "hover"},
		{"database", map[string]interface{}{"query": "SELECT 1"}, "query", "SELECT 1"},
	} {
		a, err := NewMCPAdapter(c.protocol, map[string]interface{}{})
		if err != nil {
			t.Fatal(err)
		}
		if a.Name() != "mcp-"+c.protocol || a.Type() != ToolTypeMCP || a.GetSignature() == nil {
			t.Fatal("metadata")
		}
		r, _ := a.Execute(ctx, &ToolRequest{ID: "m"})
		if r.Success || !strings.Contains(r.Error, "not connected") {
			t.Fatal(r)
		}
		if err := a.Health(ctx); err != nil {
			t.Fatal(err)
		}
		r, _ = a.Execute(ctx, &ToolRequest{ID: "m", Params: c.p})
		if !r.Success || r.Data.(map[string]interface{})[c.key] != c.value {
			t.Fatal(r)
		}
		r, _ = a.Execute(ctx, &ToolRequest{Params: map[string]interface{}{}})
		if r.Success || r.Error == "" {
			t.Fatal(r)
		}
		if c.protocol == "lsp" {
			r, _ = a.Execute(ctx, &ToolRequest{Params: map[string]interface{}{"file": "a.go"}})
			if r.Data.(map[string]interface{})["action"] != "symbols" {
				t.Fatal(r)
			}
		}
		b, err := json.Marshal(a)
		if err != nil || !strings.Contains(string(b), `"connected":true`) {
			t.Fatalf("%s %v", b, err)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		r, _ = a.Execute(ctx, &ToolRequest{})
		if r.Success {
			t.Fatal("still connected")
		}
	}
	a, _ := NewMCPAdapter("unknown", nil)
	if err := a.Health(ctx); err == nil {
		t.Fatal("nil settings")
	}
	a.settings = map[string]interface{}{}
	a.Health(ctx)
	r, _ := a.Execute(ctx, &ToolRequest{})
	if r.Success || !strings.Contains(r.Error, "unknown protocol") {
		t.Fatal(r)
	}
}

type failingAdapter struct{ *CLIAdapter }

func (a failingAdapter) Close() error { return errors.New("close failure") }
func TestFactoryLifecycleAndErrors(t *testing.T) {
	f := NewAdapterFactory()
	a := NewCLIAdapter()
	if err := f.RegisterAdapter("custom", a); err != nil {
		t.Fatal(err)
	}
	if err := f.RegisterAdapter("custom", a); err == nil {
		t.Fatal("duplicate accepted")
	}
	if got, err := f.GetAdapter("custom"); err != nil || got != a {
		t.Fatal(got, err)
	}
	copied := f.GetAllAdapters()
	delete(copied, "custom")
	if len(f.GetAllAdapters()) != 1 {
		t.Fatal("map alias")
	}
	f.UnregisterAdapter("custom")
	if _, err := f.GetAdapter("custom"); err == nil {
		t.Fatal("still registered")
	}
	cfg := &config.Config{}
	if err := json.Unmarshal([]byte(`{"optimizations":{"mcp":{"enabled":true,"tools":[{"name":"scrape","type":"web_scraping","settings":{}},{"name":"analyze","type":"code_analysis","settings":{}},{"name":"db","type":"database","settings":{}},{"name":"ignored","type":"unknown"}]}}}`), cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.CreateFromConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if len(f.GetAllAdapters()) != 5 {
		t.Fatalf("adapters: %+v", f.GetAllAdapters())
	}
	for name, err := range f.HealthCheck() {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if len(f.GetAllAdapters()) != 0 {
		t.Fatal("not cleared")
	}
	f.RegisterAdapter("bad", failingAdapter{a})
	if err := f.Close(); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatal(err)
	}
	e := NewExecutionError("bad", "failed").WithDetails("timeout", true)
	if e.Error() != "ExecutionError [bad]: failed" || e.Details["timeout"] != true {
		t.Fatal(e)
	}
}
