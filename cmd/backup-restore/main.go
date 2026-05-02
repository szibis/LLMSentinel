// Package main provides database backup and restore CLI for LLMSentinel
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	flag.Parse()

	if len(flag.Args()) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Arg(0)
	switch command {
	case "backup":
		if len(flag.Args()) < 3 {
			fmt.Println("Usage: backup-restore backup <data-dir> <output-file>")
			os.Exit(1)
		}
		if err := backup(flag.Arg(1), flag.Arg(2)); err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
			os.Exit(1)
		}

	case "restore":
		if len(flag.Args()) < 3 {
			fmt.Println("Usage: backup-restore restore <backup-file> <data-dir>")
			os.Exit(1)
		}
		if err := restore(flag.Arg(1), flag.Arg(2)); err != nil {
			fmt.Fprintf(os.Stderr, "Restore failed: %v\n", err)
			os.Exit(1)
		}

	case "verify":
		if len(flag.Args()) < 2 {
			fmt.Println("Usage: backup-restore verify <backup-file>")
			os.Exit(1)
		}
		if err := verify(flag.Arg(1)); err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

// backup creates a compressed backup of the data directory
func backup(dataDir, outputFile string) error {
	fmt.Printf("Creating backup of %s to %s...\n", dataDir, outputFile)

	// Create output file
	out, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer out.Close()

	// Create gzip writer
	gw := gzip.NewWriter(out)
	defer gw.Close()

	// Create tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add files from data directory
	err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Open file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Set relative path in archive
		relPath, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Write file content
		_, err = io.Copy(tw, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	fmt.Printf("✅ Backup created successfully: %s\n", outputFile)
	return nil
}

// restore extracts a backup to the specified directory
func restore(backupFile, dataDir string) error {
	fmt.Printf("Restoring backup from %s to %s...\n", backupFile, dataDir)

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open backup file
	in, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer in.Close()

	// Create gzip reader
	gr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	// Extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Create file path
		filePath := filepath.Join(dataDir, header.Name)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Create file
		file, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", filePath, err)
		}

		// Write content
		if _, err := io.Copy(file, tr); err != nil {
			file.Close()
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
		file.Close()

		// Set file mode
		if err := os.Chmod(filePath, os.FileMode(header.Mode)); err != nil {
			return fmt.Errorf("failed to set file mode: %w", err)
		}
	}

	fmt.Printf("✅ Backup restored successfully to %s\n", dataDir)
	return nil
}

// verify checks if a backup is valid and readable
func verify(backupFile string) error {
	fmt.Printf("Verifying backup: %s...\n", backupFile)

	// Open backup file
	in, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer in.Close()

	// Create gzip reader
	gr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("invalid gzip file: %w", err)
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	fileCount := 0
	totalSize := int64(0)
	databases := map[string]bool{}

	// List files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		fileCount++
		totalSize += header.Size

		// Identify database files
		if filepath.Ext(header.Name) == ".db" {
			databases[header.Name] = true
		}

		fmt.Printf("  %s (%d bytes)\n", header.Name, header.Size)
	}

	fmt.Printf("\n✅ Backup is valid\n")
	fmt.Printf("  Files: %d\n", fileCount)
	fmt.Printf("  Total size: %d bytes (%.1f MB)\n", totalSize, float64(totalSize)/1024/1024)
	fmt.Printf("  Databases: %d\n", len(databases))

	if fileCount == 0 {
		return fmt.Errorf("backup contains no files")
	}

	return nil
}

// printUsage prints usage information
func printUsage() {
	fmt.Println(`Database Backup and Restore Tool for LLMSentinel

Usage:
  backup-restore backup <data-dir> <output-file>
    Create a compressed backup of the database directory

  backup-restore restore <backup-file> <data-dir>
    Restore a database from a backup file

  backup-restore verify <backup-file>
    Verify the integrity of a backup file

Examples:
  # Create backup
  ./backup-restore backup ./data ./backups/db-YYYYMMDD-HHMMSS.tar.gz

  # Restore backup
  ./backup-restore restore ./backups/db-backup.tar.gz ./data-restored

  # Verify backup
  ./backup-restore verify ./backups/db-20260430-140000.tar.gz`)
}
