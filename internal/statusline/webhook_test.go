package statusline

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateWebhookURL_SSRFProtection verifies that loopback / private /
// reserved / metadata addresses are rejected to prevent SSRF (H-1, C-3).
func TestValidateWebhookURL_SSRFProtection(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errSub  string // optional substring to look for in the error message
	}{
		// SSRF: loopback
		{name: "reject https loopback v4", url: "https://127.0.0.1/metrics", wantErr: true, errSub: "reserved"},
		{name: "reject https loopback v6", url: "https://[::1]/metrics", wantErr: true, errSub: "reserved"},

		// SSRF: RFC1918 private ranges
		{name: "reject 10/8", url: "https://10.0.0.1/m", wantErr: true, errSub: "reserved"},
		{name: "reject 10/8 deep", url: "https://10.255.255.254/m", wantErr: true, errSub: "reserved"},
		{name: "reject 172.16/12 low", url: "https://172.16.0.1/m", wantErr: true, errSub: "reserved"},
		{name: "reject 172.16/12 high", url: "https://172.31.255.254/m", wantErr: true, errSub: "reserved"},
		{name: "reject 192.168/16", url: "https://192.168.1.1/m", wantErr: true, errSub: "reserved"},

		// SSRF: AWS / cloud metadata services
		{name: "reject aws imds v4", url: "https://169.254.169.254/latest/meta-data/", wantErr: true, errSub: "reserved"},
		{name: "reject ipv6 link-local", url: "https://[fe80::1]/m", wantErr: true, errSub: "reserved"},

		// Scheme enforcement
		{name: "reject http scheme", url: "http://api.example.com/m", wantErr: true, errSub: "https"},
		{name: "reject ftp scheme", url: "ftp://api.example.com/m", wantErr: true, errSub: "https"},

		// Malformed
		{name: "reject blank URL", url: "", wantErr: true},
		{name: "reject malformed", url: "://nope", wantErr: true},
		{name: "reject missing host", url: "https:///path", wantErr: true, errSub: "hostname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got: %v", tt.url, err)
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), tt.errSub)
			}
		})
	}
}

// TestValidateWebhookURL_DNSRebinding verifies hostnames whose A records
// resolve to private IPs are rejected (DNS rebinding mitigation).
func TestValidateWebhookURL_DNSRebinding(t *testing.T) {
	// localhost commonly resolves to 127.0.0.1 / ::1; both must be rejected.
	if err := validateWebhookURL("https://localhost/m"); err == nil {
		t.Fatalf("expected DNS rebinding rejection for https://localhost, got nil")
	}
}

// TestNewWebhookSource_DisabledOnInvalid verifies the constructor disables
// the source when the URL fails validation.
func TestNewWebhookSource_DisabledOnInvalid(t *testing.T) {
	cases := []string{
		"http://example.com/m",          // not https
		"https://127.0.0.1/m",           // loopback
		"https://169.254.169.254/m",     // metadata
		"https://10.0.0.1/m",            // private
	}
	for _, u := range cases {
		ws := NewWebhookSource(u, "")
		if ws.IsAvailable() {
			t.Errorf("expected webhook to be disabled for %q", u)
		}
	}
}

// TestWebhookPoll_PayloadValidation verifies the poller rejects malformed
// or out-of-range payloads.
func TestWebhookPoll_PayloadValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		errSub  string
	}{
		{
			name:    "missing required fields",
			body:    `{"model":"opus"}`,
			wantErr: true,
			errSub:  "required",
		},
		{
			name:    "malformed JSON",
			body:    `{not json`,
			wantErr: true,
			errSub:  "parse",
		},
		{
			name:    "unknown fields rejected",
			body:    `{"input_tokens":1,"output_tokens":2,"surprise":"hi"}`,
			wantErr: true,
		},
		{
			name:    "negative tokens",
			body:    `{"input_tokens":-1,"output_tokens":2}`,
			wantErr: true,
			errSub:  "negative",
		},
		{
			name:    "oversized tokens",
			body:    `{"input_tokens":1,"output_tokens":99999999}`,
			wantErr: true,
			errSub:  "maximum",
		},
		{
			name:    "valid minimal payload",
			body:    `{"input_tokens":100,"output_tokens":50}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			// Bypass URL validation for unit-level tests by constructing directly.
			ws := &WebhookSource{
				url:     srv.URL,
				enabled: true,
				client:  srv.Client(),
			}

			_, err := ws.Poll()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && tt.errSub != "" && err != nil && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("error %q missing substring %q", err.Error(), tt.errSub)
			}
		})
	}
}

// TestIsRestrictedIP exercises the helper directly for clarity.
func TestIsRestrictedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.1.2.3", true},
		{"172.16.5.6", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Errorf("could not parse IP %s", c.ip)
			continue
		}
		if got := isRestrictedIP(ip); got != c.want {
			t.Errorf("isRestrictedIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}
