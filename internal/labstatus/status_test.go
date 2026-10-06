package labstatus

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
)

func TestStatusDisplayCountsRuntimeAndMachineMemory(t *testing.T) {
	snapshot := snapshotFromJSON(t, `{"gateway":true,"runtimes":{"large":{"model_loaded":true,"stats":{"requests":25,"tokens_generated":5439},"memory":{"available_gb":6.8,"swap_used_gb":17.7,"pressure":"warning"}},"small":{"model_loaded":true,"stats":{"requests":2,"tokens_generated":24},"memory":{"available_gb":6.8,"swap_used_gb":17.7,"pressure":"warning"}}}}`)
	output := render(map[string]any{"model": map[string]any{"id": "sentinel-sonnet"}, "context_window": map[string]any{"used_percentage": float64(12)}}, snapshot)
	for _, want := range []string{"27 MLX req", "5463 tok", "6.8 GB available", "17.7 GB swap", "ctx 12%", "Sonnet", "pressure warning"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q: %s", want, output)
		}
	}
	if strings.Count(output, "GB available") != 1 {
		t.Fatal("machine memory double counted")
	}
	output = render(nil, snapshotFromJSON(t, `{"gateway":false,"runtimes":{"large":null,"small":null}}`))
	if !strings.Contains(output, "gateway offline") || !strings.Contains(output, "large unavailable") || strings.Contains(output, "0 tok") {
		t.Fatalf("missing stats invented: %s", output)
	}
	output = render(nil, snapshotFromJSON(t, `{"gateway":true,"runtimes":{"large":{"model_loaded":false,"memory":{"pressure":"critical","available_gb":1.2}}}}`))
	if !strings.Contains(output, "large loading") || !strings.Contains(output, "pressure critical") {
		t.Fatalf("loading memory missing: %s", output)
	}
}

func TestStatusRatesResetWithRuntimeAndRun(t *testing.T) {
	previous := snapshotFromJSON(t, `{"sample_time":100,"run_id":"r","runtimes":{"large":{"stats":{"uptime_s":50,"tokens_generated":100,"requests":4}}}}`)
	current := snapshotFromJSON(t, `{"sample_time":105,"run_id":"r","runtimes":{"large":{"stats":{"uptime_s":55,"tokens_generated":200,"requests":5}}}}`)
	addRates(&current, previous)
	if current.Rates["tokens_per_s"] != 20 || current.Rates["requests_per_min"] != 12 {
		t.Fatalf("wrong rates: %v", current.Rates)
	}
	current.Runtimes["large"].Stats["uptime_s"] = 1
	addRates(&current, previous)
	if len(current.Rates) != 0 {
		t.Fatal("restart counted as throughput")
	}
	current.Runtimes["large"].Stats["uptime_s"] = 55
	current.RunID = "another"
	addRates(&current, previous)
	if len(current.Rates) != 0 {
		t.Fatal("differentrun counted as throughput")
	}
}

