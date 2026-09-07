package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/target"
	"github.com/tmc/cdp/internal/chromedp"
)

func TestNetworkCollectorCapturesDownload(t *testing.T) {
	collector := newNetworkCollector()
	collector.handleEvent(&cdpbrowser.EventDownloadWillBegin{
		GUID: "download-id",
		URL:  "https://example.com/report.csv",
	})
	collector.handleEvent(&cdpbrowser.EventDownloadProgress{
		GUID:          "download-id",
		ReceivedBytes: 42,
		State:         cdpbrowser.DownloadProgressStateCompleted,
	})

	entries := collector.getEntries("report.csv", 0)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Method != "GET" || entry.Size != 42 || !entry.Finished {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestAllTabsMonitorAttachesTargetOnce(t *testing.T) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var calls atomic.Int32
	monitor := newAllTabsMonitor(ctx, false, func(context.Context) error {
		calls.Add(1)
		return nil
	})
	defer monitor.Stop()

	const targetID target.ID = "target-id"
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := monitor.attachToTarget(targetID); err != nil {
				t.Errorf("attachToTarget: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("attach called %d times, want 1", got)
	}
}
