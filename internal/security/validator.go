package security

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator performs security validation on inputs and outputs
type Validator struct {
	patterns *AttackPatterns
}

// NewValidator creates a new security validator
func NewValidator() *Validator {
	return &Validator{
		patterns: NewAttackPatterns(),
	}
}

// ValidateInput validates incoming requests for security issues
func (v *Validator) ValidateInput(input string, inputType InputType) (bool, *ValidationResult) {
	result := &ValidationResult{
		IsValid: true,
		Errors:  []string{},
	}

	switch inputType {
	case InputTypeSQL:
		return v.validateSQL(input, result)
	case InputTypeCommand:
		return v.validateCommand(input, result)
	case InputTypeWeb:
		return v.validateWeb(input, result)
	case InputTypeJSON:
		return v.validateJSON(input, result)
	default:
		return v.validateGeneric(input, result)
	}
}

// ValidateOutput sanitizes output for safe display
func (v *Validator) ValidateOutput(output string, outputType OutputType) (string, *ValidationResult) {
	result := &ValidationResult{
		IsValid: true,
		Errors:  []string{},
	}

	switch outputType {
	case OutputTypeHTML:
		return v.sanitizeHTML(output, result), result
	case OutputTypeSQL:
		return v.sanitizeSQL(output, result), result
	case OutputTypeShell:
		return v.sanitizeShell(output, result), result
	default:
		return output, result
	}
}

// validateSQL detects SQL injection patterns
func (v *Validator) validateSQL(input string, result *ValidationResult) (bool, *ValidationResult) {
	for _, pattern := range v.patterns.SQLInjectionPatterns {
		if pattern.MatchString(input) {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("SQL injection pattern detected: %s", pattern.String()))
			result.BlockedPatterns = append(result.BlockedPatterns, pattern.String())
			return false, result
		}
	}

	// Additional heuristic checks
	upperInput := strings.ToUpper(input)

	// Check for SQL keywords used suspiciously
	suspiciousKeywords := []string{"DROP TABLE", "DELETE FROM", "INSERT INTO", "UPDATE", "TRUNCATE", "UNION SELECT"}
	for _, keyword := range suspiciousKeywords {
		if strings.Contains(upperInput, keyword) && !strings.Contains(upperInput, "WHERE") {
			// Flag as suspicious (but allow if there's a WHERE clause for legitimate queries)
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Suspicious SQL keyword found: %s", keyword))
			return false, result
		}
	}

	return true, result
}

// validateCommand detects command injection patterns
func (v *Validator) validateCommand(input string, result *ValidationResult) (bool, *ValidationResult) {
	for _, pattern := range v.patterns.CommandInjectionPatterns {
		if pattern.MatchString(input) {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Command injection pattern detected: %s", pattern.String()))
			result.BlockedPatterns = append(result.BlockedPatterns, pattern.String())
			return false, result
		}
	}

	return true, result
}

// validateWeb detects XSS and web-based attacks
func (v *Validator) validateWeb(input string, result *ValidationResult) (bool, *ValidationResult) {
	for _, pattern := range v.patterns.XSSPatterns {
		if pattern.MatchString(input) {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("XSS pattern detected: %s", pattern.String()))
			result.BlockedPatterns = append(result.BlockedPatterns, pattern.String())
			return false, result
		}
	}

	return true, result
}

// validateJSON performs basic JSON validation
func (v *Validator) validateJSON(input string, result *ValidationResult) (bool, *ValidationResult) {
	// Check for basic JSON structure
	trimmed := strings.TrimSpace(input)
	if len(trimmed) == 0 {
		result.IsValid = false
		result.Errors = append(result.Errors, "JSON input is empty")
		return false, result
	}

	// Very basic check - should be valid JSON syntax
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		result.IsValid = false
		result.Errors = append(result.Errors, "Invalid JSON format")
		return false, result
	}

	return true, result
}

// validateGeneric performs generic security checks
func (v *Validator) validateGeneric(input string, result *ValidationResult) (bool, *ValidationResult) {
	// Apply XSS patterns to all generic input
	for _, pattern := range v.patterns.XSSPatterns {
		if pattern.MatchString(input) {
			result.IsValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("XSS pattern detected: %s", pattern.String()))
			result.BlockedPatterns = append(result.BlockedPatterns, pattern.String())
			return false, result
		}
	}

	return true, result
}

// sanitizeHTML escapes HTML special characters
func (v *Validator) sanitizeHTML(output string, result *ValidationResult) string {
	replacements := map[string]string{
		"&": "&amp;",
		"<": "&lt;",
		">": "&gt;",
		"\"": "&quot;",
		"'": "&#39;",
	}

	sanitized := output
	for char, replacement := range replacements {
		sanitized = strings.ReplaceAll(sanitized, char, replacement)
	}

	return sanitized
}

// sanitizeSQL escapes SQL special characters
func (v *Validator) sanitizeSQL(output string, result *ValidationResult) string {
	// Escape single quotes for SQL
	sanitized := strings.ReplaceAll(output, "'", "''")
	return sanitized
}

// sanitizeShell escapes shell special characters
func (v *Validator) sanitizeShell(output string, result *ValidationResult) string {
	// For shell output, just escape quotes and backticks
	sanitized := output
	sanitized = strings.ReplaceAll(sanitized, "`", "\\`")
	sanitized = strings.ReplaceAll(sanitized, "$", "\\$")
	return sanitized
}

// IsHighRiskInput checks if input is high risk
func (v *Validator) IsHighRiskInput(input string) bool {
	// Check multiple risk indicators
	upperInput := strings.ToUpper(input)

	// Suspicious patterns
	suspiciousPatterns := []string{
		"DROP",
		"DELETE",
		"TRUNCATE",
		"<SCRIPT",
		"JAVASCRIPT:",
		"ONERROR=",
		"${",
		"$()",
		"UNION",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(upperInput, pattern) {
			return true
		}
	}

	return false
}

// InputType represents the type of input being validated
type InputType string

const (
	InputTypeSQL     InputType = "sql"
	InputTypeCommand InputType = "command"
	InputTypeWeb     InputType = "web"
	InputTypeJSON    InputType = "json"
)

// OutputType represents the type of output being sanitized
type OutputType string

const (
	OutputTypeHTML  OutputType = "html"
	OutputTypeSQL   OutputType = "sql"
	OutputTypeShell OutputType = "shell"
)

// ValidationResult represents the result of validation
type ValidationResult struct {
	IsValid         bool
	Errors          []string
	BlockedPatterns []string
	Timestamp       int64
}
