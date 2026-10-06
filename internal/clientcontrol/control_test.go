package clientcontrol

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestControlFixedRequestsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		method, body string
	}{{[]string{"status"}, "GET", "null"}, {[]string{"training", "off"}, "POST", `{"capture_enabled":false}`}, {[]string{"policy", "local-only"}, "POST", `{"policy":"local-only"}`}, {[]string{"profile", "sonnet", "2048"}, "POST", `{"role_budgets":{"sonnet":2048}}`}} {
		method, payload, err := requestSpec(tc.args)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(payload)
		if method != tc.method || string(body) != tc.body {
			t.Fatalf("wrong control request %s %s", method, body)
		}
	}
	for _, args := range [][]string{{"profile", "sonnet", "0"}, {"profile", "sonnet", "32769"}, {"profile", "secret", "1"}, {"policy", "paid"}, {"training", "yes"}} {
		if _, _, err := requestSpec(args); err == nil {
			t.Fatalf("unsafe control accepted: %v", args)
		}
	}
}

func TestControlTransportRejectsRemoteRedirectAndOversizedResponse(t *testing.T) {
	for _, endpoint := range []string{"https://127.0.0.1:19090", "http://localhost:19090", "http://user:secret@127.0.0.1:19090", "http://example.com", "http://127.0.0.1/path", "http://127.0.0.1:0", "http://[::1]:65536"} {
		if _, err := endpointURL(endpoint); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	for _, status := range []int{200, 302, 409} {
		client := controlClient()
		calls := 0
		client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != "http://127.0.0.1:19094/sentinel/control" || r.Method != "POST" || r.Header.Get("Authorization") != "" {
				t.Errorf("wrong local request %v", r)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"capture_enabled":true}` {
				t.Errorf("wrong payload: %s", body)
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://remote.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"mode":"learning"}`)), Request: r}, nil
		})
		value, err := send(client, "http://127.0.0.1:19094", "POST", map[string]any{"capture_enabled": true})
		if calls != 1 {
			t.Fatal("redirect followed")
		}
		if status == 200 {
			if err != nil || value["mode"] != "learning" {
				t.Fatalf("good response lost: %v %v", value, err)
			}
		} else if err == nil {
			t.Fatal("error reported as success")
		}
	}
	client := controlClient()
	client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBytes+1))), Request: r}, nil
	})
	if _, err := send(client, "http://127.0.0.1:19090", "GET", nil); err == nil {
		t.Fatal("oversized response accepted")
	}
}

func TestControlAssetsUseGoExecutableAndNeverOverwrite(t *testing.T) {
	assets, err := Assets("claude", "http://127.0.0.1:19094", "/private/tmp/sentinel tools")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 9 || !strings.Contains(assets["sentinel-status.md"], "disable-model-invocation: true") || !strings.Contains(assets["sentinel-status.md"], "'/private/tmp/sentinel tools' control") || strings.Contains(assets["sentinel-status.md"], "python") || strings.Contains(assets["sentinel-status.md"], "$ARGUMENTS") {
		t.Fatalf("bad assets: %v", assets)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(parent, "preview")
	if _, err := writeCommands(directory, assets); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directory, "sentinel-status.md"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("commands not private")
	}
	if _, err := writeCommands(directory, assets); err == nil {
		t.Fatal("existing commands overwritten")
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeCommands(link, assets); err == nil {
		t.Fatal("symlink directory accepted")
	}
}

func TestControlRunPreviewIsOfflineAndInvalidActionsFail(t *testing.T) {
	var out, stderr bytes.Buffer
	if status := Run([]string{"--client", "codex", "--preview-commands"}, strings.NewReader(""), &out, &stderr); status != 0 || stderr.Len() != 0 {
		t.Fatalf("preview failed: %d %s", status, stderr.String())
	}
	var assets map[string]string
	if err := json.Unmarshal(out.Bytes(), &assets); err != nil || len(assets) != 9 {
		t.Fatalf("wrong preview: %s %v", out.String(), err)
	}
	for _, value := range assets {
		if strings.Contains(value, "disable-model-invocation") {
			t.Fatal("Claude metadata in Codex asset")
		}
	}
	out.Reset()
	stderr.Reset()
	if status := Run([]string{"profile", "sonnet", "-1"}, strings.NewReader(""), &out, &stderr); status == 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "failed") {
		t.Fatalf("invalid control reported success: %d %s", status, stderr.String())
	}
}

func TestControlRejectsSymlinkAncestorBeforeCreatingDirectories(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeCommands(filepath.Join(link, "new", "sub"), map[string]string{"sentinel-status.md": "test"}); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if _, err := os.Lstat(filepath.Join(outside, "new")); !os.IsNotExist(err) {
		t.Fatalf("external subtree mutated: %v", err)
	}
}
