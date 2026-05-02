-- LLMSentinel Database Schema v1.0
-- Final schema for v1.0.0 release - NO BREAKING CHANGES ALLOWED after release
-- Date: 2026-04-30
-- This schema is immutable for production compatibility

-- Schema version tracking (required for migrations)
CREATE TABLE IF NOT EXISTS schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version TEXT NOT NULL DEFAULT '1.0.0',
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

-- ============================================================================
-- SQLite Tables (Knowledge Graph)
-- ============================================================================

-- Nodes: Entities in the knowledge graph (functions, classes, modules, etc)
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,  -- 'function', 'class', 'module', 'variable', etc
    content TEXT,  -- Full code/documentation content
    embedding BLOB,  -- Semantic embedding vector (if applicable)
    metadata TEXT,  -- JSON metadata: scope, visibility, decorators, etc
    file_path TEXT NOT NULL,
    line_number INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT file_line_unique UNIQUE (file_path, line_number)
);

CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_created ON nodes(created_at);

-- Edges: Relationships between nodes (calls, inherits, imports, etc)
CREATE TABLE IF NOT EXISTS edges (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL,  -- 'calls', 'inherits', 'imports', 'references'
    weight REAL DEFAULT 1.0,  -- Strength of relationship
    metadata TEXT,  -- JSON: frequency, context, conditions, etc
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT source_target_unique UNIQUE (source_id, target_id, relation_type)
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(relation_type);
CREATE INDEX IF NOT EXISTS idx_edges_created ON edges(created_at);

-- Node Embeddings: Semantic vector representations
CREATE TABLE IF NOT EXISTS node_embeddings (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    embedding BLOB NOT NULL,  -- Binary-encoded embedding vector
    dimension INTEGER NOT NULL DEFAULT 384,  -- Embedding dimensions (e.g., 384 for sentence-transformers)
    model TEXT NOT NULL DEFAULT 'sentence-transformers/all-MiniLM-L6-v2',  -- Model used
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================================
-- BoltDB Buckets (Escalation & Validation History)
-- ============================================================================

-- Bucket: escalations
-- Purpose: Store model escalation/de-escalation events
-- Structure: JSON serialized EscalationEvent objects
-- Key format: <auto-increment-id>
-- Schema fields:
--   - id: int64 (auto-increment sequence)
--   - timestamp: time.Time
--   - from_model: string (source model: 'haiku', 'sonnet', 'opus')
--   - to_model: string (destination model)
--   - task_type: string (task classification)
--   - reason: string (why escalation occurred)

-- Bucket: turns
-- Purpose: Track conversation turns for circular reasoning detection
-- Structure: JSON serialized Turn objects
-- Key format: <auto-increment-id>
-- Schema fields:
--   - timestamp: time.Time
--   - model: string (model handling this turn)
--   - concepts: string (extracted concepts/focus)

-- Bucket: sessions
-- Purpose: Store session metadata and state
-- Structure: JSON serialized session objects
-- Key format: <session-id>
-- Schema fields: (varies by session type, minimal required)
--   - session_id: string (unique identifier)
--   - created_at: time.Time
--   - model_stack: array (escalation history)

-- Bucket: validation_metrics
-- Purpose: Store estimated vs actual token usage for model routing validation
-- Structure: JSON serialized ValidationMetric objects
-- Key format: <auto-increment-id>
-- Schema fields:
--   - id: int64 (auto-increment)
--   - timestamp: time.Time
--   - prompt: string (user input)
--   - detected_task_type: string
--   - detected_effort: string ('low', 'medium', 'high')
--   - routed_model: string
--   - estimated_input_tokens: int
--   - estimated_output_tokens: int
--   - estimated_total_tokens: int
--   - estimated_cost: float64
--   - actual_input_tokens: int
--   - actual_output_tokens: int
--   - actual_total_tokens: int
--   - actual_cost: float64
--   - token_error: float64 (percentage)
--   - cost_error: float64 (percentage)
--   - validated: bool

-- ============================================================================
-- Configuration & Constraints
-- ============================================================================

-- Foreign key constraints enabled
PRAGMA foreign_keys = ON;

-- Write-Ahead Logging for better concurrency and crash recovery
PRAGMA journal_mode = WAL;

-- Synchronous write mode: NORMAL balances safety vs performance
PRAGMA synchronous = NORMAL;

-- Cache size: 10000 pages (~40MB for 4KB pages)
PRAGMA cache_size = 10000;

-- Automatic vacuum to reclaim space
PRAGMA auto_vacuum = INCREMENTAL;
PRAGMA incremental_vacuum(1000);

-- ============================================================================
-- Version History
-- ============================================================================

-- v1.0.0 (2026-04-30)
--   - Initial schema for production release
--   - SQLite tables: nodes, edges, node_embeddings
--   - BoltDB buckets: escalations, turns, sessions, validation_metrics
--   - Foreign key constraints enabled
--   - WAL mode enabled for crash recovery
--   - All indices defined for query performance
--   - IMMUTABLE: No breaking schema changes permitted after v1.0.0

-- ============================================================================
-- Backward Compatibility
-- ============================================================================

-- This schema is designed to support future non-breaking migrations:
-- ✅ Adding new columns (with defaults)
-- ✅ Adding new indices
-- ✅ Adding new tables
-- ❌ Removing columns
-- ❌ Removing tables
-- ❌ Changing column types
-- ❌ Removing indices (may affect performance)

-- All migrations for v1.0.0+ MUST maintain forward compatibility
-- with this schema version to ensure production stability.
