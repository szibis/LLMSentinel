package lab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T) *environment {
	t.Helper()
	project, _ := filepath.EvalSymlinks(t.TempDir())
	root, _ := filepath.EvalSymlinks(t.TempDir())
	return &environment{project: project, root: filepath.Join(root, "lab"), executable: "/tmp/sentinel-tools", out: new(bytes.Buffer), stderr: new(bytes.Buffer)}
}

func TestPrepareRejectsSymlinkedProfileWithoutWritingTarget(t *testing.T) {
	e := fixture(t)
	target := t.TempDir()
	if err := os.MkdirAll(e.root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(e.root, "claude")); err != nil {
		t.Fatal(err)
	}
	if err := e.prepare(); err == nil {
		t.Fatal("accepted symlinked profile")
	}
	if _, err := os.Stat(filepath.Join(target, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("wrote outside owned profile", err)
	}
}

func TestPrepareSeedsExplicitClaudeControlPlugin(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude/commands/sentinel-status.md", "control-plugin/.claude-plugin/plugin.json", "control-plugin/skills/status/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(e.root, name)); err != nil {
			t.Fatal(name, err)
		}
	}
}

func TestPreparePreservesCustomLegacyStatusWrapper(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	command := "echo " + filepath.Join(e.project, "scripts", "sentinel_statusline.py")
	path := filepath.Join(e.root, "claude", "settings.json")
	if err := writeJSON(path, map[string]any{"statusLine": map[string]any{"command": command}}); err != nil {
		t.Fatal(err)
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := readJSON(path, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["statusLine"].(map[string]any)["command"] != command {
		t.Fatal("overwrote custom wrapper")
	}
}

func TestLegacyOwnershipRequiresExactGeneratedCommands(t *testing.T) {
	e := fixture(t)
	command := "python3 " + quote(filepath.Join(e.project, "scripts", "sentinel_statusline.py")) + " --root " + quote(e.root)
	if !e.ownedLegacyStatus(command) {
		t.Fatal("generated legacy status line unrecognized")
	}
	for _, custom := range []string{command + " --custom", "echo " + command, command + "; echo custom"} {
		if e.ownedLegacyStatus(custom) {
			t.Fatal("accepted custom wrapper", custom)
		}
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.root, "claude", "commands", "sentinel-status.md")
	custom := "User custom command mentioning " + filepath.Join(e.project, "scripts", "sentinel_control.py")
	if err := atomicFile(path, []byte(custom)); err != nil {
		t.Fatal(err)
	}
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != custom {
		t.Fatal("overwrote edited legacy asset", err)
	}
}

func TestPrepareRejectsSymlinkedSettingsFile(t *testing.T) {
	e := fixture(t)
	if err := privateDirectory(filepath.Join(e.root, "claude")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(e.root, "claude", "settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := e.prepare(); err == nil {
		t.Fatal("accepted symlinked settings")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "{}" {
		t.Fatal("changed external settings")
	}
}

func TestSupervisorGraceAllowsChildCleanupBeyondOrdinaryGrace(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	process, err := e.spawn([]string{"/bin/sh", "-c", "trap 'sleep 5; touch cleaned; exit 0' TERM; touch ready; while :; do sleep 1; done"}, "helper.log", allowedEnvironment("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(-process.cmd.Process.Pid, syscall.SIGKILL) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(e.root, "workspace", "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := stopProcessGrace(process, 25*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "workspace", "cleaned")); err != nil {
		t.Fatal("supervisor killed before delayed child cleanup", err)
	}
}

func TestExistingOwnedLogAndLockBecomePrivate(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run.lock", "helper.log"} {
		path := filepath.Join(e.root, name)
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := acquireLock(e.root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	process, err := e.spawn([]string{"/bin/sh", "-c", "exit 0"}, "helper.log", allowedEnvironment("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	<-process.done
	for _, name := range []string{"run.lock", "helper.log"} {
		info, err := os.Stat(filepath.Join(e.root, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("nonprivate file", name, info, err)
		}
	}
}

func TestNativeCapabilityRefusesLegacyWithoutStartingAnything(t *testing.T) {
	if err := validateNativeProfiles(map[string]any{"capabilities": map[string]any{"chat_template_kwargs": []any{"enable_thinking"}}}); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeProfiles(map[string]any{}); err == nil || !strings.Contains(err.Error(), "update the external") {
		t.Fatal("missing actionable native profile refusal", err)
	}
}

func TestPortPreflightDoesNotDisturbExistingListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sandbox restricts loopback listener: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := preflightPorts([]int{port}, 0); err == nil {
		t.Fatal("occupied port accepted")
	}
	connection, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal("listener disturbed", err)
	}
	connection.Close()
}

func TestOwnedProcessGroupStopLeavesUnrelatedProcessRunning(t *testing.T) {
	e := fixture(t)
	if err := os.MkdirAll(filepath.Join(e.root, "workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	owned, err := e.spawn([]string{"/bin/sh", "-c", "sleep 30"}, "test.log", allowedEnvironment("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	unrelated := exec.Command("/bin/sh", "-c", "sleep 30")
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { syscall.Kill(-unrelated.Process.Pid, syscall.SIGKILL); unrelated.Wait() }()
	if err := stopProcess(owned); err != nil {
		t.Fatal(err)
	}
	if !owned.exited() {
		t.Fatal("owned leader still running")
	}
	if err := unrelated.Process.Signal(os.Signal(syscall.Signal(0))); err != nil {
		t.Fatal("unrelated process signaled", err)
	}
}

func TestLabSubprocessHelper(t *testing.T) {
	id := os.Getenv("SENTINEL_LAB_START_ID")
	if id == "" {
		return
	}
	root := os.Getenv("SENTINEL_LAB_ROOT")
	lock, err := acquireLock(root)
	if err != nil {
		os.Exit(2)
	}
	state := runState{RunID: "helper-run", StartID: id, Phase: "running"}
	if err = writeJSON(filepath.Join(root, "state.json"), state); err != nil {
		os.Exit(3)
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "stop-helper-run")); err == nil { // #nosec G703 -- test helper root is provided by the parent fixture, not a network request.
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	state.Phase = "stopped"
	writeJSON(filepath.Join(root, "state.json"), state)
	lock.Close()
	os.Exit(0)
}

func TestBackgroundStartAndRunSpecificStop(t *testing.T) {
	e := fixture(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e.executable = binary
	if err = e.start(); err != nil {
		t.Fatal(err)
	}
	defer func() { e.stop(true) }()
	if !running(e.root) {
		t.Fatal("supervisor not owned")
	}
	if err = e.start(); err != nil {
		t.Fatal("idempotent start failed", err)
	}
	if err = e.stop(true); err != nil {
		t.Fatal(err)
	}
	if running(e.root) {
		t.Fatal("supervisor remained owned")
	}
}

func TestFailedBackgroundStartReportsOnlyCurrentAttempt(t *testing.T) {
	e := fixture(t)
	if err := os.MkdirAll(e.root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.root, "supervisor.log")
	os.WriteFile(path, []byte("old unrelated failure\n"), 0600)
	binary := filepath.Join(t.TempDir(), "sentinel-tools")
	os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'current startup failure\\n'\nexit 1\n"), 0600)
	os.Chmod(binary, 0700)
	e.executable = binary
	err := e.start()
	if err == nil || !strings.Contains(err.Error(), "current startup failure") || strings.Contains(err.Error(), "old unrelated failure") {
		t.Fatal("incorrect failure provenance", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "old unrelated failure") {
		t.Fatal("history erased")
	}
}

func TestCurrentRunStopMarker(t *testing.T) {
	e := fixture(t)
	lock, err := acquireLock(e.root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := writeJSON(filepath.Join(e.root, "state.json"), runState{RunID: "current-run", Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := e.stop(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "stop-current-run")); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePreservesPreferencesAndSeedsGoCommands(t *testing.T) {
	e := fixture(t)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.root, "claude", "settings.json")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "python") || !strings.Contains(string(data), "statusline") {
		t.Fatal(string(data))
	}
	var settings map[string]any
	json.Unmarshal(data, &settings)
	settings["theme"] = "dark"
	writeJSON(path, settings)
	if err := e.prepare(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "dark") {
		t.Fatal("overwrote preferences")
	}
}
func TestClientEnvironmentDoesNotInheritProviderKeysOrChangeHome(t *testing.T) {
	e := fixture(t)
	t.Setenv("HOME", "/real/home")
	t.Setenv("ANTHROPIC_API_KEY", "production-secret")
	t.Setenv("HTTPS_PROXY", "https://example.com")
	env := e.clientEnvironment("claude")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "production-secret") || strings.Contains(joined, "HTTPS_PROXY") || !strings.Contains(joined, "HOME=/real/home") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "CLAUDE_CONFIG_DIR="+filepath.Join(e.root, "claude")) {
		t.Fatal(joined)
	}
}
func model(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	os.Mkdir(dir, 0700)
	for _, file := range []string{"config.json", "tokenizer.json", "model.safetensors"} {
		os.WriteFile(filepath.Join(dir, file), []byte("{}"), 0600)
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"model_type":"qwen3_5"}`), 0600)
	os.WriteFile(filepath.Join(dir, "tokenizer_config.json"), []byte(`{"chat_template":"{{ enable_thinking }}"}`), 0600)
	return dir
}
func TestRoleModelsValidateActualFamilyTemplates(t *testing.T) {
	for _, tc := range []struct {
		family, template string
		invalid          bool
	}{
		{"lfm2_moe", "<think>reasoning</think>", false},
		{"gemma4", "{{ enable_thinking }}<|channel>thought\n<channel|>", false},
		{"qwen3_5", "{{ enable_thinking }}<think></think>", false},
		{"unknown", "{{ enable_thinking }}<think></think>", true},
		{"gemma4", "{{ enable_thinking }}<think></think>", true},
	} {
		t.Run(tc.family+tc.template, func(t *testing.T) {
			dir := model(t, "model")
			os.WriteFile(filepath.Join(dir, "config.json"), []byte(fmt.Sprintf(`{"model_type":%q}`, tc.family)), 0600)
			os.WriteFile(filepath.Join(dir, "tokenizer_config.json"), []byte(fmt.Sprintf(`{"chat_template":%q}`, tc.template)), 0600)
			err := validateModel(dir, true)
			if (err != nil) != tc.invalid {
				t.Fatalf("validation=%v want invalid=%t", err, tc.invalid)
			}
		})
	}
}
func TestLFMNativeProfileDoesNotRequireThinkingToggle(t *testing.T) {
	err := validateNativeProfiles(map[string]any{"capabilities": map[string]any{"model_family": "lfm2_moe", "thinking_control": false, "reasoning_format": "think", "chat_template_kwargs": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNativeProfilesRejectMismatchedFamilyCapabilities(t *testing.T) {
	for _, capabilities := range []map[string]any{
		{"model_family": "unknown", "thinking_control": true, "reasoning_format": "think", "chat_template_kwargs": []any{"enable_thinking"}},
		{"model_family": "gemma4", "thinking_control": true, "reasoning_format": "think", "chat_template_kwargs": []any{"enable_thinking"}},
		{"model_family": "lfm2_moe", "thinking_control": true, "reasoning_format": "think", "chat_template_kwargs": []any{"enable_thinking"}},
	} {
		if err := validateNativeProfiles(map[string]any{"capabilities": capabilities}); err == nil {
			t.Fatalf("accepted mismatched capabilities: %v", capabilities)
		}
	}
}
func TestNativeRuntimePlanAndIncompleteShardRefusal(t *testing.T) {
	e := fixture(t)
	binary := filepath.Join(t.TempDir(), "mlx-flash")
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0600)
	os.Chmod(binary, 0700)
	settings := runtimeSettings{ModelPath: model(t, "large"), SmallModelPath: model(t, "small"), Executable: binary}
	plan, err := e.runtimePlan(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 || plan[0].Port != 19091 || plan[1].Port != 19092 {
		t.Fatal(plan)
	}
	for _, item := range plan {
		if item.Command[0] != binary || strings.Contains(strings.Join(item.Command, " "), "python") {
			t.Fatal(item)
		}
	}
	os.WriteFile(filepath.Join(settings.ModelPath, "model.safetensors.index.json"), []byte(`{"weight_map":{"a":"../outside.safetensors"}}`), 0600)
	if err := validateModel(settings.ModelPath, true); err == nil {
		t.Fatal("traversal shard accepted")
	}
}
func TestOwnershipLockAndInvalidStopMarker(t *testing.T) {
	e := fixture(t)
	lock, err := acquireLock(e.root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if !running(e.root) {
		t.Fatal("owner not detected")
	}
	if _, err := acquireLock(e.root); err == nil {
		t.Fatal("second owner admitted")
	}
	writeJSON(filepath.Join(e.root, "state.json"), runState{RunID: "../other", Phase: "running"})
	if err := e.stop(false); err == nil {
		t.Fatal("invalid ownership accepted")
	}
	matches, _ := filepath.Glob(filepath.Join(e.root, "stop-*"))
	if len(matches) != 0 {
		t.Fatal(matches)
	}
}
func TestContextUsesMinimumModelWindow(t *testing.T) {
	e := fixture(t)
	os.MkdirAll(e.root, 0700)
	large, small := model(t, "large"), model(t, "small")
	os.WriteFile(filepath.Join(large, "config.json"), []byte(`{"max_position_embeddings":131072}`), 0600)
	os.WriteFile(filepath.Join(small, "config.json"), []byte(`{"text_config":{"max_position_embeddings":65536}}`), 0600)
	writeJSON(filepath.Join(e.root, "runtime.json"), runtimeSettings{ModelPath: large, SmallModelPath: small})
	if !strings.Contains(strings.Join(e.clientEnvironment("claude"), "\n"), "CLAUDE_CODE_MAX_CONTEXT_TOKENS=65536") {
		t.Fatal("minimum context missing")
	}
}
