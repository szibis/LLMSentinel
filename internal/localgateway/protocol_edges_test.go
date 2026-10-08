package localgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the real gateway and runtime HTTP boundaries. Only model generation
// is replaced: tests never download weights or run client tools.
func edgeHTTPGateway(t *testing.T, reply string) (*httptest.Server, *atomic.Int64) {
	return edgeHTTPGatewayReplies(t, func() string { return reply })
}

func edgeHTTPGatewayReplies(t testing.TB, reply func() string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	calls := new(atomic.Int64)
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply())
	}))
	t.Cleanup(runtime.Close)
	gateway, err := New(Config{Upstream: runtime.URL + "/v1", Timeout: 3 * time.Second, MaxRequestBytes: 65536, ClaudeAdapter: true, ClaudeBufferedValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gateway.Close)
	server := httptest.NewServer(gateway)
	t.Cleanup(server.Close)
	return server, calls
}

func TestNullBackendUsageRemainsUnknown(t *testing.T) {
	for _, usage := range []string{
		`{"prompt_tokens":20,"completion_tokens":null}`,
		`{"prompt_tokens":null,"completion_tokens":12}`,
		`{"prompt_tokens":null,"completion_tokens":null}`,
		`{"prompt_tokens":20}`,
		`{"completion_tokens":12}`,
		`{}`,
	} {
		for _, exact := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%t", usage, exact), func(t *testing.T) {
				s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}],"usage":%s,"mlx_flash_compress":{"native_generation_metadata":%t}}`, usage, exact)
				})
				req := claudeRequest{Model: "local", MaxTokens: 128, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Reply done."`)}}}
				messages, err := prepareClaude(req)
				if err != nil {
					t.Fatal(err)
				}
				response, err := s.gateway.inferClaude(context.Background(), req, messages)
				if err != nil {
					t.Fatal(err)
				}
				if response.UsageKnown {
					t.Fatal("null/missing usage became a known zero")
				}
			})
		}
	}
}

func edgeToolRequest(protocol string, stream bool) string {
	switch protocol {
	case "messages":
		return fmt.Sprintf(`{"model":"local","max_tokens":128,"stream":%t,"messages":[{"role":"user","content":"Record the marker."}],"tools":[{"name":"record_marker","input_schema":{"type":"object","properties":{"marker":{"type":"string"}},"required":["marker"],"additionalProperties":false}}],"tool_choice":{"type":"tool","name":"record_marker"}}`, stream)
	case "responses":
		return fmt.Sprintf(`{"model":"local","max_output_tokens":128,"stream":%t,"input":"Record the marker.","tools":[{"type":"function","name":"record_marker","parameters":{"type":"object","properties":{"marker":{"type":"string"}},"required":["marker"],"additionalProperties":false}}],"tool_choice":{"type":"function","name":"record_marker"}}`, stream)
	default:
		return fmt.Sprintf(`{"model":"local","max_tokens":128,"stream":%t,"messages":[{"role":"user","content":"Record the marker."}],"tools":[{"type":"function","function":{"name":"record_marker","parameters":{"type":"object","properties":{"marker":{"type":"string"}},"required":["marker"],"additionalProperties":false}}}],"tool_choice":{"type":"function","function":{"name":"record_marker"}}}`, stream)
	}
}

type edgeResponse struct {
	StatusCode int
	Header     http.Header
}

func edgePost(t *testing.T, server *httptest.Server, protocol, request string) (edgeResponse, string) {
	t.Helper()
	response, err := server.Client().Post(server.URL+"/v1/"+protocol, "application/json", strings.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return edgeResponse{StatusCode: response.StatusCode, Header: response.Header.Clone()}, string(body)
}

func TestProtocolEdgesRejectAmbiguousRuntimeMetadataBeforeToolEmission(t *testing.T) {
	content := `"{\"tool_calls\":[{\"name\":\"record_marker\",\"input\":{\"marker\":\"READY\"}}]}"`
	choice := `{"message":{"content":` + content + `},"finish_reason":"stop"}`
	valid := `{"choices":[` + choice + `],"usage":{"prompt_tokens":20,"completion_tokens":12}}`
	cases := map[string]string{
		"case-aliased-stop": `{"choices":[{"message":{"content":` + content + `},"finish_reason":"length","FINISH_REASON":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":12}}`,
		"duplicate-stop":    `{"choices":[{"message":{"content":` + content + `},"finish_reason":"length","finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":12}}`,
		"duplicate-usage":   `{"choices":[` + choice + `],"usage":{"prompt_tokens":20,"completion_tokens":2048,"completion_tokens":12}}`,
		"duplicate-choices": `{"choices":[],"choices":[` + choice + `],"usage":{"prompt_tokens":20,"completion_tokens":12}}`,
		"invalid-utf8":      strings.Replace(valid, "READY", "R"+string([]byte{0xff})+"ADY", 1),
		"negative-input":    strings.Replace(valid, `"prompt_tokens":20`, `"prompt_tokens":-1`, 1),
		"negative-output":   strings.Replace(valid, `"completion_tokens":12`, `"completion_tokens":-1`, 1),
		"trailing-document": valid + `{}`,
	}
	for _, protocol := range []string{"messages", "responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for name, reply := range cases {
				t.Run(fmt.Sprintf("%s/%t/%s", protocol, stream, name), func(t *testing.T) {
					server, calls := edgeHTTPGateway(t, reply)
					response, body := edgePost(t, server, protocol, edgeToolRequest(protocol, stream))
					if response.StatusCode != http.StatusBadGateway || strings.Contains(response.Header.Get("Content-Type"), "event-stream") || strings.Contains(body, "toolu_") || strings.Contains(body, "READY") {
						t.Fatalf("ambiguous backend emitted a client turn: status=%d body=%q", response.StatusCode, body)
					}
					if calls.Load() != 1 {
						t.Fatalf("malformed backend envelope was retried: %d", calls.Load())
					}
					if !json.Valid([]byte(body)) {
						t.Fatalf("error is not JSON: %q", body)
					}
				})
			}
		}
	}
}

