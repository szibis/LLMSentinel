// Package evidence seals explicitly selected synthetic CI artifacts with hashes.
// Its path policy excludes known private locations; it cannot detect arbitrary
// secrets in file contents. Callers must supply synthetic, non-sensitive inputs.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/szibis/claude-escalate/internal/taskquality"
)

const (
	MaxArtifacts           = 256
	MaxArtifactBytes int64 = 16 << 20
	MaxTotalBytes    int64 = 64 << 20
	MaxManifestBytes       = 256 << 10
)

type Provenance struct {
	SourceRevision  string  `json:"source_revision"`
	ControlRevision string  `json:"control_revision"`
	RuntimeRevision *string `json:"runtime_revision"`
	Model           *string `json:"model"`
	Client          *string `json:"client"`
}
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type Manifest struct {
	Version    int        `json:"version"`
	Scope      string     `json:"scope"`
	Provenance Provenance `json:"provenance"`
	Artifacts  []Artifact `json:"artifacts"`
}

func validatePath(path string) error {
	if path == "" || path == "." || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\\\x00\r\n") || path == ".." || strings.HasPrefix(path, "../") {
		return fmt.Errorf("artifact path must be a clean relative path: %q", path)
	}
	for _, part := range strings.Split(strings.ToLower(path), "/") {
		if part == ".sentinel-lab" || part == ".git" || strings.HasPrefix(part, ".env") || strings.Contains(part, "secret") || strings.Contains(part, "credential") || part == "captures" || part == "capture" || part == "datasets" || part == "dataset" || part == "checkpoints" || part == "checkpoint" || part == "private" || part == ".ssh" || part == ".aws" || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") {
			return fmt.Errorf("private or sensitive artifact path excluded: %q", path)
		}
	}
	return nil
}

func validRevision(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

func validateProvenance(p Provenance) error {
	if !validRevision(p.SourceRevision) || !validRevision(p.ControlRevision) {
		return fmt.Errorf("source and control revisions must be full lowercase Git hashes")
	}
	if p.RuntimeRevision != nil && !validRevision(*p.RuntimeRevision) {
		return fmt.Errorf("runtime revision must be a full Git hash or null")
	}
	for _, s := range []*string{p.Model, p.Client} {
		if s != nil && (len(*s) == 0 || len(*s) > 128 || strings.ContainsAny(*s, "\x00\r\n")) {
			return fmt.Errorf("model and client identifiers must be bounded nonempty strings or null")
		}
	}
	return nil
}

// checkedDirectory checks each existing ancestor, not just the final directory.
func checkedDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(absolute, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("evidence directory ancestors must be directories without symlinks")
		}
	}
	return absolute, nil
}

// checkedFile rejects symlinks at all root ancestors and artifact components.
func checkedFile(root, path string) (*os.File, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	var err error
	root, err = checkedDirectory(root)
	if err != nil {
		return nil, err
	}
	// Validate root components too: changing --root must not bypass the
	// exclusion of private lab captures or credentials directories.
	rootPolicyPath := filepath.ToSlash(filepath.Clean(root))
	// /private is macOS's system prefix for temporary directories.
	rootPolicyPath = strings.TrimPrefix(rootPolicyPath, "/private/")
	if err := validatePath(strings.TrimLeft(rootPolicyPath, "/")); err != nil {
		return nil, err
	}
	st, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("artifact root must be a directory without symlinks")
	}
	current := root
	parts := strings.Split(path, "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		st, err = os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink artifact path excluded: %s", path)
		}
		if i < len(parts)-1 && !st.IsDir() {
			return nil, fmt.Errorf("artifact parent is not a directory")
		}
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact must be a regular file: %s", path)
	}
	if st.Size() > MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds size limit: %s", path)
	}
	f, err := os.Open(current)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !os.SameFile(st, opened) {
		f.Close()
		return nil, fmt.Errorf("artifact changed while opening: %s", path)
	}
	return f, nil
}

