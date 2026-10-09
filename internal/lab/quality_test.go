package lab

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type labRoundTrip func(*http.Request) (*http.Response, error)

func (f labRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func protocolFixture(body string, status int) labRoundTrip {
	return func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	}
}

func legacyAsset(e *environment, current string) string {
	before, after, _ := strings.Cut(current, "```sh\n")
	command, _, _ := strings.Cut(after, "\n```\n")
	words := legacyWords(command)
	args := append([]string{"python3", filepath.Join(e.project, "scripts", "sentinel_control.py")}, words[2:]...)
	for i := range args {
		args[i] = quote(args[i])
	}
	return strings.Replace(before, "the control command itself", "the Python command itself", 1) + "```sh\n" + strings.Join(args, " ") + "\n```\n"
}

func TestPrepareMigratesOnlyOwnedLegacyDocuments(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(e.root, "claude", "settings.json")
	writeJSON(settingsPath, map[string]any{"custom": "preserve", "statusLine": map[string]any{"type": "command", "command": "rtk python3 " + quote(filepath.Join(e.project, "scripts", "sentinel_statusline.py")) + " --root " + quote(e.root)}})
	assets, err := clientcontrol.Assets("claude", endpoint, e.executable)
	if err != nil {
		t.Fatal(err)
	}
	current := assets["sentinel-status.md"]
	old := legacyAsset(e, current)
	if !e.ownedLegacyAsset(old, current) {
		t.Fatal("owned legacy fixture not recognized")
	}
	for _, path := range []string{filepath.Join(e.root, "claude", "commands", "sentinel-status.md"), filepath.Join(e.root, "control-plugin", "skills", "status", "SKILL.md")} {
		os.WriteFile(path, []byte(old), 0600)
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	readJSON(settingsPath, &settings)
	if settings["custom"] != "preserve" || strings.Contains(settings["statusLine"].(map[string]any)["command"].(string), "python") {
		t.Fatal(settings)
	}
	for _, path := range []string{filepath.Join(e.root, "claude", "commands", "sentinel-status.md"), filepath.Join(e.root, "control-plugin", "skills", "status", "SKILL.md")} {
		data, _ := os.ReadFile(path)
		if string(data) != current {
			t.Fatal("legacy asset not migrated", path)
		}
	}
	for _, tc := range []struct{ old, current string }{{"bad", "bad"}, {old, current + "suffix"}, {old + "suffix", current}, {old, "```sh\nshort\n```\n"}, {strings.Replace(old, "python3", "sh", 1), current}} {
		if e.ownedLegacyAsset(tc.old, tc.current) {
			t.Fatal("unowned legacy accepted", tc)
		}
	}
	if legacyWords("'unterminated") != nil || len(legacyPython([]string{"rtk", "python3", "script"})) != 1 {
		t.Fatal("legacy parser boundary")
	}
}

func TestModelFileIntegrityFailures(t *testing.T) {
	if err := validateModel("hub/model", true); err == nil {
		t.Fatal("Hub ID accepted")
	}
	for _, modify := range []func(string){
		func(path string) { os.Remove(filepath.Join(path, "tokenizer.json")) },
		func(path string) { os.Remove(filepath.Join(path, "model.safetensors")) },
		func(path string) { os.WriteFile(filepath.Join(path, "model.safetensors"), nil, 0600) },
		func(path string) {
			os.WriteFile(filepath.Join(path, "model.safetensors.index.json"), []byte(`{}`), 0600)
		},
		func(path string) {
			os.Rename(filepath.Join(path, "model.safetensors"), filepath.Join(path, "model-00001-of-00002.safetensors"))
		},
		func(path string) { os.WriteFile(filepath.Join(path, "config.json"), []byte("{"), 0600) },
		func(path string) { os.Remove(filepath.Join(path, "tokenizer_config.json")) },
		func(path string) { os.WriteFile(filepath.Join(path, "tokenizer_config.json"), []byte("{"), 0600) },
		func(path string) {
			os.WriteFile(filepath.Join(path, "tokenizer_config.json"), []byte(`{"chat_template":"plain"}`), 0600)
		},
		func(path string) {
			os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"model_type":"lfm2_moe"}`), 0600)
		},
	} {
		path := model(t, "model")
		modify(path)
		if err := validateModel(path, true); err == nil {
			t.Fatal("incomplete model accepted")
		}
	}
	path := model(t, "template")
	os.WriteFile(filepath.Join(path, "chat_template.jinja"), []byte("{{ enable_thinking }}"), 0600)
	if err := validateModel(path, true); err != nil {
		t.Fatal(err)
	}
	e := fixture(t)
	if _, err := e.runtimePlan(runtimeSettings{ModelPath: "relative"}); err == nil {
		t.Fatal("invalid model plan accepted")
	}
	if _, err := e.runtimePlan(runtimeSettings{ModelPath: path, Executable: filepath.Join(path, "missing")}); err == nil {
		t.Fatal("missing runtime accepted")
	}
}

func TestFileAndPreparationGuards(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(e.root, "target")
	os.WriteFile(target, []byte("unchanged"), 0600)
	if err := writeJSON(target, map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("invalid JSON written")
	}
	if err := atomicFile(filepath.Join(target, "child"), nil); err == nil {
		t.Fatal("file parent accepted")
	}
	if err := seedFile(filepath.Join(target, "child"), nil); err == nil {
		t.Fatal("file parent seeded")
	}
	link := filepath.Join(e.root, "run.lock")
	os.Symlink(target, link)
	if _, err := acquireLock(e.root); err == nil {
		t.Fatal("symlink lock accepted")
	}
	os.Remove(link)
	os.RemoveAll(e.root)
	os.WriteFile(e.root, nil, 0600)
	if _, err := acquireLock(e.root); err == nil {
		t.Fatal("file root accepted")
	}
	if _, err := e.spawn([]string{"/bin/true"}, "log", nil); err == nil {
		t.Fatal("file root process spawned")
	}
	e = fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(e.root, "gateway.log")
	os.Symlink(target, log)
	if _, err := e.spawn([]string{"/bin/true"}, "gateway.log", nil); err == nil {
		t.Fatal("symlink log accepted")
	}
	for _, leaf := range []string{"claude/settings.json", "codex/config.toml", "codex/sentinel-model-catalog.json", "claude/commands/sentinel-status.md", "control-plugin/skills/status/SKILL.md"} {
		t.Run(leaf, func(t *testing.T) {
			e := fixture(t)
			if err := e.prepare(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(e.root, leaf)
			os.Remove(path)
			os.Symlink(target, path)
			if err := e.prepare(); err == nil {
				t.Fatal("symlink prepared", leaf)
			}
		})
	}
	e = fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(e.root, "claude", "settings.json"), []byte("{"), 0600)
	if err := e.runClient([]string{"prepare"}); err == nil {
		t.Fatal("malformed settings accepted")
	}
	e = fixture(t)
	e.executable = "relative"
	if err := e.prepare(); err == nil {
		t.Fatal("relative helper accepted")
	}
	root := t.TempDir()
	t.Setenv("PATH", root)
	e = fixture(t)
	if err := e.prepare(); err == nil {
		t.Fatal("missing git hidden")
	}
}

func TestSelectedRoleNativeCapabilities(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	writeJSON(filepath.Join(e.root, "runtime.json"), runtimeSettings{SmallModelPath: "synthetic"})
	stubClients(t, e, "bad executable\n")
	t.Setenv("LAB_WORKSPACE", "")
	gateway := `{"capabilities":{"messages":true,"responses":true,"tools":true,"claude_roles":true}}`
	for _, mode := range []string{"runtime-down", "invalid-profile", "valid"} {
		e.transport = labRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Port() == "19090" {
				return protocolFixture(gateway, 200)(r)
			}
			switch mode {
			case "runtime-down":
				return protocolFixture(`{}`, 503)(r)
			case "invalid-profile":
				return protocolFixture(`{"capabilities":{"model_family":"qwen3_5","thinking_control":false}}`, 200)(r)
			}
			return protocolFixture(`{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think","chat_template_kwargs":["enable_thinking"]}}`, 200)(r)
		})
		old, _ := os.Getwd()
		err := e.launch("codex")
		os.Chdir(old)
		if err == nil {
			t.Fatal("invalid fixture executable replaced process")
		}
	}
	if err := validateNativeProfiles(map[string]any{"capabilities": map[string]any{"model_family": "qwen3_5", "thinking_control": false}}); err == nil {
		t.Fatal("invalid Qwen thinking accepted")
	}
}

