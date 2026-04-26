package indexing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/szibis/claude-escalate/internal/graph"
)

// CodeIndexer watches and indexes source code files, extracting entities and relationships
type CodeIndexer struct {
	graphDB *graph.GraphDB
	mu      sync.RWMutex
	// Track indexed files and their modification times for incremental updates
	indexedFiles map[string]time.Time
	// Track nodes by file for deletion on updates
	fileNodes map[string][]string
}

// NewCodeIndexer creates a new code indexer with graph storage
func NewCodeIndexer(graphDB *graph.GraphDB) *CodeIndexer {
	return &CodeIndexer{
		graphDB:      graphDB,
		indexedFiles: make(map[string]time.Time),
		fileNodes:    make(map[string][]string),
	}
}

// IndexFile indexes a single source code file and stores results in the graph
func (ci *CodeIndexer) IndexFile(ctx context.Context, filePath string) (*IndexingResult, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return &IndexingResult{
			FilePath: filePath,
			Errors:   []string{fmt.Sprintf("read file failed: %v", err)},
		}, fmt.Errorf("read file failed: %w", err)
	}

	// Detect language from file extension
	ext := filepath.Ext(filePath)
	language, ok := SupportedLanguages[ext]
	if !ok {
		return &IndexingResult{
			FilePath: filePath,
			Errors:   []string{fmt.Sprintf("unsupported language for extension: %s", ext)},
		}, fmt.Errorf("unsupported language: %s", ext)
	}

	// Get language-specific parser
	parser := NewParser(language)

	// Parse file and extract entities/relationships
	result, err := parser.Parse(filePath, string(content))
	if err != nil {
		return result, fmt.Errorf("parse failed: %w", err)
	}

	// Store in graph database
	if err := ci.storeInGraph(ctx, filePath, result); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("graph storage failed: %v", err))
		return result, fmt.Errorf("graph storage failed: %w", err)
	}

	// Update tracking
	ci.mu.Lock()
	info, _ := os.Stat(filePath)
	ci.indexedFiles[filePath] = info.ModTime()
	ci.fileNodes[filePath] = extractNodeIDs(result.Entities)
	ci.mu.Unlock()

	result.ParsedAt = time.Now().Unix()
	return result, nil
}

// IndexDirectory recursively indexes all supported source files in a directory
func (ci *CodeIndexer) IndexDirectory(ctx context.Context, dirPath string) (int, error) {
	count := 0
	mu := sync.Mutex{}

	walkErr := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}

		// Skip directories and non-source files
		if info.IsDir() {
			// Skip common non-source directories
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file type is supported
		ext := filepath.Ext(path)
		if _, ok := SupportedLanguages[ext]; !ok {
			return nil
		}

		// Index file
		_, indexErr := ci.IndexFile(ctx, path)
		if indexErr == nil {
			mu.Lock()
			count++
			mu.Unlock()
		}

		return nil
	})

	return count, walkErr
}

// WatchFile watches a file for changes and sends updates to the changes channel
func (ci *CodeIndexer) WatchFile(ctx context.Context, filePath string, changes chan<- *IndexingResult) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher failed: %w", err)
	}
	defer watcher.Close()

	// Watch the file's directory (fsnotify watches directories, not files)
	dirPath := filepath.Dir(filePath)
	if err := watcher.Add(dirPath); err != nil {
		return fmt.Errorf("add watch failed: %w", err)
	}

	// Initial index
	result, err := ci.IndexFile(ctx, filePath)
	if err == nil && result != nil {
		select {
		case changes <- result:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Watch for changes
	debounceTimer := time.NewTimer(100 * time.Millisecond)
	debounceTimer.Stop()
	var pendingPath string

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Only process events for the target file
			if event.Name != filePath {
				continue
			}

			// Debounce rapid file changes (editors often write multiple times)
			pendingPath = event.Name
			debounceTimer.Reset(100 * time.Millisecond)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// Log but continue watching
			_ = err

		case <-debounceTimer.C:
			if pendingPath == filePath {
				// Re-index on change
				result, _ := ci.IndexFile(ctx, filePath)
				if result != nil {
					select {
					case changes <- result:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// storeInGraph saves entities and relationships to the graph database
func (ci *CodeIndexer) storeInGraph(ctx context.Context, _ string, result *IndexingResult) error {
	if result == nil || ci.graphDB == nil {
		return nil
	}

	// Store entities as nodes
	for _, entity := range result.Entities {
		metadata := map[string]interface{}{
			"language":  entity.Language,
			"exported":  entity.Exported,
			"signature": entity.Signature,
		}
		metadataJSON, _ := json.Marshal(metadata)

		node := &graph.Node{
			ID:         entity.ID,
			Name:       entity.Name,
			Type:       mapEntityTypeToNodeType(entity.Type),
			Content:    entity.Content,
			FilePath:   entity.FilePath,
			LineNumber: entity.LineNumber,
			Metadata:   string(metadataJSON),
		}

		if err := ci.graphDB.CreateNode(ctx, node); err != nil {
			return fmt.Errorf("create node failed: %w", err)
		}
	}

	// Store relationships as edges
	for _, rel := range result.Relationships {
		edge := &graph.Edge{
			ID:           rel.SourceID + "->" + rel.TargetID,
			SourceID:     rel.SourceID,
			TargetID:     rel.TargetID,
			RelationType: mapRelationshipType(rel.Type),
			Weight:       rel.Confidence,
		}

		if err := ci.graphDB.CreateEdge(ctx, edge); err != nil {
			// Skip edge creation errors (node might not exist if not in this file)
			continue
		}
	}

	return nil
}

// mapEntityTypeToNodeType converts CodeEntity.Type to string node type
func mapEntityTypeToNodeType(entityType string) string {
	switch entityType {
	case "function":
		return graph.NodeTypeFunction
	case "class":
		return graph.NodeTypeClass
	case "method":
		return graph.NodeTypeMethod
	case "variable":
		return graph.NodeTypeVariable
	case "interface":
		return graph.NodeTypeInterface
	case "struct":
		return graph.NodeTypeStruct
	case "import":
		return graph.NodeTypeImport
	default:
		return graph.NodeTypeFunction
	}
}

// mapRelationshipType converts Relationship.Type to string relation type
func mapRelationshipType(relType string) string {
	switch relType {
	case "calls":
		return graph.RelationTypeCalls
	case "imports":
		return graph.RelationTypeImports
	case "references":
		return graph.RelationTypeReferences
	case "defines":
		return graph.RelationTypeDefines
	case "inherits":
		return graph.RelationTypeInherits
	case "implements":
		return graph.RelationTypeImplements
	default:
		return graph.RelationTypeCalls
	}
}

// extractNodeIDs extracts node IDs from entities for tracking
func extractNodeIDs(entities []*CodeEntity) []string {
	ids := make([]string, 0, len(entities))
	for _, entity := range entities {
		ids = append(ids, entity.ID)
	}
	return ids
}
