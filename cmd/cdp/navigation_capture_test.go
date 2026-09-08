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

// TestFullCaptureRecordsFailedRequest verifies that an outgoing request whose
// response fails to load (here a connection reset) is still written to disk.
// Failed requests never reach LoadingFinished, so without an explicit handler
// they would be dropped from the capture entirely.
func TestFullCaptureRecordsFailedRequest(t *testing.T) {
	skipIfNoBrowser(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, "<!doctype html><body><script>fetch('/reset').catch(()=>{})</script></body>")
		case "/reset":
			// Abort the connection so the fetch fails with ERR_CONNECTION_RESET.
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
		}
	}))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	// Navigate twice so the first page's async fetch settles and flushes before
	// the process exits.
	outDir := t.TempDir()
	cdpPath := buildCDP(t)
	chromePath := testutil.FindChrome()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cdpPath,
		"--headless", "--chrome-path", chromePath,
		"--debug-port", fmt.Sprint(freeTCPPort(t)),
		"--full-capture", "--harl", "--verbose",
		"--navigation-timeout", "10",
		"--output-dir", outDir,
	)
	cmd.Stdin = strings.NewReader("goto " + srv.URL + "/\ngoto " + srv.URL + "/\nexit\n")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		t.Fatalf("cdp timed out: %v\noutput:\n%s", err, output.String())
	}

	if _, ok := fileContaining(t, outDir, "/reset"); !ok {
		t.Fatalf("failed request not recorded to disk\noutput:\n%s", output.String())
	}
	if _, ok := fileContaining(t, outDir, "loading failed"); !ok {
		t.Fatalf("failed request recorded without the loading-failed marker\noutput:\n%s", output.String())
	}
}

func runFullCaptureNavigation(t *testing.T, url string, timeout int) (string, string, time.Duration) {
	return runFullCaptureNavigationWithWait(t, url, timeout, "domcontentloaded")
}

// startupDuration returns how long cdp spent getting ready to read a command,
// as cdp itself reported it under --verbose. Subtracting it from the wall
// clock leaves the navigation, which is what these tests are about: a cold
// browser launch takes seconds and would otherwise blow every deadline here.
func startupDuration(t *testing.T, output string) time.Duration {
	t.Helper()

	const marker = "startup: REPL initialized after "
	i := strings.LastIndex(output, marker)
	if i < 0 {
		t.Fatalf("cdp did not report its startup time; looked for %q in:\n%s", marker, output)
	}
	rest := output[i+len(marker):]
	line, _, _ := strings.Cut(rest, "\n")
	d, err := time.ParseDuration(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("unparsable startup time %q: %v", line, err)
	}
	return d
}

// runFullCaptureNavigationWithWait runs one goto under --full-capture and
// returns the output directory, the combined output, and how long the
// navigation took, excluding the time cdp spent starting up.
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
	return outDir, output.String(), elapsed - startupDuration(t, output.String())
}
