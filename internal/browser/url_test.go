package browser

import "testing"

func TestNormalizeNavigateURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"http unchanged", "https://example.com/a b", "https://example.com/a b"},
		{"safe data unchanged", "data:text/html,hello", "data:text/html,hello"},
		{"base64 unchanged", "data:text/html;base64,PHA+", "data:text/html;base64,PHA+"},
		{"escapes markup", "data:text/html,<p>a b</p>", "data:text/html,%3Cp%3Ea%20b%3C/p%3E"},
		{"keeps existing escapes", "data:text/html,<p>a%20b</p>", "data:text/html,%3Cp%3Ea%20b%3C/p%3E"},
		{"escapes fragment", "data:text/html,<a href=#x>", "data:text/html,%3Ca%20href=%23x%3E"},
		{"escapes non-ASCII", "data:text/html,<p>é</p>", "data:text/html,%3Cp%3E%C3%A9%3C/p%3E"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeNavigateURL(tt.in); got != tt.want {
				t.Errorf("normalizeNavigateURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
