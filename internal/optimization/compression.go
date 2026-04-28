package optimization

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParameterCompressor applies multiple compression techniques to request parameters
type ParameterCompressor struct {
	// Default values for common parameters
	defaults map[string]interface{}
	// Key abbreviations
	abbreviations map[string]string
}

// NewParameterCompressor creates a new parameter compressor
func NewParameterCompressor() *ParameterCompressor {
	return &ParameterCompressor{
		defaults: defaultParameterValues(),
		abbreviations: map[string]string{
			"include_metadata":   "im",
			"max_results":        "mr",
			"recursive":          "rc",
			"timeout_seconds":    "ts",
			"output_format":      "of",
			"filter_by_language": "fbl",
			"filter_by_type":     "fbt",
			"sort_by":            "sb",
			"ascending":          "asc",
			"descending":         "desc",
			"case_sensitive":     "cs",
			"include_comments":   "ic",
			"exclude_tests":      "et",
		},
	}
}

// Compress applies compression to parameters
func (pc *ParameterCompressor) Compress(params map[string]interface{}) (string, error) {
	if params == nil {
		return "{}", nil
	}

	// Apply all optimizations in sequence
	optimized := params

	// Step 1: Remove defaults
	optimized, _ = pc.RemoveDefaults(optimized)

	// Step 2: Abbreviate keys
	optimized, _ = pc.AbbreviateKeys(optimized)

	// Serialize to JSON
	jsonData, err := json.Marshal(optimized)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	return string(jsonData), nil
}

// Decompress converts compressed parameters back to original form
func (pc *ParameterCompressor) Decompress(compressed string) (map[string]interface{}, error) {
	var params map[string]interface{}

	err := json.Unmarshal([]byte(compressed), &params)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	// Reverse key abbreviations
	result := make(map[string]interface{})
	for key, value := range params {
		originalKey := pc.expandKey(key)
		result[originalKey] = value
	}

	// Restore defaults if missing
	for key, defaultValue := range pc.defaults {
		if _, exists := result[key]; !exists {
			result[key] = defaultValue
		}
	}

	return result, nil
}

// AbbreviateKeys replaces long parameter names with shorter abbreviations
func (pc *ParameterCompressor) AbbreviateKeys(params map[string]interface{}) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	abbreviated := make(map[string]interface{})

	for key, value := range params {
		abbr := pc.abbreviateKey(key)
		abbreviated[abbr] = value
	}

	return abbreviated, nil
}

// RemoveDefaults removes parameters that match default values
func (pc *ParameterCompressor) RemoveDefaults(params map[string]interface{}) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	optimized := make(map[string]interface{})

	for key, value := range params {
		if defaultValue, hasDefault := pc.defaults[key]; hasDefault {
			// Only include if different from default
			if !valuesEqual(value, defaultValue) {
				optimized[key] = value
			}
		} else {
			// Non-default parameter, always include
			optimized[key] = value
		}
	}

	return optimized, nil
}

// abbreviateKey converts a parameter name to its abbreviation
func (pc *ParameterCompressor) abbreviateKey(key string) string {
	if abbr, ok := pc.abbreviations[key]; ok {
		return abbr
	}

	// Generate abbreviation from first letters of words
	words := strings.Split(key, "_")
	result := ""
	for _, word := range words {
		if len(word) > 0 {
			result += string(word[0])
		}
	}

	if result == "" {
		return key
	}
	return result
}

// expandKey converts an abbreviation back to full parameter name
func (pc *ParameterCompressor) expandKey(abbr string) string {
	// Reverse lookup in abbreviations
	for full, short := range pc.abbreviations {
		if short == abbr {
			return full
		}
	}

	return abbr // Return as-is if no match found
}

// valuesEqual compares two values for equality
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Use JSON serialization for complex types
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)

	return string(aJSON) == string(bJSON)
}

// defaultParameterValues returns standard default values for common parameters
func defaultParameterValues() map[string]interface{} {
	return map[string]interface{}{
		"include_metadata":   false,
		"max_results":        100,
		"recursive":          true,
		"timeout_seconds":    30,
		"output_format":      "json",
		"filter_by_language": "",
		"case_sensitive":     false,
		"include_comments":   true,
		"exclude_tests":      false,
		"sort_by":            "name",
		"ascending":          true,
	}
}

// CompressionStats tracks compression performance
type CompressionStats struct {
	OriginalSize   int
	CompressedSize int
	SavingsPercent float64
	BytesSaved     int
}

// GetStats calculates compression statistics
func GetCompressionStats(original, compressed string) CompressionStats {
	origSize := len(original)
	compSize := len(compressed)
	bytesSaved := origSize - compSize

	savings := 0.0
	if origSize > 0 {
		savings = (float64(bytesSaved) / float64(origSize)) * 100
	}

	if savings < 0 {
		savings = 0 // No savings if expansion occurred
	}

	return CompressionStats{
		OriginalSize:   origSize,
		CompressedSize: compSize,
		SavingsPercent: savings,
		BytesSaved:     bytesSaved,
	}
}

// SortParamsByKey returns parameters sorted by key for consistent hashing
func SortParamsByKey(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	// Extract keys and sort
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Rebuild in sorted order
	sorted := make(map[string]interface{})
	for _, k := range keys {
		sorted[k] = params[k]
	}

	return sorted
}
