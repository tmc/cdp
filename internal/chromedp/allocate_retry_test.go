package chromedp

import (
	"context"
	"os"
	"testing"
)

// TestRunAfterAllocFailure checks that a second Run on a context whose browser
// failed to allocate reports the original error instead of panicking. Chrome
// starting and then exiting without printing a websocket URL gets Allocate past
// the point where it arms the close of c.allocated, so before this was made
// sticky the retry closed that channel twice and killed the process.
func TestRunAfterAllocFailure(t *testing.T) {
	// Any binary that starts and exits without output will do.
	const exitsImmediately = "/usr/bin/true"
	if _, err := os.Stat(exitsImmediately); err != nil {
		t.Skipf("no %s on this system", exitsImmediately)
	}

	actx, acancel := NewExecAllocator(context.Background(), ExecPath(exitsImmediately))
	defer acancel()
	ctx, cancel := NewContext(actx)
	defer cancel()

	first := Run(ctx)
	if first == nil {
		t.Fatal("Run with a browser that exits immediately = nil, want error")
	}
	if second := Run(ctx); second != first {
		t.Errorf("second Run = %v, want the first error %v", second, first)
	}
}
