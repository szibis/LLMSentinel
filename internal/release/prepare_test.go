package release

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparationPreservesHistoryAndConsumesUnreleased(t *testing.T) {
	root := t.TempDir()
	original := "# Changelog\n\n## [Unreleased]\n\n- Backfilled work.\n- feat: new routing\n\n## [0.7.0] - 2026-04-27\n\nOld history.\n"
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	notes := "feat: new routing\nfix: cache\nfeat: new routing\n"
	version, err := prepare(root, "v1.2.9", "minor", "2026-10-06", notes)
	if err != nil || version != "v1.3.0" {
		t.Fatal(version, err)
	}
	first, _ := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if !strings.Contains(string(first), "## [1.3.0] - 2026-10-06\n") || !strings.Contains(string(first), "- Backfilled work.") || !strings.HasSuffix(string(first), "## [0.7.0] - 2026-04-27\n\nOld history.\n") || strings.Count(string(first), "- feat: new routing") != 1 {
		t.Fatal(string(first))
	}
	if _, err = prepare(root, "v1.2.9", "minor", "2026-10-06", notes); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if string(first) != string(second) {
		t.Fatal("rerun changed notes")
	}
	got, _ := os.ReadFile(filepath.Join(root, "VERSION"))
	if string(got) != "v1.3.0\n" {
		t.Fatal(string(got))
	}
}

func TestPrepareCLI(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "subjects")
	if err := os.WriteFile(notes, []byte("fix: local preparation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte("# Changelog\n\n## [0.7.0] - 2026-04-27\n\nHistory.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	args := []string{"prepare", "--root", root, "--latest", "v3.5.0", "--bump", "patch", "--date", "2026-10-06", "--notes", notes}
	if code := Run(args, nil, &out, &stderr); code != 0 || out.String() != "v3.5.1\n" || stderr.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	for _, invalid := range [][]string{{"prepare", "--unknown"}, {"prepare", "unexpected"}, {"prepare", "--notes", filepath.Join(root, "missing")}} {
		if Run(invalid, nil, &out, &stderr) == 0 {
			t.Fatal("accepted invalid CLI", invalid)
		}
	}
}

func TestPreparationRejectsInvalidInputWithoutWrites(t *testing.T) {
	for _, tc := range []struct{ latest, bump, date string }{{"v1.2.3-beta", "patch", "2026-10-06"}, {"v1.2.3", "none", "2026-10-06"}, {"v1.2.3", "patch", "2026-02-30"}, {"v18446744073709551615.0.0", "major", "2026-10-06"}} {
		root := t.TempDir()
		path := filepath.Join(root, "CHANGELOG.md")
		_ = os.WriteFile(path, []byte("# Changelog\n"), 0600)
		if _, err := prepare(root, tc.latest, tc.bump, tc.date, "fix: things"); err == nil {
			t.Fatal(tc)
		}
		got, _ := os.ReadFile(path)
		if string(got) != "# Changelog\n" {
			t.Fatal("changed invalid input")
		}
	}
}

func TestReleasePlan(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		title      string
		labels     []string
		mode, bump string
		bad        bool
	}{
		{"docs: setup", nil, "skip", "", false}, {"feat(router): roles", nil, "prepare", "minor", false}, {"feat(router)!: breaking", nil, "prepare", "major", false}, {"fix: cache", nil, "prepare", "patch", false}, {"chore: version", []string{"release:minor"}, "prepare", "minor", false}, {"feat!: breaking", []string{"release:patch"}, "prepare", "patch", false}, {"fix: cache", []string{"release:patch", "release:major"}, "", "", true}, {"fix: cache", []string{"release:banana"}, "", "", true}, {"chore: release v1.3.0", []string{"release"}, "publish", "", false}, {"chore: release v1.3.0", nil, "", "", true}, {"chore: release invalid", []string{"release"}, "", "", true},
	} {
		t.Run(tc.title+strings.Join(tc.labels, ","), func(t *testing.T) {
			pr := mergedPR{Title: tc.title, MergeSHA: sha, MergedAt: "2026-10-06", Base: prBase{Ref: "main"}}
			for _, label := range tc.labels {
				pr.Labels = append(pr.Labels, prLabel{Name: label})
			}
			plan, err := selectPlan([]mergedPR{pr}, sha)
			if (err != nil) != tc.bad || err == nil && (plan.Mode != tc.mode || plan.Bump != tc.bump) {
				t.Fatal(plan, err)
			}
		})
	}
	if _, err := selectPlan([]mergedPR{{Title: "fix: other", MergeSHA: strings.Repeat("b", 40), MergedAt: "x", Base: prBase{Ref: "main"}}}, sha); err != nil {
		t.Fatal(err)
	}
}
