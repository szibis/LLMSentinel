package indexing

// CodeEntity represents an extracted code entity (function, class, etc)
type CodeEntity struct {
	ID         string // Unique ID (file:line:name)
	Name       string // Function/class name
	Type       string // function, class, method, interface, struct, variable
	FilePath   string // Relative file path
	LineNumber int    // Starting line number
	EndLine    int    // Ending line number
	Content    string // Full source code
	Signature  string // Function/method signature
	DocString  string // Documentation/comments
	Language   string // go, python, typescript, etc
	Exported   bool   // Public/exported symbol
	Metadata   map[string]interface{}
}

// Relationship represents a code relationship
type Relationship struct {
	SourceID   string  // Caller function ID
	TargetID   string  // Called function ID
	Type       string  // calls, references, imports, inherits, implements
	FilePath   string  // File where relationship appears
	LineNumber int     // Line where relationship appears
	Confidence float32 // 0.0-1.0 confidence in relationship
}

// IndexingResult represents the result of indexing a file
type IndexingResult struct {
	FilePath      string
	Entities      []*CodeEntity
	Relationships []*Relationship
	Errors        []string
	ParsedAt      int64 // Unix timestamp
}

// SupportedLanguages maps file extensions to language parsers
var SupportedLanguages = map[string]string{
	".go":   "go",
	".py":   "python",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".java": "java",
	".rs":   "rust",
	".cpp":  "cpp",
	".c":    "c",
	".h":    "c",
	".hpp":  "cpp",
}
