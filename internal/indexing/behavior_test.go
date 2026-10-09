package indexing

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIndexerFilesystemAndCancellationErrors(t *testing.T) {
	ci := NewCodeIndexer(nil)
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.go")
	if result, err := ci.IndexFile(context.Background(), missing); err == nil || result == nil || len(result.Errors) == 0 {
		t.Fatal("missing file not reported")
	}
	if err := ci.WatchFile(context.Background(), filepath.Join(dir, "missing", "x.go"), make(chan *IndexingResult)); err == nil {
		t.Fatal("missing watch directory accepted")
	}
	p := filepath.Join(dir, "source.go")
	if err := os.WriteFile(p, []byte("package p\nfunc Exported() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ci.WatchFile(ctx, p, make(chan *IndexingResult)); err != context.Canceled {
		t.Fatalf("watch cancellation=%v", err)
	}
	for _, ignored := range []string{".hidden", "node_modules", "vendor"} {
		sub := filepath.Join(dir, ignored)
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "x.go"), []byte("package p"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := ci.IndexDirectory(context.Background(), dir); err != nil || count != 1 {
		t.Fatalf("indexed count=%d err=%v", count, err)
	}
}

func FuzzParserDeterminism(f *testing.F) {
	f.Add(uint8(0), "package p\nfunc Foo() {}\nfunc Bar() {\n Foo()\n}\n")
	f.Add(uint8(1), "def foo():\n    pass\ndef bar():\n    foo()\n")
	f.Add(uint8(2), "export function foo() {}\nfunction bar() {\n foo()\n}\n")
	f.Fuzz(func(t *testing.T, language uint8, content string) {
		if len(content) > 4096 {
			content = content[:4096]
		}
		languages := []string{"go", "python", "typescript", "unsupported"}
		p := NewParser(languages[int(language)%len(languages)])
		a, err := p.Parse("fixture.source", content)
		if err != nil {
			t.Fatal(err)
		}
		b, err := p.Parse("fixture.source", content)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatal("parser output is nondeterministic")
		}
		for _, entity := range a.Entities {
			if entity == nil || entity.ID == "" || entity.FilePath != "fixture.source" || entity.LineNumber <= 0 {
				t.Fatalf("invalid entity=%+v", entity)
			}
		}
		for _, rel := range a.Relationships {
			if rel == nil || rel.SourceID == "" || rel.TargetID == "" || rel.FilePath != "fixture.source" || rel.Confidence < 0 || rel.Confidence > 1 {
				t.Fatalf("invalid relationship=%+v", rel)
			}
		}
	})
}
