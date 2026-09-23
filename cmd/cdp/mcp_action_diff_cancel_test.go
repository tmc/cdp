package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

// actionDiffBrowser starts an isolated browser with a button that the test
// reveals explicitly. Waiting for the selector must not outlive its request.
func actionDiffBrowser(t *testing.T, options ...chromedp.ContextOption) context.Context {
	t.Helper()
	skipIfNoBrowser(t)
	path := testutil.FindChrome()
	if path == "" {
		t.Skip("no Chromium browser available")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path))
	alloc, stop := chromedp.NewExecAllocator(t.Context(), opts...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc, options...)
	t.Cleanup(stop)
	ctx, stop = context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(stop)
	fixture := `<script>
window.clicks = 0;
window.reveal = () => {
 const b = document.createElement('button');
 b.id = 'late'; b.textContent = 'Click';
 b.onclick = () => window.clicks++;
 document.body.append(b);
};
</script><body></body>`
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html,"+url.PathEscape(fixture))); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// callActionDiff observes server-side completion. A canceled client CallTool
// alone does not prove the handler or its browser work stopped.
func callActionDiff(t *testing.T, s *mcpSession) (context.CancelFunc, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	entered, finished := make(chan struct{}), make(chan struct{})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	registerActionDiffTool(server, s)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				close(entered)
				defer close(finished)
			}
			return next(ctx, method, req)
		}
	})
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "action_diff", Arguments: map[string]any{
			"action": "click", "params": `{"selector":"#late"}`,
		}})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-returned:
		case <-time.After(3 * time.Second):
			t.Error("client call did not return after cancellation")
		}
	})
	return cancel, entered, finished
}

func waitActionDiff(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestActionDiffCanceledDuringBrowserSetup(t *testing.T) {
	s := &mcpSession{browserReady: make(chan struct{})}
	cancel, entered, finished := callActionDiff(t, s)
	// Unblock a broken handler before transport cleanup, even on failure.
	t.Cleanup(s.signalBrowserReady)
	waitActionDiff(t, entered, "handler entry")
	cancel()
	waitActionDiff(t, finished, "handler cancellation during setup")
}

func TestActiveContext(t *testing.T) {
	setupErr := errors.New("setup failed")
	for _, tt := range []struct {
		name        string
		session     *mcpSession
		canceled    bool
		wantErr     error
		unavailable bool
	}{
		{name: "ready", session: &mcpSession{ctx: context.Background()}},
		{name: "setup failure", session: &mcpSession{setupErr: setupErr}, wantErr: setupErr},
		{name: "unavailable", session: &mcpSession{}, unavailable: true},
		{name: "canceled and ready", session: &mcpSession{ctx: context.Background()}, canceled: true, wantErr: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.session.browserReady = make(chan struct{})
			close(tt.session.browserReady)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			got, err := tt.session.activeContext(ctx)
			if tt.unavailable {
				if got != nil || err == nil {
					t.Fatalf("activeContext = %v, %v; want unavailable", got, err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("activeContext error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.session.ctx {
				t.Fatal("activeContext returned a different tab")
			}
			if err != nil && got != nil {
				t.Fatal("activeContext returned a tab on failure")
			}
		})
	}
}

func TestRequestToolCtxAlreadyCanceled(t *testing.T) {
	req, cancel := context.WithCancel(t.Context())
	cancel()
	ctx, stop := requestToolCtx(req, context.Background(), time.Second)
	defer stop()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want synchronous cancellation", ctx.Err())
	}
}

func TestActionDiffRequestCancellation(t *testing.T) {
	queried := make(chan struct{}, 1)
	ctx := actionDiffBrowser(t, chromedp.WithDebugf(func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "DOM.querySelector") && strings.Contains(msg, "#late") {
			select {
			case queried <- struct{}{}:
			default:
			}
		}
	}))
	s := &mcpSession{ctx: ctx, refs: newRefRegistry()}
	cancel, _, finished := callActionDiff(t, s)
	waitActionDiff(t, queried, "pending selector query")
	cancel()
	waitActionDiff(t, finished, "server handler exit")
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.reveal()", nil)); err != nil {
		t.Fatal(err)
	}
	var clicked bool
	err := chromedp.Run(ctx, chromedp.Poll("window.clicks > 0", &clicked, chromedp.WithPollingTimeout(time.Second)))
	if !errors.Is(err, chromedp.ErrPollingTimeout) {
		t.Fatalf("click after cancellation: clicked=%v, error=%v; want polling timeout", clicked, err)
	}
	if err := chromedp.Run(ctx, chromedp.Click("#late", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.clicks", &count)); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("positive control clicks = %d, want 1", count)
	}
}

func TestExecuteActionTimeoutStopsPendingClick(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := &mcpSession{ctx: ctx, refs: newRefRegistry()}
	err := executeActionWithTimeout(ctx, s, "click", actionDiffParams{Selector: "#late"}, 100*time.Millisecond)
	if err == nil {
		t.Fatal("click on absent button succeeded")
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.reveal()", nil)); err != nil {
		t.Fatal(err)
	}
	// A successful poll is evidence of a late side effect. Only the polling
	// deadline counts as absence; a dead browser or evaluation error does not.
	var clicked bool
	err = chromedp.Run(ctx, chromedp.Poll("window.clicks > 0", &clicked, chromedp.WithPollingTimeout(time.Second)))
	if !errors.Is(err, chromedp.ErrPollingTimeout) {
		t.Fatalf("click after timeout: clicked=%v, error=%v; want polling timeout", clicked, err)
	}
	// Positive control on the same target proves it still accepts input.
	if err := executeActionWithTimeout(ctx, s, "click", actionDiffParams{Selector: "#late"}, time.Second); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.clicks", &count)); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("positive control clicks = %d, want 1", count)
	}
}
