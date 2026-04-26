package graph

const (
	// Schema version for migrations
	SchemaVersion = 1

	// SQL schema for nodes table
	SchemaNodes = `
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  content TEXT,
  embedding BLOB,
  metadata JSON,
  file_path TEXT,
  line_number INTEGER,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);
`

	// SQL schema for edges table
	SchemaEdges = `
CREATE TABLE IF NOT EXISTS edges (
  id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  relation_type TEXT NOT NULL,
  weight REAL DEFAULT 1.0,
  metadata JSON,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_relation ON edges(relation_type);
`

	// Metadata table for schema versioning
	SchemaMeta = `
CREATE TABLE IF NOT EXISTS schema_meta (
  version INTEGER PRIMARY KEY,
  applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
)

// InitSchema creates all required tables if they don't exist.
func InitSchema(execFunc func(string) error) error {
	// Create metadata table first
	if err := execFunc(SchemaMeta); err != nil {
		return err
	}

	// Create nodes and edges tables
	if err := execFunc(SchemaNodes); err != nil {
		return err
	}

	if err := execFunc(SchemaEdges); err != nil {
		return err
	}

	// Record schema version
	if err := execFunc(`INSERT OR IGNORE INTO schema_meta (version) VALUES (?)` ); err != nil {
		return err
	}

	return nil
}
