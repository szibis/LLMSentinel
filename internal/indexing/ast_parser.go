package indexing

import (
	"fmt"
	"regexp"
	"strings"
)

// ASTParser extracts code entities from source files
type ASTParser interface {
	Parse(filePath, content string) (*IndexingResult, error)
}

// ParserFactory creates language-specific parsers
func NewParser(language string) ASTParser {
	switch language {
	case "go":
		return &GoParser{}
	case "python":
		return &PythonParser{}
	case "typescript", "javascript":
		return &TypeScriptParser{}
	default:
		return &GenericParser{}
	}
}

// GoParser extracts entities from Go source
type GoParser struct{}

func (p *GoParser) Parse(filePath, content string) (*IndexingResult, error) {
	result := &IndexingResult{
		FilePath: filePath,
		Entities: make([]*CodeEntity, 0),
	}

	lines := strings.Split(content, "\n")

	// Find function declarations: func Name(...) { ... }
	funcRegex := regexp.MustCompile(`^\s*func\s+(\(.*?\))?\s*([a-zA-Z_]\w*)\s*\(`)
	// Find struct declarations: type Name struct { ... }
	structRegex := regexp.MustCompile(`^\s*type\s+([a-zA-Z_]\w*)\s+struct\s*{`)
	// Find interface declarations: type Name interface { ... }
	interfaceRegex := regexp.MustCompile(`^\s*type\s+([a-zA-Z_]\w*)\s+interface\s*{`)
	// Find imports: import (...)
	importRegex := regexp.MustCompile(`^\s*import\s+(?:"([^"]+)"|.*)`)

	for i, line := range lines {
		lineNum := i + 1

		// Check for function
		if matches := funcRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[len(matches)-1]
			if name != "" {
				entity := &CodeEntity{
					ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
					Name:       name,
					Type:       "function",
					FilePath:   filePath,
					LineNumber: lineNum,
					Content:    line,
					Signature:  line,
					Language:   "go",
					Exported:   strings.Contains(line, fmt.Sprintf("func %s", name)),
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Check for struct
		if matches := structRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "struct",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Language:   "go",
				Exported:   strings.Contains(name, strings.ToUpper(string(name[0]))),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for interface
		if matches := interfaceRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "interface",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Language:   "go",
				Exported:   strings.Contains(name, strings.ToUpper(string(name[0]))),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for imports
		if matches := importRegex.FindStringSubmatch(line); len(matches) > 0 && matches[1] != "" {
			importPath := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:import:%s", filePath, lineNum, importPath),
				Name:       importPath,
				Type:       "import",
				FilePath:   filePath,
				LineNumber: lineNum,
				Language:   "go",
			}
			result.Entities = append(result.Entities, entity)
		}
	}

	// Extract function calls (simple heuristic)
	result.Relationships = p.extractCalls(filePath, content, result.Entities)

	return result, nil
}

// extractCalls finds function calls in Go source
func (p *GoParser) extractCalls(filePath, content string, entities []*CodeEntity) []*Relationship {
	relationships := make([]*Relationship, 0)

	// Map function names to their IDs for quick lookup
	funcMap := make(map[string]*CodeEntity)
	for _, entity := range entities {
		if entity.Type == "function" || entity.Type == "method" {
			funcMap[entity.Name] = entity
		}
	}

	lines := strings.Split(content, "\n")
	callRegex := regexp.MustCompile(`\b([a-zA-Z_]\w*)\s*\(`)

	for i, line := range lines {
		lineNum := i + 1
		matches := callRegex.FindAllStringSubmatchIndex(line, -1)

		for _, match := range matches {
			if len(match) >= 4 {
				callName := line[match[2]:match[3]]

				// Check if called function exists
				if targetEntity, ok := funcMap[callName]; ok {
					// Find the calling function
					var caller *CodeEntity
					for _, entity := range entities {
						if entity.Type == "function" && entity.LineNumber < lineNum {
							if caller == nil || entity.LineNumber > caller.LineNumber {
								caller = entity
							}
						}
					}

					if caller != nil && caller.Name != callName {
						rel := &Relationship{
							SourceID:   caller.ID,
							TargetID:   targetEntity.ID,
							Type:       "calls",
							FilePath:   filePath,
							LineNumber: lineNum,
							Confidence: 0.95,
						}
						relationships = append(relationships, rel)
					}
				}
			}
		}
	}

	return relationships
}

// PythonParser extracts entities from Python source
type PythonParser struct{}

func (p *PythonParser) Parse(filePath, content string) (*IndexingResult, error) {
	result := &IndexingResult{
		FilePath: filePath,
		Entities: make([]*CodeEntity, 0),
	}

	lines := strings.Split(content, "\n")

	// Find function definitions: def name(...):
	defRegex := regexp.MustCompile(`^\s*def\s+([a-zA-Z_]\w*)\s*\(`)
	// Find class definitions: class Name(...):
	classRegex := regexp.MustCompile(`^\s*class\s+([a-zA-Z_]\w*)\s*[\(:]`)
	// Find imports: import ... or from ... import ...
	importRegex := regexp.MustCompile(`^\s*(?:from|import)\s+(.+)`)

	for i, line := range lines {
		lineNum := i + 1

		// Check for function
		if matches := defRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "function",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Signature:  line,
				Language:   "python",
				Exported:   !strings.HasPrefix(name, "_"),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for class
		if matches := classRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "class",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Language:   "python",
				Exported:   !strings.HasPrefix(name, "_"),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for imports
		if matches := importRegex.FindStringSubmatch(line); len(matches) > 0 {
			importPath := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:import:%s", filePath, lineNum, importPath),
				Name:       importPath,
				Type:       "import",
				FilePath:   filePath,
				LineNumber: lineNum,
				Language:   "python",
			}
			result.Entities = append(result.Entities, entity)
		}
	}

	// Extract function calls
	result.Relationships = p.extractCalls(filePath, content, result.Entities)

	return result, nil
}

