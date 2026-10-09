package clientcontrol

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (brokenReader) Close() error             { return nil }

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestControlHTTPAndCLIWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sentinel/control" {
			t.Error(r.URL)
		}
		io.WriteString(w, `{"capture_enabled":true,"role_budgets":{"haiku":1024}}`)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	if code := Run([]string{"--endpoint", server.URL, "training", "on"}, nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), `"capture_enabled": true`) {
		t.Fatalf("%d %s %s", code, &out, &stderr)
	}
	if code := Run([]string{"--endpoint", server.URL, "status"}, nil, brokenWriter{}, &stderr); code != 1 {
		t.Fatal(code)
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	directory := filepath.Join(root, "previews")
	if code := Run([]string{"--write-commands", directory}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if code := Run([]string{"--write-commands", directory}, nil, &out, &stderr); code != 1 {
		t.Fatal("overwrite accepted")
	}
	for _, args := range [][]string{{"--unknown"}, {"--preview-commands", "--client", "bad"}, {"--preview-commands", "--endpoint", "https://remote.invalid"}, {"--write-commands", "relative"}} {
		if code := Run(args, nil, &out, &stderr); code != 1 {
			t.Fatal(code)
		}
	}
}

func TestControlResponseAndAssetFailures(t *testing.T) {
	for _, body := range []string{"null", "{", "{} {}"} {
		client := &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}
		if _, err := send(client, "http://127.0.0.1", "GET", nil); err == nil {
			t.Fatal("invalid body accepted", body)
		}
	}
	client := &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := send(client, "http://127.0.0.1", "GET", nil); err == nil {
		t.Fatal("transport error hidden")
	}
	client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: brokenReader{}, Request: r}, nil
	})
	if _, err := send(client, "http://127.0.0.1", "GET", nil); err == nil {
		t.Fatal("read error hidden")
	}
	if _, err := send(client, "http://127.0.0.1", "POST", make(chan int)); err == nil {
		t.Fatal("marshal error hidden")
	}
	for _, tc := range []struct{ client, binary string }{{"bad", "/binary"}, {"claude", "relative"}, {"claude", "/bad/../binary"}} {
		if _, err := Assets(tc.client, "http://127.0.0.1", tc.binary); err == nil {
			t.Fatal(tc)
		}
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	if _, err := writeCommands(root, map[string]string{"../escape": "x"}); err == nil {
		t.Fatal("asset traversal accepted")
	}
	blocker := filepath.Join(root, "file")
	os.WriteFile(blocker, nil, 0600)
	if err := ValidateDirectory(filepath.Join(blocker, "child")); err == nil {
		t.Fatal("file ancestor accepted")
	}
}
