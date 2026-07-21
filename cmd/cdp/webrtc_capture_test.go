package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/testutil"
)

// webrtcOfferPage creates an RTCPeerConnection with a DataChannel and applies a
// local offer. The injected WebRTC capture script patches RTCPeerConnection at
// document-create time and reports setLocalDescription via console.log, so the
// offer SDP flows through the full capture pipeline.
//
// Offer creation is synchronous and needs no ICE or peer, so this exercises the
// capture wiring deterministically without depending on a loopback handshake
// completing (headless ICE is environment-dependent).
func webrtcOfferPage() string {
	return `<!doctype html><meta charset="utf-8"><body><script>
(async () => {
  const pc = new RTCPeerConnection();
  pc.createDataChannel('capture-probe');
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
})();
</script></body>`
}

// runWebRTCOfferCapture serves the offer page, drives cdp --full-capture with
// the given --webrtc-capture selection, and returns the output directory and
// combined process output. It holds the REPL on the page briefly so the async
// offer creation reports before exit flushes the recorder.
func runWebRTCOfferCapture(t *testing.T, webrtcCapture string) (string, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, webrtcOfferPage())
	}))
	defer srv.Close()

	cdpPath := buildCDP(t)
	chromePath := testutil.FindChrome()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}

	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cdpPath,
		"--headless", "--chrome-path", chromePath,
		"--debug-port", fmt.Sprint(freeTCPPort(t)),
		"--full-capture", "--harl", "--verbose",
		"--navigation-timeout", "15",
		"--webrtc-capture", webrtcCapture,
		"--output-dir", outDir,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cdp: %v", err)
	}
	_, _ = fmt.Fprintf(stdin, "goto %s/\n", srv.URL)
	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
	}
	_, _ = fmt.Fprint(stdin, "exit\n")
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil && ctx.Err() != nil {
		t.Fatalf("cdp timed out: %v\noutput:\n%s", err, output.String())
	}
	return outDir, output.String()
}

// TestFullCaptureRecordsWebRTCSDP verifies that WebRTC SDP negotiation is
// written to disk under --full-capture with the default selection. The capture
// depends on the injected JS reporting via console.log and the recorder
// receiving Runtime.consoleAPICalled events; if either the Runtime domain is
// not enabled or SDP is not selected, the offer is silently dropped.
func TestFullCaptureRecordsWebRTCSDP(t *testing.T) {
	skipIfNoBrowser(t)

	outDir, output := runWebRTCOfferCapture(t, "sdp,datachannel")

	// The offer must be recorded as an sdp capture event, and its media
	// description must be the WebRTC data-channel section (proving real SDP
	// content flowed through, not just an empty event).
	path, ok := fileContaining(t, outDir, `"type":"sdp"`)
	if !ok {
		t.Fatalf("WebRTC SDP not recorded to disk (Runtime domain likely not enabled for capture)\noutput:\n%s", output)
	}
	if _, ok := fileContaining(t, outDir, "webrtc-datachannel"); !ok {
		t.Fatalf("SDP recorded to %s but without the data-channel media description\noutput:\n%s", path, output)
	}
}

// TestFullCaptureWebRTCCaptureNone verifies that --webrtc-capture=none disables
// WebRTC capture entirely: the same offer page produces no SDP capture event.
// This proves the selection flag actually gates the pipeline.
func TestFullCaptureWebRTCCaptureNone(t *testing.T) {
	skipIfNoBrowser(t)

	outDir, output := runWebRTCOfferCapture(t, "none")

	if path, ok := fileContaining(t, outDir, `"type":"sdp"`); ok {
		t.Fatalf("--webrtc-capture=none still recorded SDP to %s\noutput:\n%s", path, output)
	}
}
