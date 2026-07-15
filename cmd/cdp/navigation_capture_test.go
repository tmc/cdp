package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/testutil"
)

func TestFullCaptureNavigationTimeoutWritesMetadata(t *testing.T) {
	skipIfNoBrowser(t)

	const marker = "navigation-timeout-fixture"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<!doctype html><title>%s</title>", marker)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	outDir, output, elapsed := runFullCaptureNavigation(t, srv.URL, 1)
	if elapsed > 8*time.Second {
		t.Fatalf("timed-out navigation took %v\noutput:\n%s", elapsed, output)
	}
	if !strings.Contains(output, "timed out after") || !strings.Contains(output, srv.URL) {
		t.Fatalf("navigation error lacks URL and timeout details:\n%s", output)
	}
	if _, ok := fileContaining(t, outDir, srv.URL); !ok {
		t.Fatalf("metadata-only HARL entry for stalled response not found\noutput:\n%s", output)
	}
}

func TestFullCaptureNavigationFiniteResponse(t *testing.T) {
	skipIfNoBrowser(t)

	const marker = "navigation-finite-fixture"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, "<!doctype html><title>%s</title>", marker)
	}))
	defer srv.Close()

	outDir, output, _ := runFullCaptureNavigation(t, srv.URL, 5)
	if _, ok := fileContaining(t, outDir, marker); !ok {
		t.Fatalf("finite response body not found in HARL\noutput:\n%s", output)
	}
}

func runFullCaptureNavigation(t *testing.T, url string, timeout int) (string, string, time.Duration) {
	t.Helper()

	cdpPath := buildCDP(t)
	chromePath := testutil.FindChrome()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}
	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cdpPath,
		"--headless",
		"--chrome-path", chromePath,
		"--debug-port", fmt.Sprint(freeTCPPort(t)),
		"--full-capture",
		"--harl",
		"--verbose",
		"--navigation-timeout", fmt.Sprint(timeout),
		"--output-dir", outDir,
	)
	cmd.Stdin = strings.NewReader("goto " + url + "\nexit\n")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	started := time.Now()
	err := cmd.Run()
	elapsed := time.Since(started)
	if err != nil {
		if ctx.Err() != nil {
			t.Fatalf("cdp timed out after %v: %v\noutput:\n%s", elapsed, err, output.String())
		}
		t.Fatalf("cdp failed: %v\noutput:\n%s", err, output.String())
	}
	if _, err := os.Stat(outDir); err != nil {
		t.Fatalf("output directory missing: %v", err)
	}
	return outDir, output.String(), elapsed
}
