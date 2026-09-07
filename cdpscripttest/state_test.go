package cdpscripttest_test

import (
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
)

// TestNewStateReportsFailedAllocation checks that a failed browser start
// returns an error. /bin/echo stands in for a browser that exits immediately.
func TestNewStateReportsFailedAllocation(t *testing.T) {
	allocCtx, cancel := chromedp.NewExecAllocator(t.Context(), chromedp.ExecPath("/bin/echo"))
	t.Cleanup(cancel)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	t.Cleanup(cancelCtx)

	if _, err := cdpscripttest.NewStateWithArtifactDir(ctx, t.TempDir(), "", t.TempDir(), nil); err == nil {
		t.Fatal("NewStateWithArtifactDir succeeded with a browser that cannot start")
	}
}
