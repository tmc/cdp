package main

import (
	"strings"
	"testing"
)

func TestLineDiff_Identical(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := lineDiff(lines, lines)
	if strings.Contains(got, "+") || strings.Contains(got, "-") {
		t.Errorf("identical lines should have no diff markers, got:\n%s", got)
	}
	if !strings.Contains(got, "  a\n") {
		t.Errorf("expected unchanged lines, got:\n%s", got)
	}
}

func TestLineDiff_Empty(t *testing.T) {
	got := lineDiff(nil, nil)
	if got != "" {
		t.Errorf("expected empty diff, got: %q", got)
	}
}

func TestLineDiff_AllAdded(t *testing.T) {
	got := lineDiff(nil, []string{"x", "y"})
	if !strings.Contains(got, "+ x") {
		t.Errorf("expected added line, got:\n%s", got)
	}
	if !strings.Contains(got, "+ y") {
		t.Errorf("expected added line, got:\n%s", got)
	}
}

func TestLineDiff_AllRemoved(t *testing.T) {
	got := lineDiff([]string{"x", "y"}, nil)
	if !strings.Contains(got, "- x") {
		t.Errorf("expected removed line, got:\n%s", got)
	}
}

func TestLineDiff_Modified(t *testing.T) {
	before := []string{"heading", `  button "Login"`, `  link "Home"`}
	after := []string{"heading", `  button "Logout"`, `  link "Home"`}
	got := lineDiff(before, after)
	// The modified line should show inline word diff.
	if !strings.Contains(got, "[-") || !strings.Contains(got, "{+") {
		t.Errorf("expected inline word diff markers, got:\n%s", got)
	}
	if !strings.Contains(got, "Login") && !strings.Contains(got, "Logout") {
		t.Errorf("expected Login/Logout in diff, got:\n%s", got)
	}
}

func TestLineDiff_AddedLine(t *testing.T) {
	before := []string{"a", "b"}
	after := []string{"a", "b", "c"}
	got := lineDiff(before, after)
	if !strings.Contains(got, "+ c") {
		t.Errorf("expected added line c, got:\n%s", got)
	}
	// a and b should be unchanged.
	if strings.Count(got, "  a\n") != 1 || strings.Count(got, "  b\n") != 1 {
		t.Errorf("expected unchanged a and b, got:\n%s", got)
	}
}

func TestWordDiff_Identical(t *testing.T) {
	got := wordDiff("hello world", "hello world")
	if got != "hello world" {
		t.Errorf("identical input should return as-is, got: %q", got)
	}
}

func TestWordDiff_SingleWordChange(t *testing.T) {
	got := wordDiff(`button "Login"`, `button "Logout"`)
	if !strings.Contains(got, "[-") || !strings.Contains(got, "{+") {
		t.Errorf("expected diff markers, got: %q", got)
	}
}

func TestWordDiff_Added(t *testing.T) {
	got := wordDiff("a b", "a b c")
	if !strings.Contains(got, "{+") {
		t.Errorf("expected added marker, got: %q", got)
	}
}

func TestWordTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 3}, // "hello", " ", "world"
		{"a  b", 3},        // "a", "  ", "b"
	}
	for _, tt := range tests {
		got := wordTokens(tt.input)
		if len(got) != tt.want {
			t.Errorf("wordTokens(%q) = %d tokens %v, want %d", tt.input, len(got), got, tt.want)
		}
	}
}

func TestDiffDom_NilTrees(t *testing.T) {
	got := diffDom(nil, nil)
	if got != "" {
		t.Errorf("nil trees should produce empty diff, got: %q", got)
	}
}

func TestDiffDom_IdenticalTrees(t *testing.T) {
	tree := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "heading", Name: "Title"},
			{Role: "text", Name: "Hello"},
		},
	}
	got := diffDom(tree, tree)
	if strings.Contains(got, "[-") || strings.Contains(got, "{+") || strings.Contains(got, "+ ") || strings.Contains(got, "- ") {
		t.Errorf("identical trees should have no diff markers, got:\n%s", got)
	}
}

func TestDiffDom_AddedChild(t *testing.T) {
	before := &domNode{
		Role:     "document",
		Children: []*domNode{{Role: "heading", Name: "Title"}},
	}
	after := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "heading", Name: "Title"},
			{Role: "button", Name: "Submit"},
		},
	}
	got := diffDom(before, after)
	if !strings.Contains(got, "+") {
		t.Errorf("expected added marker for new button, got:\n%s", got)
	}
	if !strings.Contains(got, "Submit") {
		t.Errorf("expected Submit in diff, got:\n%s", got)
	}
}

func TestDiffDom_RemovedChild(t *testing.T) {
	before := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "heading", Name: "Title"},
			{Role: "button", Name: "Delete"},
		},
	}
	after := &domNode{
		Role:     "document",
		Children: []*domNode{{Role: "heading", Name: "Title"}},
	}
	got := diffDom(before, after)
	if !strings.Contains(got, "- ") {
		t.Errorf("expected removed marker, got:\n%s", got)
	}
	if !strings.Contains(got, "Delete") {
		t.Errorf("expected Delete in diff, got:\n%s", got)
	}
}

func TestDiffDom_ModifiedAttribute(t *testing.T) {
	before := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "button", Name: "Login"},
		},
	}
	after := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "button", Name: "Logout"},
		},
	}
	got := diffDom(before, after)
	// Should show inline word diff for the name change.
	if !strings.Contains(got, "Login") || !strings.Contains(got, "Logout") {
		t.Errorf("expected Login→Logout change, got:\n%s", got)
	}
}

func TestDiffDom_CompletelyDifferent(t *testing.T) {
	before := &domNode{Role: "document", Children: []*domNode{{Role: "heading", Name: "Old"}}}
	after := &domNode{Role: "navigation", Children: []*domNode{{Role: "link", Name: "New"}}}
	got := diffDom(before, after)
	if !strings.Contains(got, "-") && !strings.Contains(got, "+") {
		t.Errorf("completely different trees should show changes, got:\n%s", got)
	}
}

func TestRenderTree(t *testing.T) {
	tree := &domNode{
		Role: "document",
		Children: []*domNode{
			{Role: "heading", Name: "Title"},
			{Role: "text", Name: "Hello"},
		},
	}
	lines := renderTree(tree, 0)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != `document` {
		t.Errorf("line 0 = %q, want %q", lines[0], `document`)
	}
	if lines[1] != `  heading "Title"` {
		t.Errorf("line 1 = %q, want %q", lines[1], `  heading "Title"`)
	}
}

func TestFormatNode(t *testing.T) {
	n := &domNode{
		Role:  "button",
		Name:  "Submit",
		Value: "ok",
		Props: map[string]string{"focused": "true"},
	}
	got := formatNode(n)
	if !strings.Contains(got, `button`) || !strings.Contains(got, `"Submit"`) || !strings.Contains(got, `value="ok"`) {
		t.Errorf("formatNode = %q, missing expected parts", got)
	}
}

func TestCountNodes(t *testing.T) {
	tree := &domNode{
		Role: "a",
		Children: []*domNode{
			{Role: "b"},
			{Role: "c", Children: []*domNode{{Role: "d"}}},
		},
	}
	if got := countNodes(tree); got != 4 {
		t.Errorf("countNodes = %d, want 4", got)
	}
	if got := countNodes(nil); got != 0 {
		t.Errorf("countNodes(nil) = %d, want 0", got)
	}
}
