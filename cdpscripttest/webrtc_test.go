//go:build cdp

package cdpscripttest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

// findChromePath returns the path to a Chromium-based browser.
// It checks CHROME_PATH, then google-chrome in PATH, then well-known
// application paths on macOS.
func findChromePath() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	if p, err := exec.LookPath("google-chrome"); err == nil {
		return p
	}
	if runtime.GOOS == "darwin" {
		candidates := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return ""
}

func TestWebRTC(t *testing.T) {
	patterns := []string{
		"testdata/rtc-*.txt",
		"testdata/network-*.txt",
	}

	var matched []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			matched = append(matched, pattern)
		}
	}
	if len(matched) == 0 {
		t.Log("run browser fixtures with: go test -tags cdp -p 1 ./cdpscripttest")
		t.Skip("no webrtc or network fixtures found")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		cdpscripttest.WebRTCAllocatorOptions()...,
	)
	opts = append(opts, chromedp.Flag("headless", true))
	opts = append(opts, chromedp.Flag("no-proxy-server", true))
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(cancel)

	baseURL := startTestServer(t)
	e := cdpscripttest.NewEngine()

	for _, pattern := range matched {
		cdpscripttest.Test(t, e, allocCtx, baseURL, pattern, nil)
	}
}
