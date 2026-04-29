package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphDB(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create graph db: %v", err)
	}
	defer db.Close()

	// Test creating a node
	ctx := context.Background()
	node := &Node{
		ID:         "func_authenticate",
		Name:       "authenticate",
		Type:       NodeTypeFunction,
		FilePath:   "auth.go",
		LineNumber: 42,
	}

	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	// Test retrieving node
	retrieved, err := db.GetNode(ctx, "func_authenticate")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}

	if retrieved.Name != "authenticate" {
		t.Errorf("expected name 'authenticate', got '%s'", retrieved.Name)
	}

	// Test creating edge
	edge := &Edge{
		ID:           "edge_1",
		SourceID:     "func_authenticate",
		TargetID:     "func_authenticate",
		RelationType: RelationTypeCalls,
		Weight:       1.0,
	}

	if err := db.CreateEdge(ctx, edge); err != nil {
		t.Fatalf("failed to create edge: %v", err)
	}

	// Test stats
	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats["node_count"] != 1 {
		t.Errorf("expected 1 node, got %d", stats["node_count"])
	}

	if stats["edge_count"] != 1 {
		t.Errorf("expected 1 edge, got %d", stats["edge_count"])
	}

	t.Logf("✓ Graph database test passed: %d nodes, %d edges", stats["node_count"], stats["edge_count"])
}

func TestGraphDBConcurrency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "graph_test_concurrent")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create graph db: %v", err)
	}
	defer db.Close()

	// Create multiple nodes concurrently
	ctx := context.Background()
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			node := &Node{
				ID:   string(rune('a' + id)),
				Name: string(rune('a' + id)),
				Type: NodeTypeFunction,
			}
			_ = db.CreateNode(ctx, node)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats["node_count"] < 10 {
		t.Errorf("expected at least 10 nodes, got %d", stats["node_count"])
	}

	t.Logf("✓ Concurrent test passed: %d nodes created", stats["node_count"])
}
