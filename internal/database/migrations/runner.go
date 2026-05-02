package migrations

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a single database migration
type Migration struct {
	Version     string // e.g., "1.0.0", "1.0.1", "1.1.0"
	Description string
	UpSQL       string   // SQL to apply migration
	DownSQL     string   // SQL to rollback migration
	Checksum    string   // SHA256 of UpSQL for integrity
}

// Runner executes migrations in sequence
type Runner struct {
	db *sql.DB
}

// NewRunner creates a new migration runner
func NewRunner(db *sql.DB) *Runner {
	return &Runner{db: db}
}

// Current returns the current schema version
func (r *Runner) Current() (string, error) {
	var version string
	err := r.db.QueryRow(`
		SELECT version FROM schema_version WHERE id = 1
	`).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return "0.0.0", nil // No migrations applied yet
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}
	return version, nil
}

// Apply executes pending migrations up to targetVersion
func (r *Runner) Apply(targetVersion string) error {
	migrations := r.getMigrations()
	current, err := r.Current()
	if err != nil {
		return err
	}

	log.Printf("Current schema version: %s, target: %s", current, targetVersion)

	for _, m := range migrations {
		if compareVersions(m.Version, current) <= 0 {
			continue // Already applied
		}
		if compareVersions(m.Version, targetVersion) > 0 {
			break // Don't apply beyond target
		}

		log.Printf("Applying migration: %s (%s)", m.Version, m.Description)
		if err := r.executeMigration(&m); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.Version, err)
		}
	}

	return nil
}

// Rollback reverts to a specific version
func (r *Runner) Rollback(targetVersion string) error {
	migrations := r.getMigrations()
	current, err := r.Current()
	if err != nil {
		return err
	}

	// Reverse order: newest first
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if compareVersions(m.Version, current) > 0 {
			continue // Not applied yet
		}
		if compareVersions(m.Version, targetVersion) <= 0 {
			break // Don't rollback beyond target
		}

		log.Printf("Rolling back migration: %s", m.Version)
		if err := r.rollbackMigration(&m); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", m.Version, err)
		}
	}

	return nil
}

// executeMigration applies a single migration in a transaction
func (r *Runner) executeMigration(m *Migration) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.Exec(m.UpSQL); err != nil {
		return fmt.Errorf("migration SQL failed: %w", err)
	}

	// Update version
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO schema_version (id, version, description)
		VALUES (1, ?, ?)
	`, m.Version, m.Description); err != nil {
		return fmt.Errorf("failed to update schema version: %w", err)
	}

	return tx.Commit()
}

// rollbackMigration reverts a single migration
func (r *Runner) rollbackMigration(m *Migration) error {
	if m.DownSQL == "" {
		return fmt.Errorf("migration %s is not reversible", m.Version)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.DownSQL); err != nil {
		return fmt.Errorf("rollback SQL failed: %w", err)
	}

	// Calculate previous version
	migrations := r.getMigrations()
	prevVersion := "0.0.0"
	for i, migration := range migrations {
		if migration.Version == m.Version && i > 0 {
			prevVersion = migrations[i-1].Version
			break
		}
	}

	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO schema_version (id, version)
		VALUES (1, ?)
	`, prevVersion); err != nil {
		return fmt.Errorf("failed to update schema version: %w", err)
	}

	return tx.Commit()
}

// getMigrations returns all defined migrations in order
func (r *Runner) getMigrations() []Migration {
	return []Migration{
		// v1.0.0: Initial schema (baseline - no migration needed)
		// All tables already created by schema initialization
		{
			Version:     "1.0.0",
			Description: "Initial schema for v1.0.0 release",
			UpSQL: `
				-- Schema already initialized, this is baseline
				SELECT 1;
			`,
			DownSQL: "-- Cannot downgrade from v1.0.0",
		},
		// Future migrations would be added here:
		// {
		//     Version:     "1.1.0",
		//     Description: "Add optional column to nodes",
		//     UpSQL: "ALTER TABLE nodes ADD COLUMN new_field TEXT;",
		//     DownSQL: "ALTER TABLE nodes DROP COLUMN new_field;",
		// },
	}
}

// compareVersions compares semantic versions
// Returns: < 0 if a < b, 0 if a == b, > 0 if a > b
func compareVersions(a, b string) int {
	// Parse versions (e.g., "1.0.0" -> [1, 0, 0])
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseVersion parses semantic version into [major, minor, patch]
func parseVersion(v string) [3]int {
	var major, minor, patch int
	fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch)
	return [3]int{major, minor, patch}
}
