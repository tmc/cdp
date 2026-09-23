package wsorigin

import (
	"net/http"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{"no origin", "", true},
		{"devtools frontend", "devtools://devtools", true},
		{"chrome extension", "chrome-extension://abcdefghijklmnop", false},
		{"http localhost", "http://localhost:9229", true},
		{"http localhost subdomain", "http://app.localhost:8080", true},
		{"http 127.0.0.1", "http://127.0.0.1:9230", true},
		{"https ipv6 loopback", "https://[::1]:9222", true},
		{"remote web page", "https://evil.example.com", false},
		{"lan host", "http://192.168.1.10:8080", false},
		{"localhost lookalike", "http://localhost.example.com", false},
		{"file scheme", "file://", false},
		{"null origin", "null", false},
		{"garbage origin", "://not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := Check(r); got != tt.want {
				t.Errorf("Check(Origin=%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}
