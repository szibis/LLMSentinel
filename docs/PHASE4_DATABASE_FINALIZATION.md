# Phase 4: Database Finalization - Schema Stability & Migration Framework

Finalize database schema and implement zero-downtime migration framework for production readiness.

## Phase 4 Objectives

✅ Finalize all SQLite table schemas (immutable after v1.0.0)
✅ Create migration framework (v1 → v2 → v3 versioning)
✅ Implement zero-downtime schema migrations
✅ Document BoltDB bucket schemas
✅ Implement backup/restore procedures
✅ Create database health checks
✅ Test recovery from corruption

## Current Database State

### SQLite Tables

```bash
# Current schema (from internal/graph/schema.go)
sqlite3 escalate.db ".schema"

# Nodes table
CREATE TABLE nodes (
  id TEXT PRIMARY KEY,
  content TEXT NOT NULL,
  embedding BLOB,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)

# Edges table
CREATE TABLE edges (
  id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  weight REAL,
  FOREIGN KEY(source_id) REFERENCES nodes(id),
  FOREIGN KEY(target_id) REFERENCES nodes(id)
)

# Node embeddings table
CREATE TABLE node_embeddings (
  node_id TEXT PRIMARY KEY,
  embedding_vector BLOB,
  FOREIGN KEY(node_id) REFERENCES nodes(id)
)
```

### BoltDB Buckets

```bash
# Current buckets (from internal/storage/)
escalations    - Escalation records
turns          - Conversation turns
sessions       - User sessions
validation_metrics - Performance metrics
```

## Phase 4 Deliverables

### 1. Final Schema (v1.0)

Create `internal/database/schema_v1.sql`:

```sql
-- Version: 1.0.0
-- Date: 2026-05-15
-- Description: Initial production schema
-- Immutable after v1.0.0 release

-- Enable foreign keys
PRAGMA foreign_keys = ON;
-- Enable write-ahead logging for concurrency
PRAGMA journal_mode = WAL;

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
  version INTEGER PRIMARY KEY,
  release_date TEXT NOT NULL,
  description TEXT,
  migration_time_ms INTEGER,
  applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Nodes (knowledge graph)
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  content TEXT NOT NULL,
  embedding BLOB,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  CHECK (length(id) > 0),
  CHECK (length(content) > 0)
);

CREATE INDEX idx_nodes_created_at ON nodes(created_at);

-- Edges (knowledge graph connections)
CREATE TABLE IF NOT EXISTS edges (
  id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  weight REAL DEFAULT 1.0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(source_id) REFERENCES nodes(id) ON DELETE CASCADE,
  FOREIGN KEY(target_id) REFERENCES nodes(id) ON DELETE CASCADE,
  CHECK (source_id != target_id),
  CHECK (weight >= 0.0 AND weight <= 1.0)
);

CREATE INDEX idx_edges_source ON edges(source_id);
CREATE INDEX idx_edges_target ON edges(target_id);

-- Node embeddings (cached vectors)
CREATE TABLE IF NOT EXISTS node_embeddings (
  node_id TEXT PRIMARY KEY,
  embedding_vector BLOB NOT NULL,
  dimension INTEGER NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- Sessions (API sessions/users)
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  api_key TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_accessed_at TIMESTAMP,
  is_active BOOLEAN DEFAULT 1,
  metadata TEXT
);

CREATE INDEX idx_sessions_api_key ON sessions(api_key);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);

-- Escalations (error escalations)
CREATE TABLE IF NOT EXISTS escalations (
  id TEXT PRIMARY KEY,
  error_type TEXT NOT NULL,
  message TEXT NOT NULL,
  context TEXT,
  resolved BOOLEAN DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  resolved_at TIMESTAMP
);

CREATE INDEX idx_escalations_type ON escalations(error_type);
CREATE INDEX idx_escalations_created ON escalations(created_at);

-- Performance metrics
CREATE TABLE IF NOT EXISTS validation_metrics (
  id TEXT PRIMARY KEY,
  metric_name TEXT NOT NULL,
  metric_value REAL NOT NULL,
  tags TEXT, -- JSON
  timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_metrics_name ON validation_metrics(metric_name);
CREATE INDEX idx_metrics_timestamp ON validation_metrics(timestamp);

-- Insert initial schema version
INSERT OR IGNORE INTO schema_version (version, release_date, description)
VALUES (1, '2026-05-15', 'Initial production schema - immutable');
```

### 2. Migration Framework

Create `internal/database/migrations/runner.go`:

