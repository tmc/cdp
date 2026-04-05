package main

import "testing"

func TestExtractExtensionID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"service worker", "chrome-extension://abcdef123456/background.js", "abcdef123456"},
		{"popup page", "chrome-extension://xyz789/popup.html", "xyz789"},
		{"bare id with slash", "chrome-extension://onlyid/", "onlyid"},
		{"no path", "chrome-extension://onlyid", "onlyid"},
		{"regular http", "https://example.com", ""},
		{"empty", "", ""},
		{"about blank", "about:blank", ""},
		{"chrome url", "chrome://extensions/", ""},
		{"nested path", "chrome-extension://ext123/pages/options.html", "ext123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractExtensionID(tt.url)
			if got != tt.want {
				t.Errorf("extractExtensionID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
