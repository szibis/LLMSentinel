package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	command := flag.String("cmd", "backup", "Command: backup or restore")
	dbPath := flag.String("db", "escalate.db", "Database file path")
	backupFile := flag.String("file", "", "Backup file path (auto-generated if not specified)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Database Backup and Restore Utility
Usage: db-backup [options]

Commands:
  backup   - Create a backup of the database
  restore  - Restore database from backup
  verify   - Verify backup integrity

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  db-backup -cmd backup -db escalate.db
  db-backup -cmd backup -db escalate.db -file escalate.db.2026-05-15.backup
  db-backup -cmd restore -file escalate.db.2026-05-15.backup -db escalate.db
  db-backup -cmd verify -file escalate.db.2026-05-15.backup
`)
	}

	flag.Parse()

	switch *command {
	case "backup":
		if err := backupDatabase(*dbPath, backupFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Backup failed: %v\n", err)
			os.Exit(1)
		}

	case "restore":
		if *backupFile == "" {
			fmt.Fprintf(os.Stderr, "❌ Restore requires -file flag\n")
			os.Exit(1)
		}
		if err := restoreDatabase(*backupFile, *dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Restore failed: %v\n", err)
			os.Exit(1)
		}

	case "verify":
		if *backupFile == "" {
			fmt.Fprintf(os.Stderr, "❌ Verify requires -file flag\n")
			os.Exit(1)
		}
		if err := verifyBackup(*backupFile); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Verification failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "❌ Unknown command: %s\n", *command)
		os.Exit(1)
	}
}

func backupDatabase(dbPath string, backupFile *string) error {
	// Generate backup filename if not specified
	if *backupFile == "" {
		timestamp := time.Now().Format("20060102-150405")
		*backupFile = fmt.Sprintf("%s.%s.backup", dbPath, timestamp)
	}

	// Verify source database exists
	srcInfo, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("database not found: %w", err)
	}

	// Open source
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer src.Close()

	// Create backup file
	dst, err := os.Create(*backupFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	// Copy with verification
	hash := sha256.New()
	multiWriter := io.MultiWriter(dst, hash)

	bytesWritten, err := io.Copy(multiWriter, src)
	if err != nil {
		os.Remove(*backupFile)
		return fmt.Errorf("backup copy failed: %w", err)
	}

	if bytesWritten != srcInfo.Size() {
		os.Remove(*backupFile)
		return fmt.Errorf("backup incomplete: wrote %d bytes, expected %d", bytesWritten, srcInfo.Size())
	}

	// Create checksum file
	checksumFile := *backupFile + ".sha256"
	checksumStr := fmt.Sprintf("%x  %s\n", hash.Sum(nil), filepath.Base(*backupFile))
	if err := os.WriteFile(checksumFile, []byte(checksumStr), 0644); err != nil {
		// Don't fail backup if checksum write fails
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not write checksum file: %v\n", err)
	}

	fmt.Printf("✅ Backup complete: %s\n", *backupFile)
	fmt.Printf("   Size: %.2f MB\n", float64(srcInfo.Size())/1024/1024)
	fmt.Printf("   Checksum: %x\n", hash.Sum(nil))
	fmt.Printf("   SHA256 file: %s\n", checksumFile)

	return nil
}

func restoreDatabase(backupFile string, dbPath string) error {
	// Verify backup exists
	backupInfo, err := os.Stat(backupFile)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Verify backup integrity
	if err := verifyBackup(backupFile); err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}

	// Open backup
	src, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer src.Close()

	// Create temporary file in same directory for atomicity
	dbDir := filepath.Dir(dbPath)
	tmpFile := filepath.Join(dbDir, "."+filepath.Base(dbPath)+".tmp")

	dst, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer dst.Close()

	// Copy backup to temp file
	bytesWritten, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("restore copy failed: %w", err)
	}

	if bytesWritten != backupInfo.Size() {
		os.Remove(tmpFile)
		return fmt.Errorf("restore incomplete: wrote %d bytes, expected %d", bytesWritten, backupInfo.Size())
	}

	// Close temp file before moving
	dst.Close()

	// Atomic swap: backup existing database and rename temp to target
	backupOld := dbPath + ".pre-restore.backup"
	if _, err := os.Stat(dbPath); err == nil {
		// Database exists, back it up first
		if err := os.Rename(dbPath, backupOld); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to backup existing database: %w", err)
		}
	}

	// Rename temp to target
	if err := os.Rename(tmpFile, dbPath); err != nil {
		// Try to restore the old database if swap failed
		if err2 := os.Rename(backupOld, dbPath); err2 != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not restore original database: %v\n", err2)
		}
		return fmt.Errorf("failed to swap database: %w", err)
	}

	fmt.Printf("✅ Restore complete: %s\n", dbPath)
	fmt.Printf("   Size: %.2f MB\n", float64(backupInfo.Size())/1024/1024)
	fmt.Printf("   Previous database backed up to: %s\n", backupOld)

	return nil
}

func verifyBackup(backupFile string) error {
	// Check file exists
	backupInfo, err := os.Stat(backupFile)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Check minimum size (SQLite database minimum)
	minSize := int64(4096) // One page
	if backupInfo.Size() < minSize {
		return fmt.Errorf("backup file too small: %d bytes (minimum %d)", backupInfo.Size(), minSize)
	}

	// Read and compute hash
	file, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	computed := fmt.Sprintf("%x", hash.Sum(nil))

	// Check for checksum file
	checksumFile := backupFile + ".sha256"
	checksumContent, err := os.ReadFile(checksumFile)
	if err == nil {
		// Verify against checksum file
		checksumStr := string(checksumContent)
		// Extract hash from file (format: "hash  filename")
		var storedHash string
		fmt.Sscanf(checksumStr, "%s", &storedHash)

		if storedHash == computed {
			fmt.Printf("✅ Backup verified: %s\n", backupFile)
			fmt.Printf("   SHA256: %s\n", computed)
			return nil
		} else {
			return fmt.Errorf("checksum mismatch: computed %s, expected %s", computed, storedHash)
		}
	}

	// No checksum file, just report hash
	fmt.Printf("✅ Backup file readable: %s\n", backupFile)
	fmt.Printf("   Size: %.2f MB\n", float64(backupInfo.Size())/1024/1024)
	fmt.Printf("   SHA256: %s\n", computed)
	fmt.Printf("   (Note: No checksum file found, file integrity not verified against original)\n")

	return nil
}
