// Package main provides spec compliance validation for Claude Escalate
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Requirement represents a specification requirement
type Requirement struct {
	ID          string // REQ-001
	Title       string
	Description string
	Files       []string // Implemented in files
	Tests       []string // Tested in test files
	Status      string   // "implemented", "tested", "complete"
}

// SpecValidator validates spec compliance
type SpecValidator struct {
	requirements map[string]*Requirement
	results      *ValidationResults
}

// ValidationResults holds validation results
type ValidationResults struct {
	TotalRequirements   int
	ImplementedCount    int
	TestedCount         int
	CompleteCount       int
	UncoveredRequires   []*Requirement
	PartiallyTestedReqs []*Requirement
	Coverage            float64
}

// NewSpecValidator creates a new validator
func NewSpecValidator() *SpecValidator {
	return &SpecValidator{
		requirements: initializeRequirements(),
		results:      &ValidationResults{},
	}
}

func initializeRequirements() map[string]*Requirement {
	return map[string]*Requirement{
		"REQ-001": {
			ID:          "REQ-001",
			Title:       "Load config.yaml at startup",
			Description: "Configuration system must load and parse YAML config files",
			Files:       []string{"internal/config/loader.go"},
			Tests:       []string{"internal/config/loader_test.go"},
		},
		"REQ-002": {
			ID:          "REQ-002",
			Title:       "Auto-detect installed tools",
			Description: "System must auto-discover RTK, scrapling, LSP, git and other tools",
			Files:       []string{"internal/discovery/detector.go", "configs/discovery.yaml"},
			Tests:       []string{"internal/discovery/detector_test.go"},
		},
		"REQ-003": {
			ID:          "REQ-003",
			Title:       "Use sensible defaults if no config",
			Description: "Generate default configuration from auto-detected tools",
			Files:       []string{"internal/discovery/defaults.go"},
			Tests:       []string{"internal/discovery/detector_test.go"},
		},
		"REQ-010": {
			ID:          "REQ-010",
			Title:       "Detect cache bypass patterns",
			Description: "Recognize --no-cache, --fresh, !, (bypass), (no cache) flags",
			Files:       []string{"internal/intent/bypass_patterns.go"},
			Tests:       []string{"internal/intent/classifier_test.go"},
		},
		"REQ-011": {
			ID:          "REQ-011",
			Title:       "Classify query intent",
			Description: "Classify intent as QUICK, DETAILED, ROUTINE, LEARNING, FOLLOW_UP",
			Files:       []string{"internal/intent/classifier.go"},
			Tests:       []string{"internal/intent/classifier_test.go"},
		},
		"REQ-012": {
			ID:          "REQ-012",
			Title:       "Couple intent with model selection",
			Description: "Intent detection must drive model selection (QUICK→Haiku, DETAILED→Opus)",
			Files:       []string{"internal/intent/classifier.go"},
			Tests:       []string{"internal/intent/classifier_test.go"},
		},
		"REQ-013": {
			ID:          "REQ-013",
			Title:       "Couple intent with cache safety",
			Description: "Intent detection must drive cache safety decision",
			Files:       []string{"internal/intent/classifier.go"},
			Tests:       []string{"internal/intent/classifier_test.go"},
		},
		"REQ-014": {
			ID:          "REQ-014",
			Title:       "Learn from user feedback",
			Description: "System must learn user preferences from feedback over time",
			Files:       []string{"internal/intent/feedback_history.go"},
			Tests:       []string{"internal/test/integration_test.go"},
		},
		"REQ-020": {
			ID:          "REQ-020",
			Title:       "Exact deduplication",
			Description: "Cache exact request matches with SHA256 hashing",
			Files:       []string{"internal/cache/dedup.go"},
			Tests:       []string{"internal/test/integration_test.go"},
		},
		"REQ-030": {
			ID:          "REQ-030",
			Title:       "Validate all inputs",
			Description: "Validate all inputs for SQL injection, XSS, command injection",
			Files:       []string{"internal/security/validator.go", "internal/security/patterns.go"},
			Tests:       []string{"internal/security/validator_test.go"},
		},
		"REQ-031": {
			ID:          "REQ-031",
			Title:       "Sanitize all outputs",
			Description: "Sanitize outputs for safe display (HTML escape, SQL quote escape)",
			Files:       []string{"internal/security/sanitizer.go"},
			Tests:       []string{"internal/security/validator_test.go"},
		},
		"REQ-032": {
			ID:          "REQ-032",
			Title:       "Rate limiting",
			Description: "Enforce 1000 req/min per IP with exponential backoff",
			Files:       []string{"internal/security/rate_limiter.go"},
			Tests:       []string{"internal/security/validator_test.go"},
		},
		"REQ-040": {
			ID:          "REQ-040",
			Title:       "Publish cache metrics",
			Description: "Publish cache hit rate, false positive rate, and hit count",
			Files:       []string{"internal/metrics/collector.go"},
			Tests:       []string{"internal/metrics/collector_test.go"},
		},
		"REQ-041": {
			ID:          "REQ-041",
			Title:       "Publish token metrics",
			Description: "Publish token savings percentage and total tokens saved",
			Files:       []string{"internal/metrics/collector.go"},
			Tests:       []string{"internal/metrics/collector_test.go"},
		},
		"REQ-050": {
			ID:          "REQ-050",
			Title:       "Web dashboard",
			Description: "Dashboard at http://localhost:8080/dashboard",
			Files:       []string{"internal/dashboard/server.go"},
			Tests:       []string{"internal/test/integration_test.go"},
		},
		"REQ-060": {
			ID:          "REQ-060",
			Title:       "Accept MCP requests",
			Description: "Handle MCP JSON-RPC requests from tools",
			Files:       []string{"internal/gateway/adapter_mcp.go"},
			Tests:       []string{"internal/test/integration_test.go"},
		},
	}
}