func TestEphemeralPortPreflightAndReadTailReset(t *testing.T) {
	if err := preflightPorts([]int{0, 0}, time.Second); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	go func() { time.Sleep(50 * time.Millisecond); listener.Close() }()
	if err := preflightPorts([]int{port}, time.Second); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, []byte("short"), 0600)
	if data, err := readTail(path, 10, 100); err != nil || string(data) != "" {
		t.Fatal(string(data), err)
	}
	if _, err := readTail(filepath.Dir(path), 10, 0); err == nil {
		t.Fatal("directory tail accepted")
	}
}

func TestCIFileCorruptionAndControlLockRefusals(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(e.ciLeasePath(), ciLease{Token: "bad token", RunID: "owned-run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readCILease(); err == nil {
		t.Fatal("invalid ownership accepted")
	}
	if err := e.cancelCIResume(); err == nil {
		t.Fatal("corrupt lease canceled")
	}
	if err := e.checkCIPause(); err == nil {
		t.Fatal("corrupt lease ignored")
	}
	if err := e.supervise(); err == nil {
		t.Fatal("corrupt lease supervised")
	}
	if _, err := e.pauseCIWith(func() (map[string]any, error) { t.Fatal("activity called for corrupt lease"); return nil, nil }, func() error { return nil }); err == nil {
		t.Fatal("corrupt lease overwritten")
	}
	if err := e.resumeCIWith("", func() error { t.Fatal("empty token starts lab"); return nil }); err != nil {
		t.Fatal(err)
	}
	os.Remove(e.ciLeasePath())
	lock, err := acquireLock(e.root)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(e.root, "state.json"), []byte("{"), 0600)
	if _, err := e.pauseCIWith(func() (map[string]any, error) { return nil, nil }, func() error { return nil }); err == nil {
		t.Fatal("invalid running state accepted")
	}
	lock.Close()
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0600)
	e.root = blocker
	for _, fn := range []func() error{e.cancelCIResume, func() error { return e.resumeCIWith("token", func() error { return nil }) }, e.restart, e.start, func() error { return e.stop(false) }} {
		if err := fn(); err == nil {
			t.Fatal("file root lock accepted")
		}
	}
	if _, err := e.pauseCIWith(func() (map[string]any, error) { return nil, nil }, func() error { return nil }); err == nil {
		t.Fatal("file root reserved")
	}
	if _, err := ciEnvironment(e.project, blocker, io.Discard); err == nil {
		t.Fatal("CI file root accepted")
	}
	if _, err := ciEnvironment("relative", "relative", io.Discard); err == nil {
		t.Fatal("relative CI paths accepted")
	}
	release, err := PauseForCI("", "", io.Discard)
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := reserveGateway("bad method", "token"); err == nil {
		t.Fatal("invalid reservation method accepted")
	}
}