// extractCalls finds function calls in Python source
func (p *PythonParser) extractCalls(filePath, content string, entities []*CodeEntity) []*Relationship {
	relationships := make([]*Relationship, 0)

	// Map function names to their IDs
	funcMap := make(map[string]*CodeEntity)
	for _, entity := range entities {
		if entity.Type == "function" {
			funcMap[entity.Name] = entity
		}
	}

	lines := strings.Split(content, "\n")
	callRegex := regexp.MustCompile(`\b([a-zA-Z_]\w*)\s*\(`)

	for i, line := range lines {
		lineNum := i + 1
		// Skip lines that are function definitions
		if strings.Contains(line, "def ") {
			continue
		}

		matches := callRegex.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				callName := line[match[2]:match[3]]

				if targetEntity, ok := funcMap[callName]; ok {
					// Find caller
					var caller *CodeEntity
					for _, entity := range entities {
						if entity.Type == "function" && entity.LineNumber < lineNum {
							if caller == nil || entity.LineNumber > caller.LineNumber {
								caller = entity
							}
						}
					}

					if caller != nil && caller.Name != callName {
						rel := &Relationship{
							SourceID:   caller.ID,
							TargetID:   targetEntity.ID,
							Type:       "calls",
							FilePath:   filePath,
							LineNumber: lineNum,
							Confidence: 0.90,
						}
						relationships = append(relationships, rel)
					}
				}
			}
		}
	}

	return relationships
}

// TypeScriptParser extracts entities from TypeScript/JavaScript source
type TypeScriptParser struct{}

func (p *TypeScriptParser) Parse(filePath, content string) (*IndexingResult, error) {
	result := &IndexingResult{
		FilePath: filePath,
		Entities: make([]*CodeEntity, 0),
	}

	lines := strings.Split(content, "\n")

	// Find function declarations: function name(...) or const name = (...) => or function* name
	funcRegex := regexp.MustCompile(`(?:function\*?\s+|const\s+)([a-zA-Z_$]\w*)\s*(?:\(|=)`)
	// Find class declarations: class Name { ... }
	classRegex := regexp.MustCompile(`^\s*(?:export\s+)?class\s+([a-zA-Z_$]\w*)`)
	// Find interface declarations: interface Name { ... }
	interfaceRegex := regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([a-zA-Z_$]\w*)`)
	// Find imports: import ... from ...
	importRegex := regexp.MustCompile(`^\s*import\s+.+\s+from\s+['"]([^'"]+)['"]`)

	for i, line := range lines {
		lineNum := i + 1

		// Check for function
		if matches := funcRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "function",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Signature:  line,
				Language:   "typescript",
				Exported:   strings.Contains(line, "export"),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for class
		if matches := classRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "class",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Language:   "typescript",
				Exported:   strings.Contains(line, "export"),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for interface
		if matches := interfaceRegex.FindStringSubmatch(line); len(matches) > 0 {
			name := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:%s", filePath, lineNum, name),
				Name:       name,
				Type:       "interface",
				FilePath:   filePath,
				LineNumber: lineNum,
				Content:    line,
				Language:   "typescript",
				Exported:   strings.Contains(line, "export"),
			}
			result.Entities = append(result.Entities, entity)
		}

		// Check for imports
		if matches := importRegex.FindStringSubmatch(line); len(matches) > 0 {
			importPath := matches[1]
			entity := &CodeEntity{
				ID:         fmt.Sprintf("%s:%d:import:%s", filePath, lineNum, importPath),
				Name:       importPath,
				Type:       "import",
				FilePath:   filePath,
				LineNumber: lineNum,
				Language:   "typescript",
			}
			result.Entities = append(result.Entities, entity)
		}
	}

	result.Relationships = p.extractCalls(filePath, content, result.Entities)

	return result, nil
}

// extractCalls finds function calls in TypeScript/JavaScript source
func (p *TypeScriptParser) extractCalls(filePath, content string, entities []*CodeEntity) []*Relationship {
	relationships := make([]*Relationship, 0)

	funcMap := make(map[string]*CodeEntity)
	for _, entity := range entities {
		if entity.Type == "function" || entity.Type == "method" {
			funcMap[entity.Name] = entity
		}
	}

	lines := strings.Split(content, "\n")
	callRegex := regexp.MustCompile(`\b([a-zA-Z_$]\w*)\s*\(`)

	for i, line := range lines {
		lineNum := i + 1
		if strings.Contains(line, "function") || strings.Contains(line, "class") {
			continue
		}

		matches := callRegex.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				callName := line[match[2]:match[3]]

				if targetEntity, ok := funcMap[callName]; ok {
					var caller *CodeEntity
					for _, entity := range entities {
						if entity.Type == "function" && entity.LineNumber < lineNum {
							if caller == nil || entity.LineNumber > caller.LineNumber {
								caller = entity
							}
						}
					}

					if caller != nil && caller.Name != callName {
						rel := &Relationship{
							SourceID:   caller.ID,
							TargetID:   targetEntity.ID,
							Type:       "calls",
							FilePath:   filePath,
							LineNumber: lineNum,
							Confidence: 0.85,
						}
						relationships = append(relationships, rel)
					}
				}
			}
		}
	}

	return relationships
}

// GenericParser is a fallback for unsupported languages
type GenericParser struct{}

func (p *GenericParser) Parse(filePath, content string) (*IndexingResult, error) {
	return &IndexingResult{
		FilePath: filePath,
		Entities: make([]*CodeEntity, 0),
		Errors:   []string{"parser not implemented for this language"},
	}, nil
}
