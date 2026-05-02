package mock

import (
	"fmt"
	"strings"

	"github.com/szibis/claude-escalate/internal/client"
)

// ComplianceValidator validates that mock responses match real Claude API format
type ComplianceValidator struct {
	strictMode   bool
	violations   []ComplianceViolation
	warnings     []string
}

// ComplianceViolation represents an API compliance issue
type ComplianceViolation struct {
	Field       string
	Expected    string
	Actual      string
	Severity    string // "error" or "warning"
	Description string
}

// NewComplianceValidator creates a new validator
func NewComplianceValidator(strictMode bool) *ComplianceValidator {
	return &ComplianceValidator{
		strictMode: strictMode,
		violations: []ComplianceViolation{},
		warnings:   []string{},
	}
}

// ValidateResponse checks a message response against Claude API compliance
func (cv *ComplianceValidator) ValidateResponse(resp *client.MessageResponse) bool {
	cv.violations = []ComplianceViolation{}
	cv.warnings = []string{}

	// Validate required fields
	cv.validateRequiredFields(resp)

	// Validate format
	cv.validateFormat(resp)

	// Validate token counts
	cv.validateTokenCounts(resp)

	// Validate IDs and metadata
	cv.validateMetadata(resp)

	// Validate content structure
	cv.validateContent(resp)

	if cv.strictMode {
		return len(cv.violations) == 0
	}

	// In non-strict mode, only errors matter (not warnings)
	for _, v := range cv.violations {
		if v.Severity == "error" {
			return false
		}
	}
	return true
}

// validateRequiredFields checks that all required fields are present
func (cv *ComplianceValidator) validateRequiredFields(resp *client.MessageResponse) {
	if resp.ID == "" {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "id",
			Expected:    "Non-empty message ID",
			Actual:      "Empty",
			Severity:    "error",
			Description: "Message ID is required",
		})
	}

	if resp.Type != "message" {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "type",
			Expected:    "message",
			Actual:      resp.Type,
			Severity:    "error",
			Description: "Type must be 'message'",
		})
	}

	if resp.Role != "assistant" {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "role",
			Expected:    "assistant",
			Actual:      resp.Role,
			Severity:    "error",
			Description: "Role must be 'assistant'",
		})
	}

	if resp.Model == "" {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "model",
			Expected:    "Non-empty model name",
			Actual:      "Empty",
			Severity:    "error",
			Description: "Model field is required",
		})
	}

	if resp.StopReason == "" {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "stop_reason",
			Expected:    "Valid stop reason",
			Actual:      "Empty",
			Severity:    "error",
			Description: "Stop reason must be set (e.g., 'end_turn', 'max_tokens')",
		})
	}
}

// validateFormat checks the format of various fields
func (cv *ComplianceValidator) validateFormat(resp *client.MessageResponse) {
	// Validate ID format (should be msg_xxx...)
	if resp.ID != "" {
		if !strings.HasPrefix(resp.ID, "msg_") {
			cv.warnings = append(cv.warnings, fmt.Sprintf(
				"ID format unusual: %s (expected msg_...)", resp.ID))
		}

		if len(resp.ID) < 8 {
			cv.violations = append(cv.violations, ComplianceViolation{
				Field:       "id",
				Expected:    "ID length >= 8",
				Actual:      fmt.Sprintf("length %d", len(resp.ID)),
				Severity:    "warning",
				Description: "ID seems unusually short",
			})
		}
	}

	// Validate model name format
	validModels := map[string]bool{
		"claude-opus":                    true,
		"claude-sonnet":                  true,
		"claude-haiku":                   true,
		"claude-3-opus-latest":           true,
		"claude-3-sonnet-latest":         true,
		"claude-3-haiku-latest":          true,
		"claude-3-5-sonnet-20241022":     true,
		"claude-3-5-haiku-20241022":      true,
		"claude-instant":                 true,
		"mock-model":                     true,
		"local-llm":                      true,
	}

	if !validModels[resp.Model] {
		cv.warnings = append(cv.warnings,
			fmt.Sprintf("Unusual model name: %s", resp.Model))
	}

	// Validate stop reason
	validStopReasons := map[string]bool{
		"end_turn":     true,
		"max_tokens":   true,
		"stop_sequence": true,
	}

	if !validStopReasons[resp.StopReason] {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "stop_reason",
			Expected:    "end_turn, max_tokens, or stop_sequence",
			Actual:      resp.StopReason,
			Severity:    "warning",
			Description: "Stop reason is non-standard",
		})
	}
}

// validateTokenCounts checks that token counts are reasonable
func (cv *ComplianceValidator) validateTokenCounts(resp *client.MessageResponse) {
	// Input tokens should be non-negative
	if resp.Usage.InputTokens < 0 {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "usage.input_tokens",
			Expected:    ">= 0",
			Actual:      fmt.Sprintf("%d", resp.Usage.InputTokens),
			Severity:    "error",
			Description: "Token count cannot be negative",
		})
	}

	// Output tokens should be non-negative
	if resp.Usage.OutputTokens < 0 {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "usage.output_tokens",
			Expected:    ">= 0",
			Actual:      fmt.Sprintf("%d", resp.Usage.OutputTokens),
			Severity:    "error",
			Description: "Token count cannot be negative",
		})
	}

	// If we have content, output tokens should be > 0
	if len(resp.Content) > 0 && resp.Usage.OutputTokens == 0 {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "usage.output_tokens",
			Expected:    "> 0",
			Actual:      "0",
			Severity:    "warning",
			Description: "Output tokens should be > 0 when content is present",
		})
	}

	// Token counts shouldn't be excessively high
	const maxReasonable = 10000000 // 10M tokens
	if resp.Usage.InputTokens > maxReasonable {
		cv.warnings = append(cv.warnings,
			fmt.Sprintf("Input tokens unusually high: %d", resp.Usage.InputTokens))
	}
	if resp.Usage.OutputTokens > maxReasonable {
		cv.warnings = append(cv.warnings,
			fmt.Sprintf("Output tokens unusually high: %d", resp.Usage.OutputTokens))
	}
}

