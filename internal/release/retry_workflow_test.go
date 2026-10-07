package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseRetryRequiresReviewedImmutableSource(t *testing.T) {
	for _, fault := range []string{"", "branch", "moving-ref", "version", "missing-tag", "diverged", "failed-build"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			tools := map[string]string{
				"gh": `#!/bin/sh
case "$*" in
 *git/ref/tags/*) [ "$FAULT" != missing-tag ] ;;
 *contents/VERSION*) if [ "$FAULT" = version ]; then echo v9.9.9; else echo v3.5.2; fi | base64 ;;
 *compare/*) if [ "$FAULT" = diverged ]; then echo diverged; else echo ahead; fi ;;
 *) exit 1 ;;
esac
`,
				"sentinel-release-tools": `#!/bin/sh
printf '%s\n' "$*" > "$MOCK_CALLS"
[ "$FAULT" != failed-build ]
`,
			}
			for name, body := range tools {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil { // #nosec G306 -- Private executable workflow fixture.
					t.Fatal(err)
				}
			}
			ref, branch := strings.Repeat("a", 40), "refs/heads/main"
			if fault == "branch" {
				branch = "refs/heads/untrusted"
			}
			if fault == "moving-ref" {
				ref = "main"
			}
			calls := filepath.Join(root, "calls")
			cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", workflowScript(t, "release.yml", "Validate reviewed release retry")) // #nosec G204 -- Checked-in workflow and private command doubles.
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNER_TEMP="+root, "MOCK_CALLS="+calls, "FAULT="+fault, "REPOSITORY=example/Sentinel", "VERSION=v3.5.2", "SOURCE_REF="+ref, "REQUEST_REF="+branch)
			output, err := cmd.CombinedOutput()
			if (err == nil) != (fault == "") {
				t.Fatalf("fault=%s err=%v %s", fault, err, output)
			}
			body, readErr := os.ReadFile(calls)
			if fault == "" && (readErr != nil || !strings.Contains(string(body), "--ref "+ref)) {
				t.Fatalf("retry source not pinned: %s %v", body, readErr)
			}
			if fault != "" && fault != "failed-build" && !os.IsNotExist(readErr) {
				t.Fatal("invalid retry reached release helper")
			}
		})
	}
}
