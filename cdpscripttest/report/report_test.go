package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriter(t *testing.T) {
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "nested", "pass")
	if err := os.MkdirAll(artifactDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "shot.png"), []byte("png"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "flow.gif"), []byte("gif"), 0o666); err != nil {
		t.Fatal(err)
	}
	framesDir := filepath.Join(artifactDir, "frames")
	if err := os.MkdirAll(framesDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framesDir, "manifest.json"), []byte("{}"), 0o666); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shot-unblurred.png", "shot.diff.png", "shot.png.fail.png"} {
		if err := os.WriteFile(filepath.Join(artifactDir, name), []byte("png"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewWriter(Options{Dir: dir, HTML: true, Combined: true}, []Script{
		{Name: "pass", Source: []byte("# Passing <script>\nnavigate /pass\n")},
		{Name: "fail", Source: []byte("# Failing\nnavigate /fail\n")},
		{Name: "pending", Source: []byte("# Pending\nnavigate /pending\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Update(Script{
		Name:        "pass",
		Source:      []byte("# Passing <script>\nnavigate /pass\n"),
		ArtifactDir: artifactDir,
		Log: "# Capture\n> screenshot shot.png\n[stdout]\n" + filepath.Join(artifactDir, "shot.png") + "\n" +
			"> screenrecord stop\n[stdout]\n" + filepath.Join(artifactDir, "flow.gif") + "\n" +
			"> screenrecord stop\n[stdout]\n" + framesDir + "\nformat: frames\nframes: 2\n",
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Update(Script{
		Name:        "fail",
		Source:      []byte("# Failing\nnavigate /fail\n"),
		ArtifactDir: filepath.Join(dir, "fail"),
		Failed:      true,
		Log:         "# Capture\n> screenshot missing.png\n[stdout]\n" + filepath.Join(dir, "fail", "missing.png") + "\n",
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PASS [pass]", "FAIL [fail]", "PEND [pending]", "nested/pass/shot.png"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("index missing %q:\n%s", want, index)
		}
	}
	detail, err := os.ReadFile(filepath.Join(artifactDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"shot.png", "shot-unblurred.png", "shot.diff.png", "shot.png.fail.png", "flow.gif", "manifest.json", "Passing &lt;script&gt;"} {
		if !strings.Contains(string(detail), want) {
			t.Errorf("html report missing %q:\n%s", want, detail)
		}
	}
	markdown, err := os.ReadFile(filepath.Join(artifactDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "frames manifest") {
		t.Errorf("frame artifact not rendered:\n%s", markdown)
	}
	missing, err := os.ReadFile(filepath.Join(dir, "fail", "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(missing), "Missing artifact") {
		t.Errorf("missing artifact not rendered:\n%s", missing)
	}
	htmlIndex, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlIndex), `href="nested/pass/report.html"`) {
		t.Errorf("combined HTML does not link nested report:\n%s", htmlIndex)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "pass", "report.html")); err != nil {
		t.Fatalf("nested linked report: %v", err)
	}
}

// TestWriterDetailExcludedFromCombined checks that a script marked detail gets
// its own report but stays out of the combined index, including out of the
// script counts.
func TestWriterDetailExcludedFromCombined(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(Options{Dir: dir, Combined: true}, []Script{
		{Name: "overview", Source: []byte("# Overview\n")},
		{Name: "deep", Source: []byte("# Deep\n"), Detail: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"overview", "deep"} {
		// The manifest carries Detail; an Update need not repeat it.
		if err := w.Update(Script{Name: name, Log: "> navigate /\n"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "[deep]") {
		t.Errorf("detail script appears in combined index:\n%s", index)
	}
	if !strings.Contains(string(index), "1 scripts: 1 passed, 0 failed\n") {
		t.Errorf("detail script counted in combined index:\n%s", index)
	}
	if _, err := os.Stat(filepath.Join(dir, "deep", "report.md")); err != nil {
		t.Errorf("detail script has no report of its own: %v", err)
	}
}

func TestWriteHTMLEscapesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteHTML(path, Script{Name: "<script>", Log: "# <section>\n> echo '<output>'\n"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"&lt;script&gt;", "&lt;section&gt;", "&lt;output&gt;"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("html missing escaped %q:\n%s", want, b)
		}
	}
}
