package sourcemap

import (
	"strings"
	"testing"
)

func TestChunkAnalysisPrompt(t *testing.T) {
	prompt := ChunkAnalysisPrompt("https://example.com/app.js", []CodeChunk{
		{
			StartOffset: 10,
			EndOffset:   20,
			StartLine:   1,
			EndLine:     1,
			HitCount:    2,
			Code:        "function run() {}",
		},
	}, "clicked login")

	for _, want := range []string{
		"Bundle URL: https://example.com/app.js",
		"Action that triggered this code: clicked login",
		"=== Chunk 1 (bytes 10-20, lines 1-1, hits 2) ===",
		"function run() {}",
		"Respond with JSON:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestFunctionAnalysisPrompt(t *testing.T) {
	source := "function run() { return 1 }\nfunction idle() { return 0 }\n"
	prompt := FunctionAnalysisPrompt("https://example.com/app.js", FunctionPromptData{
		Source: source,
		Functions: []FunctionCoverage{
			{
				Name:      "run",
				StartLine: 1,
				EndLine:   1,
				HitCount:  3,
				Ranges:    []CoverageRange{{StartOffset: 0, EndOffset: 27, Count: 3}},
			},
			{
				Name:      "idle",
				StartLine: 2,
				EndLine:   2,
				HitCount:  0,
				Ranges:    []CoverageRange{{StartOffset: 28, EndOffset: len(source), Count: 0}},
			},
		},
	}, "")

	for _, want := range []string{
		"Total functions: 2 (1 executed)",
		"--- Function: run (lines 1-1, 3 hits) ---",
		"byte range: 0-27",
		"function run()",
		"=== NON-EXECUTED FUNCTIONS (1) ===",
		"idle (bytes 28-",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`{"files":[]}`, `{"files":[]}`},
		{"```json\n{\"files\":[]}\n```", `{"files":[]}`},
		{"```\n{\"files\":[]}\n```", `{"files":[]}`},
	}
	for _, tt := range tests {
		if got := StripCodeFences(tt.in); got != tt.want {
			t.Fatalf("StripCodeFences(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
