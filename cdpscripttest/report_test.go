package cdpscripttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptName(t *testing.T) {
	tests := []struct {
		file, want string
	}{
		{"testdata/login.txt", "login"},
		{"testdata/interaction/viewport.txtar", "viewport"},
		{"viewport.txtar.txt", "viewport.txtar"},
		{"dir/script", "script"},
	}
	for _, tt := range tests {
		if got := ScriptName(tt.file); got != tt.want {
			t.Errorf("ScriptName(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestReportManifest(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"overview.txt": "# Overview script.\nnavigate /\n",
		"detail.txtar": "# report:detail\nnavigate /\n-- page.html --\n<p>hi</p>\n",
	}
	var paths []string
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0o666); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	scripts, err := ReportManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	detail := make(map[string]bool)
	for _, s := range scripts {
		detail[s.Name] = s.Detail
		if len(s.Source) == 0 {
			t.Errorf("%s: empty source", s.Name)
		}
	}
	want := map[string]bool{"overview": false, "detail": true}
	if len(detail) != len(want) {
		t.Fatalf("manifest names = %v, want %v", detail, want)
	}
	for name, d := range want {
		if got, ok := detail[name]; !ok || got != d {
			t.Errorf("manifest[%q].Detail = %v (present %v), want %v", name, got, ok, d)
		}
	}
	if _, err := ReportManifest([]string{filepath.Join(dir, "missing.txt")}); err == nil {
		t.Error("ReportManifest(missing file) succeeded, want error")
	}
}

func TestExtractReportLevel(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   ReportLevel
	}{
		{"default is overview", "navigate /page\n", ReportOverview},
		{"explicit overview", "# report:overview\n# My test.\nnavigate /page\n", ReportOverview},
		{"detail", "# report:detail\n# Low-level test.\nnavigate /page\n", ReportDetail},
		{"detail in preamble", "# My test.\n# report:detail\nnavigate /page\n", ReportDetail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReportLevel([]byte(tt.source))
			if got != tt.want {
				t.Errorf("ExtractReportLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}