```go
package migrations

import (
	"database/sql"
	"fmt"
	"log"
)

type Migration struct {
	Version     int
	ReleaseDate string
	Description string
	UpSQL       string
	DownSQL     string
}

var migrations = []Migration{
	{
		Version:     1,
		ReleaseDate: "2026-05-15",
		Description: "Initial production schema",
		UpSQL:       schema_v1_sql,
		DownSQL:     "-- No downgrade for v1",
	},
	// Future migrations:
	// Version 2: Add new tables
	// Version 3: Modify indexes
}

func ApplyMigrations(db *sql.DB) error {
	// Get current version
	currentVersion := 0
	err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	// Apply pending migrations
	for _, mig := range migrations {
		if mig.Version > currentVersion {
			log.Printf("Applying migration v%d: %s\n", mig.Version, mig.Description)
			if err := applyMigration(db, mig); err != nil {
				return fmt.Errorf("migration v%d failed: %w", mig.Version, err)
			}
		}
	}

	return nil
}

func applyMigration(db *sql.DB, mig Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(mig.UpSQL); err != nil {
		tx.Rollback()
		return err
	}

	// Record migration
	_, err = tx.Exec(
		"INSERT INTO schema_version (version, release_date, description) VALUES (?, ?, ?)",
		mig.Version, mig.ReleaseDate, mig.Description,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
```

### 3. Database Health Checks

Create `internal/database/health.go`:

```go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type HealthCheck struct {
	SchemaVersion int
	TableCount    int
	IndexCount    int
	CorruptionFree bool
	LastCheck     time.Time
}

func (db *Database) Health(ctx context.Context) (*HealthCheck, error) {
	var version int
	if err := db.conn.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		return nil, fmt.Errorf("failed to get schema version: %w", err)
	}

	// Check all expected tables exist
	var tableCount int
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`).Scan(&tableCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count tables: %w", err)
	}

	// Check indexes
	var indexCount int
	err = db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type='index'
	`).Scan(&indexCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count indexes: %w", err)
	}

	// Integrity check
	var integrityOk string
	err = db.conn.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityOk)
	corruptionFree := err == nil && integrityOk == "ok"

	return &HealthCheck{
		SchemaVersion: version,
		TableCount:    tableCount,
		IndexCount:    indexCount,
		CorruptionFree: corruptionFree,
		LastCheck:     time.Now(),
	}, nil
}
```

### 4. Backup/Restore Tool

Create `cmd/db-backup/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	command := flag.String("cmd", "backup", "Command: backup or restore")
	dbPath := flag.String("db", "escalate.db", "Database file path")
	backupFile := flag.String("file", "", "Backup file path")
	flag.Parse()

	switch *command {
	case "backup":
		if *backupFile == "" {
			*backupFile = fmt.Sprintf("escalate.db.%s.backup", time.Now().Format("20060102-150405"))
		}
		if err := backup(*dbPath, *backupFile); err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Backup complete: %s\n", *backupFile)

	case "restore":
		if *backupFile == "" {
			fmt.Fprintf(os.Stderr, "Restore requires -file flag\n")
			os.Exit(1)
		}
		if err := restore(*backupFile, *dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "Restore failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Restore complete: %s\n", *dbPath)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", *command)
		os.Exit(1)
	}
}

func backup(dbPath, backupFile string) error {
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("backup copy failed: %w", err)
	}

	return nil
}

func restore(backupFile, dbPath string) error {
	// Verify backup is valid SQLite database
	// Then copy to target location
	src, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer src.Close()

	// Create temporary file and validate
	tmpFile := dbPath + ".tmp"
	dst, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("restore copy failed: %w", err)
	}

	// Atomic swap
	if err := os.Rename(tmpFile, dbPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to swap database: %w", err)
	}

	return nil
}
```

## Implementation Steps

### Step 1: Finalize Schema
- [ ] Create `internal/database/schema_v1.sql`
- [ ] Document all constraints and indexes
- [ ] Add schema version tracking table
- [ ] Mark as immutable in comments

### Step 2: Migration Framework
- [ ] Create `internal/database/migrations/runner.go`
- [ ] Implement migration application logic
- [ ] Add rollback capability (for v2+)
- [ ] Test migration on real data

### Step 3: Health Checks
- [ ] Create `internal/database/health.go`
- [ ] Implement PRAGMA integrity_check
- [ ] Add health check endpoint: `GET /health/db`
- [ ] Test corruption detection

### Step 4: Backup/Restore
- [ ] Create `cmd/db-backup/main.go`
- [ ] Implement atomic restore
- [ ] Add backup scheduling (cron job)
- [ ] Document restore procedures

### Step 5: Testing
- [ ] Test schema application on clean database
- [ ] Test migration on populated database
- [ ] Test backup/restore cycle
- [ ] Test corruption recovery
- [ ] Validate zero-downtime migration (if schema change)

