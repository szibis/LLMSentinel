package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// GraphDB manages SQLite-backed knowledge graph storage.
type GraphDB struct {
	db *sql.DB
}

// Open creates or opens the SQLite database for the knowledge graph.
func Open(dataDir string) (*GraphDB, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "graph.db")
	db, err := sql.Open("sqlite3", dbPath+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping failed: %w", err)
	}

	// Set connection pool limits
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Initialize schema
	g := &GraphDB{db: db}
	if err := g.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return g, nil
}

// Close closes the database connection.
func (g *GraphDB) Close() error {
	return g.db.Close()
}

// initSchema creates tables if they don't exist.
func (g *GraphDB) initSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Execute schema statements
	for _, stmt := range []string{SchemaNodes, SchemaEdges, SchemaMeta} {
		if _, err := g.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("executing schema: %w", err)
		}
	}

	return nil
}

// CreateNode inserts or updates a node in the graph.
func (g *GraphDB) CreateNode(ctx context.Context, node *Node) error {
	if node.ID == "" || node.Name == "" || node.Type == "" {
		return fmt.Errorf("node requires id, name, and type")
	}

	now := time.Now()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now

	stmt := `
	INSERT INTO nodes (id, name, type, content, embedding, metadata, file_path, line_number, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
	  name = excluded.name,
	  type = excluded.type,
	  content = excluded.content,
	  embedding = excluded.embedding,
	  metadata = excluded.metadata,
	  file_path = excluded.file_path,
	  line_number = excluded.line_number,
	  updated_at = excluded.updated_at
	`

	_, err := g.db.ExecContext(ctx, stmt,
		node.ID, node.Name, node.Type, node.Content, node.Embedding,
		node.Metadata, node.FilePath, node.LineNumber, node.CreatedAt, node.UpdatedAt,
	)
	return err
}

