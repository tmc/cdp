package sourcemap

import (
	"encoding/json"
	"testing"
)

func TestEncodeVLQ(t *testing.T) {
	// Known values from the sourcemap spec:
	// 0 → "A" (vlq=0, digit=0)
	// 1 → "C" (vlq=2, digit=2)
	// -1 → "D" (vlq=3, digit=3)
	// 16 → "gB" (vlq=32: first digit=0|0x20=32→'g', then 1→'B')
	tests := []struct {
		value int
		want  string
	}{
		{0, "A"},
		{1, "C"},
		{-1, "D"},
		{2, "E"},
		{-2, "F"},
		{15, "e"},
		{16, "gB"},
		{-16, "hB"},
		{100, "oG"},
		{-100, "pG"},
	}
	for _, tt := range tests {
		got := EncodeVLQ(tt.value)
		if got != tt.want {
			t.Errorf("EncodeVLQ(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestDecodeVLQ(t *testing.T) {
	tests := []struct {
		input    string
		want     int
		consumed int
	}{
		{"A", 0, 1},
		{"C", 1, 1},
		{"D", -1, 1},
		{"E", 2, 1},
		{"F", -2, 1},
		{"e", 15, 1},
		{"gB", 16, 2},
		{"hB", -16, 2},
		{"oG", 100, 2},
		{"pG", -100, 2},
	}
	for _, tt := range tests {
		got, consumed := DecodeVLQ(tt.input)
		if got != tt.want || consumed != tt.consumed {
			t.Errorf("DecodeVLQ(%q) = (%d, %d), want (%d, %d)",
				tt.input, got, consumed, tt.want, tt.consumed)
		}
	}
}

func TestVLQ_RoundTrip(t *testing.T) {
	values := []int{0, 1, -1, 15, -15, 16, -16, 100, -100, 1000, -1000, 65535, -65535}
	for _, v := range values {
		encoded := EncodeVLQ(v)
		decoded, consumed := DecodeVLQ(encoded)
		if decoded != v || consumed != len(encoded) {
			t.Errorf("roundtrip(%d): encoded=%q, decoded=%d, consumed=%d (want %d, %d)",
				v, encoded, decoded, consumed, v, len(encoded))
		}
	}
}

func TestGenerateV3_Empty(t *testing.T) {
	data, err := GenerateV3("out.js", []string{"a.js"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sm sourceMapV3
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	if sm.Version != 3 {
		t.Errorf("version = %d, want 3", sm.Version)
	}
	if sm.Mappings != "" {
		t.Errorf("mappings = %q, want empty", sm.Mappings)
	}
}

func TestGenerateV3_SingleMapping(t *testing.T) {
	mappings := []Mapping{
		{GeneratedLine: 0, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 0, OriginalCol: 0, NameIdx: -1},
	}
	data, err := GenerateV3("out.js", []string{"src/app.js"}, []string{"console.log('hi');\n"}, mappings, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sm sourceMapV3
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	if sm.Version != 3 {
		t.Errorf("version = %d, want 3", sm.Version)
	}
	if len(sm.Sources) != 1 || sm.Sources[0] != "src/app.js" {
		t.Errorf("sources = %v", sm.Sources)
	}
	if len(sm.SourcesContent) != 1 {
		t.Errorf("sourcesContent = %v", sm.SourcesContent)
	}
	// Mapping 0,0,0,0 → all zeros → "AAAA"
	if sm.Mappings != "AAAA" {
		t.Errorf("mappings = %q, want %q", sm.Mappings, "AAAA")
	}
}

func TestGenerateV3_MultiLine(t *testing.T) {
	mappings := []Mapping{
		{GeneratedLine: 0, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 0, OriginalCol: 0, NameIdx: -1},
		{GeneratedLine: 1, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 1, OriginalCol: 0, NameIdx: -1},
		{GeneratedLine: 2, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 2, OriginalCol: 0, NameIdx: -1},
	}
	data, err := GenerateV3("out.js", []string{"a.js"}, nil, mappings, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sm sourceMapV3
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	// Three lines, each mapping to corresponding original line.
	// Line 0: col=0, src=0, origLine=0, origCol=0 → "AAAA"
	// Line 1: col=0, src=0(+0), origLine=1(+1), origCol=0 → "AACA"
	// Line 2: col=0, src=0(+0), origLine=2(+1), origCol=0 → "AACA"
	if sm.Mappings != "AAAA;AACA;AACA" {
		t.Errorf("mappings = %q, want %q", sm.Mappings, "AAAA;AACA;AACA")
	}
}

func TestGenerateV3_WithNames(t *testing.T) {
	mappings := []Mapping{
		{GeneratedLine: 0, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 0, OriginalCol: 0, NameIdx: 0},
	}
	data, err := GenerateV3("out.js", []string{"a.js"}, nil, mappings, []string{"myFunc"})
	if err != nil {
		t.Fatal(err)
	}
	var sm sourceMapV3
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	if len(sm.Names) != 1 || sm.Names[0] != "myFunc" {
		t.Errorf("names = %v", sm.Names)
	}
	// 5 fields: col=0, src=0, origLine=0, origCol=0, name=0 → "AAAAA"
	if sm.Mappings != "AAAAA" {
		t.Errorf("mappings = %q, want %q", sm.Mappings, "AAAAA")
	}
}

func TestGenerateV3_MultipleSourcesAndColumns(t *testing.T) {
	mappings := []Mapping{
		{GeneratedLine: 0, GeneratedCol: 0, SourceIdx: 0, OriginalLine: 5, OriginalCol: 10, NameIdx: -1},
		{GeneratedLine: 0, GeneratedCol: 8, SourceIdx: 1, OriginalLine: 0, OriginalCol: 0, NameIdx: -1},
	}
	data, err := GenerateV3("out.js", []string{"a.js", "b.js"}, nil, mappings, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sm sourceMapV3
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatal(err)
	}
	// First segment: col=0, src=0, origLine=5, origCol=10 → "AAKU"
	// Second segment: col=8(+8), src=1(+1), origLine=0(-5), origCol=0(-10) → "QCLV"
	// Verify by decoding:
	parts := splitMappingLine(sm.Mappings)
	if len(parts) != 2 {
		t.Fatalf("expected 2 segments, got %d in %q", len(parts), sm.Mappings)
	}
	seg0 := decodeSegment(t, parts[0])
	if seg0[0] != 0 || seg0[1] != 0 || seg0[2] != 5 || seg0[3] != 10 {
		t.Errorf("segment 0 = %v, want [0,0,5,10]", seg0)
	}
	seg1 := decodeSegment(t, parts[1])
	if seg1[0] != 8 || seg1[1] != 1 || seg1[2] != -5 || seg1[3] != -10 {
		t.Errorf("segment 1 = %v, want [8,1,-5,-10]", seg1)
	}
}

func TestBuildIdentityMappings(t *testing.T) {
	m := BuildIdentityMappings(0, 3)
	if len(m) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(m))
	}
	for i, mm := range m {
		if mm.GeneratedLine != i || mm.OriginalLine != i {
			t.Errorf("mapping %d: gen=%d, orig=%d", i, mm.GeneratedLine, mm.OriginalLine)
		}
	}
}

func TestBuildChunkMappings(t *testing.T) {
	chunk := CodeChunk{
		StartLine: 10, // 1-based
		EndLine:   12,
		Code:      "a\nb\nc\n",
	}
	m := BuildChunkMappings(0, 5, chunk)
	// Code has 4 lines (3 + trailing newline)
	if len(m) != 4 {
		t.Fatalf("expected 4 mappings, got %d", len(m))
	}
	// First mapping: gen line 5, orig line 9 (0-based = StartLine-1)
	if m[0].GeneratedLine != 5 || m[0].OriginalLine != 9 {
		t.Errorf("mapping 0: gen=%d, orig=%d, want (5, 9)", m[0].GeneratedLine, m[0].OriginalLine)
	}
}

// Test helpers.

func splitMappingLine(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else if c == ';' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func decodeSegment(t *testing.T, s string) []int {
	t.Helper()
	var values []int
	for len(s) > 0 {
		v, consumed := DecodeVLQ(s)
		if consumed == 0 {
			t.Fatalf("failed to decode VLQ from %q", s)
		}
		values = append(values, v)
		s = s[consumed:]
	}
	return values
}
