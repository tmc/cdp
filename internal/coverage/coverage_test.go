package coverage

import (
	"testing"
)

func TestOffsetToLineCol(t *testing.T) {
	tests := []struct {
		source     string
		offset     int
		wantLine   int
		wantCol    int
	}{
		{"hello", 0, 1, 1},
		{"hello", 4, 1, 5},
		{"hello\nworld", 6, 2, 1},
		{"hello\nworld", 7, 2, 2},
		{"a\nb\nc\n", 4, 3, 1},
		{"", 0, 1, 1},
	}
	for _, tt := range tests {
		line, col := offsetToLineCol(tt.source, tt.offset)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("offsetToLineCol(%q, %d) = (%d, %d), want (%d, %d)",
				tt.source, tt.offset, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		source string
		want   int
	}{
		{"", 0},
		{"hello", 1},
		{"a\nb", 2},
		{"a\nb\nc", 3},
		{"a\nb\nc\n", 4},
	}
	for _, tt := range tests {
		got := countLines(tt.source)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.source, got, tt.want)
		}
	}
}

func TestComputeDelta(t *testing.T) {
	c := New(false)

	before := &Snapshot{
		Name: "before",
		Scripts: map[string]*ScriptCoverage{
			"app.js": {
				URL:    "app.js",
				Source: "line1\nline2\nline3\nline4\nline5\n",
				Lines:  map[int]int{1: 1, 2: 1}, // lines 1-2 covered
			},
		},
	}

	after := &Snapshot{
		Name: "after",
		Scripts: map[string]*ScriptCoverage{
			"app.js": {
				URL:    "app.js",
				Source: "line1\nline2\nline3\nline4\nline5\n",
				Lines:  map[int]int{1: 1, 2: 1, 3: 2, 4: 1}, // lines 1-4 covered
			},
		},
	}

	delta := c.ComputeDelta(before, after)

	if delta.Name != "after" {
		t.Errorf("delta.Name = %q, want %q", delta.Name, "after")
	}

	sd, ok := delta.Scripts["app.js"]
	if !ok {
		t.Fatal("expected app.js in delta.Scripts")
	}

	if sd.CoveredBefore != 2 {
		t.Errorf("CoveredBefore = %d, want 2", sd.CoveredBefore)
	}
	if sd.CoveredAfter != 4 {
		t.Errorf("CoveredAfter = %d, want 4", sd.CoveredAfter)
	}
	if len(sd.NewlyCovered) != 2 {
		t.Errorf("len(NewlyCovered) = %d, want 2", len(sd.NewlyCovered))
	}
	if sd.NewlyCovered[3] != 2 {
		t.Errorf("NewlyCovered[3] = %d, want 2", sd.NewlyCovered[3])
	}
	if sd.NewlyCovered[4] != 1 {
		t.Errorf("NewlyCovered[4] = %d, want 1", sd.NewlyCovered[4])
	}
}

func TestComputeDelta_NewScript(t *testing.T) {
	c := New(false)

	before := &Snapshot{
		Name:    "before",
		Scripts: map[string]*ScriptCoverage{},
	}

	after := &Snapshot{
		Name: "after",
		Scripts: map[string]*ScriptCoverage{
			"new.js": {
				URL:    "new.js",
				Source: "a\nb\nc\n",
				Lines:  map[int]int{1: 1, 2: 1},
			},
		},
	}

	delta := c.ComputeDelta(before, after)
	sd := delta.Scripts["new.js"]
	if sd == nil {
		t.Fatal("expected new.js in delta")
	}
	if len(sd.NewlyCovered) != 2 {
		t.Errorf("NewlyCovered = %d, want 2", len(sd.NewlyCovered))
	}
	if sd.CoveredBefore != 0 {
		t.Errorf("CoveredBefore = %d, want 0", sd.CoveredBefore)
	}
}

func TestComputeDelta_NoCoverageChange(t *testing.T) {
	c := New(false)

	snap := &Snapshot{
		Name: "same",
		Scripts: map[string]*ScriptCoverage{
			"app.js": {
				URL:    "app.js",
				Source: "a\nb\n",
				Lines:  map[int]int{1: 1},
			},
		},
	}

	delta := c.ComputeDelta(snap, snap)
	sd := delta.Scripts["app.js"]
	if len(sd.NewlyCovered) != 0 {
		t.Errorf("expected no newly covered lines, got %d", len(sd.NewlyCovered))
	}
}

func TestSnapshotToLcov(t *testing.T) {
	snap := &Snapshot{
		Name: "test",
		Scripts: map[string]*ScriptCoverage{
			"app.js": {
				URL:    "app.js",
				Source: "function a() {}\nfunction b() {}\n",
				Functions: []FunctionCoverage{
					{Name: "a", StartLine: 1, EndLine: 1, HitCount: 3},
					{Name: "b", StartLine: 2, EndLine: 2, HitCount: 0},
				},
				Lines: map[int]int{1: 3, 2: 0},
			},
		},
	}

	lcov := SnapshotToLcov(snap)
	if !contains(lcov, "SF:app.js") {
		t.Error("missing SF:app.js")
	}
	if !contains(lcov, "FN:1,a") {
		t.Error("missing FN:1,a")
	}
	if !contains(lcov, "FNDA:3,a") {
		t.Error("missing FNDA:3,a")
	}
	if !contains(lcov, "FNDA:0,b") {
		t.Error("missing FNDA:0,b")
	}
	if !contains(lcov, "FNF:2") {
		t.Error("missing FNF:2")
	}
	if !contains(lcov, "FNH:1") {
		t.Error("missing FNH:1")
	}
	if !contains(lcov, "DA:1,3") {
		t.Error("missing DA:1,3")
	}
	if !contains(lcov, "DA:2,0") {
		t.Error("missing DA:2,0")
	}
	if !contains(lcov, "LH:1") {
		t.Error("missing LH:1")
	}
	if !contains(lcov, "end_of_record") {
		t.Error("missing end_of_record")
	}
}

func TestDeltaToLcov(t *testing.T) {
	delta := &Delta{
		Name: "test",
		Scripts: map[string]*ScriptDelta{
			"app.js": {
				URL:          "app.js",
				TotalLines:   10,
				NewlyCovered: map[int]int{3: 1, 5: 2},
			},
			"empty.js": {
				URL:          "empty.js",
				TotalLines:   5,
				NewlyCovered: map[int]int{},
			},
		},
	}

	lcov := DeltaToLcov(delta)
	if !contains(lcov, "SF:app.js") {
		t.Error("missing SF:app.js")
	}
	if !contains(lcov, "DA:3,1") {
		t.Error("missing DA:3,1")
	}
	if !contains(lcov, "DA:5,2") {
		t.Error("missing DA:5,2")
	}
	// empty.js should not appear (no newly covered lines).
	if contains(lcov, "SF:empty.js") {
		t.Error("empty.js should not appear in delta lcov")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
