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

func TestValidStorageArea(t *testing.T) {
	tests := []struct {
		area string
		want bool
	}{
		{"local", true},
		{"sync", true},
		{"session", true},
		{"managed", true},
		{"", false},
		{"invalid", false},
		{"LOCAL", false},
	}
	for _, tt := range tests {
		t.Run(tt.area, func(t *testing.T) {
			got := validStorageArea(tt.area)
			if got != tt.want {
				t.Errorf("validStorageArea(%q) = %v, want %v", tt.area, got, tt.want)
			}
		})
	}
}

func TestStrFromMap(t *testing.T) {
	m := map[string]any{
		"name":    "Test Extension",
		"version": "1.0",
		"count":   42,
	}
	tests := []struct {
		key  string
		want string
	}{
		{"name", "Test Extension"},
		{"version", "1.0"},
		{"count", ""},  // not a string
		{"missing", ""}, // not present
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := strFromMap(m, tt.key)
			if got != tt.want {
				t.Errorf("strFromMap(m, %q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
