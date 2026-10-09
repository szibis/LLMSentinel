package discovery

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func executableFixture(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestConfiguredDiscoveryExecutableGlobRuntimeAndSockets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, "bin")
	tool := executableFixture(t, filepath.Join(bin, "fixture-tool"))
	t.Setenv("PATH", bin)
	socket := filepath.Join(home, "mcp")
	if err := os.MkdirAll(filepath.Join(socket, "fixture-server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(socket, "not-server"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := `discovery:
  rtk:
    search_paths: ["~/bin/fixture-tool"]
  scrapling:
    search_paths: ["~/bin/fixture-*"]
  git:
    search_paths: ["missing/fixture-tool"]
  language_servers:
    fixture:
      search_paths: ["~/bin/fixture-tool"]
    missing:
      search_paths: ["absent-tool"]
  runtimes:
    fixture:
      search_paths: ["~/bin/fixture-tool"]
    missing:
      search_paths: ["absent-runtime"]
  mcp_servers:
    socket_search_paths: ["~/mcp", "absent"]
  platform_overrides:
    ` + runtime.GOOS + `:
      primary_paths: ["~/bin"]
`
	path := filepath.Join(home, "discovery.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	detected, err := DetectToolsWithConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if detected.RTKPath != tool || detected.ScraplingPath != tool || detected.GitPath != tool || detected.LSPServers["fixture"] != tool || detected.LanguageRuntimes["fixture"] != tool || len(detected.MCPServersAvailable) != 1 || len(detected.PlatformPrimaryPaths) != 1 {
		t.Fatalf("detected=%+v", detected)
	}
	if _, err := DetectToolsWithConfig(filepath.Join(home, "missing")); err == nil {
		t.Fatal("missing discovery configuration accepted")
	}
	if err := os.WriteFile(path, []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DetectToolsWithConfig(path); err == nil {
		t.Fatal("invalid discovery YAML accepted")
	}
	if got := findTool([]string{"~/missing*"}); got != "" {
		t.Errorf("missing glob=%q", got)
	}
	if got := findGlob("["); len(got) != 0 {
		t.Error("invalid glob matched")
	}
	if got := findGlob(filepath.Join(home, "*")); len(got) != 0 {
		t.Error("non-executable file/directory glob matched")
	}
	t.Setenv("FIXTURE_BIN", bin)
	if expandPath("$FIXTURE_BIN/fixture-tool") != tool {
		t.Error("environment path expansion failed")
	}
	t.Setenv("HOME", "")
	if expandPath("~/x") != "~/x" {
		t.Error("missing HOME path expansion changed input")
	}
}
func TestGeneratedDiscoveryConfigContainsEnabledTools(t *testing.T) {
	tools := &DetectedTools{RTKPath: "/fixture/rtk", ScraplingPath: "/fixture/scrapling", GitPath: "/fixture/git", LSPServers: map[string]string{"go": "/fixture/gopls"}}
	text := GenerateDefaultConfig(tools)
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatal(err)
	}
	section, ok := doc["tools"].(map[string]interface{})
	if !ok || section["cli"] == nil || section["scrapling"] == nil || section["lsp"] == nil {
		t.Fatalf("generated tools missing: %#v", doc["tools"])
	}
	empty := GenerateDefaultConfig(&DetectedTools{})
	if !strings.Contains(empty, "RTK not detected") {
		t.Fatal("disabled tool comment missing")
	}
}
func TestSaveAndLoadOrDetectUsesPrivateHomeAndReportsFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	first, err := SaveDefaultConfig("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := SaveDefaultConfig("second")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "config-auto.yaml" {
		t.Fatal("existing user config overwritten")
	}
	b, err := os.ReadFile(first)
	if err != nil || string(b) != "first" {
		t.Fatal("original config changed")
	}
	fi, err := os.Stat(second)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("config permissions not private")
	}
	text, tools, err := LoadOrDetect(first, "")
	if err != nil || text != "first" || tools != nil {
		t.Fatalf("file load=%q %+v %v", text, tools, err)
	}
	text, tools, err = LoadOrDetect("", filepath.Join(home, "missing-discovery"))
	if err != nil || text == "" || tools == nil {
		t.Fatalf("fallback discovery=%q %+v %v", text, tools, err)
	}
	cfg := filepath.Join(home, "discovery.yaml")
	if err := os.WriteFile(cfg, []byte("discovery: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if text, tools, err = LoadOrDetect("", cfg); err != nil || text == "" || tools == nil {
		t.Fatalf("configured discovery=%q %+v %v", text, tools, err)
	}
	blocked := t.TempDir()
	t.Setenv("HOME", blocked)
	if err := os.WriteFile(filepath.Join(blocked, ".claude-escalate"), []byte("obstacle"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveDefaultConfig("x"); err == nil {
		t.Fatal("blocked directory accepted")
	}
	if text, tools, err = LoadOrDetect("", ""); err == nil || text == "" || tools == nil {
		t.Fatal("save failure did not preserve detected content")
	}
	t.Setenv("HOME", "")
	if _, err := SaveDefaultConfig("x"); err == nil {
		t.Fatal("missing HOME accepted")
	}
}