func hashArtifact(root, path string) (Artifact, error) {
	f, err := checkedFile(root, path)
	if err != nil {
		return Artifact{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return Artifact{}, err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, MaxArtifactBytes+1))
	if err != nil {
		return Artifact{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return Artifact{}, err
	}
	final, err := os.Lstat(filepath.Join(root, path))
	if err != nil {
		return Artifact{}, err
	}
	if n > MaxArtifactBytes || n != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(after, final) || final.Mode()&os.ModeSymlink != 0 {
		return Artifact{}, fmt.Errorf("artifact changed while hashing: %s", path)
	}
	return Artifact{Path: path, Bytes: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// Generate hashes selected synthetic artifacts. Revisions are supplied full Git
// object IDs; this package neither invokes Git nor infers model/client identity.
func Generate(root string, paths []string, p Provenance) (Manifest, error) {
	m := Manifest{Version: 1, Scope: "synthetic-ci", Provenance: p}
	if err := validateProvenance(p); err != nil {
		return Manifest{}, err
	}
	if len(paths) == 0 || len(paths) > MaxArtifacts {
		return Manifest{}, fmt.Errorf("select between 1 and %d artifacts", MaxArtifacts)
	}
	ordered := append([]string(nil), paths...)
	sort.Strings(ordered)
	var total int64
	for i, path := range ordered {
		if i > 0 && path == ordered[i-1] {
			return Manifest{}, fmt.Errorf("duplicate artifact: %s", path)
		}
		a, err := hashArtifact(root, path)
		if err != nil {
			return Manifest{}, err
		}
		total += a.Bytes
		if total > MaxTotalBytes {
			return Manifest{}, fmt.Errorf("artifacts exceed total size limit")
		}
		m.Artifacts = append(m.Artifacts, a)
	}
	return m, nil
}

func validateManifest(m Manifest) error {
	if m.Version != 1 || m.Scope != "synthetic-ci" {
		return fmt.Errorf("unsupported evidence version or scope")
	}
	if err := validateProvenance(m.Provenance); err != nil {
		return err
	}
	if len(m.Artifacts) == 0 || len(m.Artifacts) > MaxArtifacts {
		return fmt.Errorf("invalid artifact count")
	}
	var total int64
	for i, a := range m.Artifacts {
		if err := validatePath(a.Path); err != nil {
			return err
		}
		if i > 0 && m.Artifacts[i-1].Path >= a.Path {
			return fmt.Errorf("artifacts must be uniquely sorted")
		}
		hash, err := hex.DecodeString(a.SHA256)
		if err != nil || len(hash) != sha256.Size || strings.ToLower(a.SHA256) != a.SHA256 {
			return fmt.Errorf("invalid artifact hash")
		}
		if a.Bytes < 0 || a.Bytes > MaxArtifactBytes {
			return fmt.Errorf("invalid artifact size")
		}
		total += a.Bytes
		if total > MaxTotalBytes {
			return fmt.Errorf("artifacts exceed total size limit")
		}
	}
	return nil
}

func Verify(root string, m Manifest) error {
	if err := validateManifest(m); err != nil {
		return err
	}
	for _, expected := range m.Artifacts {
		actual, err := hashArtifact(root, expected.Path)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("artifact integrity mismatch: %s", expected.Path)
		}
	}
	return nil
}

// WriteManifest atomically replaces a regular manifest file with mode 0600.
func WriteManifest(path string, m Manifest) error {
	if err := validateManifest(m); err != nil {
		return err
	}
	directory, err := checkedDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	path = filepath.Join(directory, filepath.Base(path))
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("manifest target must be regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxManifestBytes {
		return fmt.Errorf("manifest exceeds size limit")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".evidence-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadManifest(path string) (Manifest, error) {
	var m Manifest
	directory, err := checkedDirectory(filepath.Dir(path))
	if err != nil {
		return m, err
	}
	path = filepath.Join(directory, filepath.Base(path))
	st, err := os.Lstat(path)
	if err != nil {
		return m, err
	}
	if !st.Mode().IsRegular() || st.Size() > MaxManifestBytes {
		return m, fmt.Errorf("manifest must be a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(st, opened) {
		return m, fmt.Errorf("manifest changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxManifestBytes+1))
	if err != nil {
		return m, err
	}
	if len(data) > MaxManifestBytes {
		return m, fmt.Errorf("manifest exceeds size limit")
	}
	// Validate decoded key uniqueness and UTF-8 before schema decoding. Hashes
	// attest byte integrity against a trusted manifest, not source authenticity.
	if err := taskquality.DecodeEvidenceJSON(data, &m); err != nil {
		return m, err
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	var extra interface{}
	if err := d.Decode(&extra); err != io.EOF {
		return m, fmt.Errorf("trailing manifest data")
	}
	return m, validateManifest(m)
}
