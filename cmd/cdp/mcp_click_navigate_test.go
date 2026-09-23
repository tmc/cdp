package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

// TestInteractionNavigates checks that interaction tools return promptly
// when the interaction navigates the page or replaces the target element.
func TestInteractionNavigates(t *testing.T) {
	skipIfNoBrowser(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>A</title>
<a id="link" href="/b">link</a>
<a id="slowlink" href="/slow">slow link</a>
<button id="assign" onclick="location.href='/b'">assign</button>
<button id="down" onmousedown="location.href='/b'">mousedown</button>
<button id="replace" onclick="document.body.innerHTML='<h1>replaced</h1>'">replace</button>
<span id="over" onmouseover="location.href='/b'">hover me</span>
<form action="/b"><input id="q" name="q"><button id="submit" type="submit">submit</button></form>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>B</title><h1>page b</h1>`)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, `<!doctype html><title>B</title><h1>slow page b</h1>
<button id="next" onclick="document.body.innerHTML='<h1>next clicked</h1>'">next</button>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(testutil.FindChrome()))
	alloc, stop := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc)
	t.Cleanup(stop)
	if err := chromedp.Run(ctx); err != nil {
		t.Fatal(err)
	}

	s := &mcpSession{refs: newRefRegistry(), ctx: ctx, browserCtx: ctx}
	cs := scriptToolsClient(t, s, mcpConfig{})

	type call struct {
		tool string
		args map[string]any
	}
	click := func(sel string) call { return call{"click", map[string]any{"selector": sel}} }
	tests := []struct {
		name  string
		calls []call
		want  string // document.body text after the calls
	}{
		{name: "link", calls: []call{click("#link")}, want: "page b"},
		{name: "slow link", calls: []call{click("#slowlink")}, want: "slow page b"},
		{name: "location assign", calls: []call{click("#assign")}, want: "page b"},
		{name: "mousedown navigates", calls: []call{click("#down")}, want: "page b"},
		{name: "replace dom", calls: []call{click("#replace")}, want: "replaced"},
		{name: "form submit", calls: []call{click("#submit")}, want: "page b"},
		{name: "ref link", calls: []call{click("ref:link")}, want: "page b"},
		{name: "ref location assign", calls: []call{click("ref:assign")}, want: "page b"},
		// The second click is issued while the first navigation is in
		// flight; it must wait for the new document, not the stale one.
		{name: "click during navigation", calls: []call{click("#slowlink"), click("#next")}, want: "next clicked"},
		{name: "type_text submit", calls: []call{{"type_text", map[string]any{"selector": "#q", "text": "x", "submit": true}}}, want: "page b"},
		{name: "press_key enter", calls: []call{{"focus", map[string]any{"selector": "#q"}}, {"press_key", map[string]any{"key": "Enter"}}}, want: "page b"},
		{name: "hover navigates", calls: []call{{"hover", map[string]any{"selector": "#over"}}}, want: "page b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/a")); err != nil {
				t.Fatalf("navigate: %v", err)
			}
			for _, c := range tt.calls {
				args := map[string]any{"timeout": 10}
				for k, v := range c.args {
					args[k] = v
				}
				if sel, ok := args["selector"].(string); ok {
					if name, ok := strings.CutPrefix(sel, "ref:"); ok {
						args["selector"] = findRef(t, cs, name)
					}
				}
				start := time.Now()
				text, _, isErr := callText(t, t.Context(), cs, c.tool, args)
				elapsed := time.Since(start)
				t.Logf("%s %v: %s in %s", c.tool, c.args, text, elapsed)
				if isErr {
					t.Fatalf("%s %v: %s", c.tool, c.args, text)
				}
				if elapsed > 5*time.Second {
					t.Errorf("%s %v took %s, want < 5s", c.tool, c.args, elapsed)
				}
			}
			waitBodyText(t, ctx, tt.want)
		})
	}
}

// findRef returns the @ref that page_snapshot assigns to the element with
// the given accessible name.
func findRef(t *testing.T, cs *mcp.ClientSession, name string) string {
	t.Helper()
	snap, _, isErr := callText(t, t.Context(), cs, "page_snapshot", nil)
	if isErr {
		t.Fatalf("page_snapshot: %s", snap)
	}
	for _, line := range strings.Split(snap, "\n") {
		if !strings.Contains(line, `"`+name+`"`) {
			continue
		}
		if m := refPattern.FindString(line); m != "" {
			return m
		}
	}
	t.Fatalf("no @ref for %q in snapshot:\n%s", name, snap)
	return ""
}

var refPattern = regexp.MustCompile(`@\d+`)

// waitBodyText waits until the page body contains want. It retries
// evaluation errors, which are expected while a navigation commits.
func waitBodyText(t *testing.T, ctx context.Context, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		err := chromedp.Run(ctx, chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &body))
		if err == nil && strings.Contains(body, want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("body = %q, want %q", body, want)
}
