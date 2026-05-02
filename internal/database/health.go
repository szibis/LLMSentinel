package database

import (
	"database/sql"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// HealthCheck represents the result of a database health check
type HealthCheck struct {
	Status          string            `json:"status"`              // "healthy" or "degraded"
	SQLiteOK        bool              `json:"sqlite_ok"`
	BoltDBOK        bool              `json:"boltdb_ok"`
	SchemaVersion   string            `json:"schema_version"`
	SQLiteMessages  []string          `json:"sqlite_messages"`
	BoltDBMessages  []string          `json:"boltdb_messages"`
	LastCheckedAt   time.Time         `json:"last_checked_at"`
	FilePermissions map[string]string `json:"file_permissions"`
}

// CheckSQLiteHealth performs health checks on the SQLite database
func CheckSQLiteHealth(db *sql.DB) HealthCheck {
	check := HealthCheck{
		SQLiteOK:       true,
		Status:         "healthy",
		SQLiteMessages: []string{},
		LastCheckedAt:  time.Now(),
	}

	// Check schema version
	var version string
	err := db.QueryRow("SELECT version FROM schema_version WHERE id = 1").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			check.SQLiteMessages = append(check.SQLiteMessages, "schema_version table not initialized")
		} else {
			check.SQLiteOK = false
			check.Status = "degraded"
			check.SQLiteMessages = append(check.SQLiteMessages, fmt.Sprintf("failed to read schema version: %v", err))
		}
	} else {
		check.SchemaVersion = version
	}

	// Test read on critical tables
	tables := []string{"nodes", "edges", "node_embeddings"}
	for _, table := range tables {
		var count int
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s LIMIT 1", table)).Scan(&count)
		if err != nil {
			check.SQLiteOK = false
			check.Status = "degraded"
			check.SQLiteMessages = append(check.SQLiteMessages, fmt.Sprintf("table %s query failed: %v", table, err))
		}
	}

	// Test write capability (insert then rollback)
	tx, err := db.Begin()
	if err != nil {
		check.SQLiteOK = false
		check.Status = "degraded"
		check.SQLiteMessages = append(check.SQLiteMessages, fmt.Sprintf("cannot start transaction: %v", err))
	} else {
		// Test insert
		_, err := tx.Exec("INSERT INTO nodes (id, name, type, file_path) VALUES (?, ?, ?, ?)",
			"__health_check__", "health_check", "test", "/health/check")
		if err != nil {
			check.SQLiteOK = false
			check.Status = "degraded"
			check.SQLiteMessages = append(check.SQLiteMessages, fmt.Sprintf("write test failed: %v", err))
		}
		_ = tx.Rollback()
	}

	if check.SQLiteOK {
		check.SQLiteMessages = append(check.SQLiteMessages, "all checks passed")
	}

	return check
}

// CheckBoltDBHealth performs health checks on the BoltDB database
func CheckBoltDBHealth(db *bolt.DB) HealthCheck {
	check := HealthCheck{
		BoltDBOK:       true,
		Status:         "healthy",
		BoltDBMessages: []string{},
		LastCheckedAt:  time.Now(),
	}

	// Check required buckets
	buckets := []string{"escalations", "turns", "sessions", "validation_metrics"}
	err := db.View(func(tx *bolt.Tx) error {
		for _, bucketName := range buckets {
			b := tx.Bucket([]byte(bucketName))
			if b == nil {
				check.BoltDBOK = false
				check.Status = "degraded"
				check.BoltDBMessages = append(check.BoltDBMessages,
					fmt.Sprintf("bucket %s not found", bucketName))
			} else {
				count := b.Stats().KeyN
				if count >= 0 {
					check.BoltDBMessages = append(check.BoltDBMessages,
						fmt.Sprintf("bucket %s: %d keys", bucketName, count))
				}
			}
		}
		return nil
	})

	if err != nil {
		check.BoltDBOK = false
		check.Status = "degraded"
		check.BoltDBMessages = append(check.BoltDBMessages,
			fmt.Sprintf("failed to read buckets: %v", err))
	}

	// Test write capability
	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("validation_metrics"))
		if b == nil {
			return fmt.Errorf("validation_metrics bucket not found")
		}
		// Just check we can get a bucket sequence without inserting
		_, err := b.NextSequence()
		return err
	})

	if err != nil {
		check.BoltDBOK = false
		check.Status = "degraded"
		check.BoltDBMessages = append(check.BoltDBMessages,
			fmt.Sprintf("write test failed: %v", err))
	}

	if check.BoltDBOK {
		check.BoltDBMessages = append(check.BoltDBMessages, "all checks passed")
	}

	return check
}

// CombineHealthChecks merges SQLite and BoltDB health checks
func CombineHealthChecks(sqlite, boltdb HealthCheck) HealthCheck {
	combined := HealthCheck{
		LastCheckedAt:   time.Now(),
		SQLiteOK:        sqlite.SQLiteOK,
		BoltDBOK:        boltdb.BoltDBOK,
		SchemaVersion:   sqlite.SchemaVersion,
		SQLiteMessages:  sqlite.SQLiteMessages,
		BoltDBMessages:  boltdb.BoltDBMessages,
	}

	// Overall status is degraded if either DB is not OK
	if !combined.SQLiteOK || !combined.BoltDBOK {
		combined.Status = "degraded"
	} else {
		combined.Status = "healthy"
	}

	return combined
}