func TestHTTPRedirectMissingClientsAndDispatchFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1", http.StatusFound)
	}))
	defer server.Close()
	if _, err := fetchJSON(server.URL, time.Second); err == nil {
		t.Fatal("redirect followed")
	}
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	e.transport = protocolFixture(`{}`, 200)
	if err := e.doctor(); err == nil {
		t.Fatal("missing client versions hidden")
	}
	if err := e.runClient([]string{"claude"}); err == nil {
		t.Fatal("missing launch client accepted")
	}
	root := t.TempDir()
	t.Setenv("PATH", root)
	if err := e.runClient([]string{"clients-update"}); err == nil {
		t.Fatal("missing npm accepted")
	}
	if err := e.runRunner([]string{"run"}); err == nil {
		t.Fatal("missing gateway accepted")
	}
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, []byte("short"), 0600)
	if data, err := readTail(path, 10, -1); err != nil || string(data) != "short" {
		t.Fatal(string(data), err)
	}
}

func TestNullHealthAndSettingsAreRejected(t *testing.T) {
	e := fixture(t)
	e.transport = protocolFixture("null", 200)
	if _, err := e.fetchJSON(endpoint+"/health", time.Second); err == nil {
		t.Error("null health accepted")
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(e.root, "claude", "settings.json"), []byte("null"), 0600)
	if err := e.prepare(); err == nil {
		t.Error("null Claude settings accepted")
	}
}
func stubClients(t *testing.T, e *environment, body string) {
	t.Helper()
	directory := filepath.Join(e.root, "clients", "node_modules", ".bin")
	os.MkdirAll(directory, 0700)
	for _, client := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(directory, client), []byte(body), 0700); err != nil { // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
			t.Fatal(err)
		}
	}
}

