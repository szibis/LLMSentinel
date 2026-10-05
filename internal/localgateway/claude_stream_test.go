package localgateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClaudeToolStreamCarriesStructuredInput(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"text\":\"Reading file\",\"tool_calls\":[{\"name\":\"Read\",\"input\":{\"file_path\":\"hello.txt\"}}]}"},"finish_reason":"stop"}],"usage":{"completion_tokens":20}}`)
	})
	res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Read file"}],"tools":[{"name":"Read","input_schema":{"type":"object","required":["file_path"],"properties":{"file_path":{"type":"string"}}}}]}`)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, expected := range []string{`"type":"tool_use"`, `"type":"input_json_delta"`, `hello.txt`, `"stop_reason":"tool_use"`, `event: message_stop`} {
		if !strings.Contains(string(body), expected) {
			t.Errorf("missing %s: %s", expected, body)
		}
	}
}

type eventWriter struct {
	header http.Header
	events chan string
}

func (w eventWriter) Header() http.Header         { return w.header }
func (w eventWriter) WriteHeader(int)             {}
func (w eventWriter) Write(b []byte) (int, error) { w.events <- string(b); return len(b), nil }
func (w eventWriter) Flush()                      {}
func TestClaudeHeartbeatArrivesWhileModelIsStillGenerating(t *testing.T) {
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}]}`)
	})
	writer := eventWriter{http.Header{}, make(chan string, 16)}
	done := make(chan struct{})
	request := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`)).WithContext(ctx)
	go func() { s.gateway.ServeHTTP(writer, request); close(done) }()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	gotStart := false
	for {
		select {
		case event := <-writer.events:
			gotStart = gotStart || strings.Contains(event, "message_start")
			if strings.Contains(event, "event: ping") {
				if !gotStart {
					t.Error("ping preceded message_start")
				}
				close(release)
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("stream did not finish")
				}
				return
			}
		case <-timer.C:
			t.Fatal("no heartbeat before model completed")
		}
	}
}
