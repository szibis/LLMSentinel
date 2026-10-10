package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestModDispatchProducesNativeSnapshot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sentinel/control" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		io.WriteString(w, `{"policy":"local-only"}`)
	}))
	defer server.Close()
	var out, diagnostic bytes.Buffer
	code := run([]string{"mod", "--root", root, "--endpoint", server.URL, "status"}, nil, &out, &diagnostic)
	var snapshot map[string]any
	if code != 0 || json.Unmarshal(out.Bytes(), &snapshot) != nil || snapshot["scope"] != "claude-mod-status" || snapshot["training_eligible"] != false {
		t.Fatalf("mod dispatch: %d %s %s", code, &out, &diagnostic)
	}
	control, ok := snapshot["control"].(map[string]any)
	if !ok || control["policy"] != "local-only" {
		t.Fatal("native controller output lost")
	}
	for _, args := range [][]string{{"--help"}, {"help"}, nil} {
		out.Reset()
		if run(args, nil, &out, &diagnostic) != 0 || !strings.Contains(out.String(), "|mod|") {
			t.Fatal("mod absent from help", out.String())
		}
	}
}
