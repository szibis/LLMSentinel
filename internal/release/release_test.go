package release

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictVersion(t *testing.T) {
	for _, invalid := range []string{"1.2.3", "v01.2.3", "v1.2.3-beta", "v1.2.3\n", "v1.2", "v18446744073709551616.0.0"} {
		if _, err := versionKey(invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	if value, err := versionKey("v3.2.0"); err != nil || value != [3]uint64{3, 2, 0} {
		t.Fatal(value, err)
	}
}

func TestReleaseImmutableGate(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, tags, builds string
		wantError, create  bool
	}{
		{"new tested", `[]`, `[{"name":"Build","event":"push","head_branch":"main","head_sha":"` + sha + `","status":"completed","conclusion":"success"}]`, false, true},
		{"missing build", `[]`, `[]`, true, false},
		{"failed build", `[]`, `[{"name":"Build","event":"push","head_branch":"main","head_sha":"` + sha + `","status":"completed","conclusion":"failure"}]`, true, false},
		{"PR build", `[]`, `[{"name":"Build","event":"pull_request","head_branch":"main","head_sha":"` + sha + `","status":"completed","conclusion":"success"}]`, true, false},
		{"existing immutable", `[{"ref":"refs/tags/v3.2.0","object":{"type":"commit","sha":"` + sha + `"}}]`, `[{"name":"Build","event":"push","head_branch":"main","head_sha":"` + sha + `","status":"completed","conclusion":"success"}]`, false, false},
		{"tag moves", `[{"ref":"refs/tags/v3.2.0","object":{"type":"commit","sha":"` + strings.Repeat("b", 40) + `"}}]`, `[]`, true, false},
		{"older version", `[{"ref":"refs/tags/v3.3.0","object":{"type":"commit","sha":"` + sha + `"}}]`, `[]`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			api := func(path string, fields map[string]string, target any) error {
				var data string
				switch {
				case strings.Contains(path, "/commits/"):
					data = `{"sha":"` + sha + `"}`
				case strings.Contains(path, "matching-refs"):
					data = tc.tags
				case strings.Contains(path, "actions/runs"):
					data = `{"workflow_runs":` + tc.builds + `}`
				default:
					if fields["sha"] != sha || fields["ref"] != "refs/tags/v3.2.0" {
						t.Fatal("incorrect tag mutation")
					}
					created = true
					data = `{}`
				}
				return json.Unmarshal([]byte(data), target)
			}
			result, err := metadata(api, "szibis/LLMSentinel", "v3.2.0", "main")
			if (err != nil) != tc.wantError || created != tc.create || (err == nil && result != sha) {
				t.Fatal(result, err, created)
			}
		})
	}
}
