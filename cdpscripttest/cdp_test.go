//go:build cdp

package cdpscripttest_test

import (
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

func TestCDP(t *testing.T) {
	patterns := []string{
		"testdata/hyphenated-*.txt",
		"testdata/blur-*.txt",
		"testdata/screenrecord-*.txt",
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
		t.Skip("no cdp fixtures found")
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

	for _, pattern := range matched {
		cdpscripttest.Test(t, e, allocCtx, baseURL, pattern, nil)
	}
}
