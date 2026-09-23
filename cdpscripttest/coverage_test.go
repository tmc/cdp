//go:build cdp

package cdpscripttest_test

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
	"github.com/tmc/cdp/cdpscripttest/report"
)

func TestRunFilesWritesCoverage(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-proxy-server", true),
	)
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	baseURL := startTestServer(t)
	artifactDir := t.TempDir()
	e := cdpscripttest.NewEngine()

	res, err := cdpscripttest.RunFiles(t.Context(), e, []string{"testdata/coverage.txt"}, cdpscripttest.RunOptions{
		BaseURL:       baseURL,
		ArtifactDir:   artifactDir,
		AllocatorOpts: opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() != 0 {
		t.Fatalf("RunFiles failed: %+v", res.Results)
	}

	path := filepath.Join(artifactDir, "coverage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage artifact: %v", err)
	}

	var snap struct {
		Name    string         `json:"name"`
		Scripts map[string]any `json:"scripts"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode coverage artifact: %v", err)
	}
	if snap.Name != "final" {
		t.Fatalf("snapshot name = %q, want final", snap.Name)
	}
	if len(snap.Scripts) == 0 {
		t.Fatal("coverage artifact has no scripts")
	}
}

func TestRunFilesWritesReport(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-proxy-server", true),
	)
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	dir := t.TempDir()
	res, err := cdpscripttest.RunFiles(t.Context(), cdpscripttest.NewEngine(), []string{"testdata/coverage.txt"}, cdpscripttest.RunOptions{
		BaseURL:       startTestServer(t),
		AllocatorOpts: opts,
		Report:        &report.Options{Dir: dir, HTML: true, Combined: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() != 0 {
		t.Fatalf("RunFiles failed: %+v", res.Results)
	}
	for _, path := range []string{
		filepath.Join(dir, "index.md"),
		filepath.Join(dir, "index.html"),
		filepath.Join(dir, "coverage", "report.md"),
		filepath.Join(dir, "coverage", "report.html"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("report artifact %q: %v", path, err)
		}
	}
}

func TestRunFilesCombinedReportOmitsDetail(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-proxy-server", true),
	)
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	src := t.TempDir()
	files := []string{filepath.Join(src, "overview.txt"), filepath.Join(src, "internals.txtar")}
	scripts := []string{
		"# Overview fixture.\nset-base-url ${BASE_URL}\nnavigate /screenrecord-basic.html\n",
		"# report:detail\n# Internals fixture.\nset-base-url ${BASE_URL}\nnavigate /screenrecord-basic.html\n",
	}
	for i, file := range files {
		if err := os.WriteFile(file, []byte(scripts[i]), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	res, err := cdpscripttest.RunFiles(t.Context(), cdpscripttest.NewEngine(), files, cdpscripttest.RunOptions{
		BaseURL:       startTestServer(t),
		AllocatorOpts: opts,
		Report:        &report.Options{Dir: dir, Combined: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() != 0 {
		t.Fatalf("RunFiles failed: %+v", res.Results)
	}
	for _, name := range []string{"overview", "internals"} {
		if _, err := os.Stat(filepath.Join(dir, name, "report.md")); err != nil {
			t.Errorf("per-script report: %v", err)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(index, []byte("[overview]")) {
		t.Errorf("index.md does not list overview:\n%s", index)
	}
	if bytes.Contains(index, []byte("internals")) {
		t.Errorf("index.md lists the report:detail script:\n%s", index)
	}
}

func TestRunFilesWritesScreenrecordFormats(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-proxy-server", true),
	)
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	baseURL := startTestServer(t)
	for _, tt := range []struct{ name, file, path string }{
		{"png", "testdata/screenrecord-png.txt", "recording.png"},
		{"frames", "testdata/screenrecord-frames.txt", "recording-frames/manifest.json"},
		{"webm", "testdata/screenrecord-webm.txt", "recording.webm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "webm" {
				if _, err := exec.LookPath("ffmpeg"); err != nil {
					t.Skip("ffmpeg not installed")
				}
			}
			dir := t.TempDir()
			res, err := cdpscripttest.RunFiles(t.Context(), cdpscripttest.NewEngine(), []string{tt.file}, cdpscripttest.RunOptions{BaseURL: baseURL, ArtifactDir: dir, AllocatorOpts: opts})
			if err != nil {
				t.Fatal(err)
			}
			if res.Failed() != 0 {
				t.Fatalf("RunFiles failed: %+v", res.Results)
			}
			path := filepath.Join(dir, tt.path)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("artifact %q: %v", path, err)
			}
			if tt.name == "png" {
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				img, err := png.Decode(f)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := img.Bounds().Size().X, 120; got != want {
					t.Fatalf("PNG width = %d, want %d", got, want)
				}
				if got, want := img.Bounds().Size().Y, 80; got != want {
					t.Fatalf("PNG height = %d, want %d", got, want)
				}
			}
			if tt.name == "webm" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				// Every Matroska and WebM file starts with the EBML header.
				if len(data) < 4 || !bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
					t.Fatalf("artifact is not a webm file: %d bytes", len(data))
				}
			}
		})
	}
}

func TestRunFilesCleansUpScreenrecording(t *testing.T) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", true), chromedp.Flag("no-proxy-server", true))
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	dir := t.TempDir()
	res, err := cdpscripttest.RunFiles(t.Context(), cdpscripttest.NewEngine(), []string{"testdata/cleanup-recording.txt"}, cdpscripttest.RunOptions{BaseURL: startTestServer(t), ArtifactDir: dir, AllocatorOpts: opts})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() != 1 {
		t.Fatalf("failed scripts = %d, want 1", res.Failed())
	}
	if _, err := os.Stat(filepath.Join(dir, "cleanup.gif")); err != nil {
		t.Fatalf("cleanup recording: %v", err)
	}
}
