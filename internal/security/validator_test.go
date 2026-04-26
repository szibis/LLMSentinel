package security

import (
	"testing"
)

func TestValidateSQLInjection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "clean SQL",
			input:   "SELECT * FROM users WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "SQL injection: OR condition",
			input:   "SELECT * FROM users WHERE id = '' OR ''=''",
			wantErr: true,
		},
		{
			name:    "SQL injection: DROP TABLE",
			input:   "; DROP TABLE users--",
			wantErr: true,
		},
		{
			name:    "SQL injection: UNION SELECT",
			input:   "SELECT * FROM users UNION SELECT * FROM admin",
			wantErr: true,
		},
		{
			name:    "SQL injection: SQL comment",
			input:   "SELECT * FROM users -- comment",
			wantErr: true,
		},
		{
			name:    "legitimate SQL with WHERE clause",
			input:   "SELECT * FROM users WHERE name = ? AND id = ? WHERE status = 'active'",
			wantErr: false,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, result := validator.ValidateInput(tt.input, InputTypeSQL)

			if tt.wantErr && valid {
				t.Error("expected invalid, got valid")
			}
			if !tt.wantErr && !valid {
				t.Errorf("expected valid, got invalid: %v", result.Errors)
			}
			if !tt.wantErr && result.IsValid != valid {
				t.Error("result.IsValid doesn't match return value")
			}
		})
	}
}

func TestValidateCommandInjection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "clean command",
			input:   "git status --porcelain",
			wantErr: false,
		},
		{
			name:    "command injection: pipe",
			input:   "ls -la | cat /etc/passwd",
			wantErr: true,
		},
		{
			name:    "command injection: semicolon",
			input:   "ls; rm -rf /",
			wantErr: true,
		},
		{
			name:    "command injection: command substitution $(...)",
			input:   "echo $(cat /etc/passwd)",
			wantErr: true,
		},
		{
			name:    "command injection: backticks",
			input:   "echo `cat /etc/passwd`",
			wantErr: true,
		},
		{
			name:    "command injection: &&",
			input:   "ls && cat /etc/shadow",
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, result := validator.ValidateInput(tt.input, InputTypeCommand)

			if tt.wantErr && valid {
				t.Error("expected invalid, got valid")
			}
			if !tt.wantErr && !valid {
				t.Errorf("expected valid, got invalid: %v", result.Errors)
			}
		})
	}
}

func TestValidateXSS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "clean HTML",
			input:   "<div>Hello World</div>",
			wantErr: false,
		},
		{
			name:    "XSS: script tag",
			input:   "<script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:    "XSS: img onerror",
			input:   "<img src=x onerror=\"alert('xss')\">",
			wantErr: true,
		},
		{
			name:    "XSS: iframe",
			input:   "<iframe src=\"http://evil.com\"></iframe>",
			wantErr: true,
		},
		{
			name:    "XSS: javascript protocol",
			input:   "<a href=\"javascript:alert('xss')\">Click me</a>",
			wantErr: true,
		},
		{
			name:    "XSS: onclick handler",
			input:   "<div onclick=\"alert('xss')\">Click me</div>",
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, result := validator.ValidateInput(tt.input, InputTypeWeb)

			if tt.wantErr && valid {
				t.Error("expected invalid, got valid")
			}
			if !tt.wantErr && !valid {
				t.Errorf("expected valid, got invalid: %v", result.Errors)
			}
		})
	}
}

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "no special characters",
			input:  "hello world",
			expect: "hello world",
		},
		{
			name:   "encode ampersand",
			input:  "hello & world",
			expect: "hello &amp; world",
		},
		{
			name:   "encode angle brackets",
			input:  "<script>alert(1)</script>",
			expect: "&lt;script&gt;alert(1)&lt;/script&gt;",
		},
		{
			name:   "encode quotes",
			input:  `<div class="test">'quoted'</div>`,
			expect: `&lt;div class=&#34;test&#34;&gt;&#39;quoted&#39;&lt;/div&gt;`,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, result := validator.ValidateOutput(tt.input, OutputTypeHTML)

			if sanitized != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, sanitized)
			}
			if !result.IsValid {
				t.Errorf("expected valid result, got errors: %v", result.Errors)
			}
		})
	}
}

func TestIsHighRiskInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "safe input",
			input:   "select user_id from users",
			wantErr: false,
		},
		{
			name:    "contains DROP",
			input:   "DROP TABLE users",
			wantErr: true,
		},
		{
			name:    "contains script tag",
			input:   "<script>alert(1)</script>",
			wantErr: true,
		},
		{
			name:    "contains javascript protocol",
			input:   "javascript:alert(1)",
			wantErr: true,
		},
		{
			name:    "contains command substitution",
			input:   "$(whoami)",
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isRisk := validator.IsHighRiskInput(tt.input)

			if tt.wantErr && !isRisk {
				t.Error("expected high risk, got low risk")
			}
			if !tt.wantErr && isRisk {
				t.Error("expected low risk, got high risk")
			}
		})
	}
}

func TestValidateJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid JSON object",
			input:   `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "valid JSON array",
			input:   `[1, 2, 3]`,
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "not json",
			wantErr: true,
		},
	}

	validator := NewValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := validator.ValidateInput(tt.input, InputTypeJSON)

			if tt.wantErr && valid {
				t.Error("expected invalid, got valid")
			}
			if !tt.wantErr && !valid {
				t.Error("expected valid, got invalid")
			}
		})
	}
}

// Benchmark test for validation performance
func BenchmarkValidateInput(b *testing.B) {
	validator := NewValidator()
	input := "SELECT * FROM users WHERE id = 1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateInput(input, InputTypeSQL)
	}
}

// Test that security layer cannot be disabled
func TestSecurityLayerRequired(t *testing.T) {
	// Validator always has security enabled
	validator := NewValidator()
	if validator.patterns == nil {
		t.Error("security patterns not initialized")
	}

	// Even with permissive settings, validator still blocks injections
	malicious := "'; DROP TABLE users--"
	valid, _ := validator.ValidateInput(malicious, InputTypeSQL)
	if valid {
		t.Error("security layer should have blocked SQL injection")
	}
}
