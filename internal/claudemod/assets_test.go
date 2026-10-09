package claudemod

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Broad embedding or incorrect config serialization would ship developer files
// or alter the explicit executable/root arguments passed by the native adapter.
func TestAssetsShipOnlyRuntimeAdapterAndLiteralConfiguration(t *testing.T) {
	root := canonicalRoot(t)
	assets, err := Assets(root, "http://127.0.0.1:19090", "/tmp/a sentinel's-tools")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.js", "sentinel-config.json"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("deployable files: %v", names)
	}
	var config map[string]any
	if err := json.Unmarshal(assets["sentinel-config.json"], &config); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config, map[string]any{"root": root, "endpoint": "http://127.0.0.1:19090", "executable": "/tmp/a sentinel's-tools"}) {
		t.Fatalf("configuration changed literal arguments: %v", config)
	}
	var hooks struct {
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(assets["hooks/hooks.json"], &hooks); err != nil || !reflect.DeepEqual(hooks.Modules, []string{"./register.js"}) {
		t.Fatalf("native module not discoverable: %v %v", hooks, err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(assets[".claude-plugin/plugin.json"], &manifest); err != nil || manifest["name"] != "sentinel" {
		t.Fatal("invalid runtime manifest", err)
	}
	if len(assets["hooks/register.js"]) == 0 {
		t.Fatal("missing runtime adapter")
	}
	assets["hooks/register.js"][0] = 'X'
	other, err := Assets(root, "http://127.0.0.1:19090", "/tmp/sentinel-tools")
	if err != nil || other["hooks/register.js"][0] == 'X' {
		t.Fatal("caller can mutate later generated assets", err)
	}
}

func TestAssetsRejectUnsafeOrHostIncompatibleConfiguration(t *testing.T) {
	root := canonicalRoot(t)
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ root, endpoint, executable string }{
		{"/", "http://127.0.0.1:19090", "/tmp/sentinel-tools"},
		{"relative", "http://127.0.0.1:19090", "/tmp/sentinel-tools"},
		{root + "/../other", "http://127.0.0.1:19090", "/tmp/sentinel-tools"},
		{alias, "http://127.0.0.1:19090", "/tmp/sentinel-tools"},
		{root, "https://127.0.0.1:19090", "/tmp/sentinel-tools"},
		{root, "http://localhost:19090", "/tmp/sentinel-tools"},
		{root, "http://127.0.0.1:19090/", "/tmp/sentinel-tools"},
		{root, "http://127.0.0.1:080", "/tmp/sentinel-tools"},
		{root, "http://127.0.0.1:19090?query", "/tmp/sentinel-tools"},
		{root, "http://127.0.0.1:19090", "relative"},
		{root, "http://127.0.0.1:19090", "/"},
		{root, "http://127.0.0.1:19090", "/tmp/../sentinel-tools"},
		{root, "http://127.0.0.1:19090", "/tmp/sentinel\ntools"},
		{root + "\nprivate", "http://127.0.0.1:19090", "/tmp/sentinel-tools"},
	} {
		if assets, err := Assets(tc.root, tc.endpoint, tc.executable); err == nil || assets != nil {
			t.Fatalf("unsafe config accepted: %q", tc)
		}
	}
}

// Delegating default telemetry must preserve its native cache's null/stale
// values and extension maps, while all writes stay in the isolated fixture.
// The controller's default-endpoint GET is read-only, and its availability is
// deliberately not assumed. The fresh cache prevents live telemetry polling.
func TestStatusDefaultEndpointPreservesNativeTelemetry(t *testing.T) {
	root := canonicalRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	cache := map[string]any{"sample_time": float64(time.Now().UnixNano()) / 1e9, "run_id": "", "gateway": false, "gateway_stale": true, "gateway_sample_time": 1, "runtimes": map[string]any{"large": nil, "small": map[string]any{"model": "fixture", "model_loaded": false, "stale": true, "stats": map[string]any{}, "optimizations": map[string]any{"future": nil}}}, "recent_errors": 3, "last_speed": map[string]any{}}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "statusline.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, raw := runSnapshot(t, root, "http://127.0.0.1:19090")
	telemetry, ok := snapshot["telemetry"].(map[string]any)
	if !ok || telemetry["gateway"] != false || telemetry["gateway_stale"] != true || telemetry["recent_errors"] != float64(3) {
		t.Fatalf("native telemetry replaced: %s", raw)
	}
	runtimes := telemetry["runtimes"].(map[string]any)
	if runtimes["large"] != nil || !reflect.DeepEqual(runtimes["small"].(map[string]any)["optimizations"], map[string]any{"future": nil}) {
		t.Fatalf("native unknowns lost: %s", raw)
	}
	if strings.Contains(raw, "PRIVATE") {
		t.Fatal("unexpected private fixture output")
	}
}
