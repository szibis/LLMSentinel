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

func TestStatusRetainsCountersWithNativeGenerationMetadata(t *testing.T) {
	snapshot := snapshotFromJSON(t, `{"gateway":true,"runtimes":{"large":{"model_loaded":true,"stats":{"requests":4,"tokens_generated":486,"uptime_s":85,"last_generation":{"prompt_tokens":54,"generation_tokens":6,"native_generation_metadata":true,"usage_source":"exact_mlx_lm_generation"},"optional":null,"label":"native"},"memory":{"available_gb":11.6,"swap_used_gb":27,"pressure":"normal"}}}}`)
	runtime := snapshot.Runtimes["large"]
	if runtime == nil || runtime.Stats["requests"] != 4 || runtime.Stats["tokens_generated"] != 486 || runtime.Stats["uptime_s"] != 85 {
		t.Fatalf("native metadata discarded runtime counters: %+v", runtime)
	}
	if _, exists := runtime.Stats["optional"]; exists {
		t.Fatal("null counter became a known zero")
	}
	output := render(nil, snapshot)
	for _, want := range []string{"large ready", "4 MLX req", "486 tok", "11.6 GB available", "27 GB swap"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q: %s", want, output)
		}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored := snapshotFromJSON(t, string(raw))
	if restored.Runtimes["large"].Stats["tokens_generated"] != 486 {
		t.Fatal("status cache round trip lost counters")
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

func TestNativeMetadataPreservedAndLabeled(t *testing.T) {
	s := snapshotFromJSON(t, `{"sample_time":100,"runtimes":{"large":{"model_loaded":true,"stats":{"requests":1,"tokens_generated":10,"last_generation":{"generation_tps":20,"time_s":2,"timestamp":95,"native_generation_metadata":true}}}}}`)
	if s.Runtimes["large"].LastGeneration["generation_tps"] != float64(20) {
		t.Fatal("native metadata lost")
	}
	raw, _ := json.Marshal(s)
	restored := snapshotFromJSON(t, string(raw))
	if restored.Runtimes["large"].LastGeneration["time_s"] != float64(2) {
		t.Fatal("cache lost metadata")
	}
	text := render(nil, restored)
	for _, want := range []string{"20.0 decode tok/s", "2.0s generation"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
}
func TestRetainFailuresExpireAndReset(t *testing.T) {
	old := snapshotFromJSON(t, `{"sample_time":100,"run_id":"r","gateway":true,"runtimes":{"large":{"model_loaded":true,"stats":{"uptime_s":50}}}}`)
	now := snapshotFromJSON(t, `{"sample_time":105,"run_id":"r","runtimes":{"large":null}}`)
	retainRecent(&now, old)
	if now.Runtimes["large"] == nil || !now.Runtimes["large"].Stale || now.Gateway || !now.GatewayStale {
		t.Fatalf("not honest stale: %+v", now)
	}
	expired := snapshotFromJSON(t, `{"sample_time":116,"run_id":"r","runtimes":{"large":null}}`)
	retainRecent(&expired, now)
	if expired.Runtimes["large"] != nil || expired.GatewayStale {
		t.Fatal("stale retained past lifetime")
	}
	now = snapshotFromJSON(t, `{"sample_time":105,"run_id":"new","runtimes":{"large":null}}`)
	retainRecent(&now, old)
	if now.Runtimes["large"] != nil {
		t.Fatal("retained across run restart")
	}
	now = snapshotFromJSON(t, `{"sample_time":105,"run_id":"r","runtimes":{"large":{"stats":{"uptime_s":1}}}}`)
	retainRecent(&now, old)
	if now.Runtimes["large"].Stale || now.Runtimes["large"].Stats["uptime_s"] != 1 {
		t.Fatal("restart replaced with old state")
	}
}
func TestActualActivityAndColors(t *testing.T) {
	s := snapshotFromJSON(t, `{"gateway":true,"activity":{"active":[{"role":"haiku","upstream":"http://127.0.0.1:19092/v1"}]},"runtimes":{"small":{"endpoint":"http://127.0.0.1:19092/status","model_loaded":true,"memory":{"pressure":"critical"}}}}`)
	text := render(map[string]any{"model": map[string]any{"id": "sentinel-opus"}}, s)
	if !strings.Contains(text, "selected Opus") || !strings.Contains(text, "active haiku→small") {
		t.Fatalf("wrong activity %s", text)
	}
	t.Setenv("NO_COLOR", "")
	if !strings.Contains(colorize(text), "\x1b[") {
		t.Fatal("colors absent")
	}
	t.Setenv("NO_COLOR", "1")
	if strings.Contains(colorize(text), "\x1b[") {
		t.Fatal("NO_COLOR ignored")
	}
}

func TestStaleActivityAndEndpointsRemainHonest(t *testing.T) {
	old := snapshotFromJSON(t, `{"sample_time":100,"run_id":"r","activity":{"active":[{"role":"opus","upstream":"http://127.0.0.1:19091/v1"}]},"runtimes":{"large":{"endpoint":"http://127.0.0.1:19091/status","model_loaded":true,"stats":{"uptime_s":50,"requests":2,"tokens_generated":10}}}}`)
	current := snapshotFromJSON(t, `{"sample_time":105,"run_id":"r","runtime_endpoints":{"large":"http://127.0.0.1:19091/status"},"runtimes":{"large":null}}`)
	retainRecent(&current, old)
	addRates(&current, old)
	text := render(nil, current)
	if !strings.Contains(text, "last observed active opus→large (stale 5s)") || !strings.Contains(text, "large stale 5s") || len(current.Rates) != 0 {
		t.Fatalf("dishonest stale: %s %v", text, current.Rates)
	}
	current = snapshotFromJSON(t, `{"sample_time":105,"run_id":"r","runtime_endpoints":{"large":"http://127.0.0.1:20000/status"},"runtimes":{"large":null}}`)
	retainRecent(&current, old)
	if current.Runtimes["large"] != nil {
		t.Fatal("retained old endpoint")
	}
	current = snapshotFromJSON(t, `{"sample_time":116,"run_id":"r","runtimes":{"large":null}}`)
	retainRecent(&current, old)
	if current.Activity != nil {
		t.Fatal("activity retained beyond deadline")
	}
	t.Setenv("NO_COLOR", "")
	if !strings.Contains(colorize("pressure warning"), "\x1b[33mpressure warning") || !strings.Contains(colorize("pressure critical"), "\x1b[31mpressure critical") {
		t.Fatal("memory warning colors absent")
	}
}

func TestRuntimePreservesCacheAndOptimizationTelemetry(t *testing.T) {
	for _, raw := range []string{
		`{"prompt_cache":{"enabled":true,"hits":2},"optimizations":{"prompt_cache":true,"speculative":"disabled"}}`,
		`{"stats":{"prompt_cache":{"enabled":true,"hits":2},"optimizations":{"prompt_cache":true,"speculative":"disabled"}}}`,
	} {
		var r Runtime
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(r)
		var fields map[string]any
		json.Unmarshal(encoded, &fields)
		cache, ok := fields["prompt_cache"].(map[string]any)
		if !ok || cache["enabled"] != true || cache["hits"] != float64(2) {
			t.Fatalf("cache lost: %s", encoded)
		}
		opts, ok := fields["optimizations"].(map[string]any)
		if !ok || opts["speculative"] != "disabled" {
			t.Fatalf("optimizations lost: %s", encoded)
		}
	}
}

func TestRuntimeCacheTopLevelWinsAndOlderRuntimeStaysUnknown(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want any
	}{
		{`{"prompt_cache":{"hits":9},"stats":{"prompt_cache":{"hits":1}}}`, float64(9)},
		{`{"stats":{"requests":1}}`, nil},
	} {
		var r Runtime
		if err := json.Unmarshal([]byte(tc.raw), &r); err != nil {
			t.Fatal(err)
		}
		if r.PromptCache["hits"] != tc.want {
			t.Fatalf("cache precedence/unknown violated: %+v", r.PromptCache)
		}
	}
}
