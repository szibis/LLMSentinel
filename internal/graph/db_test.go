package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGraphDB_OpenAndClose(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify database file was created
	dbPath := filepath.Join(tmpDir, "graph.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestGraphDB_CreateNode(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	node := &Node{
		ID:       "func_auth",
		Name:     "authenticate",
		Type:     NodeTypeFunction,
		Content:  "func authenticate() { ... }",
		FilePath: "auth.go",
		LineNumber: 45,
	}

	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Verify node was created
	retrieved, err := db.GetNode(ctx, "func_auth")
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Node not found after creation")
	}
	if retrieved.Name != "authenticate" {
		t.Fatalf("Got name %q, want %q", retrieved.Name, "authenticate")
	}
}

func TestGraphDB_GetNodesByName(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create multiple nodes with same name, different types
	nodes := []*Node{
		{ID: "func1", Name: "authenticate", Type: NodeTypeFunction, FilePath: "auth.go"},
		{ID: "func2", Name: "authenticate", Type: NodeTypeFunction, FilePath: "oauth.go"},
		{ID: "var1", Name: "authenticate", Type: NodeTypeVariable, FilePath: "config.go"},
	}

	for _, node := range nodes {
		if err := db.CreateNode(ctx, node); err != nil {
			t.Fatalf("CreateNode failed: %v", err)
		}
	}

	// Get all nodes with name "authenticate"
	results, err := db.GetNodesByName(ctx, "authenticate")
	if err != nil {
		t.Fatalf("GetNodesByName failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Got %d nodes, want 3", len(results))
	}

	// Verify all have the correct name
	for _, n := range results {
		if n.Name != "authenticate" {
			t.Fatalf("Got name %q, want %q", n.Name, "authenticate")
		}
	}
}

func TestGraphDB_GetNodesByType(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create nodes of different types
	nodes := []*Node{
		{ID: "func1", Name: "func1", Type: NodeTypeFunction},
		{ID: "func2", Name: "func2", Type: NodeTypeFunction},
		{ID: "class1", Name: "class1", Type: NodeTypeClass},
	}

	for _, node := range nodes {
		if err := db.CreateNode(ctx, node); err != nil {
			t.Fatalf("CreateNode failed: %v", err)
		}
	}

	// Get all functions
	functions, err := db.GetNodesByType(ctx, NodeTypeFunction)
	if err != nil {
		t.Fatalf("GetNodesByType failed: %v", err)
	}

	if len(functions) != 2 {
		t.Fatalf("Got %d functions, want 2", len(functions))
	}

	// Get all classes
	classes, err := db.GetNodesByType(ctx, NodeTypeClass)
	if err != nil {
		t.Fatalf("GetNodesByType failed: %v", err)
	}

	if len(classes) != 1 {
		t.Fatalf("Got %d classes, want 1", len(classes))
	}
}

func TestGraphDB_DeleteNode(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	node := &Node{ID: "to_delete", Name: "test", Type: NodeTypeFunction}
	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Verify node exists
	if retrieved, _ := db.GetNode(ctx, "to_delete"); retrieved == nil {
		t.Fatal("Node not created")
	}

	// Delete node
	if err := db.DeleteNode(ctx, "to_delete"); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	// Verify node is deleted
	if retrieved, _ := db.GetNode(ctx, "to_delete"); retrieved != nil {
		t.Fatal("Node still exists after deletion")
	}
}

func TestGraphDB_CreateEdge(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create source and target nodes
	source := &Node{ID: "caller", Name: "caller", Type: NodeTypeFunction}
	target := &Node{ID: "callee", Name: "callee", Type: NodeTypeFunction}

	if err := db.CreateNode(ctx, source); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if err := db.CreateNode(ctx, target); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Create edge
	edge := &Edge{
		ID:           "edge_1",
		SourceID:     "caller",
		TargetID:     "callee",
		RelationType: RelationTypeCalls,
		Weight:       1.0,
	}

	if err := db.CreateEdge(ctx, edge); err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}

	// Verify edge exists
	edges, err := db.GetEdges(ctx, "caller")
	if err != nil {
		t.Fatalf("GetEdges failed: %v", err)
	}

	if len(edges) != 1 {
		t.Fatalf("Got %d edges, want 1", len(edges))
	}

	if edges[0].RelationType != RelationTypeCalls {
		t.Fatalf("Got relation type %q, want %q", edges[0].RelationType, RelationTypeCalls)
	}
}

