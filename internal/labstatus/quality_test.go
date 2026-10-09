package labstatus

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type statusFailWriter struct{}

func (statusFailWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }
func statusRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestStatuslineCachedCLIAndInstall(t *testing.T) {
	root := statusRoot(t)
	os.MkdirAll(filepath.Join(root, "claude"), 0700)
	os.WriteFile(filepath.Join(root, "claude", "settings.json"), []byte(`{"model":"fixture"}`), 0600)
	var out, stderr bytes.Buffer
	if code := Run([]string{"--root", root, "--install"}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var settings map[string]any
	raw, _ := os.ReadFile(filepath.Join(root, "claude", "settings.json"))
	if err := json.Unmarshal(raw, &settings); err != nil || settings["statusLine"] == nil {
		t.Fatal(string(raw), err)
	}
	snapshot := Snapshot{SampleTime: float64(time.Now().UnixNano()) / 1e9, Gateway: true, Runtimes: map[string]*Runtime{"large": {ModelLoaded: true, Model: "fixture-model", Stats: RuntimeStats{"requests": 2, "tokens_generated": 30}}}}
	os.MkdirAll(filepath.Join(root, "tmp"), 0700)
	if err := atomicJSON(filepath.Join(root, "tmp", "statusline.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"--root", root, "--json"}, nil, &out, &stderr); code != 0 || !strings.Contains(out.String(), "tokens_generated") {
		t.Fatal(code, &out, &stderr)
	}
	out.Reset()
	if code := Run([]string{"--root", root}, strings.NewReader(`{"model":{"id":"claude-opus-4-7"}}`), &out, &stderr); code != 0 || !strings.Contains(out.String(), "claude-opus-4-7") {
		t.Fatal(code, &out, &stderr)
	}
	if code := Run([]string{"--root", root, "--json"}, nil, statusFailWriter{}, &stderr); code != 1 {
		t.Fatal(code)
	}
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--root", filepath.Join(root, "claude", "settings.json")}, {"--root", statusRoot(t), "--install"}} {
		if code := Run(args, nil, &out, &stderr); code != 1 {
			t.Fatal(code)
		}
	}
}

func TestStatusFileBoundsAndRuntimeJSON(t *testing.T) {
	root := statusRoot(t)
	file := filepath.Join(root, "file")
	os.WriteFile(file, []byte("12345"), 0600)
	if _, err := readFile(file, 4); err == nil {
		t.Fatal("file bound ignored")
	}
	if _, err := readFile(root, 4); err == nil {
		t.Fatal("directory read accepted")
	}
	if err := atomicJSON(file, map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("bad json accepted")
	}
	if err := atomicJSON(filepath.Join(file, "child"), map[string]any{}); err == nil {
		t.Fatal("file parent accepted")
	}
	if err := atomicJSON(root, map[string]any{}); err == nil {
		t.Fatal("directory overwritten")
	}
	if err := install(root, "relative"); err == nil {
		t.Fatal("relative executable accepted")
	}
	os.MkdirAll(filepath.Join(root, "claude"), 0700)
	for _, body := range []string{"null", "{"} {
		os.WriteFile(filepath.Join(root, "claude", "settings.json"), []byte(body), 0600)
		if err := install(root, "/synthetic/sentinel-tools"); err == nil {
			t.Fatal(body)
		}
	}
	var runtime Runtime
	for _, raw := range []string{"null", `{"stats":"wrong"}`, "{"} {
		if raw == "null" {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &runtime); err == nil {
			t.Fatal("invalid runtime accepted", raw)
		}
	}
	if modelName("models--org--Qwen/snapshots/x") != "Qwen" || modelName(".") != "" || len(clean(strings.Repeat("x", 100))) != 80 {
		t.Fatal("model sanitization")
	}
	os.WriteFile(file, []byte("older\n--- Lab start marker\nnew generation\n"), 0600)
	if got := tail(file); strings.Contains(got, "older") || !strings.Contains(got, "new generation") {
		t.Fatal(got)
	}
}

func TestActivityRouteFallbackAndLegacyWordBoundaries(t *testing.T) {
	s := Snapshot{Activity: map[string]any{"active": []any{map[string]any{"upstream": "http://127.0.0.1:1111/v1"}}}, Runtimes: map[string]*Runtime{"null": nil}, RuntimeEndpoints: map[string]string{"unrelated": "http://127.0.0.1:2222/status"}}
	if label := activityLabel(s); !strings.Contains(label, "unknown role→127.0.0.1:1111") {
		t.Fatal(label)
	}
	s.Activity = map[string]any{"active": []any{5}}
	if activityLabel(s) != "idle" {
		t.Fatal(activityLabel(s))
	}
	s.Activity = map[string]any{"last_completed": map[string]any{"role": "haiku", "upstream": ":bad"}}
	if !strings.Contains(activityLabel(s), "unknown runtime") {
		t.Fatal(activityLabel(s))
	}
	project := "/synthetic/project"
	root := "/synthetic/root"
	for _, command := range []string{"  rtk python3.12 '/synthetic/project/scripts/sentinel_statusline.py' --root '/synthetic/root'  ", "python3 /synthetic/project/scripts/sentinel_statusline.py --root /synthetic/root"} {
		if !ownedLegacyCommand(command, project, root) {
			t.Fatal(command)
		}
	}
	for _, command := range []string{"python3 'unterminated", "sh /synthetic/project/scripts/sentinel_statusline.py --root /synthetic/root", "python3 /synthetic/project/scripts/sentinel_statusline.py --root /elsewhere", "python3 $(escape)"} {
		if ownedLegacyCommand(command, project, root) {
			t.Fatal(command)
		}
	}
	output := render(map[string]any{"model": map[string]any{"display_name": "Synthetic"}}, Snapshot{SampleTime: 10, Runtimes: map[string]*Runtime{"small": {ModelLoaded: false, Stale: true, SampleTime: 1, Memory: map[string]any{"available_gb": 2}}}})
	if !strings.Contains(output, "was loading") || !strings.Contains(output, "pressure unknown") {
		t.Fatal(output)
	}
}
