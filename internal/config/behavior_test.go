package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeConfigFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderExplicitConfigurationAndErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	p := filepath.Join(t.TempDir(), "config.yaml")
	l := NewLoader(p)
	writeConfigFixture(t, p, "gateway:\n  port: 9123\nthresholds:\n  cache_similarity: 0.77\n")
	cfg, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.Port != 9123 || l.GetConfig() != cfg || l.GetLoadedPath() != p {
		t.Fatalf("loaded config/path mismatch: %#v %q", cfg.Gateway, l.GetLoadedPath())
	}
	if cfg.TokenLimits.QuickAnswer != 256 || cfg.Models.Opus.ID != ModelOpus || cfg.Security.RateLimiting.RequestsPerMinute != 1000 {
		t.Fatal("missing section defaults")
	}
	old := l.GetConfig()
	for _, body := range []string{"[malformed", "gateway:\n  port: 70000\n"} {
		writeConfigFixture(t, p, body)
		if _, err := l.Load(); err == nil {
			t.Errorf("explicit invalid configuration %q accepted", body)
		}
		if l.GetConfig() != old {
			t.Error("failed load replaced prior config")
		}
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Load(); err == nil {
		t.Error("explicit missing configuration accepted")
	}
}

func TestPathExpansionUsesIsolatedHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandHome("~/data"); got != filepath.Join(home, "data") {
		t.Fatalf("home expansion = %q", got)
	}
	for _, p := range []string{"", "relative/path", "/absolute/path", "~/data"} {
		got, err := ExpandPath(p)
		if err != nil {
			t.Fatal(err)
		}
		want := p
		if p == "~/data" {
			want = filepath.Join(home, "data")
		}
		if got != want {
			t.Errorf("ExpandPath(%q)=%q", p, got)
		}
	}
	t.Setenv("HOME", "")
	if _, err := ExpandPath("~/data"); err == nil {
		t.Error("missing HOME must fail")
	}
	if homeDir() != "/tmp" {
		t.Error("legacy home fallback missing")
	}
}