func TestLabWrappersAndIsolatedClientVersions(t *testing.T) {
	e := fixture(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SENTINEL_PROJECT_ROOT", e.project)
	t.Setenv("SENTINEL_LAB_ROOT", e.root)
	var out, stderr bytes.Buffer
	if code := Run([]string{"prepare"}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, &stderr)
	}
	for _, args := range [][]string{nil, {"unknown"}, {"prepare", "extra"}} {
		if Run(args, nil, &out, &stderr) == 0 {
			t.Fatal(args)
		}
	}
	if RunRunner([]string{"logs"}, nil, &out, &stderr) != 0 || RunRunner([]string{"unknown"}, nil, &out, &stderr) != 1 {
		t.Fatal(&stderr)
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.clientBinary("codex"); err == nil {
		t.Fatal("missing isolated client accepted")
	}
	stubClients(t, e, "#!/bin/sh\nprintf 'fixture-client-version\\n'\n")
	e.transport = protocolFixture(`{"status":"ok","capabilities":{"messages":true,"responses":true,"tools":true}}`, 200)
	if err := e.doctor(); err != nil {
		t.Fatal(err)
	}
	if err := e.runClient([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"claude", "codex"} {
		if err := e.version(client); err != nil {
			t.Fatal(err)
		}
	}
	e.transport = protocolFixture(`{"error":"unavailable"}`, 503)
	if err := e.doctor(); err == nil {
		t.Fatal("missing gateway health hidden")
	}
	stubClients(t, e, "#!/bin/sh\nexit 3\n")
	if err := e.version("codex"); err == nil {
		t.Fatal("failed isolated client version hidden")
	}
}

func TestNPMUpdateUsesOnlyOfflineFixture(t *testing.T) {
	e := fixture(t)
	t.Setenv("HOME", t.TempDir())
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("PATH", root)
	if err := e.updateClients(); err == nil {
		t.Fatal("missing npm hidden")
	}
	script := "#!/bin/sh\ncase \"$*\" in *--registry=https://registry.npmjs.org*--fetch-retries=0*) exit 0 ;; *) exit 4 ;; esac\n"
	os.WriteFile(filepath.Join(root, "npm"), []byte(script), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	stubClients(t, e, "#!/bin/sh\nprintf 'offline-client\\n'\n")
	if err := e.updateClients(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(e.root, "npm-user.npmrc"), []byte("custom=true\n"), 0600)
	if err := e.updateClients(); err == nil {
		t.Fatal("custom npm config overwritten")
	}
	os.WriteFile(filepath.Join(e.root, "npm-user.npmrc"), nil, 0600)
	os.WriteFile(filepath.Join(root, "npm"), []byte("#!/bin/sh\nexit 2\n"), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	if err := e.updateClients(); err == nil {
		t.Fatal("failed npm hidden")
	}
}

func TestLabHTTPJSONAndSSEContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"status":"ok"}`) }))
	defer server.Close()
	if m, err := fetchJSON(server.URL, time.Second); err != nil || m["status"] != "ok" {
		t.Fatal(m, err)
	}
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body   string
		status int
		fail   bool
	}{
		{"data: \n\n: heartbeat\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"blue\\u001b\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", 200, false},
		{"data: {\n\n", 200, true}, {"data: {\"error\":{\"message\":\"failed\"}}\n\n", 200, true},
		{"data: {\"choices\":[{\"finish_reason\":\"length\"}]}\n\n", 200, true}, {"data: [DONE]\n\n", 200, true}, {"", 500, true}, {"data: " + strings.Repeat("x", 4*1024*1024+1), 200, true},
	} {
		e.out = new(bytes.Buffer)
		e.transport = protocolFixture(tc.body, tc.status)
		err := e.ask()
		if (err != nil) != tc.fail {
			t.Fatalf("SSE status %d fail %v err %v", tc.status, tc.fail, err)
		}
		if !tc.fail && strings.Contains(e.out.(*bytes.Buffer).String(), "\x1b") {
			t.Fatal("terminal escape forwarded")
		}
	}
	t.Setenv("PROMPT", "synthetic prompt")
	t.Setenv("MODEL_ROLE", "haiku")
	e.transport = protocolFixture("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n", 200)
	if err := e.runRunner([]string{"ask"}); err != nil {
		t.Fatal(err)
	}
	e.transport = labRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if err := e.ask(); err == nil {
		t.Fatal("transport failure hidden")
	}
	for _, raw := range []string{"{", strings.Repeat("x", 4*1024*1024+1)} {
		e.transport = protocolFixture(raw, 200)
		if _, err := e.fetchJSON(endpoint, time.Second); err == nil {
			t.Fatal("invalid json accepted")
		}
	}
	e.transport = protocolFixture(`{"status":"ok"}`, 200)
	if err := e.runRunner([]string{"status"}); err != nil {
		t.Fatal(err)
	}
	e.transport = protocolFixture(`{}`, 503)
	if err := e.status(); err == nil {
		t.Fatal("status failure hidden")
	}
}

func TestLaunchCapabilityAndWorkspaceGuards(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	stubClients(t, e, "not an executable format\n")
	t.Setenv("LAB_CLAUDE_FEATURES", "")
	for _, client := range []string{"claude", "codex"} {
		e.transport = protocolFixture(`{"capabilities":{}}`, 200)
		if err := e.launch(client); err == nil {
			t.Fatal("missing capabilities accepted")
		}
	}
	e.transport = protocolFixture(`{}`, 503)
	if err := e.launch("claude"); err == nil {
		t.Fatal("unavailable gateway accepted")
	}
	e.transport = protocolFixture(`{"capabilities":{"messages":true,"responses":true,"tools":true}}`, 200)
	t.Setenv("LAB_WORKSPACE", filepath.Join(e.root, "missing"))
	if err := e.launch("codex"); err == nil {
		t.Fatal("missing workspace accepted")
	}
	t.Setenv("LAB_WORKSPACE", filepath.Join(e.root, "workspace"))
	t.Setenv("LAB_CLAUDE_FEATURES", "bad")
	if err := e.launch("claude"); err == nil {
		t.Fatal("bad feature mode accepted")
	}
	t.Setenv("LAB_CLAUDE_FEATURES", "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := e.launch("claude"); err == nil {
		t.Fatal("invalid isolated executable replaced process")
	}
	if err := replaceProcess("missing", nil, filepath.Join(e.root, "missing"), nil, nil, nil, nil); err == nil {
		t.Fatal("missing directory accepted")
	}
	writeJSON(filepath.Join(e.root, "runtime.json"), runtimeSettings{SmallModelPath: "synthetic"})
	if err := e.launch("claude"); err == nil {
		t.Fatal("role capability missing")
	}
}

func TestSyntheticSupervisorOwnsOnlyFixtureProcesses(t *testing.T) {
	e := fixture(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ATTACH_RUNTIME", "1")
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	e.portCheck = func(ports []int, _ time.Duration) error {
		if len(ports) != 1 || ports[0] != 19090 {
			t.Errorf("attached supervisor ports %v", ports)
		}
		return nil
	}
	gateway := filepath.Join(e.project, "bin", "sentinel-gateway")
	os.MkdirAll(filepath.Dir(gateway), 0700)
	os.WriteFile(gateway, []byte("#!/bin/sh\nprintf 'synthetic gateway process only\\n'\nexit 0\n"), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	if err := e.supervise(); err == nil || !strings.Contains(err.Error(), "owned process exited") {
		t.Fatal(err)
	}
	var state runState
	if err := readJSON(filepath.Join(e.root, "state.json"), &state); err != nil || state.Phase != "stopped" || state.Runtime != "attached" {
		t.Fatal(state, err)
	}
	if data, err := os.ReadFile(filepath.Join(e.root, "gateway.log")); err != nil || !strings.Contains(string(data), "synthetic gateway") {
		t.Fatal(string(data), err)
	}
	e.portCheck = func([]int, time.Duration) error { return errors.New("fixture ports unavailable") }
	if err := e.supervise(); err == nil || !strings.Contains(err.Error(), "fixture ports") {
		t.Fatal(err)
	}
	os.Remove(gateway)
	if err := e.supervise(); err == nil || !strings.Contains(err.Error(), "missing gateway") {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(e.root, "runtime.json"), []byte("{"), 0600)
	if err := e.supervise(); err == nil {
		t.Fatal("invalid settings accepted")
	}
}

func TestRunCIOnStoppedTemporaryLab(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(e.project, "bin"), 0700)
	os.WriteFile(filepath.Join(e.project, "bin", "sentinel-tools"), []byte("#!/bin/sh\nexit 0\n"), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	var out, stderr bytes.Buffer
	args := []string{"pause", "--project", e.project, "--root", e.root}
	if code := RunCI(args, nil, &out, &stderr); code != 0 {
		t.Fatal(code, &stderr)
	}
	var paused struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(out.Bytes(), &paused); err != nil || paused.Token == "" {
		t.Fatal(&out, err)
	}
	if code := RunCI([]string{"resume", "--project", e.project, "--root", e.root, "--token", paused.Token}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, &stderr)
	}
	for _, args := range [][]string{nil, {"bad"}, {"pause", "--unknown"}, {"unknown", "--project", e.project, "--root", e.root}, {"resume", "--project", e.project, "--root", e.root, "--token", "bad"}} {
		if RunCI(args, nil, &out, &stderr) == 0 {
			t.Fatal(args)
		}
	}
	e.transport = protocolFixture(`{}`, 409)
	if err := e.reserveGateway("POST", "fixture-token"); !errors.Is(err, errCIGatewayRefused) {
		t.Fatal(err)
	}
	e.transport = protocolFixture(`{}`, 200)
	if err := e.reserveGateway("DELETE", "fixture-token"); err != nil {
		t.Fatal(err)
	}
	e.transport = labRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if err := e.reserveGateway("POST", "fixture-token"); err == nil {
		t.Fatal("reservation transport failure hidden")
	}
}

type synchronizedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedOutput) Write(raw []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(raw)
}
func (b *synchronizedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
func waitOutput(t *testing.T, out *synchronizedOutput, text string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing output %q: %s", text, out.String())
}

func TestLogFollowAppendTruncateAndSignal(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	out := &synchronizedOutput{}
	e.out = out
	path := filepath.Join(e.root, "gateway.log")
	os.WriteFile(path, []byte("initial log\n"), 0600)
	done := make(chan error, 1)
	go func() { done <- e.logs(true) }()
	waitOutput(t, out, "initial log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("appended log\n")
	file.Close()
	waitOutput(t, out, "appended log")
	os.WriteFile(path, []byte("short\n"), 0600)
	waitOutput(t, out, "[gateway.log] short")
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("log signal did not stop follow")
	}
}

func TestOwnedModelSupervisorStopsThroughPrivateMarker(t *testing.T) {
	e := fixture(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ATTACH_RUNTIME", "")
	t.Setenv("MODEL_PATH", model(t, "large"))
	t.Setenv("SMALL_MODEL_PATH", model(t, "small"))
	bin := filepath.Join(t.TempDir(), "runtime-fixture")
	script := "#!/bin/sh\nprintf 'fixture process, no server\\n'\nexec /bin/sleep 30\n"
	os.WriteFile(bin, []byte(script), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	t.Setenv("MLX_FLASH_BIN", bin)
	gateway := filepath.Join(e.project, "bin", "sentinel-gateway")
	os.MkdirAll(filepath.Dir(gateway), 0700)
	os.WriteFile(gateway, []byte(script), 0700) // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
	e.out = &synchronizedOutput{}
	e.stderr = e.out
	e.portCheck = func(ports []int, _ time.Duration) error {
		if len(ports) != 3 {
			t.Errorf("ports: %v", ports)
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- e.supervise() }()
	waitOutput(t, e.out.(*synchronizedOutput), "Lab processes started")
	var state runState
	if err := readJSON(filepath.Join(e.root, "state.json"), &state); err != nil {
		t.Fatal(err)
	}
	if state.Runtime != "owned" || state.Roles["haiku"] != "small" || len(state.Models) != 2 {
		t.Fatal(state)
	}
	if err := os.WriteFile(filepath.Join(e.root, "stop-"+state.RunID), nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("private marker did not stop fixture children")
	}
	if err := readJSON(filepath.Join(e.root, "state.json"), &state); err != nil || state.Phase != "stopped" {
		t.Fatal(state, err)
	}
}

func TestCIDirectReservationAndRestorationFailures(t *testing.T) {
	for _, status := range []int{200, 409} {
		e := fixture(t)
		if err := e.prepare(); err != nil {
			t.Fatal(err)
		}
		lock, err := acquireLock(e.root)
		if err != nil {
			t.Fatal(err)
		}
		state := runState{RunID: "owned-run", Phase: "running", Runtime: "owned", Supervisor: "go", Gateway: endpoint}
		writeJSON(filepath.Join(e.root, "state.json"), state)
		e.transport = labRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("X-Sentinel-CI-Token") == "" {
				t.Error("missing reservation token")
			}
			if status == 200 {
				lock.Close()
			}
			return protocolFixture(`{}`, status)(r)
		})
		token, err := e.pauseCI()
		lock.Close()
		if status == 409 {
			if err == nil {
				t.Fatal("refused reservation accepted")
			}
			continue
		}
		if err != nil || token == "" {
			t.Fatal(token, err)
		}
		if err := e.resumeCIWith(token, func() error { return errors.New("fixture restart failed") }); err == nil || !strings.Contains(err.Error(), "fixture restart") {
			t.Fatal(err)
		}
	}
	e := fixture(t)
	e.transport = labRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	lock, err := acquireLock(e.root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	writeJSON(filepath.Join(e.root, "state.json"), runState{RunID: "owned-run", Phase: "running", Runtime: "owned", Supervisor: "go", Gateway: endpoint})
	if _, err := e.pauseCI(); !errors.Is(err, errCIReservationUncertain) {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.ciLeasePath()); err != nil {
		t.Fatal("uncertain reservation lease discarded", err)
	}
}

func TestEnvironmentDefaultsAndSafeRunnerDispatch(t *testing.T) {
	e := fixture(t)
	t.Setenv("SENTINEL_PROJECT_ROOT", e.project)
	t.Setenv("SENTINEL_LAB_ROOT", "")
	env, err := newEnvironment(nil, io.Discard, io.Discard)
	if err != nil || env.root != filepath.Join(e.project, ".sentinel-lab") {
		t.Fatal(env, err)
	}
	t.Setenv("SENTINEL_PROJECT_ROOT", "")
	if env, err := newEnvironment(nil, io.Discard, io.Discard); err != nil || !filepath.IsAbs(env.project) {
		t.Fatal(env, err)
	}
	for _, args := range [][]string{nil, {"logs", "--bad"}, {"unknown"}} {
		if err := e.runRunner(args); err == nil {
			t.Fatal(args)
		}
	}
	if err := e.runRunner([]string{"stop"}); err != nil {
		t.Fatal(err)
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	e.executable = filepath.Join(e.root, "missing-helper")
	if err := e.runRunner([]string{"start"}); err == nil {
		t.Fatal("missing start helper accepted")
	}
	if err := e.runRunner([]string{"restart"}); err == nil {
		t.Fatal("missing restart helper accepted")
	}
}
