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
	if elapsed > 12*time.Second {
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

func TestFullCaptureNavigationWaitContract(t *testing.T) {
	skipIfNoBrowser(t)

	// The document is served in full so DOMContentLoaded fires, but a stylesheet
	// in <head> hangs forever. A pending stylesheet blocks the load event without
	// blocking DOMContentLoaded, so the two wait modes observably diverge.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dom-only":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, "<!doctype html><link rel='stylesheet' href='/never.css'><body>ready</body>")
		case "/never.css":
			<-r.Context().Done()
		}
	}))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	// DOMContentLoaded fires despite the hanging stylesheet, so this returns fast.
	_, domOutput, domElapsed := runFullCaptureNavigationWithWait(t, srv.URL+"/dom-only", 5, "domcontentloaded")
	if domElapsed > 4*time.Second {
		t.Fatalf("DOMContentLoaded navigation took %v, expected fast return\noutput:\n%s", domElapsed, domOutput)
	}

	// The load event never fires (the stylesheet hangs) and network idle must
	// not substitute for an explicit load wait, so this waits out the timeout
	// and then soft-succeeds because the document response was received.
	const loadTimeout = 3
	_, loadOutput, loadElapsed := runFullCaptureNavigationWithWait(t, srv.URL+"/dom-only", loadTimeout, "load")
	if loadElapsed < time.Duration(loadTimeout)*time.Second {
		t.Fatalf("load wait returned in %v, expected to wait ~%ds for the load event\noutput:\n%s", loadElapsed, loadTimeout, loadOutput)
	}
}

func runFullCaptureNavigation(t *testing.T, url string, timeout int) (string, string, time.Duration) {
	return runFullCaptureNavigationWithWait(t, url, timeout, "domcontentloaded")
}

func runFullCaptureNavigationWithWait(t *testing.T, url string, timeout int, wait string) (string, string, time.Duration) {
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
		"--wait", wait,
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
