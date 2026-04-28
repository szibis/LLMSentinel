package optimization

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// InputFormatter converts verbose input to structured compact form
type InputFormatter struct {
	// Patterns for identifying structured fields
	patterns map[string]*regexp.Regexp
}

// NewInputFormatter creates a new input formatter
func NewInputFormatter() *InputFormatter {
	return &InputFormatter{
		patterns: make(map[string]*regexp.Regexp),
	}
}

// CompactJSON converts a request to compact JSON form
func (f *InputFormatter) CompactJSON(req *PipelineRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("request cannot be nil")
	}

	// Create compact representation
	compact := map[string]interface{}{
		"q": req.Query,
		"i": req.Intent,
		"t": req.Tool,
		"p": req.Params,
	}

	jsonData, err := json.Marshal(compact)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	return string(jsonData), nil
}

// StructuredFormat converts verbose input description to structured JSON
func (f *InputFormatter) StructuredFormat(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("input cannot be empty")
	}

	// Parse input for common patterns
	structured := parseStructuredInput(input)

	jsonData, err := json.Marshal(structured)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	return string(jsonData), nil
}

// RemoveUnnecessaryWhitespace strips extra whitespace from input
func (f *InputFormatter) RemoveUnnecessaryWhitespace(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Remove leading/trailing whitespace
	trimmed := strings.TrimSpace(input)

	// Replace multiple spaces with single space
	multiSpace := regexp.MustCompile(`\s+`)
	compacted := multiSpace.ReplaceAllString(trimmed, " ")

	// Remove spaces around common punctuation
	compacted = strings.ReplaceAll(compacted, " : ", ":")
	compacted = strings.ReplaceAll(compacted, " , ", ",")

	return compacted, nil
}

// ShortenCommonTerms replaces verbose phrases with shorter equivalents
func (f *InputFormatter) ShortenCommonTerms(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	shortened := input

	// Replace common verbose patterns
	replacements := map[string]string{
		"Find all":           "All",
		"List all":           "All",
		"Show all":           "All",
		"Get all":            "All",
		"Find":               "Get",
		"functions that":     "funcs w/",
		"classes that":       "classes w/",
		"with documentation": "w/ docs",
		"including":          "w/",
		"not including":      "w/o",
		"and also":           "&",
	}

	for verbose, short := range replacements {
		shortened = strings.ReplaceAll(shortened, verbose, short)
	}

	return shortened, nil
}

// parseStructuredInput extracts structured fields from verbose input
func parseStructuredInput(input string) map[string]interface{} {
	result := map[string]interface{}{
		"task":    "",
		"lang":    "",
		"filter":  "",
		"context": "",
		"output":  "",
	}

	// Extract task (verb in first few words)
	words := strings.Fields(input)
	if len(words) > 0 {
		result["task"] = words[0] // Find, List, Get, etc.
	}

	// Detect language mentions
	langs := []string{"python", "go", "typescript", "javascript", "java", "rust"}
	for _, lang := range langs {
		if strings.Contains(strings.ToLower(input), lang) {
			result["lang"] = lang
			break
		}
	}

	// Extract filter conditions (words after "that", "where", "with")
	filterKeywords := []string{" that ", " where ", " with ", " for "}
	for _, kw := range filterKeywords {
		if idx := strings.Index(input, kw); idx != -1 {
			result["filter"] = strings.TrimSpace(input[idx+len(kw):])
			break
		}
	}

	// Check for output format specification
	if strings.Contains(input, "JSON") {
		result["output"] = "json"
	} else if strings.Contains(input, "CSV") {
		result["output"] = "csv"
	}

	return result
}

// FormatterStats represents formatting statistics
type FormatterStats struct {
	OriginalSize   int
	CompactedSize  int
	SavingsPercent float64
}

// GetStats returns formatting efficiency statistics
func (f *InputFormatter) GetStats(original string, formatted string) FormatterStats {
	savings := 0.0
	if len(original) > 0 {
		savings = (float64(len(original)-len(formatted)) / float64(len(original))) * 100
	}

	return FormatterStats{
		OriginalSize:   len(original),
		CompactedSize:  len(formatted),
		SavingsPercent: savings,
	}
}
