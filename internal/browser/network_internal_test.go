package browser

import (
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/network"
)

func TestTrackEvictsOldest(t *testing.T) {
	m := make(map[network.RequestID]int)
	var order []network.RequestID
	for i := 0; i < maxTrackedRequests+10; i++ {
		track(m, &order, network.RequestID(fmt.Sprint(i)), i)
	}
	// Re-tracking an existing id must not grow the map.
	track(m, &order, network.RequestID(fmt.Sprint(maxTrackedRequests+9)), -1)
	if len(m) != maxTrackedRequests || len(order) != maxTrackedRequests {
		t.Fatalf("len(m), len(order) = %d, %d, want %d", len(m), len(order), maxTrackedRequests)
	}
	if _, ok := m["0"]; ok {
		t.Error("oldest entry was not evicted")
	}
	if _, ok := m[network.RequestID(fmt.Sprint(maxTrackedRequests+9))]; !ok {
		t.Error("newest entry missing")
	}
}
