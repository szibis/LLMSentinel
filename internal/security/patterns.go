package security

import (
	"regexp"
)

// AttackPatterns holds all regex patterns for detecting attacks
type AttackPatterns struct {
	SQLInjectionPatterns    []*regexp.Regexp
	CommandInjectionPatterns []*regexp.Regexp
	XSSPatterns             []*regexp.Regexp
}

// NewAttackPatterns creates a new attack patterns detector
func NewAttackPatterns() *AttackPatterns {
	return &AttackPatterns{
		SQLInjectionPatterns:    compileSQLPatterns(),
		CommandInjectionPatterns: compileCommandPatterns(),
		XSSPatterns:             compileXSSPatterns(),
	}
}

// compileSQLPatterns compiles SQL injection detection patterns
func compileSQLPatterns() []*regexp.Regexp {
	patterns := []string{
		// Common SQL injection techniques
		`(?i)'\s*(or|and)\s*'?\s*=\s*'`,                    // ' OR '='
		`(?i)'\s*(or|and)\s*'?\s*=\s*[0-9]`,               // ' OR '=' 0/1
		`(?i);\s*(drop|delete|insert|update|truncate)`,    // ; DROP/DELETE/INSERT
		`(?i)--\s*$`,                                        // SQL comments
		`(?i)/\*.*?\*/`,                                     // /* */ comments
		`(?i)union\s+select`,                                // UNION SELECT
		`(?i)union\s+all\s+select`,                         // UNION ALL SELECT
		`(?i)exec\s*\(`,                                     // EXEC()
		`(?i)execute\s*\(`,                                  // EXECUTE()
		`(?i)script\s*>`,                                    // script>
		`(?i)\x00`,                                          // Null byte injection
		`(?i)having\s+1\s*=\s*1`,                           // HAVING 1=1
		`(?i)waitfor\s+delay`,                               // WAITFOR DELAY (SQL Server)
		`(?i)benchmark\s*\(`,                                // BENCHMARK() (MySQL)
		`(?i)sleep\s*\(`,                                    // SLEEP() (MySQL)
	}

	return compilePatterns(patterns)
}

// compileCommandPatterns compiles command injection detection patterns
func compileCommandPatterns() []*regexp.Regexp {
	patterns := []string{
		`[|&;$><\n\r]`,                                      // Shell metacharacters
		`\$\(.*\)`,                                          // Command substitution $()
		`\`.*\``,                                            // Command substitution backticks
		`(?i)eval\s*\(`,                                     // eval()
		`(?i)system\s*\(`,                                   // system()
		`(?i)exec\s*\(`,                                     // exec()
		`(?i)passthru\s*\(`,                                 // passthru()
		`(?i)shell_exec\s*\(`,                               // shell_exec()
		`(?i)proc_open\s*\(`,                                // proc_open()
		`(?i)popen\s*\(`,                                    // popen()
		`\.\.\*/`,                                           // Path traversal
		`\.\./`,                                             // Path traversal
	}

	return compilePatterns(patterns)
}

// compileXSSPatterns compiles XSS detection patterns
func compileXSSPatterns() []*regexp.Regexp {
	patterns := []string{
		`(?i)<script[^>]*>`,                                 // <script>
		`(?i)</script>`,                                     // </script>
		`(?i)<iframe[^>]*>`,                                 // <iframe>
		`(?i)<img[^>]*\s+on`,                                // <img on...
		`(?i)<svg[^>]*on`,                                   // <svg on...
		`(?i)<body[^>]*on`,                                  // <body on...
		`(?i)<input[^>]*on`,                                 // <input on...
		`(?i)javascript:`,                                   // javascript: protocol
		`(?i)data:.*script`,                                 // data: with script
		`(?i)vbscript:`,                                     // vbscript: protocol
		`(?i)onload\s*=`,                                    // onload=
		`(?i)onerror\s*=`,                                   // onerror=
		`(?i)onclick\s*=`,                                   // onclick=
		`(?i)onmouseover\s*=`,                               // onmouseover=
		`(?i)onkeydown\s*=`,                                 // onkeydown=
		`(?i)onkeyup\s*=`,                                   // onkeyup=
		`(?i)onfocus\s*=`,                                   // onfocus=
		`(?i)onchange\s*=`,                                  // onchange=
		`(?i)onsubmit\s*=`,                                  // onsubmit=
		`(?i)eval\s*\(`,                                     // eval()
		`(?i)expression\s*\(`,                               // expression()
		`(?i)alert\s*\(`,                                    // alert()
		`(?i)document\.`,                                    // document. access
		`(?i)window\.`,                                      // window. access
	}

	return compilePatterns(patterns)
}

// compilePatterns compiles string patterns into regex patterns
func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			compiled = append(compiled, regex)
		}
	}

	return compiled
}

// PatternType represents a type of attack pattern
type PatternType string

const (
	PatternTypeSQLInjection      PatternType = "sql_injection"
	PatternTypeCommandInjection  PatternType = "command_injection"
	PatternTypeXSS               PatternType = "xss"
	PatternTypePathTraversal     PatternType = "path_traversal"
	PatternTypeProtocolExploit   PatternType = "protocol_exploit"
)

// DetectionResult represents a pattern detection result
type DetectionResult struct {
	Detected    bool
	PatternType PatternType
	Pattern     string
	Input       string
	Location    int
}
