package lab

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func modFile(t *testing.T, e *environment, name string, data []byte) {
	t.Helper()
	path := filepath.Join(e.root, "control-plugin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareSeedsNativeModAndMigratesOnlyExactLegacyManifest(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "legacy"}[legacy], func(t *testing.T) {
			e := fixture(t)
			if legacy {
				modFile(t, e, ".claude-plugin/plugin.json", []byte("{\"name\":\"sentinel\",\"version\":\"1.0.0\"}\n"))
			}
			modFile(t, e, "skills/status/SKILL.md", []byte("user-owned custom skill"))
			if err := e.prepare(); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.js", "sentinel-config.json"} {
				data, err := os.ReadFile(filepath.Join(e.root, "control-plugin", name))
				if err != nil || len(data) == 0 {
					t.Fatal(name, err)
				}
				if name == ".claude-plugin/plugin.json" && bytes.Equal(data, []byte("{\"name\":\"sentinel\",\"version\":\"1.0.0\"}\n")) {
					t.Fatal("legacy manifest not upgraded")
				}
			}
			var config map[string]string
			if err := readJSON(filepath.Join(e.root, "control-plugin/sentinel-config.json"), &config); err != nil || config["root"] != e.root || config["endpoint"] != endpoint || config["executable"] != e.executable {
				t.Fatalf("config: %v %v", config, err)
			}
			if err := e.prepare(); err != nil {
				t.Fatal("repeated prepare", err)
			}
			data, _ := os.ReadFile(filepath.Join(e.root, "control-plugin/skills/status/SKILL.md"))
			if string(data) != "user-owned custom skill" {
				t.Fatal("custom skill overwritten")
			}
		})
	}
}

func TestPrepareCustomPluginDoesNotAcquireNativeModules(t *testing.T) {
	for _, custom := range []string{`{"name":"custom","version":"2.0.0"}`, "{\"name\":\"sentinel\",\"version\":\"1.0.0\",\"description\":\"custom\"}\n"} {
		e := fixture(t)
		modFile(t, e, ".claude-plugin/plugin.json", []byte(custom))
		if err := e.prepare(); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(e.root, "control-plugin/.claude-plugin/plugin.json"))
		if string(data) != custom {
			t.Fatal("custom manifest overwritten")
		}
		for _, name := range []string{"hooks/hooks.json", "hooks/register.js", "sentinel-config.json"} {
			if _, err := os.Lstat(filepath.Join(e.root, "control-plugin", name)); !os.IsNotExist(err) {
				t.Fatal("native file added to custom plugin", name, err)
			}
		}
	}
}

func TestPreparePreservesExistingModModulesAndConfiguration(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	custom := map[string]string{"hooks/hooks.json": `{"modules":["./custom.js"]}`, "hooks/register.js": "custom owned adapter", "sentinel-config.json": `{"root":"/custom","endpoint":"http://127.0.0.1:19999","executable":"/custom/tools"}`}
	for name, data := range custom {
		modFile(t, e, name, []byte(data))
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	for name, want := range custom {
		data, _ := os.ReadFile(filepath.Join(e.root, "control-plugin", name))
		if string(data) != want {
			t.Fatal("custom mod file overwritten", name)
		}
	}
}

func TestPrepareNativeModPreflightRejectsSymlinksBeforeActivation(t *testing.T) {
	for _, name := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/register.js", "sentinel-config.json", "hooks"} {
		t.Run(name, func(t *testing.T) {
			e := fixture(t)
			legacy := []byte("{\"name\":\"sentinel\",\"version\":\"1.0.0\"}\n")
			if name != ".claude-plugin/plugin.json" {
				modFile(t, e, ".claude-plugin/plugin.json", legacy)
			}
			target := filepath.Join(e.project, "target")
			if name == "hooks" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(e.root, "control-plugin", name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if err := e.prepare(); err == nil {
				t.Fatal("symlink accepted", name)
			}
			if name != ".claude-plugin/plugin.json" {
				data, _ := os.ReadFile(filepath.Join(e.root, "control-plugin/.claude-plugin/plugin.json"))
				if !bytes.Equal(data, legacy) {
					t.Fatal("manifest activated despite incomplete migration")
				}
			}
			if name != "hooks" {
				data, _ := os.ReadFile(target)
				if string(data) != "untouched" {
					t.Fatal("symlink target changed")
				}
			}
			if name != "sentinel-config.json" {
				if _, err := os.Stat(filepath.Join(e.root, "control-plugin/sentinel-config.json")); !os.IsNotExist(err) {
					t.Fatal("partial native migration occurred before preflight", err)
				}
			}
		})
	}
}

func TestPrepareNativeModCanResumePartialMigration(t *testing.T) {
	e := fixture(t)
	modFile(t, e, ".claude-plugin/plugin.json", []byte("{\"name\":\"sentinel\",\"version\":\"1.0.0\"}\n"))
	modFile(t, e, "sentinel-config.json", []byte(`{"private_custom":true}`))
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := readJSON(filepath.Join(e.root, "control-plugin/sentinel-config.json"), &config); err != nil || config["private_custom"] != true {
		t.Fatal("partial custom config overwritten", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(mustReadMod(t, e, ".claude-plugin/plugin.json"), &manifest); err != nil || manifest["version"] == "1.0.0" {
		t.Fatal("migration did not resume", err)
	}
}

func mustReadMod(t *testing.T, e *environment, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.root, "control-plugin", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
