package scrub

import (
	"strings"
	"testing"
)

func TestScrubText(t *testing.T) {
	s := New()
	if !s.Enabled() {
		t.Fatal("expected scrubber to be enabled")
	}
	tests := []struct {
		name      string
		input     string
		wantRedacted bool
		wantSubstr   string // substring expected in output
	}{
		{
			name:         "aws access key",
			input:        `aws_access_key_id = "AKIAIOSFODNN7EXAMPLE"`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:",
		},
		{
			name:         "google api key",
			input:        `apiKey: "AIzaSyA1234567890abcdefghijklmnopqrstuv"`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:",
		},
		{
			name:         "no secret",
			input:        `const greeting = "hello world"`,
			wantRedacted: false,
		},
		{
			name:         "generic api key assignment",
			input:        `api_key = "sk_live_abcdefghijklmnopqrstuvwx"`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:",
		},
		{
			name:         "github pat",
			input:        `token: "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef12"`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:",
		},
		{
			name:         "slack token",
			input:        `SLACK_TOKEN=xoxb-123456789012-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:",
		},
		{
			name:         "age secret key",
			input:        `AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ`,
			wantRedacted: true,
			wantSubstr:   "[REDACTED:age-secret-key]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, count := s.ScrubText(tt.input)
			if tt.wantRedacted && count == 0 {
				t.Errorf("expected redactions, got 0; result: %s", result)
			}
			if !tt.wantRedacted && count != 0 {
				t.Errorf("expected no redactions, got %d; result: %s", count, result)
			}
			if tt.wantSubstr != "" && count > 0 {
				if !strings.Contains(result, tt.wantSubstr) {
					t.Errorf("expected result to contain %q, got: %s", tt.wantSubstr, result)
				}
			}
		})
	}
}

func TestScrubHeaderValue(t *testing.T) {
	s := New()
	tests := []struct {
		name  string
		hdr   string
		value string
		want  string
	}{
		{"authorization", "Authorization", "Bearer abc123", "[REDACTED]"},
		{"cookie", "Cookie", "session=xyz", "[REDACTED]"},
		{"set-cookie", "Set-Cookie", "id=abc; Path=/", "[REDACTED]"},
		{"x-api-key", "X-Api-Key", "secret123", "[REDACTED]"},
		{"x-auth-token", "X-Auth-Token", "tok", "[REDACTED]"},
		{"x-csrf-token", "X-CSRF-Token", "csrf", "[REDACTED]"},
		{"proxy-authorization", "Proxy-Authorization", "Basic abc", "[REDACTED]"},
		{"content-type passthrough", "Content-Type", "application/json", "application/json"},
		{"accept passthrough", "Accept", "text/html", "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubHeaderValue(tt.hdr, tt.value)
			if got != tt.want {
				t.Errorf("ScrubHeaderValue(%q, %q) = %q, want %q", tt.hdr, tt.value, got, tt.want)
			}
		})
	}
}

func TestScrubQueryParam(t *testing.T) {
	s := New()
	tests := []struct {
		name  string
		param string
		value string
		want  string
	}{
		{"token", "token", "abc123", "[REDACTED]"},
		{"api_key", "api_key", "key123", "[REDACTED]"},
		{"apikey", "apikey", "key456", "[REDACTED]"},
		{"secret", "secret", "s3cr3t", "[REDACTED]"},
		{"password", "password", "pass", "[REDACTED]"},
		{"access_token", "access_token", "at123", "[REDACTED]"},
		{"refresh_token", "refresh_token", "rt123", "[REDACTED]"},
		{"client_secret", "client_secret", "cs123", "[REDACTED]"},
		{"session_id", "session_id", "sid", "[REDACTED]"},
		{"page passthrough", "page", "1", "1"},
		{"q passthrough", "q", "search term", "search term"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.ScrubQueryParam(tt.param, tt.value)
			if got != tt.want {
				t.Errorf("ScrubQueryParam(%q, %q) = %q, want %q", tt.param, tt.value, got, tt.want)
			}
		})
	}
}

func TestScrubberDisabled(t *testing.T) {
	s := New()
	s.enabled = false
	if s.Enabled() {
		t.Error("expected disabled")
	}
	text, count := s.ScrubText(`AKIAIOSFODNN7EXAMPLE`)
	if count != 0 || text != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("disabled scrubber should not modify text")
	}
	if got := s.ScrubHeaderValue("Authorization", "Bearer x"); got != "Bearer x" {
		t.Errorf("disabled scrubber should pass through header value")
	}
	if got := s.ScrubQueryParam("token", "abc"); got != "abc" {
		t.Errorf("disabled scrubber should pass through query param")
	}
}

