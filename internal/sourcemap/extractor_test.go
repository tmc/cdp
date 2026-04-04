package sourcemap

import (
	"testing"
)

func TestExtractChunks_Empty(t *testing.T) {
	if chunks := ExtractChunks("", nil, 0); chunks != nil {
		t.Errorf("expected nil for empty input, got %v", chunks)
	}
	if chunks := ExtractChunks("hello", nil, 0); chunks != nil {
		t.Errorf("expected nil for no ranges, got %v", chunks)
	}
	if chunks := ExtractChunks("hello", []CoverageRange{{0, 5, 0}}, 0); chunks != nil {
		t.Errorf("expected nil for zero-count ranges, got %v", chunks)
	}
}

func TestExtractChunks_SingleRange(t *testing.T) {
	source := "function hello() {\n  console.log('hi');\n}\n"
	ranges := []CoverageRange{{0, len(source), 1}}

	chunks := ExtractChunks(source, ranges, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.Code != source {
		t.Errorf("code = %q, want %q", c.Code, source)
	}
	if c.StartLine != 1 || c.StartCol != 1 {
		t.Errorf("start = (%d, %d), want (1, 1)", c.StartLine, c.StartCol)
	}
	if c.HitCount != 1 {
		t.Errorf("hit count = %d, want 1", c.HitCount)
	}
}

func TestExtractChunks_MergesOverlapping(t *testing.T) {
	source := "abcdefghij" // 10 bytes
	ranges := []CoverageRange{
		{0, 5, 1},
		{3, 8, 2}, // overlaps with first
	}
	chunks := ExtractChunks(source, ranges, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 merged chunk, got %d", len(chunks))
	}
	if chunks[0].Code != "abcdefgh" {
		t.Errorf("merged code = %q, want %q", chunks[0].Code, "abcdefgh")
	}
}

func TestExtractChunks_MultipleDisjoint(t *testing.T) {
	source := "aaaa\nbbbb\ncccc\ndddd\n"
	ranges := []CoverageRange{
		{0, 4, 1},   // "aaaa"
		{10, 14, 1},  // "cccc"
	}
	chunks := ExtractChunks(source, ranges, 0)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Code != "aaaa" {
		t.Errorf("chunk 0 = %q, want %q", chunks[0].Code, "aaaa")
	}
	if chunks[1].Code != "cccc" {
		t.Errorf("chunk 1 = %q, want %q", chunks[1].Code, "cccc")
	}
}

func TestExtractChunks_WithContext(t *testing.T) {
	source := "line1\nline2\nline3\nline4\nline5\n"
	// Chunk covers line3 only (offset 12..17 = "line3")
	ranges := []CoverageRange{{12, 17, 1}}
	chunks := ExtractChunks(source, ranges, 1)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.StartLine != 3 {
		t.Errorf("start line = %d, want 3", c.StartLine)
	}
	if c.ContextBefore == "" {
		t.Error("expected non-empty context before")
	}
	if c.ContextAfter == "" {
		t.Error("expected non-empty context after")
	}
}

func TestExtractChunks_LineCol(t *testing.T) {
	source := "ab\ncd\nef\n"
	// Range covering "cd" (bytes 3-5).
	ranges := []CoverageRange{{3, 5, 1}}
	chunks := ExtractChunks(source, ranges, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.StartLine != 2 || c.StartCol != 1 {
		t.Errorf("start = (%d, %d), want (2, 1)", c.StartLine, c.StartCol)
	}
	if c.EndLine != 2 || c.EndCol != 3 {
		t.Errorf("end = (%d, %d), want (2, 3)", c.EndLine, c.EndCol)
	}
}

func TestMergeRanges(t *testing.T) {
	tests := []struct {
		name   string
		input  []CoverageRange
		want   int
		wantEnd int
	}{
		{"empty", nil, 0, 0},
		{"single", []CoverageRange{{0, 10, 1}}, 1, 10},
		{"disjoint", []CoverageRange{{0, 5, 1}, {10, 15, 1}}, 2, 15},
		{"overlapping", []CoverageRange{{0, 10, 1}, {5, 15, 1}}, 1, 15},
		{"adjacent", []CoverageRange{{0, 10, 1}, {10, 20, 1}}, 1, 20},
		{"contained", []CoverageRange{{0, 20, 1}, {5, 10, 1}}, 1, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeRanges(tt.input)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d", len(got), tt.want)
			}
			if len(got) > 0 && got[len(got)-1].EndOffset != tt.wantEnd {
				t.Errorf("last EndOffset = %d, want %d", got[len(got)-1].EndOffset, tt.wantEnd)
			}
		})
	}
}

func TestBuildLineOffsets(t *testing.T) {
	offsets := buildLineOffsets("ab\ncd\nef")
	want := []int{0, 3, 6}
	if len(offsets) != len(want) {
		t.Fatalf("len = %d, want %d", len(offsets), len(want))
	}
	for i, w := range want {
		if offsets[i] != w {
			t.Errorf("offsets[%d] = %d, want %d", i, offsets[i], w)
		}
	}
}

func TestOffsetToLineCol(t *testing.T) {
	offsets := buildLineOffsets("ab\ncd\nef")
	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{1, 1, 2},
		{2, 1, 3}, // the newline char
		{3, 2, 1},
		{5, 2, 3},
		{6, 3, 1},
	}
	for _, tt := range tests {
		l, c := offsetToLineCol(offsets, tt.offset)
		if l != tt.wantLine || c != tt.wantCol {
			t.Errorf("offsetToLineCol(%d) = (%d, %d), want (%d, %d)",
				tt.offset, l, c, tt.wantLine, tt.wantCol)
		}
	}
}

func TestSplitFunctions(t *testing.T) {
	source := "function a() {}\nfunction b() {}\nfunction c() {}\n"
	fns := []FunctionRange{
		{"a", 0, 16, 3},
		{"b", 17, 33, 0}, // not executed
		{"c", 34, 50, 1},
	}
	chunks := SplitFunctions(source, fns)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 executed functions, got %d", len(chunks))
	}
	if chunks[0].HitCount != 3 {
		t.Errorf("chunk 0 hit count = %d, want 3", chunks[0].HitCount)
	}
}

func TestLineMap(t *testing.T) {
	chunks := []CodeChunk{
		{StartLine: 2, EndLine: 4},
		{StartLine: 8, EndLine: 10},
	}
	m := LineMap(12, chunks)
	if m[1] != -1 {
		t.Errorf("line 1 should be unmapped, got %d", m[1])
	}
	if m[2] != 0 || m[3] != 0 || m[4] != 0 {
		t.Errorf("lines 2-4 should map to chunk 0")
	}
	if m[5] != -1 {
		t.Errorf("line 5 should be unmapped, got %d", m[5])
	}
	if m[8] != 1 || m[9] != 1 || m[10] != 1 {
		t.Errorf("lines 8-10 should map to chunk 1")
	}
}

func TestCountLinesInString(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 2},
		{"a\nb", 2},
		{"a\nb\nc\n", 4},
	}
	for _, tt := range tests {
		got := CountLinesInString(tt.s)
		if got != tt.want {
			t.Errorf("CountLinesInString(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}