func TestSettingsAtomicUpdatePreservesOtherKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := filepath.Join(home, ".claude", "settings.json")
	if _, err := ReadClaudeSettings(); err == nil {
		t.Fatal("missing settings accepted")
	}
	if err := WriteClaudeSettings("x", "y"); err == nil {
		t.Fatal("missing settings update accepted")
	}
	writeConfigFixture(t, p, `{"model":"old","effortLevel":"low","custom":{"keep":true}}`)
	if err := WriteClaudeSettings(ModelSonnet, "high"); err != nil {
		t.Fatal(err)
	}
	s, err := ReadClaudeSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != ModelSonnet || s.EffortLevel != "high" {
		t.Fatalf("settings=%+v", s)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"keep": true`) {
		t.Fatal("unrelated key lost")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("settings permissions=%v", fi.Mode())
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary update remains")
	}
	writeConfigFixture(t, p, "bad json")
	if _, err := ReadClaudeSettings(); err == nil {
		t.Error("corrupt settings accepted")
	}
	if err := WriteClaudeSettings("x", "y"); err == nil {
		t.Error("corrupt settings overwritten")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteClaudeSettings("x", "y"); err == nil {
		t.Error("directory settings accepted")
	}
}

func TestEscalationDefaultsRoundtripAndFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	defaults := DefaultEscalationConfig()
	cfg, err := LoadEscalationConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, defaults) {
		t.Fatal("missing file did not yield defaults")
	}
	cfg.Budgets.DailyUSD = 42
	cfg.Statusline.Sources = []StatuslineSourceConfig{{Type: "file", Path: "metrics", Enabled: true}}
	if err := SaveEscalationConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadEscalationConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	p := filepath.Join(home, ".claude", "escalation", "config.yaml")
	writeConfigFixture(t, p, "budgets:\n  daily_usd: 3\nsentiment:\n  enabled: true\n")
	got, err = LoadEscalationConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Budgets.DailyUSD != 3 || got.Budgets.MonthlyUSD != 100 || got.Sentiment.FrustrationRiskThreshold != .7 || len(got.Statusline.Sources) != 3 {
		t.Fatalf("partial config defaults=%+v", got)
	}
	writeConfigFixture(t, p, "[broken")
	if _, err := LoadEscalationConfig(); err == nil {
		t.Fatal("corrupt YAML accepted")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEscalationConfig(); err == nil {
		t.Error("directory config accepted")
	}
	if err := SaveEscalationConfig(cfg); err == nil {
		t.Error("directory config overwritten")
	}
	other := t.TempDir()
	t.Setenv("HOME", other)
	writeConfigFixture(t, filepath.Join(other, ".claude"), "obstacle")
	if err := SaveEscalationConfig(cfg); err == nil {
		t.Error("blocked config directory accepted")
	}
	legacy := DefaultLegacyConfig()
	if legacy.SessionTimeout != 1800 || legacy.DashboardPort != 8077 || !strings.HasPrefix(legacy.DataDir, other) {
		t.Errorf("legacy defaults=%+v", legacy)
	}
}

func TestLoaderSearchSaveAndGeneratedDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CONFIG_FILE", "")
	l := NewLoader("")
	if err := l.SaveDefault(); err == nil {
		t.Fatal("saved nil configuration")
	}
	cfg, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.Port != 8077 || l.GetLoadedPath() != "" {
		t.Fatal("default configuration mismatch")
	}
	if err := l.SaveDefault(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude-escalate", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(t.TempDir(), "env.yaml")
	writeConfigFixture(t, env, "gateway:\n  port: 9001\n")
	t.Setenv("CONFIG_FILE", env)
	// Remove saved home fallback to test environment lookup.
	if err := os.Remove(filepath.Join(home, ".claude-escalate", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if cfg, err = l.Load(); err != nil || cfg.Gateway.Port != 9001 {
		t.Fatalf("env config=%+v %v", cfg, err)
	}
	writeConfigFixture(t, "config.yaml", "gateway:\n  port: 9002\n")
	t.Setenv("CONFIG_FILE", "./config.yaml")
	if cfg, err = l.Load(); err != nil || cfg.Gateway.Port != 9002 {
		t.Fatalf("local config=%+v %v", cfg, err)
	}
	if got := l.generateDefaultConfig(); got.Validate() != nil || got.Optimizations.RTK.Enabled || !got.Security.Enabled || got.TokenLimits.Learning != 1024 {
		t.Fatalf("generated defaults=%+v", got)
	}
	if got := l.generateDefaultConfigWithDiscovery(); got.Validate() != nil || !got.Optimizations.SemanticCache.Enabled {
		t.Fatal("discovered defaults invalid")
	}
	if findDiscoveryConfig() != "" {
		t.Fatal("unexpected discovery config")
	}
	writeConfigFixture(t, "discovery.yaml", "[invalid")
	if findDiscoveryConfig() != "./discovery.yaml" {
		t.Fatal("discovery path missing")
	}
	if got := l.generateDefaultConfigWithDiscovery(); got.Validate() != nil {
		t.Fatal("failed discovery fallback invalid")
	}
	writeConfigFixture(t, "discovery.yaml", "discovery:\n  rtk:\n    search_paths: [missing-rtk]\n  scrapling:\n    search_paths: [missing-scrapling]\n  git:\n    search_paths: [missing-git]\n")
	if got := l.generateDefaultConfigWithDiscovery(); got.Validate() != nil {
		t.Fatal("YAML discovery defaults invalid")
	}
}

func TestValidationBoundaries(t *testing.T) {
	for _, port := range []int{1, 65535} {
		c := DefaultConfig()
		applyNewConfigDefaults(c)
		c.Gateway.Port = port
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name   string
		modify func(*Config)
	}{
		{"port zero", func(c *Config) { c.Gateway.Port = 0 }}, {"port overflow", func(c *Config) { c.Gateway.Port = 65536 }},
		{"similarity below", func(c *Config) {
			c.Optimizations.SemanticCache.Enabled = true
			c.Optimizations.SemanticCache.SimilarityThreshold = -.1
		}},
		{"similarity above", func(c *Config) {
			c.Optimizations.SemanticCache.Enabled = true
			c.Optimizations.SemanticCache.SimilarityThreshold = 1.1
		}},
		{"false positive below", func(c *Config) {
			c.Optimizations.SemanticCache.Enabled = true
			c.Optimizations.SemanticCache.FalsePositiveLimit = -1
		}},
		{"false positive above", func(c *Config) {
			c.Optimizations.SemanticCache.Enabled = true
			c.Optimizations.SemanticCache.FalsePositiveLimit = 101
		}},
		{"rate zero", func(c *Config) { c.Security.RateLimiting.RequestsPerMinute = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultConfig()
			applyNewConfigDefaults(c)
			tc.modify(c)
			if c.Validate() == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, id := range []string{ModelHaiku, ModelSonnet, ModelOpus, "unknown", "xhaikux"} {
		want := map[string]string{ModelHaiku: "haiku", ModelSonnet: "sonnet", ModelOpus: "opus", "unknown": "unknown", "xhaikux": "haiku"}[id]
		if ModelShortName(id) != want {
			t.Errorf("short name for %q", id)
		}
		wantTier := map[string]ModelTier{"haiku": TierHaiku, "sonnet": TierSonnet, "opus": TierOpus}[want]
		if wantTier == 0 {
			wantTier = TierHaiku
		}
		if ModelTierOf(id) != wantTier {
			t.Errorf("tier for %q", id)
		}
	}
}

func TestReloaderRetainsConfigOnFailureAndRunsCallbacks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	p := filepath.Join(t.TempDir(), "config.yaml")
	writeConfigFixture(t, p, "gateway:\n  port: 9001\n")
	l := NewLoader(p)
	if _, err := l.Load(); err != nil {
		t.Fatal(err)
	}
	r := NewReloader(p, l)
	if r.GetConfigPath() != p {
		t.Fatal("wrong watched path")
	}
	calls := make(chan int, 4)
	r.OnReload(func() error { calls <- l.GetConfig().Gateway.Port; return nil })
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	writeConfigFixture(t, p, "gateway:\n  port: 9002\n")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-calls:
		if got != 9002 {
			t.Fatalf("callback saw port %d", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not reload")
	}
	writeConfigFixture(t, p, "[invalid")
	if err := r.ReloadNow(); err == nil {
		t.Error("invalid reload accepted")
	}
	if l.GetConfig().Gateway.Port != 9002 {
		t.Error("invalid reload replaced config")
	}
	writeConfigFixture(t, p, "gateway:\n  port: 9003\n")
	r.OnReload(func() error { return errors.New("callback rejection") })
	if err := r.ReloadNow(); err == nil || !strings.Contains(err.Error(), "callback failed") {
		t.Fatalf("callback error=%v", err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	r.checkAndReload()
	missing := NewReloader(p, l)
	if err := missing.Start(); err == nil {
		t.Error("missing watched file accepted")
	}
}

func TestOptionHintsOptionalFields(t *testing.T) {
	spec := &ConfigSpec{Sections: map[string]Section{"test": {Options: map[string]interface{}{"plain": map[string]interface{}{}, "full": map[string]interface{}{"title": "Timeout", "description": "Wait", "type": "integer", "default": 5, "unit": "ms"}, "invalid": 5}}}}
	for _, missing := range []string{"missing"} {
		if spec.GetSectionHint(missing) != "" || spec.GetOptionHint(missing, "x") != "" {
			t.Error("unknown hint returned")
		}
	}
	if spec.GetOptionHint("test", "invalid") != "" || spec.GetOptionHint("test", "missing") != "" {
		t.Error("invalid option hint returned")
	}
	if !strings.Contains(spec.GetOptionHint("test", "plain"), "plain") {
		t.Error("fallback title missing")
	}
	full := spec.GetOptionHint("test", "full")
	for _, want := range []string{"Timeout", "Wait", "integer", "(ms)", "5"} {
		if !strings.Contains(full, want) {
			t.Errorf("hint missing %q", want)
		}
	}
}

func TestSettingsRejectsNullObjectWithoutPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := filepath.Join(homeDir(), ".claude", "settings.json")
	writeConfigFixture(t, p, "null")
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("null settings panicked: %v", r)
		}
	}()
	if err := WriteClaudeSettings(ModelSonnet, "high"); err == nil {
		t.Error("null settings accepted")
	}
}

func TestLoaderErrorDoesNotExposeConfigurationValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	p := filepath.Join(t.TempDir(), "config.yaml")
	const secret = "s3cr3t"
	writeConfigFixture(t, p, "gateway:\n  port: "+secret+"\n")
	_, err := NewLoader(p).Load()
	if err == nil {
		t.Fatal("invalid typed value accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("configuration value exposed in load error")
	}
}
