package browser

import (
	"testing"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"at prefix", "@e1", "e1"},
		{"at prefix with larger number", "@e123", "e123"},
		{"ref= prefix", "ref=e1", "e1"},
		{"ref= prefix with larger number", "ref=e42", "e42"},
		{"bare ref", "e1", "e1"},
		{"bare ref larger", "e99", "e99"},
		{"not a ref - css selector", "#button", ""},
		{"not a ref - class", ".submit", ""},
		{"not a ref - element", "button", ""},
		{"not a ref - wrong format", "ee1", ""},
		{"not a ref - no number", "e", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRef(tt.input)
			if got != tt.want {
				t.Errorf("ParseRef(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsInteractiveRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{"button", true},
		{"link", true},
		{"textbox", true},
		{"checkbox", true},
		{"radio", true},
		{"combobox", true},
		{"slider", true},
		{"searchbox", true},
		{"switch", true},
		{"tab", true},
		{"BUTTON", true}, // case insensitive
		{"Link", true},
		{"heading", false},
		{"paragraph", false},
		{"generic", false},
		{"list", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := IsInteractiveRole(tt.role)
			if got != tt.want {
				t.Errorf("IsInteractiveRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestIsContentRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{"heading", true},
		{"cell", true},
		{"listitem", true},
		{"article", true},
		{"region", true},
		{"main", true},
		{"navigation", true},
		{"HEADING", true}, // case insensitive
		{"button", false},
		{"link", false},
		{"generic", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := IsContentRole(tt.role)
			if got != tt.want {
				t.Errorf("IsContentRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestIsStructuralRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{"generic", true},
		{"group", true},
		{"list", true},
		{"table", true},
		{"row", true},
		{"menu", true},
		{"toolbar", true},
		{"document", true},
		{"presentation", true},
		{"none", true},
		{"GENERIC", true}, // case insensitive
		{"button", false},
		{"heading", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := IsStructuralRole(tt.role)
			if got != tt.want {
				t.Errorf("IsStructuralRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestBuildSelector(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		elemName string
		want     string
	}{
		{
			name:     "button with name",
			role:     "button",
			elemName: "Submit",
			want:     `getByRole('button', { name: "Submit", exact: true })`,
		},
		{
			name:     "link without name",
			role:     "link",
			elemName: "",
			want:     `getByRole('link')`,
		},
		{
			name:     "textbox with special chars",
			role:     "textbox",
			elemName: `Email "address"`,
			want:     `getByRole('textbox', { name: "Email \"address\"", exact: true })`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelector(tt.role, tt.elemName)
			if got != tt.want {
				t.Errorf("buildSelector(%q, %q) = %q, want %q", tt.role, tt.elemName, got, tt.want)
			}
		})
	}
}

func TestRoleNameTracker(t *testing.T) {
	tracker := newRoleNameTracker()

	// First button "Submit"
	idx1 := tracker.getNextIndex("button", "Submit")
	tracker.trackRef("button", "Submit", "e1")
	if idx1 != 0 {
		t.Errorf("first button Submit index = %d, want 0", idx1)
	}

	// Second button "Submit"
	idx2 := tracker.getNextIndex("button", "Submit")
	tracker.trackRef("button", "Submit", "e2")
	if idx2 != 1 {
		t.Errorf("second button Submit index = %d, want 1", idx2)
	}

	// First button "Cancel"
	idx3 := tracker.getNextIndex("button", "Cancel")
	tracker.trackRef("button", "Cancel", "e3")
	if idx3 != 0 {
		t.Errorf("first button Cancel index = %d, want 0", idx3)
	}

	// Check duplicates
	duplicates := tracker.getDuplicateKeys()
	if !duplicates["button:Submit"] {
		t.Error("button:Submit should be a duplicate")
	}
	if duplicates["button:Cancel"] {
		t.Error("button:Cancel should not be a duplicate")
	}
}

func TestGetSnapshotStats(t *testing.T) {
	tree := `- heading "Test" [ref=e1] [level=1]
- button "Submit" [ref=e2]
- textbox "Email" [ref=e3]`

	refs := &RefMap{
		Refs: map[string]*RefEntry{
			"e1": {Role: "heading", Name: "Test"},
			"e2": {Role: "button", Name: "Submit"},
			"e3": {Role: "textbox", Name: "Email"},
		},
	}

	stats := GetSnapshotStats(tree, refs)

	if stats["refs"] != 3 {
		t.Errorf("refs = %d, want 3", stats["refs"])
	}
	if stats["interactive"] != 2 { // button and textbox are interactive
		t.Errorf("interactive = %d, want 2", stats["interactive"])
	}
	if stats["lines"] != 3 {
		t.Errorf("lines = %d, want 3", stats["lines"])
	}
}

func TestRefMapLookup(t *testing.T) {
	refMap := &RefMap{
		Refs: map[string]*RefEntry{
			"e1": {Selector: "getByRole('button', { name: \"Submit\", exact: true })", Role: "button", Name: "Submit", Nth: 0},
			"e2": {Selector: "getByRole('textbox', { name: \"Email\", exact: true })", Role: "textbox", Name: "Email", Nth: 0},
			"e3": {Selector: "getByRole('button', { name: \"Submit\", exact: true })", Role: "button", Name: "Submit", Nth: 1},
		},
	}

	// Test basic lookup
	entry, ok := refMap.Refs["e1"]
	if !ok {
		t.Fatal("e1 not found")
	}
	if entry.Role != "button" {
		t.Errorf("e1 role = %q, want button", entry.Role)
	}
	if entry.Name != "Submit" {
		t.Errorf("e1 name = %q, want Submit", entry.Name)
	}

	// Test nth disambiguation
	entry3, ok := refMap.Refs["e3"]
	if !ok {
		t.Fatal("e3 not found")
	}
	if entry3.Nth != 1 {
		t.Errorf("e3 nth = %d, want 1", entry3.Nth)
	}

	// Test missing ref
	_, ok = refMap.Refs["e99"]
	if ok {
		t.Error("e99 should not exist")
	}
}