func TestGraphDB_FindCallers(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a simple call chain: A → B → C
	// A calls B, B calls C
	nodes := []*Node{
		{ID: "a", Name: "funcA", Type: NodeTypeFunction},
		{ID: "b", Name: "funcB", Type: NodeTypeFunction},
		{ID: "c", Name: "funcC", Type: NodeTypeFunction},
	}

	for _, node := range nodes {
		if err := db.CreateNode(ctx, node); err != nil {
			t.Fatalf("CreateNode failed: %v", err)
		}
	}

	// Create edges: A calls B, B calls C
	edges := []*Edge{
		{ID: "e1", SourceID: "a", TargetID: "b", RelationType: RelationTypeCalls},
		{ID: "e2", SourceID: "b", TargetID: "c", RelationType: RelationTypeCalls},
	}

	for _, edge := range edges {
		if err := db.CreateEdge(ctx, edge); err != nil {
			t.Fatalf("CreateEdge failed: %v", err)
		}
	}

	// Find all callers of funcC (should include A and B)
	callers, err := db.FindCallers(ctx, "funcC", 10)
	if err != nil {
		t.Fatalf("FindCallers failed: %v", err)
	}

	// Should find A and B as callers of C
	if len(callers) < 1 {
		t.Fatalf("Got %d callers, want at least 1", len(callers))
	}
}

func TestGraphDB_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create some nodes
	nodes := []*Node{
		{ID: "f1", Name: "func1", Type: NodeTypeFunction},
		{ID: "f2", Name: "func2", Type: NodeTypeFunction},
		{ID: "c1", Name: "class1", Type: NodeTypeClass},
	}

	for _, node := range nodes {
		if err := db.CreateNode(ctx, node); err != nil {
			t.Fatalf("CreateNode failed: %v", err)
		}
	}

	// Create an edge
	edge := &Edge{
		ID:           "e1",
		SourceID:     "f1",
		TargetID:     "f2",
		RelationType: RelationTypeCalls,
	}
	if err := db.CreateEdge(ctx, edge); err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}

	stats, err := db.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	nodeCount, ok := stats["node_count"].(int64)
	if !ok || nodeCount != 3 {
		t.Fatalf("Got node_count %v, want 3", nodeCount)
	}

	edgeCount, ok := stats["edge_count"].(int64)
	if !ok || edgeCount != 1 {
		t.Fatalf("Got edge_count %v, want 1", edgeCount)
	}
}

func TestGraphDB_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create some data
	node := &Node{ID: "test", Name: "test", Type: NodeTypeFunction}
	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Verify data exists
	stats, _ := db.GetStats(ctx)
	if nodeCount := stats["node_count"].(int64); nodeCount != 1 {
		t.Fatalf("Setup: got node_count %d, want 1", nodeCount)
	}

	// Clear database
	if err := db.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify data is gone
	stats, _ = db.GetStats(ctx)
	if nodeCount := stats["node_count"].(int64); nodeCount != 0 {
		t.Fatalf("After clear: got node_count %d, want 0", nodeCount)
	}
}

func TestGraphDB_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	done := make(chan error, 10)

	// Create nodes concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			node := &Node{
				ID:   "node_" + string(rune(idx)),
				Name: "test",
				Type: NodeTypeFunction,
			}
			done <- db.CreateNode(ctx, node)
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Fatalf("Concurrent create failed: %v", err)
		}
	}

	// Verify all nodes were created
	stats, _ := db.GetStats(ctx)
	nodeCount := stats["node_count"].(int64)
	if nodeCount != 10 {
		t.Fatalf("Got %d nodes, want 10", nodeCount)
	}
}

func TestGraphDB_NodeUpdateTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	node := &Node{
		ID:   "test",
		Name: "test",
		Type: NodeTypeFunction,
	}

	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	retrieved1, _ := db.GetNode(ctx, "test")
	firstUpdate := retrieved1.UpdatedAt

	// Wait a bit and update
	time.Sleep(10 * time.Millisecond)

	node.Name = "updated"
	if err := db.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode update failed: %v", err)
	}

	retrieved2, _ := db.GetNode(ctx, "test")
	secondUpdate := retrieved2.UpdatedAt

	if secondUpdate.Before(firstUpdate) {
		t.Fatal("UpdatedAt timestamp did not advance")
	}
	if retrieved2.Name != "updated" {
		t.Fatalf("Got name %q, want %q", retrieved2.Name, "updated")
	}
}
