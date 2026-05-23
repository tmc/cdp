package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/testutil"
)

func TestFullCaptureSaveSourcesNewTab(t *testing.T) {
	skipIfNoBrowser(t)

	const marker = "window.__cdpSaveSourcesNewTab = 'just working';"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprintln(w, marker)
		default:
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintln(w, `<!doctype html><title>sources</title><script src="/app.js"></script>`)
		}
	}))
	defer srv.Close()

	cdpPath := buildCDP(t)
	chromePath := testutil.FindChrome()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	outDir := t.TempDir()
	debugPort := freeTCPPort(t)
	cmd := exec.CommandContext(ctx, cdpPath,
		"--headless",
		"--chrome-path", chromePath,
		"--debug-port", fmt.Sprint(debugPort),
		"--full-capture",
		"--save-sources",
		"--output-dir", outDir,
	)
	cmd.Stdin = strings.NewReader("newtab " + srv.URL + "\nexit\n")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("cdp failed: %v\n%s", err, output.String())
	}

	sourcePath, ok := fileContaining(t, filepath.Join(outDir, "sources"), marker)
	if !ok {
		t.Fatalf("captured sources do not contain marker %q\noutput:\n%s", marker, output.String())
	}
	t.Logf("captured source: %s", sourcePath)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func fileContaining(t *testing.T, root, marker string) (string, bool) {
	t.Helper()

	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), marker) {
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk captured sources: %v", err)
	}
	return found, found != ""
}
