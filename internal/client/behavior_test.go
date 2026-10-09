package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type memoryHTTPServer struct {
	URL     string
	handler http.Handler
}

func newMemoryHTTPServer(h http.Handler) *memoryHTTPServer {
	return &memoryHTTPServer{URL: "http://in-memory.invalid", handler: h}
}
func (s *memoryHTTPServer) Close() {}
func (s *memoryHTTPServer) Client() *http.Client {
	return &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, r)
		if r.Body != nil {
			r.Body.Close()
		}
		return w.Result(), nil
	})}
}
func TestRetryReplaysPOSTBody(t *testing.T) {
	var bodies []string
	s := newMemoryHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"id":"ok"}`)
	}))
	c := NewAnthropicClient("fixture")
	c.baseURL = s.URL
	c.httpClient = s.Client()
	c.retryDelay = 0
	if _, err := c.CreateMessage(context.Background(), &MessageRequest{Model: "model", MaxTokens: 1}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] == "" || bodies[1] != bodies[0] {
		t.Fatalf("request bodies not replayed: %#v", bodies)
	}
}
func TestClientOperationErrorBoundaries(t *testing.T) {
	operations := []struct {
		name string
		call func(*AnthropicClient) error
	}{
		{"message", func(c *AnthropicClient) error {
			_, err := c.CreateMessage(context.Background(), &MessageRequest{})
			return err
		}},
		{"submit", func(c *AnthropicClient) error {
			_, err := c.SubmitBatch(context.Background(), []BatchRequest{{CustomID: "x"}})
			return err
		}},
		{"status", func(c *AnthropicClient) error { _, err := c.GetBatchStatus(context.Background(), "job"); return err }},
		{"cancel", func(c *AnthropicClient) error { _, err := c.CancelBatch(context.Background(), "job"); return err }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			for _, mode := range []string{"bad-url", "transport", "status", "decode"} {
				c := NewAnthropicClient("fixture")
				c.retryDelay = 0
				c.retryMax = 1
				if mode == "bad-url" {
					c.baseURL = ":"
				} else {
					c.httpClient = &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
						if mode == "transport" {
							return nil, errors.New("transport unavailable")
						}
						status := 200
						if mode == "status" {
							status = 400
						}
						return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("invalid json")), Request: r}, nil
					})}
				}
				if err := op.call(c); err == nil {
					t.Errorf("%s error accepted", mode)
				}
			}
		})
	}
}
func TestRetryHonorsCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewAnthropicClient("fixture")
	c.retryDelay = time.Second
	c.httpClient = &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	start := time.Now()
	_, err := c.CreateMessage(ctx, &MessageRequest{})
	if !errors.Is(err, context.Canceled) || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("cancellation err=%v duration=%v", err, time.Since(start))
	}
}
