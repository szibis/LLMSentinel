// Package graph provides knowledge graph storage and traversal for relationship queries.
package graph

import (
	"time"
)

// Node represents a code entity (function, class, variable, etc) in the knowledge graph.
type Node struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`                // function, class, variable, import, etc
	Content    string    `json:"content,omitempty"`   // Full source code definition
	Embedding  []byte    `json:"embedding,omitempty"` // 384-dim float32, serialized
	Metadata   string    `json:"metadata,omitempty"`  // JSON string of custom attributes
	FilePath   string    `json:"file_path,omitempty"`
	LineNumber int       `json:"line_number,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Edge represents a relationship between two nodes in the knowledge graph.
type Edge struct {
	ID           string    `json:"id"`
	SourceID     string    `json:"source_id"`
	TargetID     string    `json:"target_id"`
	RelationType string    `json:"relation_type"`      // calls, defines, imports, references, inherits, etc
	Weight       float32   `json:"weight"`             // Confidence/strength 0.0-1.0
	Metadata     string    `json:"metadata,omitempty"` // JSON string
	CreatedAt    time.Time `json:"created_at"`
}

// GraphLookupResult represents the result of a graph query.
type GraphLookupResult struct {
	Found           bool    `json:"found"`
	NodeID          string  `json:"node_id,omitempty"`
	NodeContent     string  `json:"content,omitempty"`
	FilePath        string  `json:"file_path,omitempty"`
	LineNumber      int     `json:"line_number,omitempty"`
	RelatedNodes    []Node  `json:"related_nodes,omitempty"`
	Similarity      float32 `json:"similarity,omitempty"`       // For semantic matching
	Depth           int     `json:"depth,omitempty"`            // Traversal depth
	ConfidenceScore float32 `json:"confidence_score,omitempty"` // 0.0-1.0
	Error           string  `json:"error,omitempty"`
}

// NodeType constants for common code entities.
const (
	NodeTypeFunction  = "function"
	NodeTypeClass     = "class"
	NodeTypeVariable  = "variable"
	NodeTypeImport    = "import"
	NodeTypeMethod    = "method"
	NodeTypeModule    = "module"
	NodeTypeInterface = "interface"
	NodeTypeStruct    = "struct"
)

// RelationType constants for common relationships.
const (
	RelationTypeCalls      = "calls"
	RelationTypeDefines    = "defines"
	RelationTypeImports    = "imports"
	RelationTypeReferences = "references"
	RelationTypeInherits   = "inherits"
	RelationTypeImplements = "implements"
	RelationTypeUses       = "uses"
)
