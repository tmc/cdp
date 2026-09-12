package cdpscripttest_test

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

// Example runs an inline script through the engine. Commands that do not touch
// the browser need no chromedp context, which makes them usable in examples and
// in tests that have no browser available.
func Example() {
	workdir, err := os.MkdirTemp("", "cdpscripttest")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(workdir)

	s, err := cdpscripttest.NewStateWithArtifactDir(context.Background(), workdir, "http://localhost:8090", "", nil)
	if err != nil {
		log.Fatal(err)
	}

	script := `echo hello from ${BASE_URL}
stdout 'hello from http'
`
	e := cdpscripttest.NewEngine()
	if err := e.Execute(s.State, "inline", bufio.NewReader(strings.NewReader(script)), os.Stdout); err != nil {
		log.Fatal(err)
	}
	fmt.Println("ok")

	// Output:
	// > echo hello from ${BASE_URL}
	// [stdout]
	// hello from http://localhost:8090
	// > stdout 'hello from http'
	// matched: hello from http://localhost:8090
	// ok
}

// ExampleRunCDPScript runs a cdpscript txtar archive through the real cdpscript
// runtime. Arguments are exposed to the script as ARG1..ARGN.
func ExampleRunCDPScript() {
	dir, err := os.MkdirTemp("", "cdpscripttest")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "greet.txtar")
	archive := `-- main.cdp --
log hello ${ARG1}
`
	if err := os.WriteFile(path, []byte(archive), 0o644); err != nil {
		log.Fatal(err)
	}

	opts := cdpscripttest.CDPScriptRunOptions{Args: []string{"world"}}
	if err := cdpscripttest.RunCDPScript(context.Background(), path, opts); err != nil {
		log.Fatal(err)
	}

	// Output:
	// hello world
}

// ExampleTest shows the browser-backed setup. It is illustrative: it needs a
// running Chrome, so it has no Output comment and is compiled but not run.
func ExampleTest() {
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
	_ = chromedp.DefaultExecAllocatorOptions
	_ = cdpscripttest.Test
}

// Example_webRTC shows how to set up WebRTC testing with fake media devices.
// It needs a running Chrome, so it is compiled but not run.
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
	//     wait-visible '#status'
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
func TestExample(t *testing.T) {
	workdir := t.TempDir()
	s, err := cdpscripttest.NewState(t, context.Background(), workdir, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	e := cdpscripttest.NewEngine()
	script := `echo ok
stdout ok
`
	var log strings.Builder
	if err := e.Execute(s.State, "inline", bufio.NewReader(strings.NewReader(script)), &log); err != nil {
		t.Fatal(err)
	}
}