func TestProtocolEdgesRejectDuplicateRequestKeysBeforeInference(t *testing.T) {
	for _, protocol := range []string{"messages", "responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, duplicate := range []string{`"model":"sentinel-opus",`, `"stream":false,`, `"tools":[],`, `"Model":"sentinel-opus",`, `"TOOLS":[],`, `"\u006dodel":"sentinel-opus",`} {
				t.Run(fmt.Sprintf("%s/%t/%s", protocol, stream, duplicate), func(t *testing.T) {
					server, calls := edgeHTTPGateway(t, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`)
					request := "{" + duplicate + strings.TrimPrefix(edgeToolRequest(protocol, stream), "{")
					response, body := edgePost(t, server, protocol, request)
					if response.StatusCode != http.StatusBadRequest || calls.Load() != 0 || strings.Contains(response.Header.Get("Content-Type"), "event-stream") || !json.Valid([]byte(body)) {
						t.Fatalf("ambiguous client request reached inference: status=%d calls=%d body=%q", response.StatusCode, calls.Load(), body)
					}
				})
			}
		}
	}
}

func TestProtocolEdgesRejectNestedFieldAliasesBeforeInference(t *testing.T) {
	cases := []struct{ protocol, old, replacement string }{
		{"responses", `"input":"Record the marker."`, `"input":[{"type":"message","role":"user","ROLE":"system","content":"Record the marker."},{"type":"message","role":"user","content":"go"}]`},
		{"responses", `"input":"Record the marker."`, `"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Record","TEXT":"Override"}]}]`},
		{"responses", `"name":"record_marker"}}`, `"name":"record_marker","NAME":"record_marker"}}`},
		{"chat/completions", `"function":{"name":"record_marker"}}`, `"function":{"name":"record_marker","NAME":"record_marker"}}`},
		{"chat/completions", `"messages":`, `"response_format":{"type":"json_object","TYPE":"text"},"messages":`},
	}
	for _, c := range cases {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t/%s", c.protocol, stream, c.replacement), func(t *testing.T) {
				server, calls := edgeHTTPGateway(t, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`)
				request := strings.Replace(edgeToolRequest(c.protocol, stream), c.old, c.replacement, 1)
				if request == edgeToolRequest(c.protocol, stream) {
					t.Fatal("fixture replacement did not match")
				}
				response, body := edgePost(t, server, c.protocol, request)
				if response.StatusCode != 400 || calls.Load() != 0 || !json.Valid([]byte(body)) {
					t.Fatalf("nested alias reached inference: status=%d calls=%d body=%q", response.StatusCode, calls.Load(), body)
				}
			})
		}
	}
}

func TestProtocolEdgesRejectSerializedDuplicateArguments(t *testing.T) {
	for _, protocol := range []string{"responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", protocol, stream), func(t *testing.T) {
				var request map[string]any
				if err := json.Unmarshal([]byte(edgeToolRequest(protocol, stream)), &request); err != nil {
					t.Fatal(err)
				}
				arguments := `{"marker":"first","marker":"overwritten"}`
				if protocol == "responses" {
					request["input"] = []any{
						map[string]any{"type": "function_call", "name": "record_marker", "call_id": "call_1", "arguments": arguments},
						map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "recorded"},
						map[string]any{"type": "message", "role": "user", "content": "Record another marker."},
					}
				} else {
					request["messages"] = []any{
						map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "record_marker", "arguments": arguments}}}},
						map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "recorded"},
						map[string]any{"role": "user", "content": "Record another marker."},
					}
				}
				raw, err := json.Marshal(request)
				if err != nil {
					t.Fatal(err)
				}
				server, calls := edgeHTTPGateway(t, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`)
				response, body := edgePost(t, server, protocol, string(raw))
				if response.StatusCode != 400 || calls.Load() != 0 || !json.Valid([]byte(body)) {
					t.Fatalf("serialized duplicate reached inference: status=%d calls=%d body=%q", response.StatusCode, calls.Load(), body)
				}
			})
		}
	}
}
