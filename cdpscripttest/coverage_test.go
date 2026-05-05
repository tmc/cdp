//go:build cdp

package cdpscripttest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
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
