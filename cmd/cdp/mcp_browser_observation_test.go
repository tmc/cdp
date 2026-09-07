package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func browserObservationFixture(t *testing.T, ctx context.Context) *mcpSession {
	t.Helper()
	err := chromedp.Run(ctx, chromedp.Evaluate(`window.clicks = 0;
document.body.innerHTML = '<button id="late">Click</button><input aria-label="Entry"><span id="count">0</span>';
document.querySelector('button').onclick = () => document.querySelector('#count').textContent = String(++window.clicks);`, nil))
	if err != nil {
		t.Fatal(err)
	}
	return &mcpSession{ctx: ctx, refs: newRefRegistry()}
}

func browserObservationClient(t *testing.T, s *mcpSession) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	registerBrowserObservationTools(server, s)
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
	return cs
}

func callBrowserTool[T any](t *testing.T, cs *mcp.ClientSession, name string, args any) T {
	t.Helper()
	r, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("%s: %v", name, r.GetError())
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func observationRef(t *testing.T, state browserObserveOutput, role, name string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(role+` "`+name+`"`) + `[^\n]* (@[0-9]+)`)
	m := re.FindStringSubmatch(state.Tree)
	if m == nil {
		t.Fatalf("missing %s %q in snapshot:\n%s", role, name, state.Tree)
	}
	return m[1]
}

func browserClickCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	var n int
	if err := chromedp.Run(ctx, chromedp.Evaluate("window.clicks", &n)); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBrowserObservationActions(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	in := browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click"), Expect: &expectClause{Selector: "#count", Text: "1"}}
	out := callBrowserTool[browserActOutput](t, client, "browser_act", in)
	if out.Execution != "completed" || out.Observation != "captured" || out.Postcondition != "met" || out.FreshState == nil {
		t.Fatalf("click result = %+v", out)
	}
	if n := browserClickCount(t, ctx); n != 1 {
		t.Fatalf("clicks = %d, want 1", n)
	}
	retry := callBrowserTool[browserActOutput](t, client, "browser_act", in)
	if retry.Execution != "not_dispatched" {
		t.Fatalf("reused state = %+v", retry)
	}
	state = *out.FreshState
	typed := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "type", Ref: observationRef(t, state, "textbox", "Entry"), Text: "hello"})
	if typed.Execution != "completed" || typed.Observation != "captured" || typed.Postcondition != "not_requested" {
		t.Fatalf("type result = %+v", typed)
	}
	var value string
	if err := chromedp.Run(ctx, chromedp.Value("input", &value)); err != nil {
		t.Fatal(err)
	}
	if value != "hello" {
		t.Fatalf("input value = %q", value)
	}
}

