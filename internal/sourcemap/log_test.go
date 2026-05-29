package sourcemap

import "testing"

func TestAnalysisLog(t *testing.T) {
	mapPath := DiskPath(t.TempDir(), "https://example.com/static/app.js")
	bundleSource := "function app() { return 'hello'; }\nfunction render() { document.write('hi'); }\n"
	structure := &Structure{
		Summary: "Small app with render function",
		Files: []File{
			{
				Path:        "src/app.js",
				Description: "App entry",
				StartOffset: 0,
				EndOffset:   35,
				Functions: []Function{
					{Name: "app", StartLine: 1, EndLine: 1, Description: "Main app function"},
				},
			},
			{
				Path:        "src/render.js",
				Description: "DOM renderer",
				StartOffset: 36,
				EndOffset:   80,
			},
		},
	}

	if err := AppendAnalysisLog(mapPath, "https://example.com/static/app.js", "homepage", structure, bundleSource, false); err != nil {
		t.Fatalf("AppendAnalysisLog: %v", err)
	}
	if err := AppendAnalysisLog(mapPath, "https://example.com/static/app.js", "after-click", structure, bundleSource, true); err != nil {
		t.Fatalf("AppendAnalysisLog refinement: %v", err)
	}

	entries, err := ReadAnalysisLog(mapPath)
	if err != nil {
		t.Fatalf("ReadAnalysisLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Context != "homepage" || entries[0].IsRefinement {
		t.Fatalf("first entry = %#v", entries[0])
	}
	if entries[1].Context != "after-click" || !entries[1].IsRefinement {
		t.Fatalf("second entry = %#v", entries[1])
	}
	if got := entries[0].Files[0].Snippet; got == "" {
		t.Fatal("first file snippet is empty")
	}
	if got := entries[0].Files[0].Functions[0].Name; got != "app" {
		t.Fatalf("function name = %q, want app", got)
	}
	if got := CountAnalysisLogEntries(mapPath); got != 2 {
		t.Fatalf("CountAnalysisLogEntries = %d, want 2", got)
	}
	if got := CountAnalysisLogEntries(""); got != 0 {
		t.Fatalf("CountAnalysisLogEntries(empty) = %d, want 0", got)
	}
}
