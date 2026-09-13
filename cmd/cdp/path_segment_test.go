package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidPathSegment(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"login", false},
		{"custom_screenshot", false},
		{"step-1.2", false},
		{"login flow", false},
		{"", true},
		{".", true},
		{"..", true},
		{"../escape", true},
		{"../../../../etc/foo", true},
		{"nested/name", true},
		{"/absolute", true},
		{"trailing/", true},
	}
	for _, tt := range tests {
		err := validPathSegment("name", tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("validPathSegment(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

// The traversal names are rejected because filepath.Join resolves them out of
// the base directory instead of failing, which is what the write sites do.
func TestValidPathSegmentRejectsNamesThatEscapeJoin(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"..", "../escape", "../../etc"} {
		if err := validPathSegment("name", name); err == nil {
			t.Errorf("validPathSegment(%q) = nil, want error", name)
		}
		joined := filepath.Join(base, name)
		if strings.HasPrefix(joined, base+string(filepath.Separator)) {
			t.Errorf("Join(base, %q) = %q stayed under base; the name is harmless and need not be rejected", name, joined)
		}
	}
}