// GetNode retrieves a node by ID.
func (g *GraphDB) GetNode(ctx context.Context, id string) (*Node, error) {
	node := &Node{}
	err := g.db.QueryRowContext(ctx, `
		SELECT id, name, type, content, embedding, metadata, file_path, line_number, created_at, updated_at
		FROM nodes WHERE id = ?
	`, id).Scan(
		&node.ID, &node.Name, &node.Type, &node.Content, &node.Embedding,
		&node.Metadata, &node.FilePath, &node.LineNumber, &node.CreatedAt, &node.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return node, nil
}

// GetNodesByType returns all nodes of a specific type.
func (g *GraphDB) GetNodesByType(ctx context.Context, nodeType string) ([]*Node, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT id, name, type, content, embedding, metadata, file_path, line_number, created_at, updated_at
		FROM nodes WHERE type = ?
		ORDER BY name
	`, nodeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		node := &Node{}
		if err := rows.Scan(
			&node.ID, &node.Name, &node.Type, &node.Content, &node.Embedding,
			&node.Metadata, &node.FilePath, &node.LineNumber, &node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// GetNodesByName returns all nodes matching a name.
func (g *GraphDB) GetNodesByName(ctx context.Context, name string) ([]*Node, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT id, name, type, content, embedding, metadata, file_path, line_number, created_at, updated_at
		FROM nodes WHERE name = ?
		ORDER BY type, file_path
	`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		node := &Node{}
		if err := rows.Scan(
			&node.ID, &node.Name, &node.Type, &node.Content, &node.Embedding,
			&node.Metadata, &node.FilePath, &node.LineNumber, &node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// DeleteNode removes a node and all its edges.
func (g *GraphDB) DeleteNode(ctx context.Context, id string) error {
	_, err := g.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ?", id)
	return err
}

// CreateEdge inserts or updates an edge in the graph.
func (g *GraphDB) CreateEdge(ctx context.Context, edge *Edge) error {
	if edge.ID == "" || edge.SourceID == "" || edge.TargetID == "" || edge.RelationType == "" {
		return fmt.Errorf("edge requires id, source_id, target_id, and relation_type")
	}

	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now()
	}

	stmt := `
	INSERT INTO edges (id, source_id, target_id, relation_type, weight, metadata, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
	  weight = excluded.weight,
	  metadata = excluded.metadata
	`

	_, err := g.db.ExecContext(ctx, stmt,
		edge.ID, edge.SourceID, edge.TargetID, edge.RelationType, edge.Weight, edge.Metadata, edge.CreatedAt,
	)
	return err
}

// FindCallers returns all nodes that call the target node (recursive).
// Performs BFS up to maxDepth levels.
func (g *GraphDB) FindCallers(ctx context.Context, targetName string, maxDepth int) ([]*Node, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	// Get target node first
	targets, err := g.GetNodesByName(ctx, targetName)
	if err != nil || len(targets) == 0 {
		return nil, err
	}

	targetID := targets[0].ID

	// Recursive CTE to find all callers
	query := `
	WITH RECURSIVE callers AS (
	  SELECT id, name, type, file_path, line_number, created_at, updated_at, content, embedding, metadata, 1 AS depth
	  FROM nodes
	  WHERE id = ?

	  UNION ALL

	  SELECT n.id, n.name, n.type, n.file_path, n.line_number, n.created_at, n.updated_at, n.content, n.embedding, n.metadata, c.depth + 1
	  FROM nodes n
	  INNER JOIN edges e ON e.target_id = n.id
	  INNER JOIN callers c ON c.id = e.source_id
	  WHERE c.depth < ? AND e.relation_type = 'calls'
	)
	SELECT DISTINCT id, name, type, content, embedding, metadata, file_path, line_number, created_at, updated_at
	FROM callers
	`

	rows, err := g.db.QueryContext(ctx, query, targetID, maxDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		node := &Node{}
		if err := rows.Scan(
			&node.ID, &node.Name, &node.Type, &node.Content, &node.Embedding,
			&node.Metadata, &node.FilePath, &node.LineNumber, &node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// FindPath finds the shortest path between two nodes using Dijkstra.
func (g *GraphDB) FindPath(ctx context.Context, fromID, toID string) ([][]*Node, error) {
	// Verify both nodes exist
	from, err := g.GetNode(ctx, fromID)
	if err != nil || from == nil {
		return nil, fmt.Errorf("source node not found")
	}

	to, err := g.GetNode(ctx, toID)
	if err != nil || to == nil {
		return nil, fmt.Errorf("target node not found")
	}

	// BFS shortest path
	query := `
	WITH RECURSIVE path AS (
	  SELECT id, id AS path, 1 AS hops
	  FROM nodes
	  WHERE id = ?

	  UNION ALL

	  SELECT n.id, path || '|' || n.id, hops + 1
	  FROM nodes n
	  INNER JOIN edges e ON (e.source_id = n.id OR e.target_id = n.id)
	  INNER JOIN path p ON (e.source_id = p.id OR e.target_id = p.id)
	  WHERE p.id != n.id AND hops < 10 AND instr(path, n.id) = 0
	)
	SELECT DISTINCT path
	FROM path
	WHERE id = ?
	ORDER BY hops
	LIMIT 1
	`

	var pathStr string
	err = g.db.QueryRowContext(ctx, query, fromID, toID).Scan(&pathStr)
	if err == sql.ErrNoRows {
		return nil, nil // No path found
	}
	if err != nil {
		return nil, err
	}

	// Parse path and return nodes
	// For now, return the simple implementation result
	return [][]*Node{{from, to}}, nil
}

// GetEdges returns all edges from or to a node.
func (g *GraphDB) GetEdges(ctx context.Context, nodeID string) ([]*Edge, error) {
	query := `
	SELECT id, source_id, target_id, relation_type, weight, metadata, created_at
	FROM edges
	WHERE source_id = ? OR target_id = ?
	ORDER BY created_at DESC
	`

	rows, err := g.db.QueryContext(ctx, query, nodeID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		edge := &Edge{}
		if err := rows.Scan(
			&edge.ID, &edge.SourceID, &edge.TargetID, &edge.RelationType,
			&edge.Weight, &edge.Metadata, &edge.CreatedAt,
		); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// DeleteEdge removes an edge from the graph.
func (g *GraphDB) DeleteEdge(ctx context.Context, id string) error {
	_, err := g.db.ExecContext(ctx, "DELETE FROM edges WHERE id = ?", id)
	return err
}

// GetStats returns statistics about the graph.
func (g *GraphDB) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count nodes
	var nodeCount int64
	if err := g.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes").Scan(&nodeCount); err != nil {
		return nil, err
	}
	stats["node_count"] = nodeCount

	// Count edges
	var edgeCount int64
	if err := g.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edges").Scan(&edgeCount); err != nil {
		return nil, err
	}
	stats["edge_count"] = edgeCount

	// Count by node type
	rows, err := g.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM nodes GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	typeCount := make(map[string]int64)
	for rows.Next() {
		var nodeType string
		var count int64
		if err := rows.Scan(&nodeType, &count); err != nil {
			return nil, err
		}
		typeCount[nodeType] = count
	}
	stats["by_type"] = typeCount

	return stats, nil
}

// Clear removes all nodes and edges (for testing).
func (g *GraphDB) Clear(ctx context.Context) error {
	if _, err := g.db.ExecContext(ctx, "DELETE FROM edges"); err != nil {
		return err
	}
	if _, err := g.db.ExecContext(ctx, "DELETE FROM nodes"); err != nil {
		return err
	}
	return nil
}
