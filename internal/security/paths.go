package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePath ensures a file path is within an allowed base directory.
// This prevents directory traversal attacks (CWE-22).
func ValidatePath(requestedPath, baseDir string) (string, error) {
	// Clean both paths to normalize them
	cleanRequested := filepath.Clean(requestedPath)
	cleanBase := filepath.Clean(baseDir)

	// Handle absolute paths
	if filepath.IsAbs(cleanRequested) {
		// If baseDir is relative, make it absolute for comparison
		if !filepath.IsAbs(cleanBase) {
			absBase, err := filepath.Abs(cleanBase)
			if err != nil {
				return "", fmt.Errorf("failed to resolve base directory: %w", err)
			}
			cleanBase = absBase
		}
		// Absolute path must start with base directory
		if !strings.HasPrefix(cleanRequested, cleanBase) {
			return "", fmt.Errorf("path traversal blocked: %s not within %s", cleanRequested, cleanBase)
		}
		return cleanRequested, nil
	}

	// For relative paths, join with base
	joined := filepath.Join(cleanBase, cleanRequested)
	absJoined := filepath.Clean(joined)

	// Make base absolute for comparison
	absBase, err := filepath.Abs(cleanBase)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}

	// Verify result is within base directory
	if !strings.HasPrefix(absJoined, absBase) && absJoined != absBase {
		return "", fmt.Errorf("path traversal blocked: %s not within %s", absJoined, absBase)
	}

	return joined, nil
}

// SafePathJoin is a convenience wrapper for filepath.Join that validates the result.
// It's safe for most use cases where the baseDir is trusted and only the filename
// component comes from user input.
func SafePathJoin(baseDir string, components ...string) (string, error) {
	if len(components) == 0 {
		return baseDir, nil
	}

	// For simple cases with a single component, validate
	if len(components) == 1 {
		return ValidatePath(components[0], baseDir)
	}

	// For multiple components, join and validate
	allComponents := append([]string{baseDir}, components...)
	path := filepath.Join(allComponents...)
	return ValidatePath(path, baseDir)
}
