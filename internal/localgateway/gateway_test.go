package localgateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func gatewayServer(t *testing.T, upstream string, maxBytes int64) *httptest.Server {
	t.Helper()
	g, err := New(Config{Upstream: upstream + "/v1", Timeout: 3 * time.Second, MaxRequestBytes: maxBytes})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	s := httptest.NewServer(g)
	t.Cleanup(s.Close)
	return s
}

func TestPayloadAndStreamArriveBeforeCompletion(t *testing.T) {
	payload := `{"model":"local","messages":[{"role":"user","content":"hello"}],"stream":true,"custom":{"retain":1}}`
	got := make(chan string, 1)
	release := make(chan struct{})
	defer close(release)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- r.URL.Path + "\n" + string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	s := gatewayServer(t, upstream.URL, 4096)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", s.URL+"/v1/chat/completions", strings.NewReader(payload))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatalf("early SSE: %q %v", line, err)
	}
	if received := <-got; received != "/v1/chat/completions\n"+payload {
		t.Fatalf("rewritten payload: %s", received)
	}
}

func TestDisconnectCancelsUpstream(t *testing.T) {
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	s := gatewayServer(t, upstream.URL, 4096)
	res, err := http.Post(s.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"local","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request survived disconnect")
	}
}

func TestCapabilitiesAndRequestBounds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request forwarded") }))
	defer upstream.Close()
	s := gatewayServer(t, upstream.URL, 512)
	for _, test := range []struct {
		path, body string
		status     int
	}{
		{"/v1/responses", `{}`, 501},
		{"/v1/messages", `{}`, 501},
		{"/v1/chat/completions", `{"tools":[{"type":"function"}]}`, 501},
		{"/v1/chat/completions", `{"functions":[{}]}`, 501},
		{"/v1/chat/completions", `{"messages":[{"role":"tool","content":"evidence"}]}`, 501},
		{"/v1/chat/completions", `{`, 400},
		{"/v1/chat/completions", `{"content":"` + strings.Repeat("x", 600) + `"}`, 413},
	} {
		res, err := http.Post(s.URL+test.path, "application/json", strings.NewReader(test.body))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != test.status || !strings.Contains(string(body), `"error"`) {
			t.Errorf("%s: %d %s", test.path, res.StatusCode, body)
		}
	}
}

func TestLocalPolicyRejectsRemoteAndAmbiguousURLs(t *testing.T) {
	for _, endpoint := range []string{"https://api.openai.com/v1", "http://192.168.1.2:8080/v1", "http://user:pass@127.0.0.1/v1", "http://127.0.0.1/v1?target=remote", "http://localhost/v2", "file:///tmp/model"} {
		g, err := New(Config{Upstream: endpoint, Timeout: time.Second, MaxRequestBytes: 512})
		if err == nil {
			g.Close()
			t.Errorf("accepted %s", endpoint)
		}
	}
}

func TestHealthDoesNotClaimInferenceAndStatusProbesRuntime(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("health path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"ok","model_loaded":false}`)
	}))
	defer upstream.Close()
	s := gatewayServer(t, upstream.URL, 4096)
	for path, expected := range map[string]string{"/health": `"scope":"gateway"`, "/sentinel/status": `"model_loaded":false`} {
		res, err := http.Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.Contains(string(body), expected) {
			t.Errorf("%s: %s", path, body)
		}
	}
}

func TestUpstreamFailurePreservedAndRedirectNotFollowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Location", "https://example.com")
			w.WriteHeader(302)
			return
		}
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":"model unavailable"}`)
	}))
	defer upstream.Close()
	s := gatewayServer(t, upstream.URL, 4096)
	res, err := http.Post(s.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 503 || string(body) != `{"error":"model unavailable"}` {
		t.Fatalf("lost upstream failure: %d %s", res.StatusCode, body)
	}
	res, err = http.Get(s.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 502 {
		t.Fatalf("redirect not refused: %d", res.StatusCode)
	}
}
