package browser

import "testing"

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name          string
		text, pattern string
		caseSensitive bool
		want          bool
	}{
		{"exact", "hello", "hello", true, true},
		{"regex", "hello world", "wor.d", true, true},
		{"case sensitive miss", "Hello", "hello", true, false},
		{"case insensitive exact", "Hello", "hello", false, true},
		{"case insensitive regex", "HELLO world", "^hello", false, true},
	}
	var p Page
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.matchesPattern(tt.text, tt.pattern, tt.caseSensitive)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("matchesPattern(%q, %q, %v) = %v, want %v", tt.text, tt.pattern, tt.caseSensitive, got, tt.want)
			}
		})
	}
}

func TestMatchesURLPatternInvalid(t *testing.T) {
	var p Page
	if p.matchesURLPattern("wss://example.com/", "(") {
		t.Error("invalid pattern matched")
	}
}
