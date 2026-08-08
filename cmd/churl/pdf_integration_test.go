//go:build integration
// +build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/testutil"
)

// TestIntegrationChurl_PDF_Output checks that -output-format pdf writes a real
// PDF for the rendered page.
func TestIntegrationChurl_PDF_Output(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoChrome(t)

	churlBinary := buildChurlIntegration(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>PDF test</title></head><body><h1>Printed</h1></body></html>`)
	})

	server := testutil.TestServer(t, mux)
	defer server.Close()

	out := filepath.Join(t.TempDir(), "page.pdf")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, churlBinary,
		"--headless",
		"--output-format=pdf",
		"-o", out,
		server.URL(),
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("churl failed: %v\nOutput: %s", err, output)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF; first bytes = %q", data[:min(8, len(data))])
	}
	if len(data) < 1000 {
		t.Errorf("PDF suspiciously small: %d bytes", len(data))
	}
}

// TestIntegrationChurl_PDF_RefusesTerminal checks the guard that keeps binary
// output off a terminal. Stdout here is a pipe, not a terminal, so the guard
// must NOT fire: the PDF is written to stdout unharmed.
func TestIntegrationChurl_PDF_RefusesTerminal(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoChrome(t)

	churlBinary := buildChurlIntegration(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>pipe</body></html>`)
	})

	server := testutil.TestServer(t, mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, churlBinary,
		"--headless",
		"--output-format=pdf",
		server.URL(),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("churl failed: %v\nStderr: %s", err, stderr.String())
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("%PDF-")) {
		t.Fatalf("piped stdout should carry the PDF; got %q", stdout.Bytes()[:min(16, stdout.Len())])
	}
	if strings.Contains(stderr.String(), "would corrupt your terminal") {
		t.Error("terminal guard fired on a pipe")
	}
}
