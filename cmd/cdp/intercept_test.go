package main

import (
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// Empty/wildcard patterns.
		{"", "anything", true},
		{"*", "anything", true},
		{"*", "", true},

		// Substring match (no wildcards).
		{".map", "https://example.com/app.js.map", true},
		{".map", "https://example.com/app.js", false},
		{"example.com", "https://example.com/path", true},

		// Wildcard patterns.
		{"*.map", "bundle.js.map", true},
		{"*.map", "bundle.js", false},
		{"https://*.example.com/*", "https://cdn.example.com/app.js", true},
		{"https://*.example.com/*", "https://other.com/app.js", false},

		// Question mark.
		{"app.?s", "app.js", true},
		{"app.?s", "app.ts", true},
		{"app.?s", "app.css", false},

		// Sourcemap-specific patterns.
		{"*.js.map", "https://cdn.example.com/static/bundle.abc123.js.map", true},
		{"*/sourceMappingURL=*", "https://example.com/sourceMappingURL=data:foo", true},
	}
	for _, tt := range tests {
		got := matchPattern(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"*", "", true},
		{"*", "abc", true},
		{"a*c", "abc", true},
		{"a*c", "abbc", true},
		{"a*c", "ab", false},
		{"a?c", "abc", true},
		{"a?c", "abbc", false},
		{"**", "anything", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXYc", false},
	}
	for _, tt := range tests {
		got := wildcardMatch(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestInterceptor_AddRemoveRules(t *testing.T) {
	ic := newInterceptor()

	id1 := ic.addRule(interceptRule{URLPattern: "*.js", Stage: "request", Action: "block"})
	id2 := ic.addRule(interceptRule{URLPattern: "*.css", Stage: "response", Action: "modify"})

	rules := ic.getRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	if !ic.removeRule(id1) {
		t.Error("expected removeRule to return true")
	}
	rules = ic.getRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after removal, got %d", len(rules))
	}
	if rules[0].ID != id2 {
		t.Errorf("remaining rule ID = %q, want %q", rules[0].ID, id2)
	}

	n := ic.clearRules()
	if n != 1 {
		t.Errorf("clearRules returned %d, want 1", n)
	}
	if len(ic.getRules()) != 0 {
		t.Error("expected 0 rules after clear")
	}
}

func TestInterceptor_MatchRule(t *testing.T) {
	ic := newInterceptor()
	ic.addRule(interceptRule{URLPattern: "*.js.map", Stage: "request", Action: "fulfill"})
	ic.addRule(interceptRule{URLPattern: "*.css", Stage: "response", Action: "modify"})

	// Should match request-stage .map rule.
	r := ic.matchRule("https://example.com/bundle.js.map", "request")
	if r == nil || r.Action != "fulfill" {
		t.Errorf("expected fulfill rule for .map request, got %v", r)
	}

	// Should not match response-stage for .map (wrong stage).
	r = ic.matchRule("https://example.com/bundle.js.map", "response")
	if r != nil {
		t.Errorf("expected no match for .map response, got %v", r)
	}

	// Should match response-stage .css rule.
	r = ic.matchRule("https://example.com/style.css", "response")
	if r == nil || r.Action != "modify" {
		t.Errorf("expected modify rule for .css response, got %v", r)
	}
}