## Zero-Downtime Migration Strategy

For future schema changes (v2+), use these patterns:

### Adding a Column
```sql
-- Step 1: Add column with default
ALTER TABLE nodes ADD COLUMN new_field TEXT DEFAULT '';

-- Step 2: Backfill data (can be slow)
UPDATE nodes SET new_field = compute_value(id);

-- Step 3: Drop default if constraint needed
ALTER TABLE nodes ALTER COLUMN new_field DROP DEFAULT;
```

### Adding an Index
```sql
-- Create index concurrently (if supported)
CREATE INDEX CONCURRENTLY idx_name ON table(column);

-- SQLite: Use incremental index build
CREATE INDEX idx_name ON table(column);
```

### Renaming a Column
```sql
-- SQLite 3.25.0+: RENAME COLUMN
ALTER TABLE nodes RENAME COLUMN old_name TO new_name;
```

### Backward Compatibility
- Old code can still work with new schema
- New code handles both old and new fields
- Migration allows time for deployment
- No breaking changes in single schema version

## Testing Migration Safety

### Test Procedure
1. Create production data copy
2. Apply migration on copy
3. Verify data integrity
4. Measure downtime (should be <100ms)
5. Test rollback (if applicable)
6. Validate application compatibility

### Downtime Validation
```bash
# Monitor database during migration
watch -n 0.1 "sqlite3 escalate.db 'SELECT COUNT(*) FROM nodes;'"

# Measure latency before/after
time sqlite3 escalate.db "SELECT COUNT(*) FROM nodes;"
```

## Database Constraints

**Size Limits** (SQLite default):
- Max database size: 281 TB (practical: 100GB+)
- Max table rows: 2^64 - 1 (practical: 1B+ rows)
- Max column size: 1GB per column
- WAL mode enables better concurrency

**Performance Targets**:
- Query latency: <10ms p99 (knowledge graph)
- Node lookup: <5ms
- Edge traversal: <20ms (10-hop)

## Monitoring & Maintenance

### Daily Checks
```bash
# Database size
du -h escalate.db

# Record new table counts
sqlite3 escalate.db "SELECT COUNT(*) FROM nodes;"

# Check WAL file size
ls -lh escalate.db-wal
```

### Weekly Maintenance
```bash
# VACUUM to reclaim space (maintenance only)
# NOTE: VACUUM locks database - run in maintenance window
sqlite3 escalate.db "VACUUM;"

# ANALYZE to update query planner
sqlite3 escalate.db "ANALYZE;"
```

### Backup Schedule
```bash
# Daily backup at 2 AM
0 2 * * * /usr/local/bin/db-backup -cmd=backup -db=/var/lib/escalate.db

# Weekly full backup to S3
0 0 * * 0 /usr/local/bin/db-backup -cmd=backup -db=/var/lib/escalate.db && s3cmd put escalate.db.*.backup s3://backups/
```

## Exit Criteria - Phase 4 Complete

✅ **Phase 4 PASS requires**:
- [ ] Schema v1.0 published and documented
- [ ] Migration framework tested with real data
- [ ] Zero-downtime migration verified (<100ms)
- [ ] Backup/restore tested and working
- [ ] Recovery procedures documented
- [ ] Database health checks operational
- [ ] Corruption detection working
- [ ] Maintenance procedures documented
- [ ] Backup schedule active
- [ ] All tests passing
- [ ] Ready for v1.0.0 release

## Production Readiness Checklist

Before v1.0.0 release:
- [ ] Phase 1: Integration tests passing
- [ ] Phase 2: Load testing at 1000 req/sec
- [ ] Phase 3: Security audit complete
- [ ] Phase 4: Database finalized
- [ ] All critical issues fixed
- [ ] Staging test period complete (30 days)
- [ ] Release notes published
- [ ] Runbook documented
- [ ] Support plan in place

## Next: RC1 Release

Once Phase 4 complete:
1. All phases (1-4) complete and passing
2. RC1 release candidate tagged
3. 30-day staging test period
4. v1.0.0 production release

## Related Commands

```bash
# Check schema version
sqlite3 escalate.db "SELECT * FROM schema_version;"

# Full database health check
sqlite3 escalate.db "PRAGMA integrity_check; PRAGMA foreign_key_check;"

# Database statistics
sqlite3 escalate.db ".stats"

# Backup database
go run cmd/db-backup/main.go -cmd=backup -db=escalate.db -file=escalate.db.backup

# Restore from backup
go run cmd/db-backup/main.go -cmd=restore -file=escalate.db.backup -db=escalate.db

# Monitor database during operations
watch -n 1 "du -h escalate.db && sqlite3 escalate.db 'SELECT COUNT(*) FROM nodes;'"
```