// validateMetadata checks metadata fields
func (cv *ComplianceValidator) validateMetadata(resp *client.MessageResponse) {
	// Validate that we have content
	if len(resp.Content) == 0 {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "content",
			Expected:    "Non-empty array",
			Actual:      "Empty array",
			Severity:    "error",
			Description: "Response must have at least one content block",
		})
		return
	}

	// Validate content type
	for i, block := range resp.Content {
		if block.Type != "text" && block.Type != "tool_use" && block.Type != "tool_result" {
			cv.violations = append(cv.violations, ComplianceViolation{
				Field:       fmt.Sprintf("content[%d].type", i),
				Expected:    "text, tool_use, or tool_result",
				Actual:      block.Type,
				Severity:    "warning",
				Description: "Unusual content type",
			})
		}

		// Text blocks should have non-empty text
		if block.Type == "text" && block.Text == "" {
			cv.violations = append(cv.violations, ComplianceViolation{
				Field:       fmt.Sprintf("content[%d].text", i),
				Expected:    "Non-empty",
				Actual:      "Empty",
				Severity:    "error",
				Description: "Text content cannot be empty",
			})
		}
	}
}

// validateContent checks the response content for quality
func (cv *ComplianceValidator) validateContent(resp *client.MessageResponse) {
	if len(resp.Content) == 0 {
		return
	}

	content := resp.Content[0].Text

	// Content should be reasonably long
	if len(content) < 5 {
		cv.warnings = append(cv.warnings,
			fmt.Sprintf("Response content very short: %d chars", len(content)))
	}

	// Check for obvious non-text content (URLs, file paths, etc should be reasonable)
	if len(content) > 100000 {
		cv.warnings = append(cv.warnings,
			fmt.Sprintf("Response content unusually long: %d chars", len(content)))
	}

	// Check for common issues
	if strings.Contains(content, "\x00") {
		cv.violations = append(cv.violations, ComplianceViolation{
			Field:       "content",
			Expected:    "Valid UTF-8 text",
			Actual:      "Contains null bytes",
			Severity:    "error",
			Description: "Response contains null bytes",
		})
	}
}

// GetViolations returns all violations found
func (cv *ComplianceValidator) GetViolations() []ComplianceViolation {
	return cv.violations
}

// GetWarnings returns all warnings found
func (cv *ComplianceValidator) GetWarnings() []string {
	return cv.warnings
}

// Report generates a compliance report
func (cv *ComplianceValidator) Report() string {
	var sb strings.Builder

	if len(cv.violations) == 0 && len(cv.warnings) == 0 {
		sb.WriteString("✅ Full Claude API compliance - all checks passed\n")
		return sb.String()
	}

	if len(cv.violations) > 0 {
		sb.WriteString(fmt.Sprintf("❌ %d compliance violations:\n", len(cv.violations)))
		for _, v := range cv.violations {
			sb.WriteString(fmt.Sprintf("  - %s: %s (expected %s, got %s)\n",
				v.Field, v.Description, v.Expected, v.Actual))
		}
	}

	if len(cv.warnings) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️  %d warnings:\n", len(cv.warnings)))
		for _, w := range cv.warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	return sb.String()
}

// ModelQualityTier maps model characteristics to Claude API tiers
type ModelQualityTier struct {
	Tier            string // "haiku", "sonnet", "opus"
	AvgOutputTokens int    // Expected tokens per response
	Capability      string // "fast", "balanced", "advanced"
}

// DetectLocalModelTier tries to map a local model to a Claude tier
func DetectLocalModelTier(modelName string) ModelQualityTier {
	lower := strings.ToLower(modelName)

	// Fast models (Haiku-like)
	fastPatterns := []string{"haiku", "2.7b", "3b", "phi", "tiny", "small"}
	for _, pattern := range fastPatterns {
		if strings.Contains(lower, pattern) {
			return ModelQualityTier{
				Tier:            "haiku",
				AvgOutputTokens: 500,
				Capability:      "fast",
			}
		}
	}

	// Balanced models (Sonnet-like)
	balancedPatterns := []string{"sonnet", "7b", "8b", "13b", "medium", "mistral"}
	for _, pattern := range balancedPatterns {
		if strings.Contains(lower, pattern) {
			return ModelQualityTier{
				Tier:            "sonnet",
				AvgOutputTokens: 1000,
				Capability:      "balanced",
			}
		}
	}

	// Advanced models (Opus-like)
	advancedPatterns := []string{"opus", "70b", "largest", "llama-3"}
	for _, pattern := range advancedPatterns {
		if strings.Contains(lower, pattern) {
			return ModelQualityTier{
				Tier:            "opus",
				AvgOutputTokens: 2000,
				Capability:      "advanced",
			}
		}
	}

	// Default to Sonnet
	return ModelQualityTier{
		Tier:            "sonnet",
		AvgOutputTokens: 1000,
		Capability:      "balanced",
	}
}