func TestBrowserObservationRejectsStale(t *testing.T) {
	ctx := actionDiffBrowser(t)
	for _, name := range []string{"new snapshot", "same URL reload", "detached", "wrong target", "bad ref", "bad expect"} {
		t.Run(name, func(t *testing.T) {
			s := browserObservationFixture(t, ctx)
			client := browserObservationClient(t, s)
			state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
			in := browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")}
			switch name {
			case "new snapshot":
				callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
			case "same URL reload":
				if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
					t.Fatal(err)
				}
			case "detached":
				if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('button').outerHTML = '<button onclick="window.clicks++">Click</button><button onclick="window.clicks++">Click</button>'`, nil)); err != nil {
					t.Fatal(err)
				}
			case "wrong target":
				in.TargetID = "another-target"
			case "bad ref":
				in.Ref += "junk"
			case "bad expect":
				in.Expect = &expectClause{Selector: "[", Text: "x"}
			}
			out := callBrowserTool[browserActOutput](t, client, "browser_act", in)
			if out.Execution != "not_dispatched" || out.ErrorText == "" {
				t.Fatalf("rejected action = %+v", out)
			}
			if n := browserClickCount(t, ctx); n != 0 {
				t.Fatalf("rejected action clicked %d times", n)
			}
		})
	}
}

// faultBrowserExecutor changes only the observation or acknowledgement path;
// the underlying input is dispatched to the real local browser fixture.
type faultBrowserExecutor struct {
	base      cdp.Executor
	inputDone bool
	loseAck   bool
}

func (e *faultBrowserExecutor) Execute(ctx context.Context, method string, params, result any) error {
	if e.inputDone && !e.loseAck && method == "Page.getFrameTree" {
		return errors.New("injected capture failure")
	}
	err := e.base.Execute(ctx, method, params, result)
	if p, ok := params.(*input.DispatchMouseEventParams); ok && p.Type == input.MouseReleased && err == nil {
		e.inputDone = true
		if e.loseAck {
			return errors.New("injected lost input acknowledgement")
		}
	}
	return err
}

func TestBrowserObservationCaptureFailure(t *testing.T) {
	ctx := actionDiffBrowser(t)
	for _, loseAck := range []bool{false, true} {
		t.Run(fmt.Sprintf("loseAck=%v", loseAck), func(t *testing.T) {
			s := browserObservationFixture(t, ctx)
			var state *browserObservation
			if err := s.withBrowserObservation(t.Context(), 10*time.Second, func(ctx, actx context.Context) error {
				var err error
				state, err = s.captureBrowserObservation(ctx, actx)
				s.observation = state
				return err
			}); err != nil {
				t.Fatal(err)
			}
			in := browserActInput{StateID: state.output.StateID, TargetID: state.output.TargetID, Action: "click", Ref: observationRef(t, state.output, "button", "Click"), Expect: &expectClause{Selector: "#count", Text: "1"}}
			var out browserActOutput
			if err := s.withBrowserObservation(t.Context(), 10*time.Second, func(ctx, actx context.Context) error {
				fault := &faultBrowserExecutor{base: cdp.ExecutorFromContext(ctx), loseAck: loseAck}
				out = s.actOnBrowserObservation(cdp.WithExecutor(ctx, fault), actx, in)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if loseAck {
				if out.Execution != "dispatched_unknown" || out.Observation != "captured" || out.Postcondition != "met" {
					t.Fatalf("lost ack result = %+v", out)
				}
			} else if out.Execution != "completed" || out.Observation != "unavailable" || out.Postcondition != "unknown" || out.FreshState != nil {
				t.Fatalf("capture failure result = %+v", out)
			}
			client := browserObservationClient(t, s)
			retry := callBrowserTool[browserActOutput](t, client, "browser_act", in)
			if retry.Execution != "not_dispatched" {
				t.Fatalf("retry after fault = %+v", retry)
			}
			if n := browserClickCount(t, ctx); n != 1 {
				t.Fatalf("clicks after failure and retry = %d, want 1", n)
			}
		})
	}
}

func TestBrowserObservationCanceledGate(t *testing.T) {
	s := &mcpSession{observationGate: make(chan struct{}, 1)}
	s.observationGate <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := s.withBrowserObservation(ctx, time.Second, func(context.Context, context.Context) error {
		t.Error("canceled gate waiter ran browser work")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("gate error = %v", err)
	}
}

func TestBrowserObservationChangedTarget(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	other, cancel := chromedp.NewContext(ctx)
	t.Cleanup(cancel)
	browserObservationFixture(t, other)
	s.setActiveCtx(other, nil)
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")})
	if out.Execution != "not_dispatched" {
		t.Fatalf("changed target result = %+v", out)
	}
	if a, b := browserClickCount(t, ctx), browserClickCount(t, other); a != 0 || b != 0 {
		t.Fatalf("target clicks = %d, %d", a, b)
	}
}

func TestBrowserObservationCoveredNode(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	if err := chromedp.Run(ctx, chromedp.Evaluate(`const cover = document.createElement('div');
cover.style = 'position:fixed;inset:0;z-index:999999;background:white';
cover.onclick = () => window.clicks += 100;
document.body.append(cover);`, nil)); err != nil {
		t.Fatal(err)
	}
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")})
	if out.Execution == "completed" || out.ErrorText == "" {
		t.Errorf("covered node result = %+v", out)
	}
	if n := browserClickCount(t, ctx); n != 0 {
		t.Fatalf("covered node caused %d click effects", n)
	}
}

func TestBrowserObservationRedirectedFocus(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	if err := chromedp.Run(ctx, chromedp.Evaluate(`const other = document.createElement('input'); other.id = 'other'; document.body.append(other);
document.querySelector('input').onfocus = () => other.focus();`, nil)); err != nil {
		t.Fatal(err)
	}
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "type", Ref: observationRef(t, state, "textbox", "Entry"), Text: "hello"})
	if out.Execution == "completed" {
		t.Errorf("redirected focus result = %+v", out)
	}
	var values []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll('input'), n => n.value)`, &values)); err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if value != "" {
			t.Fatalf("redirected focus typed into an input: %q", values)
		}
	}
}

func TestBrowserObservationConcurrentActions(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	in := browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")}
	type result struct {
		value *mcp.CallToolResult
		err   error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			r, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "browser_act", Arguments: in})
			results <- result{r, err}
		}()
	}
	close(start)
	completed, rejected := 0, 0
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		b, err := json.Marshal(r.value.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out browserActOutput
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		switch out.Execution {
		case "completed":
			completed++
		case "not_dispatched":
			rejected++
		default:
			t.Fatalf("competing action = %+v", out)
		}
	}
	if completed != 1 || rejected != 1 {
		t.Fatalf("completed=%d rejected=%d", completed, rejected)
	}
	if n := browserClickCount(t, ctx); n != 1 {
		t.Fatalf("competing actions clicked %d times", n)
	}
}

func TestBrowserObservationShadowAndDescendants(t *testing.T) {
	ctx := actionDiffBrowser(t)
	s := browserObservationFixture(t, ctx)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('button').innerHTML = '<span>Click</span>';
document.querySelector('input').remove();
const host = document.createElement('div'); host.id = 'host'; document.body.append(host);
host.attachShadow({mode:'open'}).innerHTML = '<input aria-label="Entry">';`, nil)); err != nil {
		t.Fatal(err)
	}
	client := browserObservationClient(t, s)
	state := callBrowserTool[browserObserveOutput](t, client, "browser_observe", map[string]any{})
	out := callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "click", Ref: observationRef(t, state, "button", "Click")})
	if out.Execution != "completed" || out.FreshState == nil {
		t.Fatalf("descendant click result = %+v", out)
	}
	if n := browserClickCount(t, ctx); n != 1 {
		t.Fatalf("descendant clicks = %d", n)
	}
	state = *out.FreshState
	out = callBrowserTool[browserActOutput](t, client, "browser_act", browserActInput{StateID: state.StateID, TargetID: state.TargetID, Action: "type", Ref: observationRef(t, state, "textbox", "Entry"), Text: "shadow"})
	if out.Execution != "completed" {
		t.Fatalf("shadow type result = %+v", out)
	}
	var value string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#host').shadowRoot.querySelector('input').value`, &value)); err != nil {
		t.Fatal(err)
	}
	if value != "shadow" {
		t.Fatalf("shadow input = %q", value)
	}
}
