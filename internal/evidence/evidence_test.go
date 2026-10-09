package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// macOS's system temporary path may itself contain /var or /tmp symlinks.
// Resolve only the test fixture base; tested redirect symlinks remain intact.
func evidenceTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func writeArtifact(t *testing.T, root, path, body string) {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func revisions() Provenance {
	return Provenance{SourceRevision: strings.Repeat("a", 40), ControlRevision: strings.Repeat("b", 40)}
}

func TestAncestorSymlinksCannotRedirectEvidence(t *testing.T) {
	outside := evidenceTempDir(t)
	writeArtifact(t, outside, "nested/safe.txt", "synthetic")
	realRoot := filepath.Join(outside, "nested")
	m, err := Generate(realRoot, []string{"safe.txt"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	base := evidenceTempDir(t)
	link := filepath.Join(base, "redirect")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	redirected := filepath.Join(link, "nested")
	if _, err := Generate(redirected, []string{"safe.txt"}, revisions()); err == nil {
		t.Error("Generate followed symlinked root ancestor")
	}
	if err := Verify(redirected, m); err == nil {
		t.Error("Verify followed symlinked root ancestor")
	}
	if err := WriteManifest(filepath.Join(redirected, "manifest.json"), m); err == nil {
		t.Error("manifest wrote through symlinked ancestor")
	}
	if err := WriteManifest(filepath.Join(realRoot, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(filepath.Join(redirected, "manifest.json")); err == nil {
		t.Error("manifest read through symlinked ancestor")
	}
	if err := os.Chmod(filepath.Join(realRoot, "manifest.json"), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(realRoot, "manifest.json"), 0600)
	if _, err := ReadManifest(filepath.Join(realRoot, "manifest.json")); err == nil {
		t.Error("unreadable manifest accepted")
	}
	if got, err := checkedDirectory(string(filepath.Separator)); err != nil || got != string(filepath.Separator) {
		t.Fatal(got, err)
	}
}

func TestManifestRejectsAmbiguousOrLossyJSON(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "synthetic")
	m, err := Generate(root, []string{"safe.txt"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	cases := []string{
		strings.Replace(text, `"source_revision":`, `"source_revision":"`+strings.Repeat("c", 40)+`","source_revision":`, 1),
		strings.Replace(text, `"source_revision":`, `"source_revision":"`+strings.Repeat("c", 40)+`","source\u005frevision":`, 1),
		strings.Replace(text, `"sha256":`, `"sha256":"`+strings.Repeat("d", 64)+`","sha256":`, 1),
		strings.Replace(text, `"sha256":`, `"sha256":"`+strings.Repeat("d", 64)+`","sha\u003256":`, 1),
		strings.Replace(text, `"model":null`, "\"model\":\"\xff\"", 1),
	}
	for i, body := range cases {
		path := filepath.Join(root, "ambiguous.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadManifest(path); err == nil {
			t.Errorf("case%d accepted ambiguous/lossy JSON", i)
		}
	}
}

func TestCreateVerifySyntheticEvidence(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "tests/result.json", `{"passed":true}`)
	writeArtifact(t, root, "coverage.out", "mode: atomic\n")
	m, err := Generate(root, []string{"tests/result.json", "coverage.out"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != 1 || m.Scope != "synthetic-ci" || m.Provenance.Model != nil || m.Provenance.Client != nil || m.Artifacts[0].Path != "coverage.out" {
		t.Fatal(m)
	}
	if err := Verify(root, m); err != nil {
		t.Fatal(err)
	}
	again, err := Generate(root, []string{"coverage.out", "tests/result.json"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(again)
	if !bytes.Equal(a, b) {
		t.Fatal("manifest not deterministic")
	}
	path := filepath.Join(root, "manifest.json")
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal(st, err)
	}
	got, err := ReadManifest(path)
	if err != nil || Verify(root, got) != nil {
		t.Fatal(got, err)
	}
	writeArtifact(t, root, "coverage.out", "mutated")
	if Verify(root, got) == nil {
		t.Fatal("tampering accepted")
	}
	if err := os.Remove(filepath.Join(root, "coverage.out")); err != nil {
		t.Fatal(err)
	}
	if Verify(root, got) == nil {
		t.Fatal("missing artifact accepted")
	}
}

func TestPrivatePathsTraversalAndSymlinksRejected(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	for _, path := range []string{"", ".", "../outside", "/absolute", "a/../safe.txt", "a\\b", ".sentinel-lab/captures/x", ".git/config", ".env", "secrets/key.json", "credentials.json", "captures/payload.json", "datasets/train.txt", "checkpoints/model.bin"} {
		if _, err := Generate(root, []string{path}, revisions()); err == nil {
			t.Errorf("unsafe path accepted %q", path)
		}
	}
	outside := evidenceTempDir(t)
	writeArtifact(t, outside, "outside.txt", "private")
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(root, []string{"link.txt"}, revisions()); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(root, []string{"linked-dir/outside.txt"}, revisions()); err == nil {
		t.Fatal("symlink directory accepted")
	}
	if _, err := Generate(filepath.Join(root, "linked-dir"), []string{"outside.txt"}, revisions()); err == nil {
		t.Fatal("symlink root accepted")
	}
	if _, err := Generate(root, []string{"safe.txt", "safe.txt"}, revisions()); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := Generate(root, []string{"linked-dir"}, revisions()); err == nil {
		t.Fatal("directory artifact accepted")
	}
	privateRoot := filepath.Join(root, ".sentinel-lab", "synthetic")
	writeArtifact(t, privateRoot, "safe.txt", "private")
	if _, err := Generate(privateRoot, []string{"safe.txt"}, revisions()); err == nil {
		t.Fatal("private root accepted")
	}
}

func TestManifestBoundsAndValidation(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	for _, p := range []Provenance{{}, {SourceRevision: strings.Repeat("x", 129), ControlRevision: "c", RuntimeRevision: nil}} {
		if _, err := Generate(root, []string{"safe.txt"}, p); err == nil {
			t.Fatal("invalid provenance accepted")
		}
	}
	if _, err := Generate(root, nil, revisions()); err == nil {
		t.Fatal("empty evidence accepted")
	}
	paths := make([]string, 257)
	if _, err := Generate(root, paths, revisions()); err == nil {
		t.Fatal("too many artifacts accepted")
	}
	large := filepath.Join(root, "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxArtifactBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := Generate(root, []string{"large.bin"}, revisions()); err == nil {
		t.Fatal("large artifact accepted")
	}
	m, err := Generate(root, []string{"safe.txt"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*Manifest){func(m *Manifest) { m.Version = 2 }, func(m *Manifest) { m.Scope = "real-client" }, func(m *Manifest) { m.Artifacts[0].SHA256 = "bad" }, func(m *Manifest) { m.Artifacts[0].Bytes = -1 }, func(m *Manifest) { m.Artifacts[0].Bytes++ }, func(m *Manifest) { m.Artifacts[0].Path = ".sentinel-lab/private" }} {
		b, _ := json.Marshal(m)
		var changed Manifest
		json.Unmarshal(b, &changed)
		alter(&changed)
		if Verify(root, changed) == nil {
			t.Fatal("invalid manifest accepted", changed)
		}
	}
}

func TestManifestReadWriteFailures(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	m, err := Generate(root, []string{"safe.txt"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "manifest.json")
	for _, body := range []string{"{", `{"version":1,"unknown":true}`, `{} {}`, strings.Repeat(" ", MaxManifestBytes+1)} {
		writeArtifact(t, root, "manifest.json", body)
		if _, err := ReadManifest(path); err == nil {
			t.Fatal("malformed manifest accepted")
		}
	}
	if _, err := ReadManifest(filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing manifest accepted")
	}
	if err := WriteManifest(filepath.Join(root, "safe.txt", "manifest.json"), m); err == nil {
		t.Fatal("invalid parent accepted")
	}
	os.Remove(path)
	if err := os.Symlink(filepath.Join(root, "safe.txt"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(path); err == nil {
		t.Fatal("symlink manifest read")
	}
	if err := WriteManifest(path, m); err == nil {
		t.Fatal("symlink manifest overwritten")
	}
}

func TestRun(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	manifest := filepath.Join(root, "manifest.json")
	var out, stderr bytes.Buffer
	args := []string{"create", "--root", root, "--manifest", manifest, "--source-revision", strings.Repeat("a", 40), "--control-revision", strings.Repeat("b", 40), "--artifact", "safe.txt"}
	if code := Run(args, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if code := Run([]string{"verify", "--root", root, "--manifest", manifest}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(out.String(), "verified") {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{nil, {"unknown"}, {"create", "--bad"}, {"create"}, {"verify"}, {"verify", "--root", root, "--manifest", filepath.Join(root, "absent")}, {"create", "--root", root, "--manifest", manifest, "--source-revision", strings.Repeat("a", 40), "--control-revision", strings.Repeat("b", 40), "--artifact", "absent"}} {
		if Run(args, nil, &out, &stderr) == 0 {
			t.Fatal("invalid command accepted", args)
		}
	}
}

func FuzzArtifactPathSafety(f *testing.F) {
	for _, s := range []string{"safe.txt", "../x", ".sentinel-lab/captures/x", "foo/bar", "a\\b", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := validatePath(s)
		if err == nil {
			if filepath.IsAbs(s) || filepath.Clean(s) != s || strings.Contains(s, "\\") || strings.HasPrefix(s, "../") {
				t.Fatalf("unsafe path %q", s)
			}
		}
	})
}

func TestProvenanceAndManifestStructuralFailures(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	p := revisions()
	for _, bad := range []string{"bad", strings.Repeat("A", 40), strings.Repeat("z", 40)} {
		p.RuntimeRevision = &bad
		if _, err := Generate(root, []string{"safe.txt"}, p); err == nil {
			t.Fatal("bad runtime", bad)
		}
	}
	p = revisions()
	for _, bad := range []string{"", strings.Repeat("x", 129), "with\nnewline"} {
		p.Model = &bad
		if _, err := Generate(root, []string{"safe.txt"}, p); err == nil {
			t.Fatal("bad model")
		}
	}
	model, client, runtime := "synthetic-model", "synthetic-client", strings.Repeat("c", 40)
	p = revisions()
	p.Model = &model
	p.Client = &client
	p.RuntimeRevision = &runtime
	m, err := Generate(root, []string{"safe.txt"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(filepath.Join(root, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(filepath.Join(root, "manifest.json"), m); err != nil {
		t.Fatal("replace", err)
	}
	for _, alter := range []func(*Manifest){func(m *Manifest) { m.Provenance = Provenance{} }, func(m *Manifest) { m.Artifacts = nil }, func(m *Manifest) { m.Artifacts = append(m.Artifacts, m.Artifacts[0]) }, func(m *Manifest) { m.Artifacts[0].Bytes = MaxArtifactBytes + 1 }, func(m *Manifest) { m.Artifacts = make([]Artifact, 257) }, func(m *Manifest) {
		m.Artifacts = []Artifact{{Path: "a", SHA256: strings.Repeat("a", 64), Bytes: MaxArtifactBytes}, {Path: "b", SHA256: strings.Repeat("a", 64), Bytes: MaxArtifactBytes}, {Path: "c", SHA256: strings.Repeat("a", 64), Bytes: MaxArtifactBytes}, {Path: "d", SHA256: strings.Repeat("a", 64), Bytes: MaxArtifactBytes}, {Path: "e", SHA256: strings.Repeat("a", 64), Bytes: 1}}
	}} {
		data, _ := json.Marshal(m)
		var changed Manifest
		json.Unmarshal(data, &changed)
		alter(&changed)
		if Verify(root, changed) == nil || WriteManifest(filepath.Join(root, "bad.json"), changed) == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"create", "--root", root, "--manifest", filepath.Join(root, "manifest.json"), "--source-revision", p.SourceRevision, "--control-revision", p.ControlRevision, "--runtime-revision", runtime, "--model", model, "--client", client, "--artifact", "safe.txt"}, nil, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
}

func TestFilesystemFailureAndAggregateLimits(t *testing.T) {
	root := evidenceTempDir(t)
	writeArtifact(t, root, "safe.txt", "safe")
	for _, tc := range []struct{ root, path string }{{filepath.Join(root, "missing"), "safe.txt"}, {filepath.Join(root, "safe.txt"), "safe.txt"}, {root, "safe.txt/child"}, {root, "missing.txt"}, {root, "directory"}} {
		os.Mkdir(filepath.Join(root, "directory"), 0700)
		if _, err := Generate(tc.root, []string{tc.path}, revisions()); err == nil {
			t.Fatal("bad filesystem accepted", tc)
		}
	}
	denied := filepath.Join(root, "denied.txt")
	os.WriteFile(denied, []byte("x"), 0000)
	if _, err := Generate(root, []string{"denied.txt"}, revisions()); err == nil {
		t.Fatal("unreadable artifact accepted")
	}
	paths := []string{"a.bin", "b.bin", "c.bin", "d.bin", "e.bin"}
	for _, path := range paths {
		f, err := os.Create(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(MaxArtifactBytes); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if _, err := Generate(root, paths, revisions()); err == nil {
		t.Fatal("aggregate limit exceeded")
	}
	if _, err := ReadManifest(filepath.Join(root, "directory")); err == nil {
		t.Fatal("directory manifest accepted")
	}
	m, err := Generate(root, []string{"safe.txt"}, revisions())
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(filepath.Join(root, strings.Repeat("x", 300)), m); err == nil {
		t.Fatal("invalid target accepted")
	}
	if err := WriteManifest(filepath.Join(root, "missing-dir", "manifest.json"), m); err == nil {
		t.Fatal("missing parent accepted")
	}
}
