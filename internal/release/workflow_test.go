package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func workflowScript(t *testing.T, file, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", file))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if line == "      - name: "+name {
			found = true
		}
		if found && line == "        run: |" {
			var body []string
			for _, next := range lines[i+1:] {
				if next != "" && !strings.HasPrefix(next, "          ") {
					break
				}
				body = append(body, strings.TrimPrefix(next, "          "))
			}
			return strings.Join(body, "\n")
		}
	}
	t.Fatal("workflow shell step missing", name)
	return ""
}

func shellDecision(t *testing.T, file, name string, settings map[string]string) (string, string) {
	t.Helper()
	path := t.TempDir()
	doubles := map[string]string{
		"docker": "#!/bin/sh\nprintf 'docker %s\\n' \"$*\" >> \"$MOCK_CALLS\"\n",
		"gh":     "#!/bin/sh\ncase \"$*\" in\n *branches/main*) printf '%s\\n' \"$MOCK_MAIN\" ;;\n *matching-refs/tags/v*) printf '%s\\n' \"$MOCK_LATEST\" ;;\n release*) printf 'gh %s\\n' \"$*\" >> \"$MOCK_CALLS\" ;;\n *) printf '%s\\n' \"$MOCK_TITLE\" ;;\nesac\n",
		"git":    "#!/bin/sh\ncase \"$*\" in\n 'tag --points-at HEAD') printf '%s\\n' \"$MOCK_EXISTING\" ;;\n 'tag --list v* --sort=-version:refname') printf '%s\\n' \"$MOCK_LATEST\" ;;\n *) printf '%s\\n' \"$*\" >> \"$MOCK_CALLS\" ;;\nesac\n",
	}
	for name, body := range doubles {
		if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0700); err != nil { // #nosec G306 -- Private executable test double requires owner execute permission.
			t.Fatal(err)
		}
	}
	output := filepath.Join(path, "outputs")
	calls := filepath.Join(path, "calls")
	if err := os.WriteFile(output, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", workflowScript(t, file, name)) // #nosec G204 -- Checked-in workflow shell tested against private command doubles.
	cmd.Env = append(os.Environ(), "PATH="+path+string(os.PathListSeparator)+os.Getenv("PATH"), "GITHUB_OUTPUT="+output, "MOCK_CALLS="+calls, "REPOSITORY=example/Sentinel", "COMMIT_SHA=abc")
	for key, value := range settings {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("workflow failed: %v %s", err, result)
	}
	values, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	commands, _ := os.ReadFile(calls)
	return string(values), string(commands)
}

func TestWorkflowReleaseDecisions(t *testing.T) {
	for _, tc := range []struct{ sha, current string }{{"abc", "true"}, {"newer", "false"}} {
		output, _ := shellDecision(t, "auto-release.yml", "Check current main", map[string]string{"MOCK_MAIN": tc.sha})
		if output != "current="+tc.current+"\n" {
			t.Fatal(output)
		}
	}
	for _, version := range []string{"v4.2.9", "v4.3.0"} {
		for _, name := range []string{"Promote newest semantic version to latest", "Publish release binaries"} {
			_, calls := shellDecision(t, "release.yml", name, map[string]string{"VERSION": version, "IMAGE": "ghcr.io/example/sentinel", "MOCK_LATEST": "v4.3.0"})
			marker := "gh release edit"
			if strings.HasPrefix(name, "Promote") {
				marker = "docker buildx imagetools create"
			}
			if strings.Contains(calls, marker) != (version == "v4.3.0") {
				t.Fatal(version, name, calls)
			}
		}
	}
}

func TestJobCacheCleanupPreservesHostCache(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "job-go-cache")
	module := filepath.Join(cache, "mod", "example.com", "lib@v1.0.0")
	if err := os.MkdirAll(module, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "source.go"), []byte("package library\n"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(module, 0500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(module, 0700) }()
	host := filepath.Join(root, "host-cache")
	if err := os.Mkdir(host, 0700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(host, "keep")
	if err := os.WriteFile(keep, []byte("host cache"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-e", "-c", workflowScript(t, "qwen-metal.yml", "Remove job runtime")) // #nosec G204 -- Checked-in workflow shell tested against private command doubles.
	cmd.Env = append(os.Environ(), "QWEN_JOB_GO_ROOT="+cache, "QWEN_JOB_VENV=")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(output), err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("job cache survived", err)
	}
	if body, err := os.ReadFile(keep); err != nil || string(body) != "host cache" {
		t.Fatal("host cache changed", err)
	}
}

func TestPreparationWorkflowReusesBranchAndDispatchesPinnedBuild(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "first run", true: "rerun"}[existing], func(t *testing.T) {
			root := t.TempDir()
			calls := filepath.Join(root, "calls")
			tools := map[string]string{
				"gh": `#!/bin/bash
set -eu
printf 'gh %s\n' "$*" >> "$MOCK_CALLS"
case "$*" in
 *matching-refs/tags/v*) echo v3.5.0 ;;
 *branches/main*) echo "$COMMIT_SHA" ;;
 'pr list '*) if [ "$EXISTING" = true ]; then echo '{"number":47,"state":"OPEN"}'; else echo '{}'; fi ;;
 *git/ref/heads/release/v3.6.0*) echo bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ;;
esac
`,
				"git": `#!/bin/bash
set -eu
printf 'git %s\n' "$*" >> "$MOCK_CALLS"
case "$*" in
 'log '*) echo 'feat: roles' ;;
 'ls-remote '*) if [ "$EXISTING" = true ]; then echo 'bbbb refs/heads/release/v3.6.0'; fi ;;
esac
`,
				"sentinel-release-tools": "#!/bin/sh\necho v3.6.0\n",
			}
			for name, body := range tools {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil { // #nosec G306 -- Private executable workflow test doubles.
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", workflowScript(t, "auto-release.yml", "Prepare reviewable release PR")) // #nosec G204 -- Checked-in shell runs against private command doubles.
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNER_TEMP="+root, "MOCK_CALLS="+calls, "REPOSITORY=example/Sentinel", "COMMIT_SHA="+strings.Repeat("a", 40), "BUMP=minor", fmt.Sprintf("EXISTING=%t", existing))
			if result, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("workflow failed: %v %s", err, result)
			}
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			operations := string(data)
			if strings.Contains(operations, "git push") == existing || strings.Contains(operations, "gh pr create") == existing || strings.Contains(operations, "--force") && strings.Contains(operations, "git push --force") {
				t.Fatal(operations)
			}
			if !strings.Contains(operations, "gh workflow run build.yml --repo example/Sentinel --ref release/v3.6.0 -f source_sha="+strings.Repeat("b", 40)) {
				t.Fatal("dispatch did not pin the preparation commit", operations)
			}
		})
	}
}
