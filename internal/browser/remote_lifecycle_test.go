package browser

import (
	"context"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/chromedp"
)

func TestColdRemoteClose(t *testing.T) {
	alloc, stopAlloc := chromedp.NewRemoteAllocator(context.Background(), "ws://127.0.0.1:1/devtools/browser/unused")
	ctx, stop := chromedp.NewContext(alloc, chromedp.WithExistingTarget("unused"))
	b := &Browser{attachedToTab: true}
	b.setRemoteContext(ctx, stop, stopAlloc)
	done := make(chan error, 1)
	go func() {
		if err := b.Close(); err != nil {
			done <- err
			return
		}
		done <- b.Close()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("repeated Close before allocation blocked")
	}
}