func TestStatusInstallPreservesPreferencesAndRejectsSymlinks(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "claude"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "claude", "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","permissions":{"defaultMode":"default"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := install(root, "/private/tmp/sentinel-tools"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	var settings map[string]any
	json.Unmarshal(before, &settings)
	status := settings["statusLine"].(map[string]any)
	if settings["theme"] != "dark" || status["refreshInterval"] != float64(5) || !strings.Contains(status["command"].(string), "sentinel-tools statusline --root") {
		t.Fatalf("settingschanged: %s", before)
	}
	if err := install(root, "/private/tmp/sentinel-tools"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("existing status line overwritten")
	}

	legacy, _ := json.Marshal(map[string]any{"theme": "dark", "statusLine": map[string]any{"type": "command", "command": "python3 /private/scripts/sentinel_statusline.py --root " + clientcontrol.ShellQuote(root), "refreshInterval": 17}})
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := install(root, "/private/tmp/sentinel-tools"); err != nil {
		t.Fatal(err)
	}
	migrated, _ := os.ReadFile(path)
	if strings.Contains(string(migrated), "python3") || !strings.Contains(string(migrated), "sentinel-tools statusline") {
		t.Fatalf("owned Python command not migrated: %s", migrated)
	}
	if err := json.Unmarshal(migrated, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["statusLine"].(map[string]any)["refreshInterval"] != float64(17) {
		t.Fatalf("edited refresh interval overwritten: %s", migrated)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), path); err != nil {
		t.Fatal(err)
	}
	if err := install(root, "/private/tmp/sentinel-tools"); err == nil {
		t.Fatal("settings symlink accepted")
	}
}

func TestStatusInstallPreservesCustomLegacyVariants(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "claude"), 0700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	executable := filepath.Join(project, "bin", "sentinel-tools")
	script := filepath.Join(project, "scripts", "sentinel_statusline.py")
	base := "python3 " + clientcontrol.ShellQuote(script) + " --root " + clientcontrol.ShellQuote(root)
	for _, command := range []string{base + " --custom", base + " | myformatter", "echo " + base, "python3 /another/repo/scripts/sentinel_statusline.py --root " + root, "python3 " + script + " --root /another/lab"} {
		raw, _ := json.Marshal(map[string]any{"theme": "dark", "statusLine": map[string]any{"type": "command", "command": command, "refreshInterval": 17}})
		path := filepath.Join(root, "claude", "settings.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := install(root, executable); err != nil {
			t.Fatal(err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(raw, after) {
			t.Fatalf("custom status line overwritten: %s", command)
		}
	}
}

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestStatusPollIgnoresRemoteStateAndReadsBoundedLogTail(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"gateway":"https://remote.invalid","run_id":"r","models":[{"name":"large","port":"443/redirect"},{"name":"small","port":19092}]}`), 0600)
	os.WriteFile(filepath.Join(root, "gateway.log"), []byte("old Claude adapter HTTP 500:\n--- Lab start r\nattempting one format correction\nClaude adapter HTTP 422:\n"), 0600)
	os.WriteFile(filepath.Join(root, "runtime-small.log"), []byte("Inference complete tokens=12 tok_per_s=3.5\n"), 0600)
	client := statusClient()
	client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(r.URL.String(), "http://127.0.0.1:") || r.Header.Get("Authorization") != "" {
			t.Fatalf("remote/authenticated status request: %s", r.URL)
		}
		body := `{"status":"ok"}`
		if r.URL.Path == "/status" {
			body = `{"model_loaded":true,"model":"Qwen","stats":{"requests":2,"tokens_generated":12}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	snapshot := collect(root, client)
	if !snapshot.Gateway || len(snapshot.Runtimes) != 1 || snapshot.RecentCorrections != 1 || snapshot.RecentErrors != 1 || snapshot.LastSpeed["small"] != 3.5 {
		t.Fatalf("bad telemetry: %+v", snapshot)
	}
}

func snapshotFromJSON(t *testing.T, raw string) Snapshot {
	t.Helper()
	var value Snapshot
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStatusRunUsesPrivateCacheWithoutPolling(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SampleTime: float64(time.Now().UnixNano()) / 1e9, Gateway: true, Runtimes: map[string]*Runtime{"large": {ModelLoaded: true, Stats: map[string]float64{"requests": 7, "tokens_generated": 40}}}}
	cache := filepath.Join(root, "tmp", "statusline.json")
	if err := atomicJSON(cache, snapshot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cache)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cache is not private")
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"--root", root}, strings.NewReader(`{"model":{"id":"sentinel-opus"}}`), &out, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), "Opus") || !strings.Contains(out.String(), "7 MLX req") {
		t.Fatalf("cached Run failed %d %s %s", code, out.String(), stderr.String())
	}
	target := filepath.Join(root, "outside")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "telemetry.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(filepath.Join(link, "telemetry.json"), maxBytes); err == nil {
		t.Fatal("symlink parent accepted")
	}
}

func TestStatusFetchRejectsRedirectsAndOversizedBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{302, `{}`}, {200, strings.Repeat("x", maxBytes+1)}, {200, `[]`}} {
		client := statusClient()
		calls := 0
		client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://remote.invalid"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
		})
		if raw := fetch(client, "http://127.0.0.1:19090/health"); raw != nil {
			t.Fatalf("unsafe telemetry accepted %s", raw)
		}
		if calls != 1 {
			t.Fatal("redirect followed")
		}
	}
}

func TestStatusRejectsSymlinkAncestorBeforeAnyMutation(t *testing.T) {
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
	if err := atomicJSON(filepath.Join(link, "new", "sub", "status.json"), map[string]any{}); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if _, err := os.Lstat(filepath.Join(outside, "new")); !os.IsNotExist(err) {
		t.Fatalf("external subtree mutated: %v", err)
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"--root", link, "--json"}, strings.NewReader(""), &out, &stderr); code == 0 {
		t.Fatal("symlink root accepted")
	}
	if _, err := os.Lstat(filepath.Join(outside, "tmp")); !os.IsNotExist(err) {
		t.Fatalf("symlink-root cache created: %v", err)
	}
}
