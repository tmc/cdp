package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tmc/cdp/internal/browser"
	"github.com/tmc/cdp/internal/browserprofile"
	"github.com/tmc/cdp/internal/testutil"
)

func TestCheckPDFOutput(t *testing.T) {
	tests := []struct {
		name     string
		opts     options
		terminal bool
		want     error
	}{
		{"html to terminal", options{outputFormat: "html"}, true, nil},
		{"bad spec ignored for html", options{outputFormat: "html", pdfSpec: "page=bogus"}, true, nil},
		{"pdf to file", options{outputFormat: "pdf", outputFile: "x.pdf", pdfSpec: "page=a4"}, true, nil},
		{"pdf to pipe", options{outputFormat: "pdf"}, false, nil},
		{"pdf to terminal", options{outputFormat: "pdf"}, true, errBinaryToTerminal},
		{"bad spec", options{outputFormat: "pdf", outputFile: "x.pdf", pdfSpec: "page=bogus"}, false, errPDFOptions},
		{"bad spec to terminal", options{outputFormat: "pdf", pdfSpec: "page=bogus"}, true, errPDFOptions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPDFOutput(tt.opts, func() bool { return tt.terminal })
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("checkPDFOutput = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRunChallengeRetryRecordsHAR checks that when a headless navigation hits
// an anti-bot challenge and churl retries in a new browser, the HAR records
// the retried page rather than only the challenge. The server shows a
// Cloudflare-style title on the first request and the real page afterwards.
func TestRunChallengeRetryRecordsHAR(t *testing.T) {
	skipIfNoBrowser(t)
	defer testutil.CleanupOrphanedBrowsers(t)

	// Keep the retry headless; the test is about what it records.
	saved := challengeRetryOptions
	challengeRetryOptions = []browser.Option{browser.WithHeadless(true)}
	t.Cleanup(func() { challengeRetryOptions = saved })

	var pages atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if pages.Add(1) == 1 {
			fmt.Fprint(w, `<html><head><title>Just a moment...</title></head><body>checking</body></html>`)
			return
		}
		fmt.Fprint(w, `<html><head><title>Real</title><link rel="stylesheet" href="/real.css"></head><body>real content</body></html>`)
	})
	mux.HandleFunc("/real.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, `body { color: black }`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	pm, err := browserprofile.NewProfileManager()
	if err != nil {
		t.Fatal(err)
	}
	chromePath, _ := detectChromePath()
	dir := t.TempDir()
	opts := options{
		outputFile:       filepath.Join(dir, "out.html"),
		outputFormat:     "html",
		harFile:          filepath.Join(dir, "out.har"),
		chromePath:       chromePath,
		headless:         true,
		timeout:          60,
		waitNetworkIdle:  true,
		stableTimeout:    5,
		waitForChallenge: true,
		method:           "GET",
		followRedirect:   true,
	}
	if err := run(context.Background(), pm, ts.URL+"/", opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := pages.Load(); n < 2 {
		t.Fatalf("server saw %d page loads, want a retry", n)
	}

	out, err := os.ReadFile(opts.outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "real content") {
		t.Errorf("output does not contain the retried page:\n%s", out)
	}

	data, err := os.ReadFile(opts.harFile)
	if err != nil {
		t.Fatal(err)
	}
	var har struct {
		Log struct {
			Entries []struct {
				Request struct {
					URL string `json:"url"`
				} `json:"request"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &har); err != nil {
		t.Fatalf("parse HAR: %v", err)
	}
	var urls []string
	for _, e := range har.Log.Entries {
		urls = append(urls, e.Request.URL)
	}
	if !slices.Contains(urls, ts.URL+"/real.css") {
		t.Errorf("HAR does not record the retried page; entries: %q", urls)
	}
}
