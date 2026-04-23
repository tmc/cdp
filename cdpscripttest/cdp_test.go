//go:build cdp

package cdpscripttest_test

import (
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

func TestCDP(t *testing.T) {
	if matches, _ := filepath.Glob("testdata/blur-*.txt"); len(matches) == 0 {
		t.Skip("no blur fixtures found")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-proxy-server", true),
	)
	if p := findChromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(cancel)

	baseURL := startTestServer(t)
	e := cdpscripttest.NewEngine()

	cdpscripttest.Test(t, e, allocCtx, baseURL, "testdata/blur-*.txt", nil)
}
