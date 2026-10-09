package graph

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// GraphDB manages knowledge graph operations
type GraphDB struct {
	db *sql.DB
	mu sync.RWMutex
}

// New creates a new graph database connection with a full path
func New(dbPath string) (*GraphDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite foreign-key enforcement is per connection. A single connection
	// keeps the configured pragma and in-memory databases consistent.
	db.SetMaxOpenConns(1)
	initialized := false
	defer func() {
		if !initialized {
			db.Close()
		}
	}()

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := InitSchema(db); err != nil {
		return nil, err
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	initialized = true
	return &GraphDB{db: db}, nil
}

// Open creates a new graph database connection in a directory
func Open(dirPath string) (*GraphDB, error) {
	dbPath := filepath.Join(dirPath, "graph.db")
	return New(dbPath)
}

// Close closes the database connection
func (g *GraphDB) Close() error {
	return g.db.Close()
}

// CreateNode creates a new node in the graph
func (g *GraphDB) CreateNode(ctx context.Context, node *Node) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = now
	}

	_, err := g.db.ExecContext(ctx, `
		INSERT INTO nodes (id, name, type, content, metadata, file_path, line_number, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, node.Name, node.Type, node.Content, node.Metadata, node.FilePath, node.LineNumber, node.CreatedAt, node.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	return nil
}

// GetNode retrieves a node by ID
func (g *GraphDB) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var node Node
	var content sql.NullString
	var filePath sql.NullString
	var lineNum sql.NullInt64

	err := g.db.QueryRowContext(ctx, `
		SELECT id, name, type, content, metadata, file_path, line_number, created_at, updated_at
		FROM nodes WHERE id = ?
	`, nodeID).Scan(&node.ID, &node.Name, &node.Type, &content, &node.Metadata, &filePath, &lineNum, &node.CreatedAt, &node.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("node not found")
		}
		return nil, err
	}

	node.Content = content.String
	node.FilePath = filePath.String
	if lineNum.Valid {
		node.LineNumber = int(lineNum.Int64)
	}

	return &node, nil
}

// GetNodesByType retrieves all nodes of a specific type
func (g *GraphDB) GetNodesByType(ctx context.Context, nodeType string) ([]*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	rows, err := g.db.QueryContext(ctx, `
		SELECT id, name, type, content, metadata, file_path, line_number, created_at, updated_at
		FROM nodes WHERE type = ?
		ORDER BY name LIMIT 1000
	`, nodeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		var content sql.NullString
		var filePath sql.NullString
		var lineNum sql.NullInt64

		if err := rows.Scan(&node.ID, &node.Name, &node.Type, &content, &node.Metadata, &filePath, &lineNum, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}

		node.Content = content.String
		node.FilePath = filePath.String
		if lineNum.Valid {
			node.LineNumber = int(lineNum.Int64)
		}
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// FindCallers finds all functions that call a given function
func (g *GraphDB) FindCallers(ctx context.Context, targetName string) ([]*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	rows, err := g.db.QueryContext(ctx, `
		SELECT n.id, n.name, n.type, n.content, n.metadata, n.file_path, n.line_number, n.created_at, n.updated_at
		FROM nodes n
		WHERE n.id IN (
			SELECT source_id FROM edges
			WHERE target_id IN (SELECT id FROM nodes WHERE name = ?)
			AND relation_type = 'calls'
		)
		ORDER BY n.name LIMIT 100
	`, targetName)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		var content sql.NullString
		var filePath sql.NullString
		var lineNum sql.NullInt64

		if err := rows.Scan(&node.ID, &node.Name, &node.Type, &content, &node.Metadata, &filePath, &lineNum, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}

		node.Content = content.String
		node.FilePath = filePath.String
		if lineNum.Valid {
			node.LineNumber = int(lineNum.Int64)
		}
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// CreateEdge creates a relationship between two nodes
func (g *GraphDB) CreateEdge(ctx context.Context, edge *Edge) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now()
	}

	_, err := g.db.ExecContext(ctx, `
		INSERT INTO edges (id, source_id, target_id, relation_type, weight, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, edge.ID, edge.SourceID, edge.TargetID, edge.RelationType, edge.Weight, edge.Metadata, edge.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create edge: %w", err)
	}
	return nil
}

// GetRelated retrieves nodes related to a given node
func (g *GraphDB) GetRelated(ctx context.Context, nodeID string, relationType string) ([]*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	rows, err := g.db.QueryContext(ctx, `
		SELECT n.id, n.name, n.type, n.content, n.metadata, n.file_path, n.line_number, n.created_at, n.updated_at
		FROM nodes n
		JOIN edges e ON n.id = e.target_id
		WHERE e.source_id = ? AND e.relation_type = ?
		ORDER BY n.name LIMIT 100
	`, nodeID, relationType)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var node Node
		var content sql.NullString
		var filePath sql.NullString
		var lineNum sql.NullInt64

		if err := rows.Scan(&node.ID, &node.Name, &node.Type, &content, &node.Metadata, &filePath, &lineNum, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}

		node.Content = content.String
		node.FilePath = filePath.String
		if lineNum.Valid {
			node.LineNumber = int(lineNum.Int64)
		}
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// Stats returns database statistics
func (g *GraphDB) Stats() (map[string]int64, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[string]int64)

	var nodeCount int64
	if err := g.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&nodeCount); err != nil {
		return nil, fmt.Errorf("failed to count nodes: %w", err)
	}
	stats["node_count"] = nodeCount

	var edgeCount int64
	if err := g.db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&edgeCount); err != nil {
		return nil, fmt.Errorf("failed to count edges: %w", err)
	}
	stats["edge_count"] = edgeCount

	return stats, nil
}

// GetStats returns database statistics with context support
func (g *GraphDB) GetStats(ctx context.Context) (map[string]int64, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return g.Stats()
}

// Vacuum optimizes the database
func (g *GraphDB) Vacuum() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.db.Exec("VACUUM")
	return err
}

// Clear removes all nodes and edges from the database
func (g *GraphDB) Clear(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, err := g.db.ExecContext(ctx, "DELETE FROM edges"); err != nil {
		return fmt.Errorf("failed to clear edges: %w", err)
	}

	if _, err := g.db.ExecContext(ctx, "DELETE FROM nodes"); err != nil {
		return fmt.Errorf("failed to clear nodes: %w", err)
	}

	return nil
}
