//go:build cdp

package cdpscripttest_test

import (
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

func TestCDP(t *testing.T) {
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
