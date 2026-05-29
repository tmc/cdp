package sourcemap

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateFromStructureMultiline(t *testing.T) {
	source := "function app() {\n  render();\n}\nfunction render() {}\n"
	structure := &Structure{
		Summary: "test bundle",
		Files: []File{
			{
				Path:        "src/app.js",
				Description: "app entry",
				StartLine:   1,
				EndLine:     3,
				Functions: []Function{
					{Name: "app", StartLine: 1, EndLine: 3},
				},
			},
			{
				Path:        "src/render.js",
				Description: "renderer",
				StartLine:   4,
				EndLine:     4,
				Functions: []Function{
					{Name: "render", StartLine: 4, EndLine: 4},
				},
			},
		},
	}

	data, err := GenerateFromStructure(source, structure)
	if err != nil {
		t.Fatalf("GenerateFromStructure: %v", err)
	}

	var sm struct {
		Version        int      `json:"version"`
		File           string   `json:"file"`
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
		Names          []string `json:"names"`
		Mappings       string   `json:"mappings"`
	}
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatalf("unmarshal sourcemap: %v", err)
	}
	if sm.Version != 3 {
		t.Fatalf("version = %d, want 3", sm.Version)
	}
	if got, want := strings.Join(sm.Sources, ","), "src/app.js,src/render.js"; got != want {
		t.Fatalf("sources = %q, want %q", got, want)
	}
	if got, want := strings.Join(sm.Names, ","), "app,render"; got != want {
		t.Fatalf("names = %q, want %q", got, want)
	}
	if sm.Mappings == "" {
		t.Fatal("mappings is empty")
	}
	if len(sm.SourcesContent) != 2 || !strings.Contains(sm.SourcesContent[0], "render();") {
		t.Fatalf("unexpected sourcesContent: %#v", sm.SourcesContent)
	}
}

func TestGenerateFromStructureSingleLineUsesOffsets(t *testing.T) {
	source := strings.Repeat("a", 1200) + strings.Repeat("b", 800)
	structure := &Structure{
		Files: []File{
			{Path: "src/a.js", StartOffset: 0, EndOffset: 1200},
			{Path: "src/b.js", StartOffset: 1200, EndOffset: 2000},
		},
	}

	data, err := GenerateFromStructure(source, structure)
	if err != nil {
		t.Fatalf("GenerateFromStructure: %v", err)
	}

	var sm struct {
		SourcesContent []string `json:"sourcesContent"`
		Mappings       string   `json:"mappings"`
	}
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatalf("unmarshal sourcemap: %v", err)
	}
	if got := len(sm.SourcesContent[0]); got != 1200 {
		t.Fatalf("first source length = %d, want 1200", got)
	}
	if got := len(sm.SourcesContent[1]); got != 800 {
		t.Fatalf("second source length = %d, want 800", got)
	}
	if !strings.Contains(sm.Mappings, ",") {
		t.Fatalf("mappings %q does not contain multiple same-line segments", sm.Mappings)
	}
}

func TestGenerateFromStructureRequiresFiles(t *testing.T) {
	if _, err := GenerateFromStructure("x", nil); err == nil {
		t.Fatal("nil structure error = nil")
	}
	if _, err := GenerateFromStructure("x", &Structure{}); err == nil {
		t.Fatal("empty structure error = nil")
	}
}
