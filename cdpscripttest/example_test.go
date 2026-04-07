package cdpscripttest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/cdpscripttest"
)

// Example shows the minimal setup for running cdpscripttest scripts.
func Example() {
	// In a real test file:
	//
	// func TestCDP(t *testing.T) {
	//     opts := append(chromedp.DefaultExecAllocatorOptions[:],
	//         chromedp.Flag("headless", true),
	//     )
	//     allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	//     defer cancel()
	//
	//     e := cdpscripttest.NewEngine()
	//     cdpscripttest.Test(t, e, allocCtx, "http://localhost:8090", "testdata/*.txt", nil)
	// }
	_ = context.Background
	_ = chromedp.DefaultExecAllocatorOptions
	_ = cdpscripttest.NewEngine
}

// Example_webRTC shows how to set up WebRTC testing with fake media devices.
func Example_webRTC() {
	// WebRTC tests need fake device flags so Chrome provides media streams
	// without real hardware. Use WebRTCAllocatorOptions for this.
	//
	// func TestWebRTC(t *testing.T) {
	//     opts := append(chromedp.DefaultExecAllocatorOptions[:],
	//         cdpscripttest.WebRTCAllocatorOptions()...,
	//     )
	//     opts = append(opts, chromedp.Flag("headless", true))
	//
	//     allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	//     defer cancel()
	//
	//     e := cdpscripttest.NewEngine()
	//     cdpscripttest.Test(t, e, allocCtx, "http://localhost:8080", "testdata/rtc-*.txt", nil)
	// }
	//
	// A typical WebRTC test script (testdata/rtc-basic.txt):
	//
	//     # Inject the monitoring script before navigation so the
	//     # RTCPeerConnection monkey-patch runs at page load.
	//     rtc-inject
	//     navigate /video-call.html
	//     waitVisible '#status'
	//
	//     # Wait up to 60s for the connection to reach "connected".
	//     rtc-wait connected 60s
	//
	//     # Inspect peer connection state.
	//     rtc-peers
	//     stdout 'connected'
	//
	//     # Check outbound video stats (frames are being sent).
	//     sleep 2s
	//     rtc-stats-video --direction outbound
	//     stdout 'framesSent'
	//
	//     # Use conditions to gate commands on connection state.
	//     [rtc-state connected] rtc-ice
	//     stdout 'candidate:'
	//
	//     # Verify a data channel exists.
	//     [rtc-dc chat] rtc-tracks
	//     stdout 'video'
	_ = context.Background
	_ = chromedp.DefaultExecAllocatorOptions
	_ = cdpscripttest.NewEngine
	_ = cdpscripttest.WebRTCAllocatorOptions
}

// TestExample demonstrates running a single inline script.
// Skipped unless CDPSCRIPT_URL is set.
func TestExample(t *testing.T) {
	t.Skip("set CDPSCRIPT_URL and remove this skip to run")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancel)

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	t.Cleanup(tabCancel)

	workdir := t.TempDir()
	s, err := cdpscripttest.NewState(t, tabCtx, workdir, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	e := cdpscripttest.NewEngine()
	script := `navigate /
title
stdout 'Example Domain'
`
	cdpscripttest.Run(t, e, s, "inline", strings.NewReader(script))
}
