package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

// TestToolsRunAsBrowserActions calls MCP tools that send CDP commands
// directly. A cdproto command needs a context that carries an executor,
// which only chromedp.Run provides; the session's tab context does not.
func TestToolsRunAsBrowserActions(t *testing.T) {
	skipIfNoBrowser(t)

	var (
		mu      sync.Mutex
		headers = map[string]string{}
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers[r.URL.Path] = r.Header.Get("X-Cdp-Test")
		mu.Unlock()
		fmt.Fprint(w, `<!doctype html><title>tools</title>
<label for="name">Name</label><input id="name">
<input id="file" type="file" aria-label="Upload">
<div style="height:3000px"></div>
<button id="far">Far away</button>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(testutil.FindChrome()))
	alloc, stop := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc)
	t.Cleanup(stop)
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/")); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	s := &mcpSession{refs: newRefRegistry(), ctx: ctx, browserCtx: ctx, outputDir: dir}
	s.dialogs = enableDialogCapture(ctx)
	cs := scriptToolsClient(t, s, mcpConfig{})

	call := func(t *testing.T, tool string, args map[string]any) string {
		t.Helper()
		text, _, isErr := callText(t, t.Context(), cs, tool, args)
		if isErr {
			t.Fatalf("%s %v: %s", tool, args, text)
		}
		return text
	}
	eval := func(t *testing.T, expr string) string {
		t.Helper()
		var v any
		if err := runWithTimeout(ctx, 10*time.Second, chromedp.Evaluate(expr, &v)); err != nil {
			t.Fatalf("evaluate %s: %v", expr, err)
		}
		return fmt.Sprint(v)
	}
	navigate := func(t *testing.T, path string) {
		t.Helper()
		if err := runWithTimeout(ctx, 10*time.Second, chromedp.Navigate(srv.URL+path)); err != nil {
			t.Fatalf("navigate %s: %v", path, err)
		}
	}

	t.Run("emulation", func(t *testing.T) {
		call(t, "set_device", map[string]any{"device": "pixel-7"})
		call(t, "set_viewport", map[string]any{"width": 800, "height": 600})
		if got := eval(t, "window.innerWidth"); got != "800" {
			t.Errorf("innerWidth = %s, want 800", got)
		}
		call(t, "set_user_agent", map[string]any{"user_agent": "cdp-test-agent"})
		if got := eval(t, "navigator.userAgent"); got != "cdp-test-agent" {
			t.Errorf("userAgent = %q, want cdp-test-agent", got)
		}
		call(t, "set_throttling", map[string]any{"network_preset": "fast-4g", "cpu_rate": 2})
		call(t, "set_throttling", map[string]any{"network_preset": "none", "cpu_rate": 1})
		call(t, "set_offline", map[string]any{"offline": true})
		call(t, "set_offline", map[string]any{"offline": false})
		call(t, "set_geolocation", map[string]any{"latitude": 1.5, "longitude": 2.5})
		call(t, "set_extra_headers", map[string]any{"headers": map[string]string{"X-Cdp-Test": "yes"}})
		navigate(t, "/headers")
		mu.Lock()
		got := headers["/headers"]
		mu.Unlock()
		if got != "yes" {
			t.Errorf("X-Cdp-Test header = %q, want yes", got)
		}
		call(t, "set_extra_headers", map[string]any{"headers": map[string]string{}})
	})

	t.Run("snapshot refs", func(t *testing.T) {
		navigate(t, "/")
		ref := findRef(t, cs, "Name")
		n, err := strconv.Atoi(strings.TrimPrefix(ref, "@"))
		if err != nil {
			t.Fatal(err)
		}
		// The AX snapshot records backend node IDs; the JS fallback does not.
		if e, ok := s.refs.get(n); !ok || e.BackendNodeID == 0 {
			t.Fatalf("ref %s = %+v, want a backend node ID", ref, e)
		}
		call(t, "focus", map[string]any{"selector": ref})
		if got := eval(t, "document.activeElement.id"); got != "name" {
			t.Errorf("activeElement = %q, want name", got)
		}
		call(t, "screenshot", map[string]any{"annotate": true})
	})

	t.Run("upload_file", func(t *testing.T) {
		navigate(t, "/")
		f := filepath.Join(dir, "upload.txt")
		if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		call(t, "upload_file", map[string]any{"selector": findRef(t, cs, "Upload"), "files": []string{f}})
		if got := eval(t, "document.getElementById('file').files[0]?.name"); got != "upload.txt" {
			t.Errorf("uploaded file = %q, want upload.txt", got)
		}
	})

	t.Run("scroll", func(t *testing.T) {
		navigate(t, "/")
		call(t, "scroll", map[string]any{"selector": findRef(t, cs, "Far away")})
		if got := eval(t, "window.scrollY > 0"); got != "true" {
			t.Errorf("scrollY > 0 = %s, want true", got)
		}
	})

	t.Run("trace", func(t *testing.T) {
		path := filepath.Join(dir, "trace.json")
		call(t, "start_trace", nil)
		navigate(t, "/")
		call(t, "stop_trace", map[string]any{"path": path})
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cookies and state", func(t *testing.T) {
		navigate(t, "/")
		host := strings.TrimPrefix(srv.URL, "http://")
		host = host[:strings.LastIndex(host, ":")]
		call(t, "set_cookie", map[string]any{"name": "tool", "value": "set", "domain": host})
		if got := call(t, "get_cookies", nil); !strings.Contains(got, `"tool"`) {
			t.Fatalf("get_cookies = %s, want cookie tool", got)
		}
		path := filepath.Join(dir, "state.json")
		call(t, "save_state", map[string]any{"path": path})
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"tool"`) {
			t.Fatalf("saved state has no cookie tool:\n%s", data)
		}
		eval(t, `document.cookie = "tool=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/"`)
		call(t, "load_state", map[string]any{"path": path})
		if got := eval(t, "document.cookie"); !strings.Contains(got, "tool=set") {
			t.Errorf("document.cookie = %q, want tool=set", got)
		}
	})

	t.Run("cdpscript cookie", func(t *testing.T) {
		script := "goto " + srv.URL + "/\ncookie set script value\ncookie get script\ncookie clear script\ncookie clear\n"
		text, out, isErr := callText(t, t.Context(), cs, "run_cdpscript", map[string]any{"script": script})
		m, _ := out.(map[string]any)
		if isErr {
			t.Fatalf("run_cdpscript: %s %v", text, out)
		}
		if got := m["stdout"]; got != "value" {
			t.Errorf("cookie get stdout = %q, want value", got)
		}
	})

	t.Run("intercept", func(t *testing.T) {
		call(t, "intercept_request", map[string]any{
			"url_pattern": "*/intercepted", "action": "fulfill", "body": "fulfilled", "content_type": "text/html",
		})
		navigate(t, "/intercepted")
		waitBodyText(t, ctx, "fulfilled")
		call(t, "remove_intercept", map[string]any{"all": true})
	})

	t.Run("handle_dialog", func(t *testing.T) {
		navigate(t, "/")
		eval(t, `setTimeout(() => { window.answer = confirm("sure?") }, 0); 0`)
		deadline := time.Now().Add(5 * time.Second)
		for s.dialogs.getPending() == nil {
			if time.Now().After(deadline) {
				t.Fatal("no dialog opened")
			}
			time.Sleep(20 * time.Millisecond)
		}
		callText(t, t.Context(), cs, "handle_dialog", map[string]any{"accept": true})
		if got := eval(t, "window.answer"); got != "true" {
			t.Errorf("confirm returned %s, want true", got)
		}
	})
}
