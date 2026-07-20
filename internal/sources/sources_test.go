package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/page"
)

func TestSourcePathUsesSiteGroup(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "capture"), false)
	c.pageDomain = "www.lesswrong.com"
	got := c.sourcePath("cdn.lesswrong.com", "_compiled", "app.js")
	want := filepath.Join(c.OutputDir(), "www.lesswrong.com", "_sources", "cdn.lesswrong.com", "_compiled", "app.js")
	if got != want {
		t.Fatalf("sourcePath = %q, want %q", got, want)
	}
}

func TestSourcePathTracksNavigatedPage(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "capture"), false)
	c.Listener(context.Background())(&page.EventFrameNavigated{Frame: &cdp.Frame{URL: "https://www.lesswrong.com/posts/test"}})
	want := filepath.Join(c.OutputDir(), "www.lesswrong.com", "_sources", "cdn.lesswrong.com", "_compiled", "app.js")
	if got := c.sourcePath("cdn.lesswrong.com", "_compiled", "app.js"); got != want {
		t.Fatalf("sourcePath = %q, want %q", got, want)
	}
}

func TestSourcePathCanUseRequestDomainLayout(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "capture"), false)
	c.SetGroupByPage(false)
	want := filepath.Join(c.OutputDir(), "_sources", "cdn.lesswrong.com", "_compiled", "app.js")
	if got := c.sourcePath("cdn.lesswrong.com", "_compiled", "app.js"); got != want {
		t.Fatalf("sourcePath = %q, want %q", got, want)
	}
}

func TestCapPathSegments(t *testing.T) {
	long := strings.Repeat("a", 300)
	other := strings.Repeat("b", 300)
	tests := []struct {
		name string
		in   string
	}{
		{"short unchanged", "am=1/d=1/app.js"},
		{"single long segment", long},
		{"long segment among short", "a/" + long + "/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capPathSegments(tt.in)
			for _, seg := range strings.Split(got, "/") {
				if len(seg) > maxPathSegment+16 {
					t.Fatalf("segment %d bytes exceeds cap: %q", len(seg), seg)
				}
			}
			if !strings.ContainsRune(tt.in, '/') && len(tt.in) <= maxPathSegment && got != tt.in {
				t.Fatalf("short input mangled: %q -> %q", tt.in, got)
			}
		})
	}
	// Distinct overlong segments must not collide after capping.
	if capPathSegments(long) == capPathSegments(other) {
		t.Fatal("distinct long segments collided")
	}
}

// TestSourcePathCapsLongURLSegments guards the write path against URLs whose
// single path component exceeds the filesystem name limit (Google boq module
// lists), which previously made mkdir fail with "file name too long".
func TestSourcePathCapsLongURLSegments(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "capture"), false)
	c.SetGroupByPage(false)
	relPath := "_/js/exm=" + strings.Repeat("Ab1,", 200)
	got := c.sourcePath("www.gstatic.com", "_compiled", relPath)
	for _, seg := range strings.Split(got, string(filepath.Separator)) {
		if len(seg) > maxPathSegment+16 {
			t.Fatalf("path segment too long (%d bytes): %q", len(seg), seg)
		}
	}
	if err := os.MkdirAll(filepath.Dir(got), 0o755); err != nil {
		t.Fatalf("mkdir capped path: %v", err)
	}
}

// TestCloseDuringDispatch exercises the shutdown race: Close closing fetchCh
// while Listener-dispatched events are still being queued from other
// goroutines. When the non-blocking send ran outside the collector mutex,
// a close landing between the dispatcher's unlock and its send panicked
// (a select default does not protect a send on a closed channel). Each
// iteration re-arms incremental mode, lets dispatchers spin, then closes
// mid-flight.
func TestCloseDuringDispatch(t *testing.T) {
	c := New(t.TempDir(), false)

	// Fetches fail immediately (not a chromedp context); this test only
	// cares about queueing racing Close.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listener := c.Listener(ctx)

	for iter := range 50 {
		// Put the collector in incremental mode without a browser,
		// mirroring what Enable sets up. A tiny buffer exercises both
		// the send and the channel-full default paths.
		c.mu.Lock()
		c.fetchCh = make(chan fetchItem, 4)
		c.done = make(chan struct{})
		c.fetchContext, c.fetchCancel = context.WithCancel(context.Background())
		c.incremental = true
		c.mu.Unlock()
		go c.backgroundFetcher()

		stop := make(chan struct{})
		var wg sync.WaitGroup
		for g := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
					}
					listener(&debugger.EventScriptParsed{
						ScriptID: cdp.ScriptID(fmt.Sprintf("%d-%d-%d", iter, g, i)),
						URL:      "https://example.com/app.js",
					})
				}
			}()
		}

		time.Sleep(200 * time.Microsecond) // let dispatchers reach the send
		c.Close()                          // must not panic mid-dispatch
		close(stop)
		wg.Wait()
	}
}

func TestSplitURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantOrigin string
		wantRel    string
	}{
		{
			name:       "https with path",
			raw:        "https://example.com/static/app.js",
			wantOrigin: "example.com",
			wantRel:    "static/app.js",
		},
		{
			name:       "https root falls back to index",
			raw:        "https://example.com/",
			wantOrigin: "example.com",
			wantRel:    "index",
		},
		{
			name:       "file URL groups under file scheme (percent-decoded path)",
			raw:        "file:///Applications/Wispr%20Flow.app/Contents/Resources/app.asar/.webpack/renderer/status/index.js",
			wantOrigin: "file",
			wantRel:    "Applications/Wispr Flow.app/Contents/Resources/app.asar/.webpack/renderer/status/index.js",
		},
		{
			name:       "chrome-extension uses host",
			raw:        "chrome-extension://abcdefghijklmnop/background.js",
			wantOrigin: "abcdefghijklmnop",
			wantRel:    "background.js",
		},
		{
			name:       "unparseable returns empty",
			raw:        "://not a url",
			wantOrigin: "",
			wantRel:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOrigin, gotRel := splitURL(tt.raw)
			if gotOrigin != tt.wantOrigin || gotRel != tt.wantRel {
				t.Errorf("splitURL(%q) = (%q, %q), want (%q, %q)",
					tt.raw, gotOrigin, gotRel, tt.wantOrigin, tt.wantRel)
			}
		})
	}
}

// TestDispatchBeforeEnableDoesNotPanic verifies that a scriptParsed event
// arriving on a Listener before Enable has been called is silently
// dropped (incr=false branch) rather than panicking on a nil channel
// send.
func TestDispatchBeforeEnableDoesNotPanic(t *testing.T) {
	c := New(t.TempDir(), false)
	listener := c.Listener(context.Background())
	// Must not panic: incremental is false, fetchCh is nil — the dispatch
	// reads incremental under c.mu and skips the channel send.
	listener(&debugger.EventScriptParsed{ScriptID: "1", URL: "https://a/x.js"})
	if got, want := len(c.scripts), 1; got != want {
		t.Errorf("c.scripts len = %d, want %d (event should still be recorded for CaptureAll)", got, want)
	}
}

// TestEnableArmsIncrementalBeforeReplayBurst is a regression test for the
// bug that broke save-sources: Enable used to flip c.incremental to true
// AFTER calling Debugger.enable. The replay burst from Debugger.enable
// fires synchronously in the chromedp event loop while Run is still
// blocked, so any listener that checks c.incremental during the burst
// saw false and dropped every replayed scriptParsed event. This test
// simulates the burst by invoking the listener while Enable is in
// progress and confirms items reach fetchCh.
func TestEnableArmsIncrementalBeforeReplayBurst(t *testing.T) {
	c := New(t.TempDir(), false)
	listener := c.Listener(context.Background())

	// Simulate the dispatch path manually (we can't run real chromedp
	// here): pre-arm by hand the same way Enable does, then dispatch.
	c.mu.Lock()
	c.fetchCh = make(chan fetchItem, 4)
	c.incremental = true
	c.mu.Unlock()

	listener(&debugger.EventScriptParsed{ScriptID: "1", URL: "https://a/x.js"})
	listener(&debugger.EventScriptParsed{ScriptID: "2", URL: "https://a/y.js"})

	if got := len(c.fetchCh); got != 2 {
		t.Fatalf("fetchCh len = %d, want 2 (replay burst events were dropped)", got)
	}
}

// TestListenerTagsCtx verifies that Listener(ctx) closures stamp each
// queued fetchItem with the ctx they were created against, so the
// background fetcher can route GetScriptSource to the correct
// session-scoped target context. Regression test for cross-session
// ScriptID aliasing observed when capturing across tab switches.
func TestListenerTagsCtx(t *testing.T) {
	c := New(t.TempDir(), false)
	c.fetchCh = make(chan fetchItem, 4)
	c.incremental = true

	type ctxKey string
	ctxA := context.WithValue(context.Background(), ctxKey("tab"), "A")
	ctxB := context.WithValue(context.Background(), ctxKey("tab"), "B")

	listenerA := c.Listener(ctxA)
	listenerB := c.Listener(ctxB)

	listenerA(&debugger.EventScriptParsed{ScriptID: "1", URL: "https://a/x.js"})
	listenerB(&debugger.EventScriptParsed{ScriptID: "1", URL: "https://b/x.js"})

	first := <-c.fetchCh
	if first.ctx.Value(ctxKey("tab")) != "A" {
		t.Errorf("first item ctx = %v, want A", first.ctx.Value(ctxKey("tab")))
	}
	if first.url != "https://a/x.js" {
		t.Errorf("first item url = %q, want https://a/x.js", first.url)
	}
	second := <-c.fetchCh
	if second.ctx.Value(ctxKey("tab")) != "B" {
		t.Errorf("second item ctx = %v, want B", second.ctx.Value(ctxKey("tab")))
	}
	if second.url != "https://b/x.js" {
		t.Errorf("second item url = %q, want https://b/x.js", second.url)
	}
}
