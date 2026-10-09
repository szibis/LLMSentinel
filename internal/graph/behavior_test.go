package graph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openFixtureGraph(t *testing.T) *GraphDB {
	t.Helper()
	g, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}
func TestGraphQueriesForeignKeysAndPersistence(t *testing.T) {
	ctx := context.Background()
	g := openFixtureGraph(t)
	for _, n := range []*Node{{ID: "target", Name: "Target", Type: NodeTypeFunction, Content: "target", Metadata: "{}", FilePath: "source.go", LineNumber: 3}, {ID: "caller", Name: "Caller", Type: NodeTypeFunction, Metadata: "{}", LineNumber: 1}, {ID: "other", Name: "Other", Type: NodeTypeClass, Metadata: "{}"}} {
		if err := g.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	edge := &Edge{ID: "call", SourceID: "caller", TargetID: "target", RelationType: RelationTypeCalls, Weight: .95}
	if err := g.CreateEdge(ctx, edge); err != nil {
		t.Fatal(err)
	}
	if err := g.CreateEdge(ctx, &Edge{ID: "orphan", SourceID: "missing", TargetID: "target", RelationType: RelationTypeCalls}); err == nil {
		t.Fatal("foreign key constraint not enforced")
	}
	if err := g.CreateNode(ctx, &Node{ID: "target", Name: "Duplicate", Type: NodeTypeFunction}); err == nil {
		t.Fatal("duplicate node accepted")
	}
	if err := g.CreateEdge(ctx, edge); err == nil {
		t.Fatal("duplicate edge accepted")
	}
	node, err := g.GetNode(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if node.Content != "target" || node.LineNumber != 3 || node.CreatedAt.IsZero() || node.UpdatedAt.IsZero() {
		t.Fatalf("node=%+v", node)
	}
	nodes, err := g.GetNodesByType(ctx, NodeTypeFunction)
	if err != nil || len(nodes) != 2 || nodes[0].Name != "Caller" {
		t.Fatalf("nodes=%+v %v", nodes, err)
	}
	callers, err := g.FindCallers(ctx, "Target")
	if err != nil || len(callers) != 1 || callers[0].ID != "caller" {
		t.Fatalf("callers=%+v %v", callers, err)
	}
	related, err := g.GetRelated(ctx, "caller", RelationTypeCalls)
	if err != nil || len(related) != 1 || related[0].ID != "target" {
		t.Fatalf("related=%+v %v", related, err)
	}
	if _, err := g.GetNode(ctx, "missing"); err == nil {
		t.Fatal("missing node accepted")
	}
	stats, err := g.GetStats(ctx)
	if err != nil || stats["node_count"] != 3 || stats["edge_count"] != 1 {
		t.Fatalf("stats=%+v %v", stats, err)
	}
	if err := g.Vacuum(); err != nil {
		t.Fatal(err)
	}
	if err := g.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	stats, err = g.Stats()
	if err != nil || stats["node_count"] != 0 || stats["edge_count"] != 0 {
		t.Fatalf("cleared stats=%+v %v", stats, err)
	}
}
func TestGraphCancellationAndClosedDatabaseBoundaries(t *testing.T) {
	g := openFixtureGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.CreateNode(ctx, &Node{}); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateNode=%v", err)
	}
	if _, err := g.GetNode(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetNode=%v", err)
	}
	if _, err := g.GetNodesByType(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetNodesByType=%v", err)
	}
	if _, err := g.FindCallers(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("FindCallers=%v", err)
	}
	if err := g.CreateEdge(ctx, &Edge{}); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateEdge=%v", err)
	}
	if _, err := g.GetRelated(ctx, "x", "calls"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRelated=%v", err)
	}
	if _, err := g.GetStats(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("GetStats=%v", err)
	}
	if err := g.Clear(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Clear=%v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	ctx = context.Background()
	if err := g.CreateNode(ctx, &Node{}); err == nil {
		t.Error("closed CreateNode accepted")
	}
	if _, err := g.GetNode(ctx, "x"); err == nil {
		t.Error("closed GetNode accepted")
	}
	if _, err := g.GetNodesByType(ctx, "x"); err == nil {
		t.Error("closed GetNodesByType accepted")
	}
	if _, err := g.FindCallers(ctx, "x"); err == nil {
		t.Error("closed FindCallers accepted")
	}
	if err := g.CreateEdge(ctx, &Edge{}); err == nil {
		t.Error("closed CreateEdge accepted")
	}
	if _, err := g.GetRelated(ctx, "x", "calls"); err == nil {
		t.Error("closed GetRelated accepted")
	}
	if err := g.Clear(ctx); err == nil {
		t.Error("closed Clear accepted")
	}
	if err := g.Vacuum(); err == nil {
		t.Error("closed Vacuum accepted")
	}
	if err := InitSchema(g.db); err == nil {
		t.Error("closed schema accepted")
	}
	if _, err := g.Stats(); err == nil {
		t.Error("closed Stats silently accepted")
	}
}
func TestGraphConstructionFailures(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing", "db")); err == nil {
		t.Fatal("missing parent accepted")
	}
	path := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("invalid database accepted")
	}
}
func TestGraphNullOptionalFieldsAndCorruptRows(t *testing.T) {
	g := openFixtureGraph(t)
	ctx := context.Background()
	if _, err := g.db.Exec(`INSERT INTO nodes (id,name,type,metadata) VALUES ('nullable','Nullable','function','{}')`); err != nil {
		t.Fatal(err)
	}
	got, err := g.GetNode(ctx, "nullable")
	if err != nil || got.Content != "" || got.FilePath != "" || got.LineNumber != 0 {
		t.Fatalf("nullable node=%+v %v", got, err)
	}
	if _, err := g.db.Exec(`INSERT INTO nodes (id,name,type,metadata,created_at) VALUES ('bad','Bad','function','{}','invalid-date')`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GetNodesByType(ctx, "function"); err == nil {
		t.Error("corrupt date in type query accepted")
	}
	if err := g.CreateEdge(ctx, &Edge{ID: "bad-edge", SourceID: "nullable", TargetID: "bad", RelationType: RelationTypeCalls}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GetRelated(ctx, "nullable", RelationTypeCalls); err == nil {
		t.Error("corrupt date in related query accepted")
	}
	if _, err := g.FindCallers(ctx, "Bad"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.db.Exec(`UPDATE nodes SET created_at='invalid-date' WHERE id='nullable'`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.FindCallers(ctx, "Bad"); err == nil {
		t.Error("corrupt caller date accepted")
	}
	if _, err := g.db.Exec("DROP TABLE nodes"); err != nil {
		t.Fatal(err)
	}
	if err := g.Clear(ctx); err == nil {
		t.Error("missing nodes clear accepted")
	}
}
