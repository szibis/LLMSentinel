// Package release resolves immutable release metadata against successful main CI.
package release

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var stable = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func versionKey(value string) ([3]uint64, error) {
	var result [3]uint64
	if !stable.MatchString(value) {
		return result, errors.New("expected a stable semantic version such as v3.1.0")
	}
	for i, part := range strings.Split(value[1:], ".") {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return result, errors.New("version component is too large")
		}
		result[i] = n
	}
	return result, nil
}

func less(a, b [3]uint64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

type object struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
}
type tag struct {
	Ref    string `json:"ref"`
	Object object `json:"object"`
}
type apiCall func(path string, fields map[string]string, result any) error

func ghAPI(path string, fields map[string]string, result any) error {
	args := []string{"api", path}
	if fields != nil {
		args = append(args, "--method", "POST")
		for _, key := range []string{"ref", "sha"} {
			args = append(args, "-f", key+"="+fields[key])
		}
	}
	data, err := exec.Command("gh", args...).Output()
	if err != nil {
		return errors.New("GitHub API request failed")
	}
	return json.Unmarshal(data, result)
}

func metadata(api apiCall, repository, version, ref string) (string, error) {
	requested, err := versionKey(version)
	if err != nil {
		return "", err
	}
	if !repositoryPattern.MatchString(repository) || ref == "" {
		return "", errors.New("repository and ref are required")
	}
	prefix := "repos/" + repository
	var commit object
	if err = api(prefix+"/commits/"+url.PathEscape(ref), nil, &commit); err != nil {
		return "", err
	}
	if !commitPattern.MatchString(commit.SHA) {
		return "", errors.New("invalid commit returned by GitHub")
	}
	var tags []tag
	if err = api(prefix+"/git/matching-refs/tags/v", nil, &tags); err != nil {
		return "", err
	}
	exists := false
	for _, candidate := range tags {
		if candidate.Ref == "refs/tags/"+version {
			exists = true
			target := candidate.Object
			seen := map[string]bool{}
			for target.Type == "tag" {
				if !commitPattern.MatchString(target.SHA) || seen[target.SHA] || len(seen) >= 32 {
					return "", errors.New("invalid annotated tag chain")
				}
				seen[target.SHA] = true
				var annotated struct {
					Object object `json:"object"`
				}
				if err = api(prefix+"/git/tags/"+target.SHA, nil, &annotated); err != nil {
					return "", err
				}
				target = annotated.Object
			}
			if target.Type != "commit" || target.SHA != commit.SHA {
				return "", errors.New("existing release tag points to a different commit; tags never move")
			}
		}
	}
	if !exists {
		for _, candidate := range tags {
			previous, e := versionKey(strings.TrimPrefix(candidate.Ref, "refs/tags/"))
			if e == nil && !less(previous, requested) {
				return "", errors.New("a new release version must exceed existing stable tags")
			}
		}
	}
	var runs struct {
		Runs []struct {
			Name, Event, Status, Conclusion string
			Branch                          string `json:"head_branch"`
			SHA                             string `json:"head_sha"`
		} `json:"workflow_runs"`
	}
	if err = api(prefix+"/actions/runs?head_sha="+commit.SHA+"&event=push&per_page=100", nil, &runs); err != nil {
		return "", err
	}
	passed := false
	for _, run := range runs.Runs {
		if run.Name == "Build" && run.Event == "push" && run.Branch == "main" && run.SHA == commit.SHA {
			passed = run.Status == "completed" && run.Conclusion == "success"
			break
		}
	}
	if !passed {
		return "", errors.New("selected commit must have a successful completed Build on main")
	}
	if !exists {
		var created tag
		if err = api(prefix+"/git/refs", map[string]string{"ref": "refs/tags/" + version, "sha": commit.SHA}, &created); err != nil {
			return "", err
		}
	}
	return commit.SHA, nil
}

// Run implements sentinel-tools release; publication remains an explicit workflow action.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "prepare" {
		return runPrepare(args[1:], out, stderr)
	}
	flags := flag.NewFlagSet("release", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repository", "", "owner/repository")
	version := flags.String("version", "", "stable v-prefixed version")
	ref := flags.String("ref", "main", "tested main commit or ref")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected arguments")
		return 2
	}
	sha, err := metadata(ghAPI, *repository, *version, *ref)
	if err == nil {
		if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
			var f *os.File
			f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) // #nosec G703 -- Existing GITHUB_OUTPUT file supplied by the Actions runner.
			if err == nil {
				_, err = fmt.Fprintf(f, "version=%s\nref=%s\n", *version, sha)
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(out, "Publishing %s from tested commit %s\n", *version, sha)
	return 0
}
