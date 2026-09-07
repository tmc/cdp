package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/tmc/cdp/internal/chromedp"
)

func browserFrameFixture(t *testing.T, ctx context.Context) (*mcpSession, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/child" {
			fmt.Fprint(w, `<button onclick="document.querySelector('#count').textContent=String(++window.clicks)">Click</button><input aria-label="Entry"><span id="count">0</span><script>window.clicks=0</script>`)
		} else {
			fmt.Fprint(w, `<button onclick="window.clicks++">Click</button><iframe src="/child" style="margin:80px;width:500px;height:300px"></iframe><script>window.clicks=0</script>`)
		}
	}))
	t.Cleanup(server.Close)
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL)); err != nil {
		t.Fatal(err)
	}
	var frame string
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tree, err := page.GetFrameTree().Do(ctx)
		if err != nil {
			return err
		}
		if len(tree.ChildFrames) != 1 {
			return fmt.Errorf("child frames = %d", len(tree.ChildFrames))
		}
		frame = string(tree.ChildFrames[0].Frame.ID)
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	return &mcpSession{ctx: ctx, refs: newRefRegistry()}, frame
}

func TestBrowserObservationChildFrame(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s, frame := browserFrameFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{FrameID: frame})
	if state.FrameID != frame {
		t.Fatalf("frame = %s, want %s", state.FrameID, frame)
	}
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{
		StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click"),
		Expect: &expectClause{Selector: "#count", Text: "1"},
	})
	if out.Execution != "completed" || out.Postcondition != "met" || out.FreshState == nil || out.FreshState.FrameID != frame {
		t.Fatalf("child click = %+v", out)
	}
	if n := browserClickCount(t, ctx); n != 0 {
		t.Fatalf("parent clicks = %d", n)
	}
	state = *out.FreshState
	out = callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{
		StateID: state.StateID, TargetID: state.TargetID, Action: "type", Ref: observationRef(t, state, "textbox", "Entry"), Text: "child only",
	})
	if out.Execution != "completed" {
		t.Fatalf("child type = %+v", out)
	}
	var value string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('iframe').contentDocument.querySelector('input').value`, &value)); err != nil {
		t.Fatal(err)
	}
	if value != "child only" {
		t.Fatalf("child input = %q", value)
	}
}

func TestBrowserObservationStaleFrame(t *testing.T) {
	ctx := actionDiffBrowser(t)
	for _, mutation := range []string{
		`document.querySelector('iframe').remove()`,
		`document.querySelector('iframe').contentDocument.body.innerHTML='<button onclick="parent.clicks++">Click</button>'`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s, frame := browserFrameFixture(t, ctx)
			client := browserObservationClient(t, s)
			state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{FrameID: frame})
			ref := observationRef(t, state, "button", "Click")
			if err := chromedp.Run(ctx, chromedp.Evaluate(mutation, nil)); err != nil {
				t.Fatal(err)
			}
			out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: ref})
			if out.Execution != "not_dispatched" || out.ErrorText == "" {
				t.Fatalf("stale frame = %+v", out)
			}
			if n := browserClickCount(t, ctx); n != 0 {
				t.Fatalf("guard clicks = %d", n)
			}
		})
	}
}

func TestBrowserObservationNavigation(t *testing.T) {
	ctx := actionDiffBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/error" {
			w.WriteHeader(500)
		}
		fmt.Fprintf(w, `<span id="status">%s</span>`, r.URL.Path)
	}))
	defer server.Close()
	for _, path := range []string{"/normal", "/error", "/normal#fragment"} {
		t.Run(path, func(t *testing.T) {
			s, frame := browserFrameFixture(t, ctx)
			client := browserObservationClient(t, s)
			state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{FrameID: frame})
			old := state
			out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{
				StateID: state.StateID, TargetID: state.TargetID, Action: "navigate", URL: server.URL + path,
				Expect: &expectClause{Selector: "#status", Text: map[string]string{"/normal": "/normal", "/error": "/error", "/normal#fragment": "/normal"}[path]},
			})
			if out.Execution != "completed" || out.Observation != "captured" || out.Postcondition != "met" || out.FreshState == nil {
				t.Fatalf("navigate = %+v", out)
			}
			state = *out.FreshState
			if state.URL != server.URL+path {
				t.Fatalf("observed URL = %q, want %q", state.URL, server.URL+path)
			}
			if state.FrameID != frame || state.LoaderID == old.LoaderID || state.DocBackendNodeID == old.DocBackendNodeID {
				t.Fatalf("navigation identity old=%+v new=%+v", old, state)
			}
			var parentURL string
			if err := chromedp.Run(ctx, chromedp.Location(&parentURL)); err != nil {
				t.Fatal(err)
			}
			if parentURL == state.URL {
				t.Fatalf("child navigation changed parent URL to %s", parentURL)
			}
			out = callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "navigate", URL: server.URL + path + "#next", TimeoutMS: 2000})
			if out.Execution != "completed" || out.Observation != "captured" || out.FreshState == nil || out.FreshState.LoaderID != state.LoaderID || out.FreshState.URL != server.URL+path+"#next" {
				t.Fatalf("same-document navigate = %+v", out)
			}
		})
	}
}

type navigationFaultExecutor struct {
	base      cdp.Executor
	mode      string
	navigated bool
	cancel    context.CancelFunc
}

func (e *navigationFaultExecutor) Execute(ctx context.Context, method string, params, result any) error {
	if e.navigated && e.mode == "capture" && method == "Page.getFrameTree" {
		return errors.New("injected navigation capture failure")
	}
	err := e.base.Execute(ctx, method, params, result)
	if method == "Page.navigate" && err == nil {
		e.navigated = true
		if e.mode == "ack" {
			return errors.New("injected lost navigation acknowledgement")
		}
		if e.mode == "cancel" {
			e.cancel()
		}
	}
	return err
}

func TestBrowserObservationNavigationFaults(t *testing.T) {
	ctx := actionDiffBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<body>ready</body>") }))
	defer server.Close()
	for _, mode := range []string{"capture", "ack", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s := browserObservationFixture(t, ctx)
			client := browserObservationClient(t, s)
			state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{})
			in := browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "navigate", URL: server.URL + "/" + mode, Expect: &expectClause{Selector: "body", Text: ""}}
			var out browserActOutput
			if err := s.withBrowserObservation(t.Context(), 5*time.Second, func(ctx, actx context.Context) error {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				fault := &navigationFaultExecutor{base: cdp.ExecutorFromContext(ctx), mode: mode, cancel: cancel}
				out = s.actOnBrowserObservation(cdp.WithExecutor(ctx, fault), actx, in)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			want := "completed"
			if mode == "ack" {
				want = "dispatched_unknown"
			}
			if out.Execution != want || out.ErrorText == "" {
				t.Fatalf("%s result = %+v", mode, out)
			}
			if mode != "ack" && (out.Observation != "unavailable" || out.Postcondition != "unknown" || out.FreshState != nil) {
				t.Fatalf("%s observation = %+v", mode, out)
			}
			retry := callBrowserTool[browserActOutput](t, client, "browser_act", in)
			if retry.Execution != "not_dispatched" {
				t.Fatalf("retry after %s = %+v", mode, retry)
			}
			var location string
			if err := chromedp.Run(ctx, chromedp.Location(&location)); err != nil {
				t.Fatal(err)
			}
			if location != in.URL {
				t.Fatalf("navigation did not occur: %q, want %q", location, in.URL)
			}
		})
	}
}

func TestBrowserObservationFrameReload(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s, frame := browserFrameFixture(t, ctx)
	client := browserObservationClient(t, s)
	// The default follows the selected frame, but an explicit frame does not
	// mutate selection. Both paths must retain the same document ownership.
	s.activeFrameID = cdp.FrameID(frame)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{})
	if state.FrameID != frame {
		t.Fatalf("selected frame = %s", state.FrameID)
	}
	if err := s.withBrowserObservation(t.Context(), 5*time.Second, func(ctx, actx context.Context) error {
		ack, err := navigateObservedBrowserFrame(ctx, cdp.FrameID(frame), state.URL)
		if err == nil && !ack {
			return errors.New("reload not acknowledged")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")})
	if out.Execution != "not_dispatched" {
		t.Fatalf("reloaded state = %+v", out)
	}
	fresh := callBrowserTool[browserObserveOutput](t, client, "browser_observe", browserObserveInput{})
	out = callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: fresh.StateID, TargetID: fresh.TargetID, Action: "click", Ref: observationRef(t, fresh, "button", "Click"), Expect: &expectClause{Selector: "#count", Text: "1"}})
	if out.Execution != "completed" || out.Postcondition != "met" {
		t.Fatalf("fresh after reload = %+v", out)
	}
	if n := browserClickCount(t, ctx); n != 0 {
		t.Fatalf("parent clicks = %d", n)
	}
}
