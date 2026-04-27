package sources

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/debugger"
)

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