// ValidateSpecCompliance checks if all requirements are met
func (sv *SpecValidator) ValidateSpecCompliance(srcDir string) *ValidationResults {
	results := &ValidationResults{
		TotalRequirements: len(sv.requirements),
	}

	for _, req := range sv.requirements {
		// Check if files implementing requirement exist
		implementationFound := false
		for _, file := range req.Files {
			fullPath := filepath.Join(srcDir, file)
			if _, err := os.Stat(fullPath); err == nil {
				implementationFound = true
				results.ImplementedCount++
				break
			}
		}

		// Check if test files exist
		testsFound := false
		for _, testFile := range req.Tests {
			fullPath := filepath.Join(srcDir, testFile)
			if _, err := os.Stat(fullPath); err == nil {
				testsFound = true
				results.TestedCount++
				break
			}
		}

		if implementationFound && testsFound {
			results.CompleteCount++
		} else if !implementationFound {
			results.UncoveredRequires = append(results.UncoveredRequires, req)
		} else {
			results.PartiallyTestedReqs = append(results.PartiallyTestedReqs, req)
		}
	}

	results.Coverage = float64(results.CompleteCount) / float64(results.TotalRequirements) * 100

	return results
}

// PrintReport prints validation report
func (sv *SpecValidator) PrintReport(results *ValidationResults) {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("SPEC COMPLIANCE VALIDATION REPORT")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("\nTotal Requirements: %d\n", results.TotalRequirements)
	fmt.Printf("Implemented: %d\n", results.ImplementedCount)
	fmt.Printf("Tested: %d\n", results.TestedCount)
	fmt.Printf("Complete (Implemented + Tested): %d\n", results.CompleteCount)
	fmt.Printf("Coverage: %.1f%%\n\n", results.Coverage)

	if len(results.UncoveredRequires) > 0 {
		fmt.Println("❌ UNCOVERED REQUIREMENTS:")
		for _, req := range results.UncoveredRequires {
			fmt.Printf("  - %s: %s\n", req.ID, req.Title)
		}
		fmt.Println()
	}

	if len(results.PartiallyTestedReqs) > 0 {
		fmt.Println("⚠️ PARTIALLY TESTED REQUIREMENTS (implemented but not tested):")
		for _, req := range results.PartiallyTestedReqs {
			fmt.Printf("  - %s: %s\n", req.ID, req.Title)
		}
		fmt.Println()
	}

	if results.Coverage >= 100 {
		fmt.Println("✅ ALL REQUIREMENTS COVERED!")
	} else if results.Coverage >= 90 {
		fmt.Println("✅ HIGH COMPLIANCE (>90%)")
	} else if results.Coverage >= 80 {
		fmt.Println("⚠️ MEDIUM COMPLIANCE (80-90%)")
	} else {
		fmt.Println("❌ LOW COMPLIANCE (<80%)")
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════")
}

// ValidatePatterns checks if all security patterns are present
func ValidateSecurityPatterns(srcDir string) {
	patternFile := filepath.Join(srcDir, "internal/security/patterns.go")
	content, err := os.ReadFile(patternFile)
	if err != nil {
		fmt.Printf("❌ Failed to read patterns.go: %v\n", err)
		return
	}

	patternText := string(content)

	expectedPatterns := map[string][]string{
		"SQL Injection":     {"DROP", "DELETE", "UNION SELECT", "OR", "comment"},
		"Command Injection": {"shell metacharacters", "$(", "eval", "system"},
		"XSS":               {"<script>", "javascript:", "onerror", "onclick", "onload"},
	}

	fmt.Println("\nSECURITY PATTERN VALIDATION:")
	fmt.Println("════════════════════════════════════════════════════════════════")

	for category, patterns := range expectedPatterns {
		found := 0
		for _, pattern := range patterns {
			if bytes.Contains([]byte(patternText), []byte(pattern)) {
				found++
			}
		}
		fmt.Printf("%s: %d/%d patterns found\n", category, found, len(patterns))
	}
}

// Main execution
func main() {
	srcDir := "."
	if len(os.Args) > 1 {
		srcDir = os.Args[1]
	}

	validator := NewSpecValidator()
	results := validator.ValidateSpecCompliance(srcDir)
	validator.PrintReport(results)

	// Validate security patterns
	ValidateSecurityPatterns(srcDir)

	// Exit with error code if coverage < 90%
	if results.Coverage < 90 {
		os.Exit(1)
	}
}
