package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeGitHubCLI(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	sha := strings.Repeat("a", 40)
	files := map[string]string{"commit.json": `{"sha":"` + sha + `","type":"commit"}`, "tags.json": `[]`, "runs.json": `{"workflow_runs":[{"name":"Build","event":"push","status":"completed","conclusion":"success","head_branch":"main","head_sha":"` + sha + `"}]}`, "created.json": `{"ref":"refs/tags/v1.0.0"}`}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nif [ \"$SENTINEL_TEST_GH_MODE\" = fail ]; then exit 1; fi\nif [ \"$SENTINEL_TEST_GH_MODE\" = malformed ]; then printf '{'; exit 0; fi\ncase \"$2\" in\n*/commits/*) file=commit.json ;;\n*/git/matching-refs/*) file=tags.json ;;\n*/actions/runs*) file=runs.json ;;\n*/git/refs) file=created.json ;;\n*) exit 2 ;;\nesac\n/bin/cat \"$SENTINEL_TEST_GH_ROOT/$file\"\n"
	if err := os.WriteFile(filepath.Join(root, "gh"), []byte(script), 0700); err != nil { // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
		t.Fatal(err)
	}
	t.Setenv("PATH", root+":/usr/bin:/bin")
	t.Setenv("SENTINEL_TEST_GH_ROOT", root)
	t.Setenv("SENTINEL_TEST_GH_MODE", "")
	t.Setenv("GITHUB_OUTPUT", "")
}

func TestReleaseCLIWithOfflineGitHubProcess(t *testing.T) {
	fakeGitHubCLI(t)
	var out, stderr bytes.Buffer
	args := []string{"--repository", "fixture/repo", "--version", "v1.0.0"}
	path := filepath.Join(t.TempDir(), "output")
	os.WriteFile(path, nil, 0600)
	t.Setenv("GITHUB_OUTPUT", path)
	if code := Run(args, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "version=v1.0.0\nref="+strings.Repeat("a", 40)) {
		t.Fatal(string(raw))
	}
	t.Setenv("GITHUB_OUTPUT", filepath.Join(t.TempDir(), "missing"))
	if code := Run(args, nil, &out, &stderr); code != 1 {
		t.Fatal("missing output accepted")
	}
	t.Setenv("GITHUB_OUTPUT", "")
	if code := Run(args, nil, &out, &stderr); code != 0 {
		t.Fatal(code)
	}
	for _, mode := range []string{"fail", "malformed"} {
		t.Setenv("SENTINEL_TEST_GH_MODE", mode)
		if code := Run(args, nil, &out, &stderr); code != 1 {
			t.Fatal(code)
		}
	}
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--repository", "bad", "--version", "v1.0.0"}, {"prepare", "--unknown"}, {"prepare", "extra"}} {
		if code := Run(args, nil, &out, &stderr); code == 0 {
			t.Fatal(args)
		}
	}
}

func TestMetadataPropagatesEachAPIFailure(t *testing.T) {
	for failAt := 1; failAt <= 4; failAt++ {
		calls := 0
		api := func(path string, fields map[string]string, result any) error {
			calls++
			if calls == failAt {
				return errors.New("offline API failure")
			}
			body := `{}`
			switch {
			case strings.Contains(path, "/commits/"):
				body = `{"sha":"` + strings.Repeat("a", 40) + `"}`
			case strings.Contains(path, "matching-refs"):
				body = `[]`
			case strings.Contains(path, "actions/runs"):
				body = `{"workflow_runs":[{"name":"Build","event":"push","status":"completed","conclusion":"success","head_branch":"main","head_sha":"` + strings.Repeat("a", 40) + `"}]}`
			}
			return json.Unmarshal([]byte(body), result)
		}
		if _, err := metadata(api, "fixture/repo", "v1.0.0", "main"); err == nil {
			t.Fatal("API failure hidden", failAt)
		}
	}
	if less([3]uint64{1, 2, 3}, [3]uint64{1, 2, 3}) || !less([3]uint64{1, 2, 3}, [3]uint64{1, 2, 4}) {
		t.Fatal("version ordering")
	}
}

func TestAnnotatedTagsAreImmutableAndBounded(t *testing.T) {
	sha, tagSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, mode := range []string{"valid", "cycle", "bad-sha", "api-error", "invalid-commit"} {
		api := func(path string, fields map[string]string, target any) error {
			body := `{}`
			switch {
			case strings.Contains(path, "/commits/"):
				value := sha
				if mode == "invalid-commit" {
					value = "bad"
				}
				body = `{"sha":"` + value + `"}`
			case strings.Contains(path, "matching-refs"):
				value := tagSHA
				if mode == "bad-sha" {
					value = "bad"
				}
				body = `[{"ref":"refs/tags/v1.0.0","object":{"type":"tag","sha":"` + value + `"}}]`
			case strings.Contains(path, "/git/tags/"):
				if mode == "api-error" {
					return errors.New("offline")
				}
				kind, value := "commit", sha
				if mode == "cycle" {
					kind, value = "tag", tagSHA
				}
				body = `{"object":{"type":"` + kind + `","sha":"` + value + `"}}`
			case strings.Contains(path, "actions/runs"):
				body = `{"workflow_runs":[{"name":"unrelated"},{"name":"Build","event":"push","status":"completed","conclusion":"success","head_branch":"main","head_sha":"` + sha + `"}]}`
			default:
				t.Fatal("annotated existing tag must not be created", fields)
			}
			return json.Unmarshal([]byte(body), target)
		}
		result, err := metadata(api, "fixture/repo", "v1.0.0", "main")
		if (err == nil) != (mode == "valid") || (err == nil && result != sha) {
			t.Fatal(mode, result, err)
		}
	}
}
